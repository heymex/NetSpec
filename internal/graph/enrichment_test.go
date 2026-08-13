package graph

import (
	"testing"

	"github.com/netspec/netspec/internal/config"
)

func testBool(v bool) *bool { return &v }

func sampleConfig() *config.Config {
	return &config.Config{
		DesiredState: config.DesiredStateConfig{
			Devices: map[string]config.DeviceConfig{
				"asw-hb1-01": {
					Interfaces: map[string]config.InterfaceConfig{
						"Gi1/0/1":  {Description: "ap-floor1-01", DesiredState: "up", Monitor: false},
						"Gi1/0/2":  {Description: "ap-floor1-02", DesiredState: "up", Monitor: false},
						"Gi1/0/10": {Description: "bldg-uplink-hb1", DesiredState: "up", Monitor: true},
						"Po1":      {Description: "po1-uplink", DesiredState: "up", Monitor: true},
						"Gi1/0/48": {Description: "user-desk-12", DesiredState: "up", Monitor: true},
					},
				},
				"asw-hb2-01": {
					Interfaces: map[string]config.InterfaceConfig{
						"Gi1/0/1": {Description: "ap-lobby-01", DesiredState: "up", Monitor: false},
						"Po1":     {Description: "po1-uplink", DesiredState: "up", Monitor: true},
					},
				},
				"csw-mcd-01": {
					Interfaces: map[string]config.InterfaceConfig{
						"Port-channel20": {Description: "po20-core", DesiredState: "up", Monitor: true},
						"Hu1/0/1":        {Description: "t|po20|dsw-mcd-01:hu1/0/1|po1.", DesiredState: "up", Monitor: true},
					},
				},
				"rtr-edge-01": {
					Interfaces: map[string]config.InterfaceConfig{
						"Gi0/0": {Description: "wan", DesiredState: "up", Monitor: true},
					},
				},
			},
		},
		Rules: config.RulesConfig{
			DeviceRoles: []config.DeviceRole{
				{
					Prefix: "csw",
					Name:   "Core Switch",
					PortRules: []config.PortRule{
						{Label: "Trunk / Uplink Ports", Match: "t|*", Monitor: testBool(true), DesiredState: "up"},
						{Label: "Port-Channel Uplinks", Match: "po*", Monitor: testBool(true), DesiredState: "up"},
					},
				},
				{
					Prefix: "asw",
					Name:   "Access Switch",
					PortRules: []config.PortRule{
						{Label: "Trunk / Uplink Ports", Match: "t|*", Monitor: testBool(true), DesiredState: "up"},
						{Label: "Port-Channel Uplinks", Match: "po*", Monitor: testBool(true), DesiredState: "up"},
						{Label: "Wireless APs", Match: "ap*", Monitor: testBool(false)},
						{Label: "Building / Infrastructure", Match: "bldg*", Monitor: testBool(true), DesiredState: "up"},
					},
				},
			},
		},
	}
}

func TestBuildIndexPortRoles(t *testing.T) {
	idx := BuildIndex(sampleConfig())
	if idx.DeviceCount() != 4 {
		t.Fatalf("DeviceCount = %d, want 4", idx.DeviceCount())
	}
	if idx.Len() != 10 {
		t.Fatalf("Len = %d, want 10", idx.Len())
	}

	id, ok := idx.Lookup("asw-hb1-01", "Gi1/0/1")
	if !ok {
		t.Fatal("Lookup Gi1/0/1 missing")
	}
	if id.PortRole != "Wireless APs" {
		t.Errorf("PortRole = %q, want Wireless APs", id.PortRole)
	}
	if id.DeviceRole != "Access Switch" || id.DeviceRolePrefix != "asw" {
		t.Errorf("device role = %q/%q", id.DeviceRole, id.DeviceRolePrefix)
	}
	if id.Monitored {
		t.Error("AP port should be monitored=false in fixture")
	}
	if id.Alias != "ap-floor1-01" {
		t.Errorf("Alias = %q", id.Alias)
	}
}

func TestLookupTelemetryNativeName(t *testing.T) {
	idx := BuildIndex(sampleConfig())
	// VM stores telemetry-native names; config key is short SNMP form.
	id, ok := idx.Lookup("asw-hb1-01", "GigabitEthernet1/0/10")
	if !ok {
		t.Fatal("Lookup GigabitEthernet1/0/10 failed")
	}
	if id.Interface != "Gi1/0/10" {
		t.Errorf("Interface = %q, want Gi1/0/10", id.Interface)
	}
	if id.PortRole != "Building / Infrastructure" {
		t.Errorf("PortRole = %q", id.PortRole)
	}

	id, ok = idx.Lookup("csw-mcd-01", "port-channel20")
	if !ok {
		t.Fatal("Lookup port-channel20 failed")
	}
	if id.Interface != "Port-channel20" {
		t.Errorf("Interface = %q, want Port-channel20", id.Interface)
	}
	if id.PortRole != "Port-Channel Uplinks" {
		t.Errorf("PortRole = %q", id.PortRole)
	}
}

func TestFilterWirelessAPs(t *testing.T) {
	idx := BuildIndex(sampleConfig())
	got := idx.Filter(Filter{PortRole: "Wireless APs"})
	if len(got) != 3 {
		t.Fatalf("Wireless APs count = %d, want 3", len(got))
	}
	for _, id := range got {
		if id.PortRole != "Wireless APs" {
			t.Errorf("unexpected PortRole %q", id.PortRole)
		}
		if id.DeviceRolePrefix != "asw" {
			t.Errorf("AP on non-asw device %s", id.Device)
		}
	}
}

func TestFilterDevicePrefixMatchesNOC(t *testing.T) {
	idx := BuildIndex(sampleConfig())
	got := idx.Filter(Filter{DevicePrefix: "asw"})
	if len(got) != 7 {
		t.Fatalf("asw interfaces = %d, want 7", len(got))
	}
	for _, id := range got {
		if id.DeviceRole != "Access Switch" {
			t.Errorf("%s role = %q", id.Device, id.DeviceRole)
		}
	}

	// Unmatched hostname prefix → no device role, still listed when filtering by device only.
	other := idx.Filter(Filter{Device: "rtr-edge-01"})
	if len(other) != 1 || other[0].DeviceRole != "" {
		t.Fatalf("rtr-edge-01 = %+v", other)
	}
}

func TestFilterPortChannelUplinksMonitored(t *testing.T) {
	idx := BuildIndex(sampleConfig())
	mon := true
	got := idx.Filter(Filter{PortRole: "Port-Channel Uplinks", Monitored: &mon})
	if len(got) != 3 {
		t.Fatalf("Po uplinks monitored = %d, want 3", len(got))
	}
	want := map[string]bool{
		"asw-hb1-01/Po1":           true,
		"asw-hb2-01/Po1":           true,
		"csw-mcd-01/Port-channel20": true,
	}
	for _, id := range got {
		key := id.Device + "/" + id.Interface
		if !want[key] {
			t.Errorf("unexpected %s", key)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("missing %+v", want)
	}
}

func TestFilterBuildingHB1(t *testing.T) {
	idx := BuildIndex(sampleConfig())
	got := idx.Filter(Filter{DevicePrefix: "asw-hb1", PortRole: "Wireless APs"})
	if len(got) != 2 {
		t.Fatalf("HB1 APs = %d, want 2", len(got))
	}
}

func TestPortRoleLabels(t *testing.T) {
	idx := BuildIndex(sampleConfig())
	labels := idx.PortRoleLabels()
	want := []string{"Building / Infrastructure", "Port-Channel Uplinks", "Trunk / Uplink Ports", "Wireless APs"}
	if len(labels) != len(want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Errorf("labels[%d] = %q, want %q", i, labels[i], want[i])
		}
	}
}

func TestBuildIndexNilSafe(t *testing.T) {
	idx := BuildIndex(nil)
	if idx.Len() != 0 {
		t.Fatal("nil config should yield empty index")
	}
	if _, ok := idx.Lookup("x", "y"); ok {
		t.Fatal("lookup on empty should fail")
	}
	if got := idx.Filter(Filter{PortRole: "Wireless APs"}); len(got) != 0 {
		t.Fatalf("filter on empty = %d", len(got))
	}
}
