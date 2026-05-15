package topology

import (
	"fmt"
	"math"
	"strings"

	"github.com/netspec/netspec/internal/discovery"
)

// RenderNeighborSVG draws a simple hub-and-spoke diagram (local device center,
// neighbors around) for wizard preview without Graphviz.
func RenderNeighborSVG(hostname string, edges []discovery.TopologyEdge) string {
	host := strings.TrimSpace(hostname)
	if host == "" {
		host = "device"
	}
	if len(edges) == 0 {
		return ""
	}
	const w, h = 520, 320
	cx, cy := w/2, h/2
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" class="netspec-topology-svg" role="img" aria-label="LLDP neighbor sketch">`, w, h))
	b.WriteString(`<defs><style>.ns-t{fill:#e6edf3;font-family:system-ui,sans-serif;font-size:11px}.ns-s{fill:#8b949e;font-size:9px}.ns-edge{stroke:#30363d;stroke-width:1.2}</style></defs>`)
	b.WriteString(`<rect width="100%" height="100%" fill="#0d1117"/>`)

	n := len(edges)
	radius := 118
	if n > 8 {
		radius = 132
	}
	for i, e := range edges {
		ang := -math.Pi/2 + (2*math.Pi*float64(i)/float64(max(1, n)))
		x2 := cx + int(float64(radius)*math.Cos(ang))
		y2 := cy + int(float64(radius)*math.Sin(ang))
		b.WriteString(fmt.Sprintf(`<line class="ns-edge" x1="%d" y1="%d" x2="%d" y2="%d"/>`, cx, cy, x2, y2))
		rd := strings.TrimSpace(e.RemoteDevice)
		if rd == "" {
			rd = "?"
		}
		lp := strings.TrimSpace(e.LocalPort)
		label := rd
		if lp != "" {
			label = lp + " → " + rd
		}
		// Remote node box
		tw := minInt(140, 8+len(label)*6)
		b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="26" rx="4" fill="#161b22" stroke="#30363d"/>`, x2-tw/2, y2-13, tw))
		b.WriteString(fmt.Sprintf(`<text class="ns-t" x="%d" y="%d" text-anchor="middle">%s</text>`, x2, y2+4, escapeXML(trunc(label, 28))))
		proto := strings.ToUpper(strings.TrimSpace(e.Protocol))
		if proto != "" {
			b.WriteString(fmt.Sprintf(`<text class="ns-s" x="%d" y="%d" text-anchor="middle">%s</text>`, x2, y2+20, escapeXML(proto)))
		}
	}

	// Local hub
	b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="44" fill="#238636" stroke="#2ea043" stroke-width="2"/>`, cx, cy))
	b.WriteString(fmt.Sprintf(`<text class="ns-t" x="%d" y="%d" text-anchor="middle" fill="#fff" font-weight="600">%s</text>`, cx, cy+4, escapeXML(trunc(host, 22))))
	b.WriteString(fmt.Sprintf(`<text class="ns-s" x="%d" y="%d" text-anchor="middle" fill="#c9d1d9">local</text>`, cx, cy+18))
	b.WriteString(`</svg>`)
	return b.String()
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, `&`, `&amp;`)
	s = strings.ReplaceAll(s, `<`, `&lt;`)
	s = strings.ReplaceAll(s, `>`, `&gt;`)
	s = strings.ReplaceAll(s, `"`, `&quot;`)
	return s
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
