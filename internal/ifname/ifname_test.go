package ifname

import "testing"

func TestCanonicalAndMatch(t *testing.T) {
	t.Parallel()
	if got, want := Canonical("TenGigabitEthernet8/1/4"), Canonical("Te8/1/4"); got != want || got == "" {
		t.Fatalf("Canonical mismatch: %q vs %q", got, want)
	}
	if !Match("TenGigabitEthernet8/1/4", "Te8/1/4") {
		t.Fatal("expected Match true")
	}
	if Match("Te8/1/4", "Te8/1/5") {
		t.Fatal("expected Match false")
	}
}

func TestResolveConfigKey(t *testing.T) {
	t.Parallel()
	keys := []string{"Gi1/0/1", "Port-channel48", "Te8/1/4"}
	if got := ResolveConfigKey(keys, "TenGigabitEthernet8/1/4"); got != "Te8/1/4" {
		t.Fatalf("ResolveConfigKey: got %q", got)
	}
	if got := ResolveConfigKey(keys, "Gi1/0/1"); got != "Gi1/0/1" {
		t.Fatalf("ResolveConfigKey exact: got %q", got)
	}
	if got := ResolveConfigKey(keys, "Ethernet666"); got != "Ethernet666" {
		t.Fatalf("ResolveConfigKey unknown: got %q", got)
	}
}
