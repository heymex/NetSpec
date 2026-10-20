package alerter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/evaluator"
	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

func TestProcessStateChange_ResolvedClearsActiveAlert(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			AlertBehavior: config.AlertBehavior{
				DeduplicationWindow: time.Minute,
			},
			AlertRules: map[string]config.AlertRule{
				"default": {Channels: nil},
			},
		},
	}
	engine := NewEngine(cfg, nil, zerolog.Nop(), "")

	engine.ProcessStateChange(evaluator.StateChange{
		Device:    "asw-at1-01",
		Interface: "Port-channel48",
		AlertType: "port_channel_member_down",
		Severity:  "critical",
		Message:   "port-channel Port-channel48 has 2/2 members down: Te1/1/4, Te8/1/4",
	})
	// Drain the async event.
	engine.process(<-engine.events)

	active := engine.GetActiveAlerts()
	if len(active) != 1 {
		t.Fatalf("expected 1 active alert, got %d", len(active))
	}

	engine.ProcessStateChange(evaluator.StateChange{
		Device:    "asw-at1-01",
		Interface: "Port-channel48",
		AlertType: "port_channel_member_down",
		Severity:  "info",
		Message:   "port-channel Port-channel48 member policy healthy (2/2 members up)",
		Resolved:  true,
	})
	engine.process(<-engine.events)

	if got := engine.GetActiveAlerts(); len(got) != 0 {
		t.Fatalf("expected alert cleared after resolve, still have %d: %#v", len(got), got)
	}
}

func TestProcessStateChange_ResolvedNoopWithoutActive(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			AlertBehavior: config.AlertBehavior{DeduplicationWindow: time.Minute},
		},
	}
	engine := NewEngine(cfg, nil, zerolog.Nop(), "")
	engine.ProcessStateChange(evaluator.StateChange{
		Device:    "sw1",
		Interface: "Port-channel1",
		AlertType: "port_channel_member_down",
		Resolved:  true,
		Message:   "healthy",
	})
	engine.process(<-engine.events)
	if got := engine.GetActiveAlerts(); len(got) != 0 {
		t.Fatalf("unexpected alerts: %#v", got)
	}
}

// Ensure UpsertActiveAlertForTest still works with resolve path identity.
func TestProcessStateChange_ResolvedMatchesUpsertedAlert(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			AlertBehavior: config.AlertBehavior{DeduplicationWindow: time.Minute},
		},
	}
	engine := NewEngine(cfg, nil, zerolog.Nop(), "")
	engine.UpsertActiveAlertForTest(&types.Alert{
		ID:        "legacy",
		Device:    "asw-at1-01",
		Entity:    "Port-channel48",
		AlertType: "port_channel_member_down",
		Severity:  "critical",
		State:     "firing",
		FiredAt:   time.Now().Add(-3 * time.Hour),
		Message:   "stale false positive from prior restart",
	})

	engine.ProcessStateChange(evaluator.StateChange{
		Device:    "asw-at1-01",
		Interface: "Port-channel48",
		AlertType: "port_channel_member_down",
		Resolved:  true,
		Message:   "recovered",
	})
	engine.process(<-engine.events)

	if got := engine.GetActiveAlerts(); len(got) != 0 {
		t.Fatalf("stale alert should clear on resolve: %#v", got)
	}
}

func TestCloseAlert_RefiresAfterDedupWindow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	engine := newTestEngine(t, dir, time.Hour)
	fireMismatch(engine, "sw1", "Gi1/0/1")

	active := engine.GetActiveAlerts()
	if len(active) != 1 {
		t.Fatalf("expected 1 active alert, got %d", len(active))
	}
	if _, err := engine.CloseAlert(active[0].ID, "tester"); err != nil {
		t.Fatal(err)
	}
	if got := engine.GetActiveAlerts(); len(got) != 0 {
		t.Fatalf("close should remove the alert, still have %#v", got)
	}
	key := "sw1|Gi1/0/1|interface_state_mismatch"
	if _, ok := engine.suppressedUntil[key]; ok {
		t.Fatalf("close must not latch %s off", key)
	}

	// The condition is still true, but the close stamps lastFired so the
	// dedup window holds.
	fireMismatch(engine, "sw1", "Gi1/0/1")
	if got := engine.GetActiveAlerts(); len(got) != 0 {
		t.Fatalf("alert re-fired inside the dedup window: %#v", got)
	}

	engine.mu.Lock()
	engine.lastFired[key] = time.Now().Add(-2 * time.Hour)
	engine.mu.Unlock()
	fireMismatch(engine, "sw1", "Gi1/0/1")
	if got := engine.GetActiveAlerts(); len(got) != 1 {
		t.Fatalf("persisting fault should re-fire after the dedup window, got %#v", got)
	}

	// Restart must not bring the indefinite latch back. The re-fired alert is
	// still inside its own dedup window, so a second sample stays quiet, and
	// the restored alert is the one we just fired.
	restarted := newTestEngine(t, dir, time.Hour)
	if _, ok := restarted.suppressedUntil[key]; ok {
		t.Fatal("restart restored a close suppression")
	}
	if got := restarted.GetActiveAlerts(); len(got) != 1 {
		t.Fatalf("restart should keep the re-fired alert, got %#v", got)
	}
}

func TestLoadState_DropsIndefiniteCloseSuppression(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	key := "sw1|Gi1/0/1|interface_state_mismatch"
	state := persistedState{
		ActiveAlerts: map[string]*types.Alert{
			key: {
				ID:        "closed-yesterday",
				Device:    "sw1",
				Entity:    "Gi1/0/1",
				AlertType: "interface_state_mismatch",
				Severity:  "critical",
				State:     "resolved",
				Message:   "Manually closed by tester",
			},
		},
		LastFired: map[string]time.Time{
			key: time.Now().Add(-24 * time.Hour),
		},
		SuppressedUntil: map[string]time.Time{
			key: {},
		},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alert-state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	engine := newTestEngine(t, dir, time.Minute)
	if got := engine.GetActiveAlerts(); len(got) != 0 {
		t.Fatalf("resolved tombstone should not be active, got %#v", got)
	}
	if _, ok := engine.suppressedUntil[key]; ok {
		t.Fatal("zero-time suppression was restored")
	}

	fireMismatch(engine, "sw1", "Gi1/0/1")
	if got := engine.GetActiveAlerts(); len(got) != 1 {
		t.Fatalf("fault hidden by an old close should fire again, got %#v", got)
	}
}

func TestProcessStateChange_ResolveAllowsImmediateRefire(t *testing.T) {
	t.Parallel()

	engine := newTestEngine(t, "", time.Hour)
	fireMismatch(engine, "sw1", "Gi1/0/1")
	engine.ProcessStateChange(evaluator.StateChange{
		Device:    "sw1",
		Interface: "Gi1/0/1",
		AlertType: "interface_state_mismatch",
		Severity:  "info",
		Message:   "interface Gi1/0/1 recovered to up",
		Resolved:  true,
	})
	engine.process(<-engine.events)
	if got := engine.GetActiveAlerts(); len(got) != 0 {
		t.Fatalf("expected recovery to clear the alert, got %#v", got)
	}

	fireMismatch(engine, "sw1", "Gi1/0/1")
	if got := engine.GetActiveAlerts(); len(got) != 1 {
		t.Fatalf("recurrence after recovery should fire inside the dedup window, got %#v", got)
	}
}

func newTestEngine(t *testing.T, dir string, window time.Duration) *Engine {
	t.Helper()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			AlertBehavior: config.AlertBehavior{DeduplicationWindow: window},
		},
	}
	return NewEngine(cfg, nil, zerolog.Nop(), dir)
}

func fireMismatch(engine *Engine, device, iface string) {
	engine.ProcessStateChange(evaluator.StateChange{
		Device:    device,
		Interface: iface,
		AlertType: "interface_state_mismatch",
		Severity:  "critical",
		Message:   "interface " + iface + " expected up got down",
	})
	engine.process(<-engine.events)
}
