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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

// appriseNotifyKey matches keys for POST /notify/{key}/ (Apprise-API stored configs).
var appriseNotifyKey = regexp.MustCompile(`^[\w-]{1,128}$`)

// Notifier sends alerts through an Apprise-API instance (linuxserver/apprise-api or caronc/apprise-api).
type Notifier struct {
	logger   zerolog.Logger
	client   *http.Client
	channels map[string]config.ChannelConfig
}

// NewNotifier builds a notifier using channel definitions from alerts.yaml (merged into cfg.Alerts).
func NewNotifier(logger zerolog.Logger, channels map[string]config.ChannelConfig) *Notifier {
	if channels == nil {
		channels = make(map[string]config.ChannelConfig)
	}
	timeout := 10 * time.Second
	if v := strings.TrimSpace(os.Getenv("APPRISE_NOTIFY_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			timeout = d
		}
	}
	return &Notifier{
		logger:   logger.With().Str("component", "notifier").Logger(),
		client:   &http.Client{Timeout: timeout},
		channels: channels,
	}
}

// SendAlert delivers an alert to the given logical channel names (from alert_rules).
// Supports type "apprise" (default) and "openclaw". type "slack_chatops" is skipped here
// (handled by SlackNotifier in the alert engine).
func (n *Notifier) SendAlert(alert *types.Alert, channelNames []string) error {
	if len(channelNames) == 0 {
		return nil
	}

	apiBase := strings.TrimSpace(os.Getenv("APPRISE_API_URL"))
	title := fmt.Sprintf("NetSpec: %s", alert.Severity)
	body := n.formatMessage(alert)
	notifyType := appriseNotifyType(alert.Severity, alert.State)

	var errs []error

	for _, name := range channelNames {
		ch, ok := n.channels[name]
		if !ok {
			n.logger.Warn().Str("channel", name).Msg("unknown alert channel referenced in alert_rules")
			errs = append(errs, fmt.Errorf("channel %q: not defined in alerts.channels", name))
			continue
		}
		if !severityAllows(ch.SeverityFilter, alert.Severity) {
			n.logger.Debug().
				Str("channel", name).
				Str("severity", alert.Severity).
				Msg("channel skipped due to severity_filter")
			continue
		}

		switch ch.Type {
		case "slack_chatops":
			// Handled by SlackNotifier in the alert engine.
			n.logger.Debug().Str("channel", name).Str("type", ch.Type).Msg("skipping non-apprise channel")
			continue
		case "openclaw":
			webhookURL := strings.TrimSpace(os.Getenv(ch.URLEnv))
			if webhookURL == "" {
				n.logger.Warn().
					Str("channel", name).
					Str("url_env", ch.URLEnv).
					Msg("openclaw webhook URL environment variable is empty")
				errs = append(errs, fmt.Errorf("channel %q: environment variable %s is not set or empty", name, ch.URLEnv))
				continue
			}
			token := ""
			if ch.TokenEnv != "" {
				token = strings.TrimSpace(os.Getenv(ch.TokenEnv))
				if token == "" {
					n.logger.Warn().
						Str("channel", name).
						Str("token_env", ch.TokenEnv).
						Msg("openclaw token environment variable is empty")
					errs = append(errs, fmt.Errorf("channel %q: environment variable %s is not set or empty", name, ch.TokenEnv))
					continue
				}
			}
			if err := n.deliverOpenClaw(webhookURL, token, name, alert); err != nil {
				n.logger.Error().Err(err).Str("channel", name).Msg("failed to send openclaw notification")
				errs = append(errs, fmt.Errorf("channel %q: %w", name, err))
			} else {
				n.logger.Info().Str("channel", name).Str("alert_id", alert.ID).Msg("openclaw notification sent")
			}
		case "apprise", "":
			if apiBase == "" {
				errs = append(errs, fmt.Errorf("channel %q: APPRISE_API_URL is not set; cannot deliver Apprise notifications (set it to your Apprise-API base URL, e.g. http://localhost:8086)", name))
				continue
			}
			serviceURL := strings.TrimSpace(os.Getenv(ch.URLEnv))
			if serviceURL == "" {
				n.logger.Warn().
					Str("channel", name).
					Str("url_env", ch.URLEnv).
					Msg("notification URL environment variable is empty")
				errs = append(errs, fmt.Errorf("channel %q: environment variable %s is not set or empty", name, ch.URLEnv))
				continue
			}
			if err := n.deliver(apiBase, serviceURL, title, body, notifyType, name, scrubServiceURL(serviceURL)); err != nil {
				n.logger.Error().Err(err).Str("channel", name).Msg("failed to send notification")
				errs = append(errs, fmt.Errorf("channel %q: %w", name, err))
			} else {
				n.logger.Info().Str("channel", name).Str("alert_id", alert.ID).Msg("notification sent")
			}
		default:
			n.logger.Warn().Str("channel", name).Str("type", ch.Type).Msg("unsupported alert channel type")
			errs = append(errs, fmt.Errorf("channel %q: unsupported type %q", name, ch.Type))
		}
	}

	return errors.Join(errs...)
}

// ChannelTestOutcome is one channel's result from NotifyAppriseTest.
type ChannelTestOutcome struct {
	Channel string `json:"channel"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// NotifyAppriseTest sends a synthetic warning to Apprise for each named channel (or all channels if names is empty).
// Uses the same delivery path as real alerts. Channels whose severity_filter excludes "warning" are reported as ok with a skip message.
func (n *Notifier) NotifyAppriseTest(channelNames []string) ([]ChannelTestOutcome, error) {
	apiBase := strings.TrimSpace(os.Getenv("APPRISE_API_URL"))
	if apiBase == "" {
		return nil, fmt.Errorf("APPRISE_API_URL is not set; cannot deliver notifications (set it to your Apprise-API base URL, e.g. http://localhost:8086)")
	}

	names := append([]string(nil), channelNames...)
	if len(names) == 0 {
		for name := range n.channels {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no alert channels are configured in alerts.yaml")
	}

	const testSeverity = "warning"
	title := "NetSpec: notification test"
	body := "This is a manual test from the NetSpec web UI. If you see this, Apprise routing and your destination URL are working."
	notifyType := appriseNotifyType(testSeverity, "firing")

	outcomes := make([]ChannelTestOutcome, 0, len(names))
	var errs []error

	for _, name := range names {
		ch, ok := n.channels[name]
		if !ok {
			outcomes = append(outcomes, ChannelTestOutcome{Channel: name, OK: false, Message: "unknown channel (not in alerts.yaml)"})
			errs = append(errs, fmt.Errorf("channel %q: not defined in alerts.channels", name))
			continue
		}
		if ch.Type != "" && ch.Type != "apprise" {
			outcomes = append(outcomes, ChannelTestOutcome{
				Channel: name,
				OK:      true,
				Message: fmt.Sprintf("skipped: channel type %q is not delivered via Apprise test (use a real alert or openclaw webhook)", ch.Type),
			})
			continue
		}
		if !severityAllows(ch.SeverityFilter, testSeverity) {
			outcomes = append(outcomes, ChannelTestOutcome{
				Channel: name,
				OK:      true,
				Message: fmt.Sprintf("skipped: channel severity_filter does not include %q (test uses warning severity)", testSeverity),
			})
			continue
		}
		serviceURL := strings.TrimSpace(os.Getenv(ch.URLEnv))
		if serviceURL == "" {
			outcomes = append(outcomes, ChannelTestOutcome{
				Channel: name,
				OK:      false,
				Message: fmt.Sprintf("environment variable %s is not set or empty", ch.URLEnv),
			})
			errs = append(errs, fmt.Errorf("channel %q: environment variable %s is not set or empty", name, ch.URLEnv))
			continue
		}
		if err := n.deliver(apiBase, serviceURL, title, body, notifyType, name, scrubServiceURL(serviceURL)); err != nil {
			outcomes = append(outcomes, ChannelTestOutcome{Channel: name, OK: false, Message: err.Error()})
			errs = append(errs, fmt.Errorf("channel %q: %w", name, err))
		} else {
			outcomes = append(outcomes, ChannelTestOutcome{Channel: name, OK: true, Message: "delivered"})
			n.logger.Info().Str("channel", name).Msg("apprise test notification sent")
		}
	}

	return outcomes, errors.Join(errs...)
}

func severityAllows(filter []string, severity string) bool {
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

func appriseNotifyType(severity, state string) string {
	if strings.EqualFold(state, "resolved") {
		return "success"
	}
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return "failure"
	case "warning":
		return "warning"
	case "info":
		return "info"
	default:
		return "warning"
	}
}

func (n *Notifier) deliver(apiBase, serviceURL, title, body, notifyType, channelName, serviceURLRedacted string) error {
	reqURL, payload, err := buildNotifyRequest(apiBase, serviceURL, title, body, notifyType)
	if err != nil {
		return err
	}

	mode := "keyed"
	if _, ok := payload["urls"]; ok {
		mode = "stateless"
	}
	parsed, _ := url.Parse(reqURL)
	path := parsed.Path
	if path == "" {
		path = "/"
	}
	n.logger.Debug().
		Str("channel", channelName).
		Str("delivery", mode).
		Str("path", path).
		Str("url_env_target", serviceURLRedacted).
		Msg("apprise notify")

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal notify payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("apprise-api %s", summarizeAppriseAPIError(resp.StatusCode, respBody))
	}
	return nil
}

// scrubServiceURL returns a log-safe hint for an Apprise URL or raw notify key.
func scrubServiceURL(serviceURL string) string {
	s := strings.TrimSpace(serviceURL)
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "://"); i > 0 {
		return s[:i+3] + "(redacted)"
	}
	return s
}

// summarizeAppriseAPIError turns Apprise-API JSON error bodies into a short string for logs and wrapped errors.
func summarizeAppriseAPIError(status int, body []byte) string {
	s := strings.TrimSpace(string(body))
	if s == "" {
		return fmt.Sprintf("HTTP %d (empty body)", status)
	}
	var wrap struct {
		Error   string          `json:"error"`
		Details json.RawMessage `json:"details"`
	}
	if json.Unmarshal(body, &wrap) == nil && (wrap.Error != "" || len(bytes.TrimSpace(wrap.Details)) > 0) {
		d := strings.TrimSpace(string(wrap.Details))
		if len(d) > 400 {
			d = d[:400] + "…"
		}
		if wrap.Error != "" && d != "" {
			return fmt.Sprintf("HTTP %d: %s details=%s", status, wrap.Error, d)
		}
		if wrap.Error != "" {
			return fmt.Sprintf("HTTP %d: %s", status, wrap.Error)
		}
		return fmt.Sprintf("HTTP %d: details=%s", status, d)
	}
	if len(s) > 500 {
		return fmt.Sprintf("HTTP %d: %s…", status, s[:500])
	}
	return fmt.Sprintf("HTTP %d: %s", status, s)
}

// buildNotifyRequest chooses Apprise-API stateless vs keyed notify URL and JSON body.
// - Service URLs (contain "://") use POST /notify/ with "urls" set (NotifyByUrlForm).
// - Otherwise, if the value matches the API key pattern, POST /notify/{key}/ without "urls" (NotifyForm + stored config).
func buildNotifyRequest(apiBase, serviceURL, title, body, notifyType string) (reqURL string, payload map[string]string, err error) {
	base, err := url.Parse(strings.TrimSpace(apiBase))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", nil, fmt.Errorf("invalid APPRISE_API_URL %q", apiBase)
	}

	payload = map[string]string{
		"title":  title,
		"body":   body,
		"format": "text",
		"type":   notifyType,
	}

	if strings.Contains(serviceURL, "://") {
		u := base.JoinPath("notify")
		payload["urls"] = serviceURL
		return u.String(), payload, nil
	}

	if appriseNotifyKey.MatchString(serviceURL) {
		u := base.JoinPath("notify", serviceURL)
		return u.String(), payload, nil
	}

	return "", nil, fmt.Errorf("value must be an Apprise service URL (contains ://) or a stored notify key (letters, digits, underscore, hyphen, max 128 chars); got %q", serviceURL)
}

// formatMessage formats an alert into a notification message
func (n *Notifier) formatMessage(alert *types.Alert) string {
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
	msgBody := fmt.Sprintf("%s\n\nDevice: %s\nInterface: %s\nSeverity: %s\nState: %s",
		alert.Message, alert.Device, alert.Entity, alert.Severity, alert.State)

	if alert.ResolvedAt != nil {
		msgBody += fmt.Sprintf("\nResolved at: %s", alert.ResolvedAt.Format(time.RFC3339))
	}

	return fmt.Sprintf("%s\n\n%s", title, msgBody)
}
