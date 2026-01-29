package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/rs/zerolog"
)

// Notifier handles sending alerts via Apprise
type Notifier struct {
	logger zerolog.Logger
	client *http.Client
}

// NewNotifier creates a new Apprise notifier
func NewNotifier(logger zerolog.Logger) *Notifier {
	return &Notifier{
		logger: logger,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendAlert sends an alert to the specified channels
func (n *Notifier) SendAlert(alert *alerter.Alert, channelNames []string) error {
	// Get channel configs
	channels := make([]Channel, 0, len(channelNames))
	for _, name := range channelNames {
		// For MVP, we'll use Apprise API directly
		// In production, this would look up channel config
		url := os.Getenv(fmt.Sprintf("APPRISE_%s_URL", name))
		if url == "" {
			n.logger.Warn().
				Str("channel", name).
				Msg("Channel URL not found, skipping")
			continue
		}

		channels = append(channels, Channel{
			Name: name,
			URL:  url,
		})
	}

	// Format message
	message := n.formatMessage(alert)

	// Send to each channel
	for _, channel := range channels {
		if err := n.sendToApprise(channel.URL, message, alert.Severity); err != nil {
			n.logger.Error().
				Err(err).
				Str("channel", channel.Name).
				Msg("Failed to send notification")
			// Continue to other channels
		} else {
			n.logger.Info().
				Str("channel", channel.Name).
				Str("alert_id", alert.ID).
				Msg("Notification sent")
		}
	}

	return nil
}

// Channel represents a notification channel
type Channel struct {
	Name string
	URL  string
}

// formatMessage formats an alert into a notification message
func (n *Notifier) formatMessage(alert *alerter.Alert) string {
	var emoji string
	switch alert.Severity {
	case "critical":
		emoji = "🔴"
	case "warning":
		emoji = "⚠️"
	default:
		emoji = "ℹ️"
	}

	if alert.State == "resolved" {
		emoji = "🟢"
	}

	title := fmt.Sprintf("%s NetSpec Alert: %s", emoji, alert.AlertType)
	body := fmt.Sprintf("%s\n\nDevice: %s\nInterface: %s\nSeverity: %s\nState: %s",
		alert.Message, alert.Device, alert.Entity, alert.Severity, alert.State)

	if alert.ResolvedAt != nil {
		body += fmt.Sprintf("\nResolved at: %s", alert.ResolvedAt.Format(time.RFC3339))
	}

	return fmt.Sprintf("%s\n\n%s", title, body)
}

// sendToApprise sends a message to Apprise API
func (n *Notifier) sendToApprise(url, message, severity string) error {
	// For MVP, we'll use Apprise API endpoint
	// Apprise API expects: POST /notify/{service} with body
	// For simplicity, we'll use the URL directly as Apprise service URL
	
	// If URL contains "://", it's already an Apprise service URL
	// Otherwise, assume it's an Apprise API endpoint
	
	// Simple implementation: if it looks like an Apprise service URL, use it directly
	// Otherwise, POST to Apprise API
	
	// For MVP, we'll assume Apprise service URLs are provided
	// Format: slack://tokenA/tokenB/tokenC
	// We'll use Apprise library or HTTP API
	
	// Simple HTTP POST to Apprise API (if running as service)
	// For MVP, we'll use direct Apprise service URLs
	
	// Create request body
	payload := map[string]string{
		"body": message,
		"title": fmt.Sprintf("NetSpec: %s", severity),
		"format": "text",
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Try Apprise API endpoint first (if APPRISE_API_URL is set)
	apiURL := os.Getenv("APPRISE_API_URL")
	if apiURL != "" {
		req, err := http.NewRequest("POST", fmt.Sprintf("%s/notify/%s", apiURL, url), bytes.NewBuffer(jsonData))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := n.client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("Apprise API error: %d - %s", resp.StatusCode, string(body))
		}

		return nil
	}

	// Fallback: log that we would send (for MVP without Apprise service)
	n.logger.Info().
		Str("url", url).
		Str("message", message).
		Msg("Would send notification (Apprise not configured)")

	return nil
}
