package alerter

import (
	"fmt"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/evaluator"
	"github.com/netspec/netspec/internal/notifier"
	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

// Engine manages alert lifecycle and routing
type Engine struct {
	config      *config.Config
	notifier    *notifier.Notifier
	logger      zerolog.Logger
	activeAlerts map[string]*types.Alert
	mu          sync.RWMutex
}


// NewEngine creates a new alert engine
func NewEngine(cfg *config.Config, notifier *notifier.Notifier, logger zerolog.Logger) *Engine {
	return &Engine{
		config:       cfg,
		notifier:     notifier,
		logger:       logger,
		activeAlerts: make(map[string]*types.Alert),
	}
}

// ProcessStateChange processes a state change and generates alerts
func (e *Engine) ProcessStateChange(change evaluator.StateChange) {
	e.mu.Lock()
	defer e.mu.Unlock()

	alertID := fmt.Sprintf("%s:%s:%s", change.Device, change.Interface, change.AlertType)

	// Check if alert already exists
	existing, exists := e.activeAlerts[alertID]
	if exists && existing.State == "firing" {
		// Alert already firing, skip duplicate
		e.logger.Debug().
			Str("alert_id", alertID).
			Msg("Alert already firing, skipping duplicate")
		return
	}

	// Create new alert
	alert := &types.Alert{
		ID:          alertID,
		Device:      change.Device,
		Entity:      change.Interface,
		AlertType:   change.AlertType,
		Severity:    change.Severity,
		State:       "firing",
		FiredAt:     time.Now(),
		Message:     change.Message,
		RelatedState: change.RelatedState,
	}

	e.activeAlerts[alertID] = alert

	// Route alert to channels
	channels := e.getChannelsForSeverity(change.Severity)
	
	e.logger.Info().
		Str("alert_id", alertID).
		Str("device", change.Device).
		Str("interface", change.Interface).
		Str("severity", change.Severity).
		Msg("Alert fired")

	// Send notification
	if err := e.notifier.SendAlert(alert, channels); err != nil {
		e.logger.Error().
			Err(err).
			Str("alert_id", alertID).
			Msg("Failed to send alert notification")
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
func (e *Engine) getChannelsForSeverity(severity string) []string {
	// Check for severity-specific rule
	if rule, ok := e.config.Alerts.AlertRules[severity]; ok {
		return rule.Channels
	}

	// Fall back to default
	if rule, ok := e.config.Alerts.AlertRules["default"]; ok {
		return rule.Channels
	}

	return []string{}
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
