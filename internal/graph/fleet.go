package graph

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/graph/vm"
	"github.com/netspec/netspec/internal/ifname"
	"github.com/netspec/netspec/internal/webui"
)

// FleetTalker is one ranked interface for the fleet top-talkers table.
type FleetTalker struct {
	Device     string   `json:"device"`
	Interface  string   `json:"interface"` // config / display key when known
	Telemetry  string   `json:"telemetry_interface"`
	Alias      string   `json:"alias,omitempty"`
	PortRole   string   `json:"port_role,omitempty"`
	InBPS      *float64 `json:"in_bps"`
	OutBPS     *float64 `json:"out_bps"`
	SpeedBPS   *float64 `json:"speed_bps,omitempty"`
	UtilPct    *float64 `json:"util_pct,omitempty"` // max(in,out)/speed*100 when speed known
	GraphPath  string   `json:"graph_path"`
	NetSpecPath string  `json:"netspec_path,omitempty"`
}

// FleetDeviceHeat is per-device worst util for the honeycomb.
type FleetDeviceHeat struct {
	Device  string   `json:"device"`
	UtilPct *float64 `json:"util_pct,omitempty"`
	Sev     string   `json:"sev"` // ok|warning|critical|unknown — hex coloring
}

// FleetSnapshot is the fleet/aggregate payload.
type FleetSnapshot struct {
	Range      string            `json:"range"`
	PortRole   string            `json:"port_role,omitempty"`
	Count      int               `json:"count"`
	Talkers    []FleetTalker     `json:"talkers"`
	Devices    []FleetDeviceHeat `json:"devices"`
	HexSVG     string            `json:"-"` // rendered separately for HTML
}

// FleetOptions controls FetchFleetSnapshot.
type FleetOptions struct {
	PortRole       string
	DevicePrefix   string
	Device         string
	MonitoredOnly  bool
	Limit          int
	RateWindow     time.Duration // e.g. 5m for rate()
	NetSpecBaseURL string        // optional public NetSpec origin
}

// FetchFleetSnapshot ranks interfaces from the enrichment index by current bps/util.
func FetchFleetSnapshot(ctx context.Context, client *vm.Client, idx *Index, opts FleetOptions) (*FleetSnapshot, error) {
	if client == nil {
		return nil, fmt.Errorf("vm client is nil")
	}
	if opts.Limit <= 0 {
		opts.Limit = 25
	}
	if opts.RateWindow <= 0 {
		opts.RateWindow = 5 * time.Minute
	}
	win := fmt.Sprintf("%ds", int(opts.RateWindow.Seconds()))

	f := Filter{
		PortRole:     opts.PortRole,
		DevicePrefix: opts.DevicePrefix,
		Device:       opts.Device,
	}
	if opts.MonitoredOnly {
		t := true
		f.Monitored = &t
	}
	idents := []InterfaceIdentity{}
	if idx != nil {
		idents = idx.Filter(f)
	}
	if len(idents) == 0 {
		return &FleetSnapshot{Range: win, PortRole: opts.PortRole, Talkers: []FleetTalker{}, Devices: []FleetDeviceHeat{}}, nil
	}

	var inS, outS, speedS []vm.Series
	var inErr, outErr, speedErr error
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		inS, inErr = client.Query(ctx, fmt.Sprintf(`rate(if_in_octets_total[%s])*8`, win))
	}()
	go func() {
		defer wg.Done()
		outS, outErr = client.Query(ctx, fmt.Sprintf(`rate(if_out_octets_total[%s])*8`, win))
	}()
	go func() {
		defer wg.Done()
		speedS, speedErr = client.Query(ctx, `if_speed_bps`)
	}()
	wg.Wait()
	if inErr != nil {
		return nil, fmt.Errorf("fleet in_bps: %w", inErr)
	}
	if outErr != nil {
		return nil, fmt.Errorf("fleet out_bps: %w", outErr)
	}
	if speedErr != nil {
		return nil, fmt.Errorf("fleet speed: %w", speedErr)
	}

	inMap := seriesValueMap(inS)
	outMap := seriesValueMap(outS)
	speedMap := seriesValueMap(speedS)

	talkers := make([]FleetTalker, 0, len(idents))
	for _, id := range idents {
		tele := id.Interface
		if names, err := telemetryIfaceCache.interfaces(ctx, client, id.Device); err == nil {
			for _, n := range names {
				if ifname.Match(n, id.Interface) {
					tele = n
					break
				}
			}
		}
		key := fleetKey(id.Device, tele)
		// Also try config key in case labels already match short form.
		inV := firstValue(inMap, key, fleetKey(id.Device, id.Interface))
		outV := firstValue(outMap, key, fleetKey(id.Device, id.Interface))
		speedV := firstValue(speedMap, key, fleetKey(id.Device, id.Interface))
		if inV == nil && outV == nil {
			continue
		}
		t := FleetTalker{
			Device:      id.Device,
			Interface:   id.Interface,
			Telemetry:   tele,
			Alias:       id.Alias,
			PortRole:    id.PortRole,
			InBPS:       inV,
			OutBPS:      outV,
			SpeedBPS:    speedV,
			GraphPath:   interfacePagePath(id.Device, id.Interface),
			NetSpecPath: netspecDevicePath(opts.NetSpecBaseURL, id.Device),
		}
		if speedV != nil && *speedV > 0 {
			peak := 0.0
			if inV != nil {
				peak = math.Abs(*inV)
			}
			if outV != nil && math.Abs(*outV) > peak {
				peak = math.Abs(*outV)
			}
			u := peak / *speedV * 100
			if !math.IsNaN(u) && !math.IsInf(u, 0) {
				t.UtilPct = &u
			}
		}
		talkers = append(talkers, t)
	}

	// Heatmap uses the full filtered set; the table is truncated to Limit.
	devHeat := aggregateDeviceHeat(talkers)

	sort.Slice(talkers, func(i, j int) bool {
		return talkerScore(talkers[i]) > talkerScore(talkers[j])
	})
	if len(talkers) > opts.Limit {
		talkers = talkers[:opts.Limit]
	}

	return &FleetSnapshot{
		Range:    win,
		PortRole: opts.PortRole,
		Count:    len(talkers),
		Talkers:  talkers,
		Devices:  devHeat,
	}, nil
}

func talkerScore(t FleetTalker) float64 {
	peak := 0.0
	if t.InBPS != nil {
		peak = math.Abs(*t.InBPS)
	}
	if t.OutBPS != nil && math.Abs(*t.OutBPS) > peak {
		peak = math.Abs(*t.OutBPS)
	}
	if t.UtilPct != nil {
		// Prefer % util over raw bps so constrained links rank ahead of fat idle pipes.
		return 1e15 + *t.UtilPct
	}
	return peak
}

func aggregateDeviceHeat(talkers []FleetTalker) []FleetDeviceHeat {
	best := map[string]*float64{}
	for _, t := range talkers {
		if t.UtilPct == nil {
			continue
		}
		prev := best[t.Device]
		if prev == nil || *t.UtilPct > *prev {
			v := *t.UtilPct
			best[t.Device] = &v
		}
	}
	names := make([]string, 0, len(best))
	for d := range best {
		names = append(names, d)
	}
	// Include devices that appeared in talkers even without util (bps-only).
	seen := map[string]struct{}{}
	for _, t := range talkers {
		seen[t.Device] = struct{}{}
	}
	for d := range seen {
		if _, ok := best[d]; !ok {
			names = append(names, d)
		}
	}
	sort.Strings(names)
	// dedupe
	out := make([]FleetDeviceHeat, 0, len(seen))
	done := map[string]struct{}{}
	for _, d := range names {
		if _, ok := done[d]; ok {
			continue
		}
		done[d] = struct{}{}
		h := FleetDeviceHeat{Device: d, Sev: ""}
		if u := best[d]; u != nil {
			h.UtilPct = u
			h.Sev = utilToHexSev(*u)
		} else {
			h.Sev = "ok"
		}
		out = append(out, h)
	}
	return out
}

func utilToHexSev(pct float64) string {
	switch {
	case pct >= 80:
		return "critical"
	case pct >= 50:
		return "warning"
	default:
		return "ok"
	}
}

// RenderFleetHexSVG builds a NOC-style honeycomb colored by util buckets.
// linkBase is typically "/fleet" or "/fleet?port_role=…" (device is appended).
func RenderFleetHexSVG(devices []FleetDeviceHeat, linkBase string) string {
	names := make([]string, 0, len(devices))
	sev := map[string]string{}
	for _, d := range devices {
		names = append(names, d.Device)
		sev[d.Device] = d.Sev
	}
	layout := webui.BuildHexMapLayout(names, sev, webui.DefaultHexRadius)
	if layout.Empty {
		return `<div class="hex-empty">No devices in this filter</div>`
	}
	if linkBase == "" {
		linkBase = "/fleet"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" class="hex-map-svg" viewBox="%.4f %.4f %.4f %.4f" preserveAspectRatio="xMidYMid meet" role="img" aria-label="Utilization honeycomb">`,
		layout.ViewMinX, layout.ViewMinY, layout.ViewWidth, layout.ViewHeight,
	))
	utilByDev := map[string]*float64{}
	for _, d := range devices {
		utilByDev[d.Device] = d.UtilPct
	}
	for _, t := range layout.Tiles {
		sep := "?"
		if strings.Contains(linkBase, "?") {
			sep = "&"
		}
		href := linkBase + sep + "device=" + url.QueryEscape(t.DeviceName)
		title := t.DeviceName + " — " + t.WorstSev
		if u := utilByDev[t.DeviceName]; u != nil {
			title = fmt.Sprintf("%s — %.1f%% util", t.DeviceName, *u)
		}
		d := webui.HexPathD(t.CX, t.CY, layout.Radius)
		b.WriteString(`<a class="hex-link" href="` + htmlEscapeAttr(href) + `">`)
		b.WriteString(fmt.Sprintf(
			`<path class="hex-shape %s" d="%s" fill="%s" stroke="%s" stroke-width="%.2f"><title>%s</title></path>`,
			htmlEscapeAttr(t.Class), d, htmlEscapeAttr(t.Fill), htmlEscapeAttr(t.Stroke), t.StrokeW, htmlEscapeAttr(title),
		))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</svg>`)
	return b.String()
}

func seriesValueMap(series []vm.Series) map[string]*float64 {
	out := make(map[string]*float64, len(series))
	for _, s := range series {
		dev := s.Metric["device"]
		iface := s.Metric["interface"]
		if dev == "" || iface == "" || len(s.Points) == 0 || s.Points[0].V == nil {
			continue
		}
		v := *s.Points[0].V
		out[fleetKey(dev, iface)] = &v
	}
	return out
}

func firstValue(m map[string]*float64, keys ...string) *float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func fleetKey(device, iface string) string {
	return strings.ToLower(device) + "\x00" + ifname.Canonical(iface)
}

func netspecDevicePath(base, device string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" || device == "" {
		return ""
	}
	return base + "/device/" + url.PathEscape(device)
}

func htmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, `&`, "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, `<`, "&lt;")
	s = strings.ReplaceAll(s, `>`, "&gt;")
	return s
}

// FormatBPS renders a bits/s value for the fleet table (SI units).
func FormatBPS(v *float64) string {
	if v == nil {
		return "—"
	}
	x := math.Abs(*v)
	switch {
	case x >= 1e12:
		return fmt.Sprintf("%.2f Tbps", *v/1e12)
	case x >= 1e9:
		return fmt.Sprintf("%.2f Gbps", *v/1e9)
	case x >= 1e6:
		return fmt.Sprintf("%.2f Mbps", *v/1e6)
	case x >= 1e3:
		return fmt.Sprintf("%.1f kbps", *v/1e3)
	default:
		return fmt.Sprintf("%.0f bps", *v)
	}
}

// FormatUtilPct renders utilization percent for the fleet table.
func FormatUtilPct(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", *v)
}
