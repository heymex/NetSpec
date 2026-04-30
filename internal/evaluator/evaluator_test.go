package evaluator

import (
	"fmt"
	"testing"

	"github.com/netspec/netspec/internal/config"
	"github.com/rs/zerolog"
)

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

	setMemberState(e, device, "Gi1/0/1", "up")
	setMemberState(e, device, "Gi1/0/2", "up")
	setMemberState(e, device, "Gi1/0/3", "down")
	setMemberState(e, device, "Gi1/0/4", "down")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, interfaceState{OperStatus: "up"})
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

	setMemberState(e, device, "Gi1/0/1", "up")
	setMemberState(e, device, "Gi1/0/2", "up")
	setMemberState(e, device, "Gi1/0/3", "up")
	setMemberState(e, device, "Gi1/0/4", "down")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, interfaceState{OperStatus: "up"})
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

	setMemberState(e, device, "Gi1/0/1", "down")
	setMemberState(e, device, "Gi1/0/2", "down")

	changes := e.evaluateChannelMembers(device, channel, ifaceCfg, interfaceState{OperStatus: "down"})
	if len(changes) != 1 {
		t.Fatalf("expected one change, got %d", len(changes))
	}
	if got := changes[0].Severity; got != "critical" {
		t.Fatalf("expected critical severity for channel down, got %q", got)
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
