package alerter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/evaluator"
	"github.com/netspec/netspec/internal/notifier"
	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

// NotifyFunc is called when an alert fires or resolves
type NotifyFunc func(alert types.Alert)

// EscalateFunc is called when an alert escalates to additional channels
type EscalateFunc func(alert types.Alert, channels []string)

// Engine manages alert lifecycle and routing
type Engine struct {
	config       *config.Config
	notifier     *notifier.Notifier
	logger       zerolog.Logger
	activeAlerts map[string]*types.Alert
	lastFired    map[string]time.Time // dedup tracking
	mu           sync.RWMutex
	flap         *FlapDetector
	escalation   *EscalationManager
	events       chan AlertEvent
	notify       NotifyFunc
	escalate     EscalateFunc
}

// AlertEvent represents an alert event from the evaluator
type AlertEvent struct {
	Device    string
	Entity    string
	AlertType string
	Severity  string
	Firing    bool
	Message   string
	Related   map[string]string
}


// NewEngine creates a new alert engine with full Phase 2 features
func NewEngine(cfg *config.Config, notifier *notifier.Notifier, logger zerolog.Logger) *Engine {
	l := logger.With().Str("component", "alerter").Logger()

	var flapDetector *FlapDetector
	if cfg.Alerts.AlertBehavior.FlapDetection.Enabled {
		threshold := 3 // default
		if cfg.Alerts.AlertBehavior.FlapDetection.Threshold > 0 {
			threshold = cfg.Alerts.AlertBehavior.FlapDetection.Threshold
		}
		window := 5 * time.Minute // default
		if cfg.Alerts.AlertBehavior.FlapDetection.Window > 0 {
			window = cfg.Alerts.AlertBehavior.FlapDetection.Window
		}
		flapDetector = NewFlapDetector(l, threshold, window)
	}

	var escMgr *EscalationManager
	escRules := make(map[string]EscalationRule)
	for name, ch := range cfg.Alerts.Channels {
		if ch.EscalationDelay > 0 {
			escRules[name] = EscalationRule{
				Channel: name,
				Delay:   time.Duration(ch.EscalationDelay) * time.Second,
			}
		}
	}
	if len(escRules) > 0 {
		escMgr = NewEscalationManager(l, escRules, nil) // Will be set via SetEscalationNotify
	}

	notifyFn := func(alert types.Alert) {
		channels := getChannelsForSeverity(cfg, alert.Severity)
		if err := notifier.SendAlert(&alert, channels); err != nil {
			l.Error().Err(err).Str("alert_id", alert.ID).Msg("Failed to send alert notification")
		}
	}

	engine := &Engine{
		config:       cfg,
		notifier:     notifier,
		logger:       l,
		activeAlerts: make(map[string]*types.Alert),
		lastFired:    make(map[string]time.Time),
		flap:         flapDetector,
		escalation:   escMgr,
		events:       make(chan AlertEvent, 500),
		notify:       notifyFn,
	}

	if escMgr != nil {
		engine.escalate = func(alert types.Alert, channels []string) {
			alert.Message = fmt.Sprintf("[ESCALATED] %s", alert.Message)
			for _, chName := range channels {
				ch, ok := cfg.Alerts.Channels[chName]
				if !ok {
					continue
				}
				url := getChannelURL(ch.URLEnv)
				if url == "" {
					continue
				}
				if err := notifier.SendAlert(&alert, []string{chName}); err != nil {
					l.Error().Err(err).Str("channel", chName).Msg("escalation notification failed")
				} else {
					l.Warn().Str("channel", chName).Str("alert", alert.ID).Msg("escalation notification sent")
				}
			}
		}
		escMgr.onEscalate = engine.escalate
	}

	return engine
}

// Events returns the channel to send alert events to
func (e *Engine) Events() chan<- AlertEvent {
	return e.events
}

// Run processes alert events until the channel is closed
func (e *Engine) Run() {
	// Periodic flap cleanup
	if e.flap != nil {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		go func() {
			for range ticker.C {
				e.flap.Cleanup()
				e.checkFlapRecovery()
			}
		}()
	}

	for ev := range e.events {
		e.process(ev)
	}
}

// Stop cleans up escalation timers
func (e *Engine) Stop() {
	if e.escalation != nil {
		e.escalation.Stop()
	}
	close(e.events)
}

// ProcessStateChange processes a state change and generates alerts (legacy method)
func (e *Engine) ProcessStateChange(change evaluator.StateChange) {
	ev := AlertEvent{
		Device:    change.Device,
		Entity:    change.Interface,
		AlertType: change.AlertType,
		Severity:  change.Severity,
		Firing:    true,
		Message:   change.Message,
		Related:   change.RelatedState,
	}
	select {
	case e.events <- ev:
	default:
		e.logger.Warn().Msg("Alert event channel full, dropping")
	}
}

// process handles an alert event
func (e *Engine) process(ev AlertEvent) {
	key := fmt.Sprintf("%s|%s|%s", ev.Device, ev.Entity, ev.AlertType)
	entityKey := fmt.Sprintf("%s|%s", ev.Device, ev.Entity)

	e.mu.Lock()
	defer e.mu.Unlock()

	if ev.Firing {
		// Record state change for flap detection
		if e.flap != nil {
			flapping, justStarted := e.flap.RecordChange(entityKey)
			if flapping {
				if justStarted {
					// Send a single "flapping detected" alert instead of individual ones
					flapAlert := &types.Alert{
						ID:        fmt.Sprintf("flap-%s-%d", entityKey, time.Now().UnixMilli()),
						Device:    ev.Device,
						Entity:    ev.Entity,
						AlertType: "flapping_detected",
						Severity:  "warning",
						State:     "firing",
						FiredAt:   time.Now(),
						Message:   fmt.Sprintf("Flapping detected on %s %s: suppressing individual alerts", ev.Device, ev.Entity),
					}
					e.activeAlerts["flap|"+entityKey] = flapAlert
					if e.notify != nil {
						e.notify(*flapAlert)
					}
				}
				// Suppress the actual alert
				return
			}
		}

		// Check dedup
		dedupWindow := e.config.Alerts.AlertBehavior.DeduplicationWindow
		if dedupWindow == 0 {
			dedupWindow = 5 * time.Minute
		}
		if last, ok := e.lastFired[key]; ok {
			if time.Since(last) < dedupWindow {
				e.logger.Debug().Str("key", key).Msg("alert deduplicated")
				return
			}
		}

		now := time.Now()
		alert := &types.Alert{
			ID:           fmt.Sprintf("%s-%d", key, now.UnixMilli()),
			Device:       ev.Device,
			Entity:       ev.Entity,
			AlertType:    ev.AlertType,
			Severity:     ev.Severity,
			State:        "firing",
			FiredAt:      now,
			Message:      ev.Message,
			RelatedState: ev.Related,
		}
		e.activeAlerts[key] = alert
		e.lastFired[key] = now

		e.logger.Warn().
			Str("device", ev.Device).
			Str("entity", ev.Entity).
			Str("type", ev.AlertType).
			Str("severity", ev.Severity).
			Msg("alert fired")

		if e.notify != nil {
			e.notify(*alert)
		}

		// Start escalation timer if configured
		if e.escalation != nil {
			channels := getChannelsForSeverity(e.config, ev.Severity)
			e.escalation.StartEscalation(*alert, channels)
		}
	} else {
		// Resolve
		existing, ok := e.activeAlerts[key]
		if !ok {
			return
		}
		now := time.Now()
		existing.State = "resolved"
		existing.ResolvedAt = &now
		existing.Message = ev.Message

		e.logger.Info().
			Str("device", ev.Device).
			Str("entity", ev.Entity).
			Str("type", ev.AlertType).
			Msg("alert resolved")

		if !existing.Suppressed {
			if e.notify != nil {
				e.notify(*existing)
			}
		}

		// Cancel escalation
		if e.escalation != nil {
			e.escalation.CancelEscalation(ev.Device, ev.Entity, ev.AlertType)
		}

		delete(e.activeAlerts, key)
	}
}

// checkFlapRecovery checks if flapping has stopped
func (e *Engine) checkFlapRecovery() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for key, alert := range e.activeAlerts {
		if alert.AlertType != "flapping_detected" {
			continue
		}
		entityKey := alert.Device + "|" + alert.Entity
		if e.flap.CheckStable(entityKey) {
			now := time.Now()
			alert.State = "resolved"
			alert.ResolvedAt = &now
			alert.Message = fmt.Sprintf("Flapping stopped on %s %s", alert.Device, alert.Entity)

			if e.notify != nil {
				e.notify(*alert)
			}
			delete(e.activeAlerts, key)
		}
	}
}

// ResolveAlert marks an alert as resolved
func (e *Engine) ResolveAlert(device, entity, alertType string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	alertID := fmt.Sprintf("%s:%s:%s", device, entity, alertType)
	alert, exists := e.activeAlerts[alertID]
	if !exists || alert.State == "resolved" {
		return
	}

	now := time.Now()
	alert.State = "resolved"
	alert.ResolvedAt = &now
	duration := now.Sub(alert.FiredAt)

	// Update message for recovery
	alert.Message = fmt.Sprintf("Recovered: %s (was down for %s)", alert.Message, duration.Round(time.Second))

	e.logger.Info().
		Str("alert_id", alertID).
		Dur("duration", duration).
		Msg("Alert resolved")

	// Send recovery notification
	channels := e.getChannelsForSeverity(alert.Severity)
	if err := e.notifier.SendAlert(alert, channels); err != nil {
		e.logger.Error().
			Err(err).
			Str("alert_id", alertID).
			Msg("Failed to send recovery notification")
	}
}

// getChannelsForSeverity returns notification channels for a given severity
func getChannelsForSeverity(cfg *config.Config, severity string) []string {
	// Check for severity-specific rule
	if rule, ok := cfg.Alerts.AlertRules[severity]; ok {
		return rule.Channels
	}

	// Fall back to default
	if rule, ok := cfg.Alerts.AlertRules["default"]; ok {
		return rule.Channels
	}

	return []string{}
}

// getChannelURL gets channel URL from environment variable
func getChannelURL(envVar string) string {
	return "" // Will be handled by notifier
}

// GetActiveAlerts returns all active alerts
func (e *Engine) GetActiveAlerts() []*types.Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()

	alerts := make([]*types.Alert, 0, len(e.activeAlerts))
	for _, alert := range e.activeAlerts {
		if alert.State == "firing" {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}
