package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/types"
)

// openClawWebhookPayload is the JSON body POSTed to OpenClaw Gateway webhooks
// (mapped hooks, or wrapped by an OpenClaw transform into wake/agent actions).
type openClawWebhookPayload struct {
	Event string         `json:"event"`
	Alert openClawAlert  `json:"alert"`
	Links *openClawLinks `json:"links,omitempty"`
}

type openClawAlert struct {
	ID           string            `json:"id"`
	Device       string            `json:"device"`
	Entity       string            `json:"entity"`
	AlertType    string            `json:"alert_type"`
	Severity     string            `json:"severity"`
	State        string            `json:"state"`
	FiredAt      string            `json:"fired_at"`
	Message      string            `json:"message"`
	RelatedState map[string]string `json:"related_state"`
}

type openClawLinks struct {
	Alert  string `json:"alert"`
	Device string `json:"device"`
}

func openClawEventName(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "resolved":
		return "alert.resolved"
	case "acked":
		return "alert.acked"
	default:
		return "alert.firing"
	}
}

func buildOpenClawPayload(alert *types.Alert, publicBase string) openClawWebhookPayload {
	related := alert.RelatedState
	if related == nil {
		related = map[string]string{}
	}

	payload := openClawWebhookPayload{
		Event: openClawEventName(alert.State),
		Alert: openClawAlert{
			ID:           alert.ID,
			Device:       alert.Device,
			Entity:       alert.Entity,
			AlertType:    alert.AlertType,
			Severity:     alert.Severity,
			State:        alert.State,
			FiredAt:      alert.FiredAt.UTC().Format(time.RFC3339Nano),
			Message:      alert.Message,
			RelatedState: related,
		},
	}

	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	if base != "" {
		deviceLink := base + "/device/" + url.PathEscape(alert.Device)
		// Synthetic pipeline/host entities are not real devices.
		if strings.HasPrefix(alert.Device, "__") {
			deviceLink = base + "/"
		}
		payload.Links = &openClawLinks{
			Alert:  base + "/alerts",
			Device: deviceLink,
		}
	}
	return payload
}

func (n *Notifier) deliverOpenClaw(webhookURL, token, channelName string, alert *types.Alert) error {
	publicBase := strings.TrimSpace(os.Getenv("NETSPEC_PUBLIC_URL"))
	payload := buildOpenClawPayload(alert, publicBase)

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal openclaw payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("x-openclaw-token", token)
	}

	n.logger.Debug().
		Str("channel", channelName).
		Str("event", payload.Event).
		Str("alert_id", alert.ID).
		Str("url", scrubServiceURL(webhookURL)).
		Msg("openclaw webhook notify")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("openclaw webhook %s", summarizeHTTPError(resp.StatusCode, respBody))
	}
	return nil
}

func summarizeHTTPError(status int, body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return fmt.Sprintf("HTTP %d (empty body)", status)
	}
	if len(s) > 500 {
		return fmt.Sprintf("HTTP %d: %s…", status, s[:500])
	}
	return fmt.Sprintf("HTTP %d: %s", status, s)
}
