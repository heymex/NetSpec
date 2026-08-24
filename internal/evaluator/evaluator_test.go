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
	if len(changes) != 1 {
		t.Fatalf("expected resolve when down_pct <= warning threshold, got %d", len(changes))
	}
	if !changes[0].Resolved || changes[0].AlertType != alertTypeMemberDown {
		t.Fatalf("expected resolved member_down, got %#v", changes[0])
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

func TestEvaluateChannelMembers_MissingCacheNotCountedDown(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "asw-at1-01"
	channel := "Port-channel48"
	ifaceCfg := config.InterfaceConfig{
		Members: &config.MemberConfig{
			Required: []string{"Te1/1/4", "Te8/1/4"},
		},
		MemberPolicy: &config.MemberPolicy{},
	}
	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Te1/1/4": {},
			"Te8/1/4": {},
		},
	}

	// Cold start / partial hydration: channel seen up, members not yet in cache.
	setMemberState(e, device, channel, "up")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	for _, c := range changes {
		if c.AlertType == alertTypeMemberDown && !c.Resolved {
			t.Fatalf("missing member cache must not fire member_down; got %#v", c)
		}
	}
}

func TestEvaluateChannelMembers_UnknownOperNotCountedDown(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "sw1"
	channel := "Port-channel48"
	ifaceCfg := config.InterfaceConfig{
		Members: &config.MemberConfig{
			Required: []string{"Te1/1/4", "Te8/1/4"},
		},
		MemberPolicy: &config.MemberPolicy{},
	}
	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Te1/1/4": {},
			"Te8/1/4": {},
		},
	}

	setMemberState(e, device, channel, "up")
	setMemberState(e, device, "Te1/1/4", "up")
	setMemberState(e, device, "Te8/1/4", "unknown")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	for _, c := range changes {
		if c.AlertType == alertTypeMemberDown && !c.Resolved {
			t.Fatalf("unknown member oper must not fire member_down; got %#v", c)
		}
	}
}

func TestEvaluateChannelMembers_PartialHydrationThenHealthyResolve(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "asw-at1-01"
	channel := "Port-channel48"
	ifaceCfg := config.InterfaceConfig{
		DesiredState: "up",
		Monitor:      true,
		Members: &config.MemberConfig{
			Required: []string{"Te1/1/4", "Te8/1/4"},
		},
		MemberPolicy: &config.MemberPolicy{},
	}
	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Te1/1/4": {DesiredState: "up", Monitor: true},
			"Te8/1/4": {DesiredState: "up", Monitor: true},
		},
	}
	cfg := &config.Config{DesiredState: config.DesiredStateConfig{
		Devices: map[string]config.DeviceConfig{device: deviceCfg},
	}}
	e.config = cfg

	// Startup ordering: evaluate channel before members are cached — must not fire.
	early := e.EvaluateInterfaceSnapshot(device, channel, "up", "enabled")
	for _, c := range early {
		if c.AlertType == alertTypeMemberDown && !c.Resolved {
			t.Fatalf("early channel snapshot must not fire member_down; got %#v", c)
		}
	}

	// First member arrives alone — still incomplete, no fire.
	mid := e.EvaluateInterfaceSnapshot(device, "Te1/1/4", "up", "enabled")
	for _, c := range mid {
		if c.AlertType == alertTypeMemberDown && !c.Resolved {
			t.Fatalf("partial member hydration must not fire member_down; got %#v", c)
		}
	}

	// Second member up completes hydration → explicit resolve (clears sticky alerts).
	done := e.EvaluateInterfaceSnapshot(device, "Te8/1/4", "up", "enabled")
	var sawResolve bool
	for _, c := range done {
		if c.AlertType == alertTypeMemberDown && c.Resolved {
			sawResolve = true
		}
		if c.AlertType == alertTypeMemberDown && !c.Resolved {
			t.Fatalf("healthy hydration must not fire member_down; got %#v", c)
		}
	}
	if !sawResolve {
		t.Fatal("expected resolved member_down once all members are known up")
	}
}

func TestEvaluateChannelMembers_DownThenUpEmitsResolve(t *testing.T) {
	t.Parallel()

	e := NewEvaluator(nil, zerolog.Nop())
	device := "sw1"
	channel := "Port-channel48"
	ifaceCfg := config.InterfaceConfig{
		Members: &config.MemberConfig{
			Required: []string{"Te1/1/4", "Te8/1/4"},
		},
		MemberPolicy: &config.MemberPolicy{},
	}
	deviceCfg := config.DeviceConfig{
		Interfaces: map[string]config.InterfaceConfig{
			channel:   ifaceCfg,
			"Te1/1/4": {},
			"Te8/1/4": {},
		},
	}

	setMemberState(e, device, channel, "up")
	setMemberState(e, device, "Te1/1/4", "down")
	setMemberState(e, device, "Te8/1/4", "down")

	down := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	var fired bool
	for _, c := range down {
		if c.AlertType == alertTypeMemberDown && !c.Resolved {
			fired = true
		}
	}
	if !fired {
		t.Fatal("expected member_down fire when both members are down")
	}

	setMemberState(e, device, "Te1/1/4", "up")
	setMemberState(e, device, "Te8/1/4", "up")
	up := e.evaluateChannelMembers(device, channel, ifaceCfg, deviceCfg)
	var resolved bool
	for _, c := range up {
		if c.AlertType == alertTypeMemberDown && c.Resolved {
			resolved = true
		}
		if c.AlertType == alertTypeMemberDown && !c.Resolved {
			t.Fatalf("recovery must not re-fire member_down; got %#v", c)
		}
	}
	if !resolved {
		t.Fatal("expected resolved member_down after members recover")
	}
}

func TestClassifyMemberOper(t *testing.T) {
	t.Parallel()
	cases := []struct {
		present bool
		oper    string
		want    string
	}{
		{false, "", "unknown"},
		{true, "", "unknown"},
		{true, "unknown", "unknown"},
		{true, "up", "up"},
		{true, "UP", "up"},
		{true, "down", "down"},
	}
	for _, tc := range cases {
		if got := classifyMemberOper(tc.present, tc.oper); got != tc.want {
			t.Fatalf("classifyMemberOper(%v,%q)=%q want %q", tc.present, tc.oper, got, tc.want)
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
