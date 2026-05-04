package webui

import (
	"fmt"
	"html"
	"html/template"
	"math"
	"net/url"
	"sort"
	"strings"
)

// DefaultHexRadius is the circumradius R for pointy-top hexagons (SVG units).
const DefaultHexRadius = 22.0

// HexMaxTiles caps honeycomb size per overview panel (see GOAL_hex_host_overview.md).
const HexMaxTiles = 64

// HexAlert is minimal alert data for hex severity aggregation (avoids importing alert types).
type HexAlert struct {
	Device   string
	Severity string
}

// HexTile is one device cell in the honeycomb layout.
type HexTile struct {
	DeviceName string
	GridCol    int
	GridRow    int
	CX, CY     float64
	RawSev     string // pre-bucket severity (alerts + snmp hints)
	WorstSev   string // display bucket: ok, warning, critical
	Fill       string
	Stroke     string
	StrokeW    float64
	Class      string
}

// HexMapLayout holds computed geometry for rendering.
type HexMapLayout struct {
	Radius    float64
	ViewMinX  float64
	ViewMinY  float64
	ViewWidth float64
	ViewHeight float64
	Tiles     []HexTile
	Empty     bool // true when zero devices
}

// WorstSeverityPerDevice returns the worst severity per device from active alerts.
// Order: critical > warning > info > unknown (unknown maps like info for coloring).
func WorstSeverityPerDevice(alerts []HexAlert) map[string]string {
	out := make(map[string]string)
	for _, a := range alerts {
		dev := strings.TrimSpace(a.Device)
		if dev == "" {
			continue
		}
		sev := strings.TrimSpace(a.Severity)
		prev, ok := out[dev]
		if !ok || severityRank(sev) > severityRank(prev) {
			out[dev] = sev
		}
	}
	return out
}

// MergeHexSeverityWithSNMP overlays SNMP reachability hints onto alert-derived worst severity.
// reach values: "ok" | "unknown" | "fail" (from collector.SNMPReach* constants).
func MergeHexSeverityWithSNMP(worstFromAlerts map[string]string, reach map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range worstFromAlerts {
		out[k] = v
	}
	for dev, r := range reach {
		prev := out[dev]
		out[dev] = worseHexRaw(prev, reachToHexRaw(r))
	}
	return out
}

func reachToHexRaw(r string) string {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "fail":
		return "unreachable"
	case "unknown":
		return "unknown"
	default:
		return ""
	}
}

func worseHexRaw(a, b string) string {
	if severityRankForHex(b) > severityRankForHex(a) {
		return b
	}
	return a
}

func severityRankForHex(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "fatal", "error", "unreachable":
		return 4
	case "warning", "warn":
		return 3
	case "info":
		return 2
	case "unknown":
		return 2
	default:
		if s == "" {
			return 0
		}
		return 1
	}
}

func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "fatal", "error":
		return 4
	case "warning", "warn":
		return 3
	case "info":
		return 2
	default:
		return 1
	}
}

// DisplayBucket maps raw severity to UI bucket for fills (ok | warning | critical).
func DisplayBucket(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "critical", "fatal", "error", "unreachable":
		return "critical"
	case "warning", "warn":
		return "warning"
	case "info":
		return "warning"
	case "unknown":
		return "warning"
	default:
		if raw == "" {
			return "ok"
		}
		return "warning"
	}
}

func tileStyle(bucket string) (fill, stroke string, strokeW float64, class string) {
	switch bucket {
	case "critical":
		return "#f85149", "#30363d", 1.5, "hex-critical"
	case "warning":
		return "#d29922", "#30363d", 1.5, "hex-warning"
	default:
		return "none", "#3fb950", 1.5, "hex-ok"
	}
}

// BuildHexMapLayout packs devices in a row-major honeycomb (odd rows offset by half width).
func BuildHexMapLayout(deviceNames []string, worstSeverityByDevice map[string]string, radius float64) HexMapLayout {
	if radius <= 0 {
		radius = DefaultHexRadius
	}
	names := append([]string(nil), deviceNames...)
	sort.Strings(names)
	if len(names) > HexMaxTiles {
		names = names[:HexMaxTiles]
	}
	if len(names) == 0 {
		return HexMapLayout{Radius: radius, Empty: true}
	}

	w := radius * math.Sqrt(3)
	vSpace := radius * 2 * 0.75
	cols := int(math.Ceil(math.Sqrt(float64(len(names)))))
	if cols < 1 {
		cols = 1
	}

	tiles := make([]HexTile, 0, len(names))
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64

	for i, name := range names {
		col := i % cols
		row := i / cols
		cx := float64(col)*w + float64(row%2)*(w/2)
		cy := float64(row) * vSpace

		raw := ""
		if worstSeverityByDevice != nil {
			raw = worstSeverityByDevice[name]
		}
		bucket := DisplayBucket(raw)
		fill, stroke, sw, cls := tileStyle(bucket)

		tiles = append(tiles, HexTile{
			DeviceName: name,
			GridCol:    col,
			GridRow:    row,
			CX:         cx,
			CY:         cy,
			RawSev:     raw,
			WorstSev:   bucket,
			Fill:       fill,
			Stroke:     stroke,
			StrokeW:    sw,
			Class:      cls,
		})

		for k := 0; k < 6; k++ {
			angle := -math.Pi/2 + float64(k)*math.Pi/3
			px := cx + radius*math.Cos(angle)
			py := cy + radius*math.Sin(angle)
			minX = math.Min(minX, px)
			minY = math.Min(minY, py)
			maxX = math.Max(maxX, px)
			maxY = math.Max(maxY, py)
		}
	}

	pad := radius + 6
	vw := maxX - minX + 2*pad
	vh := maxY - minY + 2*pad
	vx := minX - pad
	vy := minY - pad

	return HexMapLayout{
		Radius:     radius,
		ViewMinX:   vx,
		ViewMinY:   vy,
		ViewWidth:  vw,
		ViewHeight: vh,
		Tiles:      tiles,
	}
}

// HexPathD returns an SVG path d attribute for a pointy-top hex centered at (cx, cy).
func HexPathD(cx, cy, R float64) string {
	var b strings.Builder
	for i := 0; i < 6; i++ {
		angle := -math.Pi/2 + float64(i)*math.Pi/3
		x := cx + R*math.Cos(angle)
		y := cy + R*math.Sin(angle)
		if i == 0 {
			fmt.Fprintf(&b, "M %.4f %.4f ", x, y)
		} else {
			fmt.Fprintf(&b, "L %.4f %.4f ", x, y)
		}
	}
	b.WriteString("Z")
	return b.String()
}

// RenderHexMapSVG returns an HTML fragment with a single <svg> honeycomb (or empty-state div).
func RenderHexMapSVG(layout HexMapLayout) template.HTML {
	if layout.Empty {
		return template.HTML(`<div class="hex-overview-empty"><p>No devices configured</p></div>`)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" class="hex-map-svg" viewBox="%.4f %.4f %.4f %.4f" preserveAspectRatio="xMidYMid meet" role="img" aria-label="Host overview honeycomb">`,
		layout.ViewMinX, layout.ViewMinY, layout.ViewWidth, layout.ViewHeight)

	for _, t := range layout.Tiles {
		d := HexPathD(t.CX, t.CY, layout.Radius)
		title := html.EscapeString(HumanHexTitle(t.DeviceName, t.RawSev, t.WorstSev))
		href := "/device/" + url.PathEscape(t.DeviceName)
		fmt.Fprintf(&b, `<a class="hex-link" href="%s">`, html.EscapeString(href))
		fmt.Fprintf(&b, `<path class="hex-shape %s" d="%s" fill="%s" stroke="%s" stroke-width="%.2f"/>`,
			html.EscapeString(t.Class), d, html.EscapeString(t.Fill), html.EscapeString(t.Stroke), t.StrokeW)
		fmt.Fprintf(&b, `<title>%s</title>`, title)
		b.WriteString(`</a>`)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

func humanWorstLabel(bucket string) string {
	switch bucket {
	case "critical":
		return "critical"
	case "warning":
		return "warning"
	default:
		return "ok"
	}
}

// HumanHexTitle builds a short tooltip label from raw severity (pre-bucket) and display bucket.
func HumanHexTitle(deviceName, rawBeforeBucket, displayBucket string) string {
	raw := strings.ToLower(strings.TrimSpace(rawBeforeBucket))
	switch raw {
	case "unreachable":
		return deviceName + " — SNMP unreachable"
	case "unknown":
		return deviceName + " — awaiting SNMP"
	default:
		return deviceName + " — " + humanWorstLabel(displayBucket)
	}
}
