package discovery

import "testing"

func TestNormalizePortLabel_CiscoGi(t *testing.T) {
	t.Parallel()
	if got, want := normalizePortLabel("GigabitEthernet1/0/39"), normalizePortLabel("Gi1/0/39"); got != want {
		t.Fatalf("normalize: got %q want %q", got, want)
	}
}

func TestMatchPortDescToIfIndex(t *testing.T) {
	t.Parallel()
	ifaces := []Interface{
		{Index: 101, Name: "Gi1/0/39"},
		{Index: 102, Name: "Te1/1/1"},
	}
	if got := matchPortDescToIfIndex("GigabitEthernet1/0/39", ifaces); got != 101 {
		t.Fatalf("ifIndex=%d", got)
	}
	if got := matchPortDescToIfIndex("Gi1/0/39", ifaces); got != 101 {
		t.Fatalf("ifIndex=%d", got)
	}
}

func TestResolveLLDPLocalIfIndex_tableForcesMap(t *testing.T) {
	t.Parallel()
	ifaces := []Interface{{Index: 48, Name: "Gi1/0/1"}}
	// Table present but port 9 not in map — must not fall back to ifIndex 9.
	if got := resolveLLDPLocalIfIndex(map[int]int{10: 48}, true, 9, ifaces); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
	if got := resolveLLDPLocalIfIndex(map[int]int{9: 48}, true, 9, ifaces); got != 48 {
		t.Fatalf("got %d want 48", got)
	}
}

func TestResolveLLDPLocalIfIndex_legacyIndex(t *testing.T) {
	t.Parallel()
	ifaces := []Interface{{Index: 5, Name: "Gi0/5"}}
	if got := resolveLLDPLocalIfIndex(nil, false, 5, ifaces); got != 5 {
		t.Fatalf("got %d", got)
	}
}

func TestAttachNeighbors_skipsDownPorts(t *testing.T) {
	t.Parallel()
	ifaces := []Interface{{Index: 9, Name: "Gi1/9", OperStatus: "down"}}
	byIndex := map[int][]PortNeighbor{
		9: {{Protocol: "lldp", RemoteSysName: "ap-floor1"}},
	}
	out, edges := AttachNeighbors("sw", ifaces, byIndex)
	if len(out[0].Neighbors) != 0 || len(edges) != 0 {
		t.Fatalf("neighbors=%d edges=%d", len(out[0].Neighbors), len(edges))
	}
}
