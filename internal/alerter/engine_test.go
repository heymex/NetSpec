package alerter

import (
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

func testEngine(t *testing.T) *Engine {
	t.Helper()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			AlertBehavior: config.AlertBehavior{DeduplicationWindow: time.Minute},
		},
	}
	return NewEngine(cfg, nil, zerolog.Nop(), "")
}

func TestSyncTelemetryIngestStale_FireAndResolve(t *testing.T) {
	t.Parallel()
	engine := testEngine(t)
	last := time.Now().UTC().Add(-13 * time.Hour)

	engine.SyncTelemetryIngestStale(true, last, 5*time.Minute)
	engine.process(<-engine.events)

	active := engine.GetActiveAlerts()
	if len(active) != 1 {
		t.Fatalf("expected 1 active alert, got %d", len(active))
	}
	a := active[0]
	if a.AlertType != AlertTypeTelemetryIngestStale {
		t.Fatalf("alert type: %q", a.AlertType)
	}
	if a.Device != PipelineSyntheticDevice || a.Entity != IngestSyntheticEntity {
		t.Fatalf("identity: %s %s", a.Device, a.Entity)
	}
	if a.Severity != "warning" {
		t.Fatalf("severity: %q", a.Severity)
	}
	if a.RelatedState["last_event_at"] != last.Format(time.RFC3339) {
		t.Fatalf("related last_event_at: %#v", a.RelatedState)
	}

	engine.SyncTelemetryIngestStale(false, time.Now().UTC(), 5*time.Minute)
	engine.process(<-engine.events)
	if got := engine.GetActiveAlerts(); len(got) != 0 {
		t.Fatalf("expected resolve to clear alert, still have %d: %#v", len(got), got)
	}
}

func TestSyncTelemetryIngestStale_ResolveNoopWithoutActive(t *testing.T) {
	t.Parallel()
	engine := testEngine(t)
	engine.SyncTelemetryIngestStale(false, time.Time{}, 5*time.Minute)
	select {
	case ev := <-engine.events:
		t.Fatalf("expected no event when ingest is healthy and no alert is active, got %#v", ev)
	default:
	}
}
