// Package webhook provides HTTP handlers for inbound ChatOps callbacks.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/notifier"
	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

// AlertManager is the engine interface required by the Slack webhook handler.
type AlertManager interface {
	AckAlert(alertID, by, note string) (*types.Alert, error)
	CloseAlert(alertID, by string) (*types.Alert, error)
}

// SlackHandler handles POST /webhook/slack/interactions from Slack's interactivity API.
type SlackHandler struct {
	signingSecret string
	engine        AlertManager
	slack         *notifier.SlackNotifier
	logger        zerolog.Logger
}

// NewSlackHandler creates a SlackHandler. signingSecret may be empty to disable signature validation
// (not recommended for production).
func NewSlackHandler(signingSecret string, engine AlertManager, slack *notifier.SlackNotifier, logger zerolog.Logger) *SlackHandler {
	return &SlackHandler{
		signingSecret: signingSecret,
		engine:        engine,
		slack:         slack,
		logger:        logger.With().Str("component", "slack-webhook").Logger(),
	}
}

type slackInteractionPayload struct {
	Type string `json:"type"`
	User struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"user"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value"`
	} `json:"actions"`
	Message struct {
		TS string `json:"ts"`
	} `json:"message"`
	Channel struct {
		ID string `json:"id"`
	} `json:"channel"`
}

// HandleInteractions is the HTTP handler for Slack interactive component callbacks.
func (h *SlackHandler) HandleInteractions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	if err := h.verifySignature(r, body); err != nil {
		h.logger.Warn().Err(err).Str("remote", r.RemoteAddr).Msg("Slack signature verification failed")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	vals, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	payloadJSON := vals.Get("payload")
	if payloadJSON == "" {
		http.Error(w, "missing payload", http.StatusBadRequest)
		return
	}

	var payload slackInteractionPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		h.logger.Error().Err(err).Msg("Failed to decode Slack interaction payload")
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}

	// Slack requires a response within 3 seconds — acknowledge immediately.
	w.WriteHeader(http.StatusOK)

	if len(payload.Actions) == 0 {
		return
	}

	action := payload.Actions[0]
	alertID := strings.TrimSpace(action.Value)
	userName := coalesce(payload.User.Name, payload.User.Username, payload.User.ID)
	channelID := payload.Channel.ID
	msgTS := payload.Message.TS

	switch action.ActionID {
	case "ack_alert":
		h.handleAck(alertID, userName, channelID, msgTS)
	case "close_alert":
		h.handleClose(alertID, userName, channelID, msgTS)
	default:
		h.logger.Warn().Str("action_id", action.ActionID).Msg("Unknown Slack action")
	}
}

func (h *SlackHandler) handleAck(alertID, by, channelID, msgTS string) {
	alert, err := h.engine.AckAlert(alertID, by, "")
	if err != nil {
		h.logger.Error().Err(err).Str("alert_id", alertID).Str("by", by).Msg("AckAlert failed")
		return
	}
	h.logger.Info().Str("alert_id", alertID).Str("by", by).Msg("Alert acknowledged via Slack")
	if h.slack != nil && channelID != "" && msgTS != "" {
		if err := h.slack.UpdateAlert(alert, channelID, msgTS); err != nil {
			h.logger.Error().Err(err).Str("alert_id", alertID).Msg("Failed to update Slack message after ack")
		}
	}
}

func (h *SlackHandler) handleClose(alertID, by, channelID, msgTS string) {
	alert, err := h.engine.CloseAlert(alertID, by)
	if err != nil {
		h.logger.Error().Err(err).Str("alert_id", alertID).Str("by", by).Msg("CloseAlert failed")
		return
	}
	h.logger.Info().Str("alert_id", alertID).Str("by", by).Msg("Alert closed via Slack")
	if h.slack != nil && channelID != "" && msgTS != "" {
		if err := h.slack.UpdateAlert(alert, channelID, msgTS); err != nil {
			h.logger.Error().Err(err).Str("alert_id", alertID).Msg("Failed to update Slack message after close")
		}
	}
}

// verifySignature validates the Slack request signature using HMAC-SHA256.
// See: https://api.slack.com/authentication/verifying-requests-from-slack
func (h *SlackHandler) verifySignature(r *http.Request, body []byte) error {
	if h.signingSecret == "" {
		return nil
	}
	ts := r.Header.Get("X-Slack-Request-Timestamp")
	sig := r.Header.Get("X-Slack-Signature")
	if ts == "" || sig == "" {
		return fmt.Errorf("missing Slack signature headers")
	}

	var tsUnix int64
	if _, err := fmt.Sscanf(ts, "%d", &tsUnix); err != nil {
		return fmt.Errorf("invalid timestamp header: %w", err)
	}
	// Reject requests older than 5 minutes to prevent replay attacks.
	age := time.Since(time.Unix(tsUnix, 0))
	if age > 5*time.Minute || age < -5*time.Minute {
		return fmt.Errorf("timestamp out of acceptable range (%s)", age.Round(time.Second))
	}

	mac := hmac.New(sha256.New, []byte(h.signingSecret))
	fmt.Fprintf(mac, "v0:%s:%s", ts, body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return "unknown"
}
