package alerter

import (
	"context"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

// EscalateFunc is called when an alert escalates to additional channels.
type EscalateFunc func(alert types.Alert, channels []string)

// EscalationRule defines when and where to escalate an unresolved alert.
type EscalationRule struct {
	Channel string
	Delay   time.Duration
}

// EscalationManager tracks unresolved alerts and triggers escalation after configured delays.
type EscalationManager struct {
	log        zerolog.Logger
	rules      map[string]EscalationRule // channel name -> rule
	onEscalate EscalateFunc
	mu         sync.Mutex
	timers     map[string]context.CancelFunc // alert key -> cancel func
}

// NewEscalationManager creates a new escalation manager.
func NewEscalationManager(log zerolog.Logger, rules map[string]EscalationRule, onEscalate EscalateFunc) *EscalationManager {
	return &EscalationManager{
		log:        log.With().Str("component", "escalation").Logger(),
		rules:      rules,
		onEscalate: onEscalate,
		timers:     make(map[string]context.CancelFunc),
	}
}

// StartEscalation begins escalation timers for a fired alert.
// For each channel with an escalation_delay, a goroutine waits and then escalates.
func (m *EscalationManager) StartEscalation(alert types.Alert, channels []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var escalationChannels []string
	var maxDelay time.Duration

	for _, ch := range channels {
		rule, ok := m.rules[ch]
		if !ok || rule.Delay <= 0 {
			continue
		}
		escalationChannels = append(escalationChannels, ch)
		if rule.Delay > maxDelay {
			maxDelay = rule.Delay
		}
	}

	if len(escalationChannels) == 0 {
		return
	}

	key := alert.Device + "|" + alert.Entity + "|" + alert.AlertType
	// Cancel any existing timer for this key
	if cancel, ok := m.timers[key]; ok {
		cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.timers[key] = cancel

	m.log.Debug().
		Str("key", key).
		Dur("delay", maxDelay).
		Strs("channels", escalationChannels).
		Msg("escalation timer started")

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(maxDelay):
			m.log.Warn().
				Str("key", key).
				Strs("channels", escalationChannels).
				Msg("escalating unresolved alert")
			if m.onEscalate != nil {
				m.onEscalate(alert, escalationChannels)
			}
			m.mu.Lock()
			delete(m.timers, key)
			m.mu.Unlock()
		}
	}()
}

// CancelEscalation cancels pending escalation for a resolved alert.
func (m *EscalationManager) CancelEscalation(device, entity, alertType string) {
	key := device + "|" + entity + "|" + alertType
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, ok := m.timers[key]; ok {
		cancel()
		delete(m.timers, key)
		m.log.Debug().Str("key", key).Msg("escalation cancelled")
	}
}

// Stop cancels all pending escalation timers.
func (m *EscalationManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, cancel := range m.timers {
		cancel()
		delete(m.timers, key)
	}
}
