package topology

import (
	"testing"

	"github.com/netspec/netspec/internal/discovery"
)

func TestRenderNeighborSVG_nonEmpty(t *testing.T) {
	t.Parallel()
	svg := RenderNeighborSVG("asw-test", []discovery.TopologyEdge{
		{LocalPort: "Gi1/0/1", RemoteDevice: "ap-1", Protocol: "lldp"},
	})
	if svg == "" || len(svg) < 100 {
		t.Fatalf("unexpected svg len %d", len(svg))
	}
	if len(svg) < 20 || svg[:4] != "<svg" {
		previewLen := 20
		if len(svg) < previewLen {
			previewLen = len(svg)
		}
		t.Fatalf("expected svg prefix, got %q", svg[:previewLen])
	}
}