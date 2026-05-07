package evaluator

import (
	"fmt"
	"testing"

	"github.com/netspec/netspec/internal/config"
	"github.com/rs/zerolog"
)

func TestEvaluateInterfaceSnapshotWithSource_pushSNMP_setsBothTimestamps(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DesiredState: config.DesiredStateConfig{
			Devices: map[string]config.DeviceConfig{
				"sw1": {
					Interfaces: map[string]config.InterfaceConfig{
						"Gi1/0/1": {DesiredState: "up", Monitor: true},
					},
				},
			},
		},
	}
	e := NewEvaluator(cfg, zerolog.Nop())
	e.EvaluateInterfaceSnapshotWithSource("sw1", "Gi1/0/1", "up", "enabled", "push_snmp")
	st, ok := e.GetInterfaceState("sw1", "Gi1/0/1")
	if !ok {
		t.Fatal("expected interface state")
	}
	if st.LastSNMPValidation.IsZero() {
		t.Fatal("expected LastSNMPValidation")
	}
	if st.LastTelemetryValidation.IsZero() {
		t.Fatal("expected LastTelemetryValidation")
	}

	e.EvaluateInterfaceSnapshotWithSource("sw1", "Gi1/0/1", "up", "enabled", "snmp")
	st2, _ := e.GetInterfaceState("sw1", "Gi1/0/1")
	if st2.LastTelemetryValidation.IsZero() {
		t.Fatal("SNMP-only source must not zero out prior telemetry stamp")
	}
}

func TestEvaluateChannelMembers_DefaultThresholds(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "sw1"
	channel := "Port-channel10"
	ifaceCfg := config.InterfaceConfig{
		DesiredState: "up",
		Monitor:      true,
		Members: &config.MemberConfig{
			Required: []string{"Gi1/0/1", "Gi1/0/2", "Gi1/0/3", "Gi1/0/4"},
		},
		MemberPolicy: &config.MemberPolicy{
			Mode:    "min_active",
			Minimum: 2,
		},
	}
	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Gi1/0/1": {},
			"Gi1/0/2": {},
			"Gi1/0/3": {},
			"Gi1/0/4": {},
		},
	}

	setMemberState(e, device, "Gi1/0/1", "up")
	setMemberState(e, device, "Gi1/0/2", "up")
	setMemberState(e, device, "Gi1/0/3", "down")
	setMemberState(e, device, "Gi1/0/4", "down")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	if len(changes) != 1 {
		t.Fatalf("expected one change, got %d", len(changes))
	}
	if got := changes[0].Severity; got != "critical" {
		t.Fatalf("expected critical severity at 50%% down, got %q", got)
	}
	if got := changes[0].RelatedState["down_pct"]; got != "50.0" {
		t.Fatalf("expected down_pct=50.0, got %q", got)
	}
}

func TestEvaluateChannelMembers_WarningThresholdSuppressesLowLoss(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "sw1"
	channel := "Port-channel20"
	ifaceCfg := config.InterfaceConfig{
		DesiredState: "up",
		Monitor:      true,
		Members: &config.MemberConfig{
			Required: []string{"Gi1/0/1", "Gi1/0/2", "Gi1/0/3", "Gi1/0/4"},
		},
		MemberPolicy: &config.MemberPolicy{
			Mode:                "min_active",
			Minimum:             3,
			WarningThresholdPct: ptrFloat(30),
		},
	}

	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Gi1/0/1": {},
			"Gi1/0/2": {},
			"Gi1/0/3": {},
			"Gi1/0/4": {},
		},
	}

	setMemberState(e, device, "Gi1/0/1", "up")
	setMemberState(e, device, "Gi1/0/2", "up")
	setMemberState(e, device, "Gi1/0/3", "up")
	setMemberState(e, device, "Gi1/0/4", "down")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	if len(changes) != 0 {
		t.Fatalf("expected no alert when down_pct <= warning threshold, got %d", len(changes))
	}
}

func TestEvaluateChannelMembers_ChannelDownAlwaysCritical(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "sw1"
	channel := "Port-channel30"
	ifaceCfg := config.InterfaceConfig{
		DesiredState: "up",
		Monitor:      true,
		Members: &config.MemberConfig{
			Required: []string{"Gi1/0/1", "Gi1/0/2"},
		},
		MemberPolicy: &config.MemberPolicy{
			Mode:    "min_active",
			Minimum: 1,
		},
		Alerts: config.AlertSeverity{
			ChannelDown: "info",
		},
	}
	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Gi1/0/1": {},
			"Gi1/0/2": {},
		},
	}

	setMemberState(e, device, "Gi1/0/1", "down")
	setMemberState(e, device, "Gi1/0/2", "down")
	setMemberState(e, device, channel, "down")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	if len(changes) != 1 {
		t.Fatalf("expected one change, got %d", len(changes))
	}
	if got := changes[0].Severity; got != "critical" {
		t.Fatalf("expected critical severity for channel down, got %q", got)
	}
}

func TestEvaluateChannelMembers_RequiredLongNamesMatchShortCacheKeys(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "sw1"
	channel := "Port-channel48"
	ifaceCfg := config.InterfaceConfig{
		Members: &config.MemberConfig{
			Required: []string{
				"TenGigabitEthernet8/1/1",
				"TenGigabitEthernet8/1/2",
				"TenGigabitEthernet8/1/3",
				"TenGigabitEthernet8/1/4",
			},
		},
		MemberPolicy: &config.MemberPolicy{},
	}
	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Te8/1/1": {},
			"Te8/1/2": {},
			"Te8/1/3": {},
			"Te8/1/4": {},
		},
	}

	setMemberState(e, device, "Te8/1/1", "up")
	setMemberState(e, device, "Te8/1/2", "up")
	setMemberState(e, device, "Te8/1/3", "up")
	setMemberState(e, device, "Te8/1/4", "down")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	if len(changes) != 1 {
		t.Fatalf("expected one change, got %d", len(changes))
	}
	if got := changes[0].AlertType; got != alertTypeMemberDown {
		t.Fatalf("expected member down alert, got %q", got)
	}
}

func TestEvaluateChannelMembers_SingleMemberDownNotPortChannelDown(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "sw1"
	channel := "Port-channel48"
	ifaceCfg := config.InterfaceConfig{
		Members: &config.MemberConfig{
			Required: []string{"Te8/1/1", "Te8/1/2"},
		},
		MemberPolicy: &config.MemberPolicy{},
	}
	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Te8/1/1": {},
			"Te8/1/2": {},
		},
	}

	setMemberState(e, device, "Te8/1/1", "up")
	setMemberState(e, device, "Te8/1/2", "down")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	for _, c := range changes {
		if c.AlertType == alertTypeChannelDown {
			t.Fatalf("unexpected port_channel_down when LAG oper state is unknown in cache")
		}
	}
}

func setMemberState(e *Evaluator, device, member, oper string) {
	key := fmt.Sprintf("%s:%s", device, member)
	e.mu.Lock()
	e.stateCache[key] = interfaceState{OperStatus: oper}
	e.mu.Unlock()
}

func ptrFloat(v float64) *float64 {
	return &v
}
