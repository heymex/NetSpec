package alerter

import (
	"fmt"
	"os"
	"strings"
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

// Engine manages alert lifecycle and routing
type Engine struct {
	config        *config.Config
	notifier      *notifier.Notifier
	slackNotifier *notifier.SlackNotifier
	logger        zerolog.Logger
	activeAlerts  map[string]*types.Alert
	lastFired     map[string]time.Time // dedup tracking
	mu            sync.RWMutex
	flap          *FlapDetector
	escalation    *EscalationManager
	events        chan AlertEvent
	notify        NotifyFunc
}

// SetSlackNotifier attaches the Slack ChatOps notifier to the engine.
func (e *Engine) SetSlackNotifier(sn *notifier.SlackNotifier) {
	e.slackNotifier = sn
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
		escFn := func(alert types.Alert, channels []string) {
			alert.Message = fmt.Sprintf("[ESCALATED] %s", alert.Message)
			for _, chName := range channels {
				if _, ok := cfg.Alerts.Channels[chName]; !ok {
					continue
				}
				if err := notifier.SendAlert(&alert, []string{chName}); err != nil {
					l.Error().Err(err).Str("channel", chName).Msg("escalation notification failed")
				} else {
					l.Warn().Str("channel", chName).Str("alert", alert.ID).Msg("escalation notification sent")
				}
			}
		}
		escMgr.onEscalate = escFn
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

// SyncSNMPReachability raises or clears a synthetic host-level alert when SNMP contact fails or recovers.
func (e *Engine) SyncSNMPReachability(device string, reachable bool, errMsg string) {
	device = strings.TrimSpace(device)
	if device == "" {
		return
	}
	msg := fmt.Sprintf("SNMP connectivity restored for device %s", device)
	if !reachable {
		if strings.TrimSpace(errMsg) != "" {
			msg = fmt.Sprintf("SNMP unreachable for device %s: %s", device, errMsg)
		} else {
			msg = fmt.Sprintf("SNMP unreachable for device %s", device)
		}
	}
	ev := AlertEvent{
		Device:    device,
		Entity:    "__snmp__",
		AlertType: "snmp_unreachable",
		Severity:  "warning",
		Firing:    !reachable,
		Message:   msg,
	}
	select {
	case e.events <- ev:
	default:
		e.logger.Warn().Str("device", device).Msg("alert event channel full, dropping snmp reachability")
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

		// If the alert is already acknowledged, suppress re-notification regardless of dedup window.
		if existing, ok := e.activeAlerts[key]; ok && existing.State == "acked" {
			e.logger.Debug().Str("key", key).Msg("alert already acked, suppressing re-fire")
			return
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

		// Post interactive Slack Block Kit message.
		e.postSlackAlert(alert)

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

		if e.notify != nil {
			e.notify(*existing)
		}

		// Update Slack message to resolved state before removing from map.
		e.updateSlackAlert(existing)

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
			e.updateSlackAlert(alert)
			delete(e.activeAlerts, key)
		}
	}
}

// ResolveAlert marks an alert as resolved
func (e *Engine) ResolveAlert(device, entity, alertType string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	alertID := fmt.Sprintf("%s|%s|%s", device, entity, alertType)
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
	channels := getChannelsForSeverity(e.config, alert.Severity)
	if err := e.notifier.SendAlert(alert, channels); err != nil {
		e.logger.Error().
			Err(err).
			Str("alert_id", alertID).
			Msg("Failed to send recovery notification")
	}
}

func slackSeverityAllows(filter []string, severity string) bool {
	if len(filter) == 0 {
		return true
	}
	s := strings.ToLower(strings.TrimSpace(severity))
	for _, f := range filter {
		if strings.ToLower(strings.TrimSpace(f)) == s {
			return true
		}
	}
	return false
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

// GetActiveAlerts returns all firing and acknowledged alerts.
func (e *Engine) GetActiveAlerts() []*types.Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()

	alerts := make([]*types.Alert, 0, len(e.activeAlerts))
	for _, alert := range e.activeAlerts {
		if alert.State == "firing" || alert.State == "acked" {
			alerts = append(alerts, alert)
		}
	}
	return alerts
}

// AckAlert acknowledges a firing alert, suppressing further re-notifications.
// Returns a copy of the updated alert so callers can update external systems (e.g. Slack message).
func (e *Engine) AckAlert(alertID, by, note string) (*types.Alert, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, alert := range e.activeAlerts {
		if alert.ID != alertID {
			continue
		}
		if alert.State == "resolved" {
			return nil, fmt.Errorf("alert %s is already resolved", alertID)
		}
		now := time.Now()
		alert.AckedAt = &now
		alert.AckedBy = by
		alert.AckNote = note
		alert.State = "acked"
		e.logger.Info().Str("alert_id", alertID).Str("by", by).Msg("alert acknowledged")
		cp := *alert
		return &cp, nil
	}
	return nil, fmt.Errorf("alert %s not found", alertID)
}

// CloseAlert manually resolves an alert (user-initiated from Slack).
// Sends a resolved notification through configured Apprise channels and removes the alert.
func (e *Engine) CloseAlert(alertID, by string) (*types.Alert, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for key, alert := range e.activeAlerts {
		if alert.ID != alertID {
			continue
		}
		now := time.Now()
		alert.State = "resolved"
		alert.ResolvedAt = &now
		alert.Message = fmt.Sprintf("Manually closed by %s via Slack", by)
		if alert.AckedBy == "" {
			alert.AckedBy = by
		}

		if e.escalation != nil {
			e.escalation.CancelEscalation(alert.Device, alert.Entity, alert.AlertType)
		}
		if e.notify != nil {
			e.notify(*alert)
		}

		e.logger.Info().Str("alert_id", alertID).Str("by", by).Msg("alert closed via Slack")
		cp := *alert
		delete(e.activeAlerts, key)
		return &cp, nil
	}
	return nil, fmt.Errorf("alert %s not found", alertID)
}

// postSlackAlert posts a new Block Kit message for the given alert via all slack_chatops channels.
// Must be called with e.mu held.
func (e *Engine) postSlackAlert(alert *types.Alert) {
	if e.slackNotifier == nil {
		return
	}
	channels := getChannelsForSeverity(e.config, alert.Severity)
	for _, chName := range channels {
		ch, ok := e.config.Alerts.Channels[chName]
		if !ok || ch.Type != "slack_chatops" {
			continue
		}
		if !slackSeverityAllows(ch.SeverityFilter, alert.Severity) {
			continue
		}
		channelID := strings.TrimSpace(os.Getenv(ch.ChannelEnv))
		if channelID == "" {
			e.logger.Warn().Str("channel", chName).Str("channel_env", ch.ChannelEnv).Msg("slack_chatops channel_env is empty")
			continue
		}
		ts, err := e.slackNotifier.PostAlert(alert, channelID)
		if err != nil {
			e.logger.Error().Err(err).Str("alert_id", alert.ID).Str("channel", chName).Msg("Failed to post Slack ChatOps alert")
			continue
		}
		// Store TS on the live alert pointer so updates (ack, resolve) can edit the same message.
		alert.SlackMsgTS = ts
		alert.SlackChannelID = channelID
	}
}

// updateSlackAlert edits the existing Slack message for the given alert.
// Must be called with e.mu held.
func (e *Engine) updateSlackAlert(alert *types.Alert) {
	if e.slackNotifier == nil || alert.SlackMsgTS == "" {
		return
	}
	if err := e.slackNotifier.UpdateAlert(alert, alert.SlackChannelID, alert.SlackMsgTS); err != nil {
		e.logger.Error().Err(err).Str("alert_id", alert.ID).Str("state", alert.State).Msg("Failed to update Slack message")
	}
}

// ClearAlertsForDevice removes all in-memory active/dedup alert state for a device.
// Use this when a device is removed from monitoring config.
func (e *Engine) ClearAlertsForDevice(device string) int {
	device = strings.TrimSpace(device)
	if device == "" {
		return 0
	}
	prefix := device + "|"
	flapPrefix := "flap|" + prefix

	e.mu.Lock()
	defer e.mu.Unlock()

	removed := 0
	for key, alert := range e.activeAlerts {
		if strings.HasPrefix(key, prefix) || strings.HasPrefix(key, flapPrefix) {
			if e.escalation != nil {
				e.escalation.CancelEscalation(alert.Device, alert.Entity, alert.AlertType)
			}
			delete(e.activeAlerts, key)
			removed++
		}
	}
	for key := range e.lastFired {
		if strings.HasPrefix(key, prefix) {
			delete(e.lastFired, key)
		}
	}

	return removed
}
