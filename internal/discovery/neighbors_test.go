package discovery

import "testing"

func TestParseLLDPRemOID(t *testing.T) {
	t.Parallel()
	key, ok := parseLLDPRemOID("1.0.8802.1.1.2.1.4.1.1.9.0.9.1")
	if !ok || key.localPortNum != 9 || key.remIndex != 1 {
		t.Fatalf("got %+v ok=%v", key, ok)
	}
}

func TestParseCDPCacheOID(t *testing.T) {
	t.Parallel()
	ifIdx, devIdx, ok := parseCDPCacheOID("1.3.6.1.4.1.9.9.23.1.2.1.1.6.48.2")
	if !ok || ifIdx != 48 || devIdx != 2 {
		t.Fatalf("if=%d dev=%d ok=%v", ifIdx, devIdx, ok)
	}
}

func TestAttachNeighbors_buildsEdges(t *testing.T) {
	t.Parallel()
	ifaces := []Interface{{Index: 9, Name: "Gi1/9", OperStatus: "up"}}
	byIndex := map[int][]PortNeighbor{
		9: {{Protocol: "lldp", RemoteSysName: "ap-floor1", RemotePortID: "eth0"}},
	}
	out, edges := AttachNeighbors("osw-ms1-01", ifaces, byIndex)
	if len(out[0].Neighbors) != 1 {
		t.Fatalf("neighbors=%d", len(out[0].Neighbors))
	}
	if len(edges) != 1 || edges[0].RemoteDevice != "ap-floor1" {
		t.Fatalf("edges=%+v", edges)
	}
}
