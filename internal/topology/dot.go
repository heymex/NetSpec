package topology

import (
	"fmt"
	"strings"

	"github.com/netspec/netspec/internal/discovery"
)

// RenderDOT builds a directed Graphviz graph for one device's walk topology.
func RenderDOT(hostname string, edges []discovery.TopologyEdge) string {
	local := sanitizeID(hostname)
	if local == "" {
		local = "local"
	}
	var b strings.Builder
	b.WriteString("digraph netspec_neighbors {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box, style=rounded, fontname=Helvetica];\n")
	b.WriteString("  edge [fontname=Helvetica, fontsize=10];\n")
	b.WriteString(fmt.Sprintf("  %q [label=%q, fillcolor=\"#238636\", style=\"rounded,filled\", fontcolor=white];\n",
		local, hostname))

	seenRemote := map[string]struct{}{}
	for _, e := range edges {
		remote := sanitizeID(e.RemoteDevice)
		if remote == "" {
			continue
		}
		if _, ok := seenRemote[remote]; !ok {
			seenRemote[remote] = struct{}{}
			b.WriteString(fmt.Sprintf("  %q [label=%q];\n", remote, e.RemoteDevice))
		}
		edgeLabel := e.Protocol
		if e.RemotePort != "" {
			edgeLabel = edgeLabel + " " + e.RemotePort
		}
		b.WriteString(fmt.Sprintf("  %q -> %q [label=%q, taillabel=%q];\n",
			local, remote, edgeLabel, e.LocalPort))
	}
	b.WriteString("}\n")
	return b.String()
}

func sanitizeID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
