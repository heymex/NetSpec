package topology

import (
	"strings"
	"testing"

	"github.com/netspec/netspec/internal/discovery"
)

func TestRenderDOT_includesNodesAndEdges(t *testing.T) {
	t.Parallel()
	dot := RenderDOT("asw-hcd-01", []discovery.TopologyEdge{{
		LocalDevice:  "asw-hcd-01",
		LocalPort:    "Gi1/0/48",
		RemoteDevice: "dsw-hcd-01",
		RemotePort:   "Te1/1/1",
		Protocol:     "lldp",
	}})
	if !strings.Contains(dot, "digraph netspec_neighbors") {
		t.Fatal("missing digraph header")
	}
	if !strings.Contains(dot, "asw_hcd_01") || !strings.Contains(dot, "dsw_hcd_01") {
		t.Fatalf("missing nodes: %s", dot)
	}
	if !strings.Contains(dot, "Gi1/0/48") {
		t.Fatal("missing local port label")
	}
}
