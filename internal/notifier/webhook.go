package notifier

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/types"
)

// webhookPayload is the JSON body POSTed to generic webhook endpoints.
type webhookPayload struct {
	Event string        `json:"event"`
	Alert webhookAlert  `json:"alert"`
	Links *webhookLinks `json:"links,omitempty"`
}

type webhookAlert struct {
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

type webhookLinks struct {
	Alert  string `json:"alert"`
	Device string `json:"device"`
}

func webhookEventName(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "resolved":
		return "alert.resolved"
	case "acked":
		return "alert.acked"
	default:
		return "alert.firing"
	}
}

func buildWebhookPayload(alert *types.Alert, publicBase string) webhookPayload {
	related := alert.RelatedState
	if related == nil {
		related = map[string]string{}
	}

	payload := webhookPayload{
		Event: webhookEventName(alert.State),
		Alert: webhookAlert{
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
		payload.Links = &webhookLinks{
			Alert:  base + "/alerts",
			Device: base + "/device/" + url.PathEscape(alert.Device),
		}
	}
	return payload
}

func (n *Notifier) deliverWebhookChannel(channelName string, ch config.ChannelConfig, alert *types.Alert) error {
	publicBase := strings.TrimSpace(os.Getenv("NETSPEC_PUBLIC_URL"))
	payload := buildWebhookPayload(alert, publicBase)
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	var errs []error
	for _, ep := range ch.ResolvedWebhookEndpoints() {
		if err := n.postWebhook(channelName, ep, payload.Event, alert.ID, jsonData); err != nil {
			n.logger.Error().Err(err).Str("channel", channelName).Str("endpoint", ep.Name).Msg("failed to send webhook notification")
			errs = append(errs, fmt.Errorf("endpoint %s: %w", ep.Name, err))
			continue
		}
		n.logger.Info().Str("channel", channelName).Str("endpoint", ep.Name).Str("alert_id", alert.ID).Msg("webhook notification sent")
	}
	return errors.Join(errs...)
}

func (n *Notifier) postWebhook(channelName string, ep config.WebhookEndpoint, event, alertID string, jsonData []byte) error {
	webhookURL := strings.TrimSpace(os.Getenv(ep.URLEnv))
	if webhookURL == "" {
		return fmt.Errorf("environment variable %s is not set or empty", ep.URLEnv)
	}
	parsed, err := url.Parse(webhookURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("environment variable %s is not an http(s) URL", ep.URLEnv)
	}

	token := ""
	if ep.TokenEnv != "" {
		token = strings.TrimSpace(os.Getenv(ep.TokenEnv))
		if token == "" {
			return fmt.Errorf("environment variable %s is not set or empty", ep.TokenEnv)
		}
		if strings.ContainsAny(token, "\r\n") {
			return fmt.Errorf("environment variable %s contains invalid header characters", ep.TokenEnv)
		}
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if err := applyWebhookAuth(req, ep, token); err != nil {
		return err
	}

	n.logger.Debug().
		Str("channel", channelName).
		Str("endpoint", ep.Name).
		Str("event", event).
		Str("alert_id", alertID).
		Str("url", scrubServiceURL(webhookURL)).
		Msg("webhook notify")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook %s", summarizeHTTPError(resp.StatusCode, respBody))
	}
	return nil
}

func applyWebhookAuth(req *http.Request, ep config.WebhookEndpoint, token string) error {
	if ep.Auth == "none" || token == "" {
		return nil
	}
	switch ep.Auth {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+token)
	case "header":
		req.Header.Set(ep.HeaderName, token)
	case "bearer_and_header":
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set(ep.HeaderName, token)
	default:
		return fmt.Errorf("unsupported auth %q", ep.Auth)
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
