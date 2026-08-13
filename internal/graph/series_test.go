package graph

import (
	"testing"

	"github.com/netspec/netspec/internal/graph/vm"
)

func TestEscapeLabel(t *testing.T) {
	got := vm.EscapeLabel(`core"sw\01`)
	want := `core\"sw\\01`
	if got != want {
		t.Fatalf("EscapeLabel: got %q want %q", got, want)
	}
}

func TestSelector(t *testing.T) {
	got := vm.Selector("csw-mcd-01", "GigabitEthernet1/0/1")
	want := `{device="csw-mcd-01",interface="GigabitEthernet1/0/1"}`
	if got != want {
		t.Fatalf("Selector: got %q want %q", got, want)
	}
}

func TestScalePointsAndNilUtil(t *testing.T) {
	v1, v2 := 1e9, 2e9
	pts := []Point{{T: 1, V: &v1}, {T: 2, V: &v2}, {T: 3, V: nil}}
	scaled := scalePoints(pts, 100.0/1e10) // 10 Gbps
	if scaled[0].V == nil || mathAbs(*scaled[0].V-10) > 0.01 {
		t.Fatalf("expected ~10%% util, got %+v", scaled[0])
	}
	nilled := nilPoints(pts)
	if nilled[0].V != nil || nilled[1].V != nil {
		t.Fatalf("nilPoints should clear values: %+v", nilled)
	}
}

func TestParseDeviceInterfacePath(t *testing.T) {
	cases := []struct {
		raw        string
		wantDevice string
		wantIface  string
		wantOK     bool
	}{
		{"/device/csw-mcd-01/interface/Port-channel20", "csw-mcd-01", "Port-channel20", true},
		{"/device/csw-mcd-01/interface/GigabitEthernet1%2F0%2F1", "csw-mcd-01", "GigabitEthernet1/0/1", true},
		{"/device/csw-mcd-01/interface/GigabitEthernet1/0/1", "csw-mcd-01", "GigabitEthernet1/0/1", true},
		{"/api/device/csw-mcd-01/interface/Port-channel20/series", "csw-mcd-01", "Port-channel20", true},
		{"/device/only", "", "", false},
	}
	for _, tc := range cases {
		d, iface, ok := parseDeviceInterfacePath(tc.raw)
		if ok != tc.wantOK || d != tc.wantDevice || iface != tc.wantIface {
			t.Fatalf("%s: got (%q,%q,%v) want (%q,%q,%v)", tc.raw, d, iface, ok, tc.wantDevice, tc.wantIface, tc.wantOK)
		}
	}
}

func mathAbs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
