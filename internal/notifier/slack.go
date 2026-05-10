package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

const slackAPIBase = "https://slack.com/api"

// SlackNotifier posts interactive Block Kit alert messages to Slack via the Web API.
// It requires a bot token with chat:write scope and a channel ID per channel.
type SlackNotifier struct {
	botToken string
	client   *http.Client
	logger   zerolog.Logger
}

// NewSlackNotifier reads the bot token from botTokenEnv and returns a SlackNotifier.
// Returns nil if the env var is empty (feature disabled).
func NewSlackNotifier(botTokenEnv string, logger zerolog.Logger) *SlackNotifier {
	token := strings.TrimSpace(os.Getenv(botTokenEnv))
	if token == "" {
		return nil
	}
	return &SlackNotifier{
		botToken: token,
		client:   &http.Client{Timeout: 10 * time.Second},
		logger:   logger.With().Str("component", "slack-chatops").Logger(),
	}
}

type slackAPIResp struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	TS      string `json:"ts,omitempty"`
	Channel string `json:"channel,omitempty"`
}

// PostAlert posts a new Block Kit alert message and returns the Slack message timestamp.
func (s *SlackNotifier) PostAlert(alert *types.Alert, channelID string) (ts string, err error) {
	payload := map[string]interface{}{
		"channel": channelID,
		"text":    s.fallbackText(alert),
		"blocks":  s.buildBlocks(alert),
	}
	resp, err := s.callAPI("chat.postMessage", payload)
	if err != nil {
		return "", fmt.Errorf("chat.postMessage: %w", err)
	}
	s.logger.Info().Str("alert_id", alert.ID).Str("ts", resp.TS).Msg("Slack alert posted")
	return resp.TS, nil
}

// UpdateAlert edits the existing Slack message in-place (acked, resolved, etc.).
func (s *SlackNotifier) UpdateAlert(alert *types.Alert, channelID, msgTS string) error {
	if channelID == "" || msgTS == "" {
		return nil
	}
	payload := map[string]interface{}{
		"channel": channelID,
		"ts":      msgTS,
		"text":    s.fallbackText(alert),
		"blocks":  s.buildBlocks(alert),
	}
	_, err := s.callAPI("chat.update", payload)
	if err != nil {
		return fmt.Errorf("chat.update: %w", err)
	}
	s.logger.Info().Str("alert_id", alert.ID).Str("state", alert.State).Msg("Slack alert updated")
	return nil
}

func (s *SlackNotifier) callAPI(method string, payload interface{}) (*slackAPIResp, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, slackAPIBase+"/"+method, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.botToken)

	httpResp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer httpResp.Body.Close()

	var resp slackAPIResp
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if !resp.OK {
		return nil, fmt.Errorf("slack api: %s", resp.Error)
	}
	return &resp, nil
}

func (s *SlackNotifier) fallbackText(alert *types.Alert) string {
	emoji := alertEmoji(alert)
	switch alert.State {
	case "acked":
		return fmt.Sprintf("%s [ACKNOWLEDGED] %s on %s / %s by %s",
			emoji, alert.AlertType, alert.Device, alert.Entity, alert.AckedBy)
	case "resolved":
		return fmt.Sprintf("✅ [RESOLVED] %s on %s / %s", alert.AlertType, alert.Device, alert.Entity)
	default:
		return fmt.Sprintf("%s %s on %s / %s (%s)", emoji, alert.AlertType, alert.Device, alert.Entity, strings.ToUpper(alert.Severity))
	}
}

func alertEmoji(alert *types.Alert) string {
	if alert.State == "resolved" {
		return "✅"
	}
	switch strings.ToLower(alert.Severity) {
	case "critical":
		return "🔴"
	case "warning":
		return "⚠️"
	default:
		return "ℹ️"
	}
}

// slackText is the Slack text object used inside blocks.
type slackText struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji,omitempty"`
}

func (s *SlackNotifier) buildBlocks(alert *types.Alert) []interface{} {
	emoji := alertEmoji(alert)

	var headerStr string
	switch alert.State {
	case "acked":
		headerStr = fmt.Sprintf("%s NetSpec — %s [ACKNOWLEDGED]", emoji, alert.AlertType)
	case "resolved":
		headerStr = fmt.Sprintf("%s NetSpec — %s [RESOLVED]", emoji, alert.AlertType)
	default:
		headerStr = fmt.Sprintf("%s NetSpec — %s", emoji, alert.AlertType)
	}

	header := map[string]interface{}{
		"type": "header",
		"text": slackText{Type: "plain_text", Text: headerStr, Emoji: true},
	}

	fields := []slackText{
		{Type: "mrkdwn", Text: fmt.Sprintf("*Device:*\n%s", alert.Device)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Interface:*\n%s", alert.Entity)},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Severity:*\n%s", strings.ToUpper(alert.Severity))},
		{Type: "mrkdwn", Text: fmt.Sprintf("*Alert ID:*\n`%s`", alert.ID)},
	}
	details := map[string]interface{}{
		"type":   "section",
		"fields": fields,
	}

	var statusLines []string
	switch alert.State {
	case "acked":
		statusLines = append(statusLines, fmt.Sprintf("*Status:* ACKNOWLEDGED by *%s*", alert.AckedBy))
		if alert.AckedAt != nil {
			statusLines = append(statusLines, fmt.Sprintf("*Acked at:* %s", alert.AckedAt.UTC().Format("2006-01-02 15:04:05 UTC")))
		}
		if alert.AckNote != "" {
			statusLines = append(statusLines, fmt.Sprintf("*Note:* %s", alert.AckNote))
		}
	case "resolved":
		statusLines = append(statusLines, "*Status:* RESOLVED")
		if alert.ResolvedAt != nil {
			statusLines = append(statusLines, fmt.Sprintf("*Resolved at:* %s", alert.ResolvedAt.UTC().Format("2006-01-02 15:04:05 UTC")))
			duration := alert.ResolvedAt.Sub(alert.FiredAt).Round(time.Second)
			statusLines = append(statusLines, fmt.Sprintf("*Duration:* %s", duration))
		}
		if alert.AckedBy != "" {
			statusLines = append(statusLines, fmt.Sprintf("*Closed by:* %s", alert.AckedBy))
		}
	default:
		statusLines = append(statusLines, fmt.Sprintf("*Status:* FIRING since %s", alert.FiredAt.UTC().Format("2006-01-02 15:04:05 UTC")))
	}
	statusBlock := map[string]interface{}{
		"type": "section",
		"text": slackText{Type: "mrkdwn", Text: strings.Join(statusLines, "\n")},
	}

	msgBlock := map[string]interface{}{
		"type": "section",
		"text": slackText{Type: "mrkdwn", Text: alert.Message},
	}

	blocks := []interface{}{header, details, statusBlock, msgBlock}

	if len(alert.RelatedState) > 0 {
		keys := make([]string, 0, len(alert.RelatedState))
		for k := range alert.RelatedState {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var sb strings.Builder
		for _, k := range keys {
			fmt.Fprintf(&sb, "• *%s:* %s\n", k, alert.RelatedState[k])
		}
		blocks = append(blocks, map[string]interface{}{
			"type": "section",
			"text": slackText{Type: "mrkdwn", Text: strings.TrimRight(sb.String(), "\n")},
		})
	}

	// Show action buttons only while the alert is actively firing.
	if alert.State == "firing" {
		blocks = append(blocks, map[string]interface{}{"type": "divider"})
		blocks = append(blocks, map[string]interface{}{
			"type": "actions",
			"elements": []interface{}{
				map[string]interface{}{
					"type":      "button",
					"style":     "primary",
					"action_id": "ack_alert",
					"value":     alert.ID,
					"text":      slackText{Type: "plain_text", Text: "Acknowledge", Emoji: true},
				},
				map[string]interface{}{
					"type":      "button",
					"style":     "danger",
					"action_id": "close_alert",
					"value":     alert.ID,
					"text":      slackText{Type: "plain_text", Text: "Close Alert", Emoji: true},
					"confirm": map[string]interface{}{
						"title":   slackText{Type: "plain_text", Text: "Close this alert?"},
						"text":    slackText{Type: "mrkdwn", Text: "Removes from active monitoring. Will re-alert if the problem persists."},
						"confirm": slackText{Type: "plain_text", Text: "Yes, close it"},
						"deny":    slackText{Type: "plain_text", Text: "Cancel"},
					},
				},
			},
		})
	}

	return blocks
}
