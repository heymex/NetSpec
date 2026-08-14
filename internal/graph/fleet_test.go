package graph

import (
	"testing"
)

func TestUtilToHexSev(t *testing.T) {
	cases := []struct {
		pct  float64
		want string
	}{
		{10, "ok"},
		{49.9, "ok"},
		{50, "warning"},
		{79.9, "warning"},
		{80, "critical"},
		{100, "critical"},
	}
	for _, c := range cases {
		if got := utilToHexSev(c.pct); got != c.want {
			t.Errorf("utilToHexSev(%v)=%q want %q", c.pct, got, c.want)
		}
	}
}

func TestTalkerScorePrefersUtil(t *testing.T) {
	in := 1e9
	util := 90.0
	a := FleetTalker{InBPS: &in}
	b := FleetTalker{InBPS: &in, UtilPct: &util}
	if talkerScore(b) <= talkerScore(a) {
		t.Fatalf("util should rank higher")
	}
}

func TestNetSpecDevicePath(t *testing.T) {
	got := netspecDevicePath("http://lab:8088/", "csw-mcd-01")
	want := "http://lab:8088/device/csw-mcd-01"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if netspecDevicePath("", "x") != "" {
		t.Fatal("empty base should yield empty path")
	}
}

func TestAbsoluteGraphURLs(t *testing.T) {
	got := AbsoluteInterfaceURL("http://lab:8090/", "asw-st1-01", "Gi1/0/1")
	want := "http://lab:8090/device/asw-st1-01/interface/Gi1%2F0%2F1"
	if got != want {
		t.Fatalf("AbsoluteInterfaceURL = %q want %q", got, want)
	}
	fleet := AbsoluteFleetDeviceURL("http://lab:8090", "csw-mcd-01")
	if fleet != "http://lab:8090/fleet?device=csw-mcd-01" {
		t.Fatalf("AbsoluteFleetDeviceURL = %q", fleet)
	}
	if AbsoluteInterfaceURL("", "d", "i") != "" {
		t.Fatal("empty base")
	}
}

func TestAggregateDeviceHeat(t *testing.T) {
	u1, u2 := 40.0, 85.0
	in := 1.0
	talkers := []FleetTalker{
		{Device: "asw-a", UtilPct: &u1, InBPS: &in},
		{Device: "asw-a", UtilPct: &u2, InBPS: &in},
		{Device: "asw-b", InBPS: &in},
	}
	heat := aggregateDeviceHeat(talkers)
	by := map[string]FleetDeviceHeat{}
	for _, h := range heat {
		by[h.Device] = h
	}
	if by["asw-a"].Sev != "critical" || by["asw-a"].UtilPct == nil || *by["asw-a"].UtilPct != 85 {
		t.Fatalf("asw-a heat = %+v", by["asw-a"])
	}
	if by["asw-b"].Sev != "ok" {
		t.Fatalf("asw-b sev = %q", by["asw-b"].Sev)
	}
}
