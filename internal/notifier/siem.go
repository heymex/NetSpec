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

const (
	defaultSplunkSourcetype = "netspec:alert"
	defaultSplunkSource     = "netspec"
)

// splunkHECPayload is the HTTP Event Collector envelope.
// event carries the same structured JSON as webhook channels.
type splunkHECPayload struct {
	Time       float64        `json:"time"`
	Host       string         `json:"host,omitempty"`
	Source     string         `json:"source,omitempty"`
	SourceType string         `json:"sourcetype,omitempty"`
	Index      string         `json:"index,omitempty"`
	Event      webhookPayload `json:"event"`
}

// elasticDocument is the webhook payload plus @timestamp for ECS-friendly indexing.
type elasticDocument struct {
	Timestamp string `json:"@timestamp"`
	webhookPayload
}

func (n *Notifier) deliverSplunkChannel(channelName string, ch config.ChannelConfig, alert *types.Alert) error {
	publicBase := strings.TrimSpace(os.Getenv("NETSPEC_PUBLIC_URL"))
	event := buildWebhookPayload(alert, publicBase)

	source := strings.TrimSpace(ch.Source)
	if source == "" {
		source = defaultSplunkSource
	}
	sourcetype := strings.TrimSpace(ch.Sourcetype)
	if sourcetype == "" {
		sourcetype = defaultSplunkSourcetype
	}
	index := ch.ResolvedSIEMIndex()

	hec := splunkHECPayload{
		Time:       float64(alert.FiredAt.UTC().UnixNano()) / 1e9,
		Host:       alert.Device,
		Source:     source,
		SourceType: sourcetype,
		Index:      index,
		Event:      event,
	}
	jsonData, err := json.Marshal(hec)
	if err != nil {
		return fmt.Errorf("marshal splunk payload: %w", err)
	}

	var errs []error
	for _, ep := range ch.ResolvedSIEMEndpoints() {
		if err := n.postSplunkHEC(channelName, ep, event.Event, alert.ID, jsonData); err != nil {
			n.logger.Error().Err(err).Str("channel", channelName).Str("endpoint", ep.Name).Msg("failed to send splunk notification")
			errs = append(errs, fmt.Errorf("endpoint %s: %w", ep.Name, err))
			continue
		}
		n.logger.Info().Str("channel", channelName).Str("endpoint", ep.Name).Str("alert_id", alert.ID).Msg("splunk notification sent")
	}
	return errors.Join(errs...)
}

func (n *Notifier) postSplunkHEC(channelName string, ep config.WebhookEndpoint, event, alertID string, jsonData []byte) error {
	hecURL, token, err := resolveURLAndToken(ep)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("environment variable %s is not set or empty", ep.TokenEnv)
	}

	req, err := http.NewRequest(http.MethodPost, hecURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Splunk "+token)

	n.logger.Debug().
		Str("channel", channelName).
		Str("endpoint", ep.Name).
		Str("event", event).
		Str("alert_id", alertID).
		Str("url", scrubServiceURL(hecURL)).
		Msg("splunk hec notify")

	return n.doSIEMRequest(req)
}

func (n *Notifier) deliverElasticsearchChannel(channelName string, ch config.ChannelConfig, alert *types.Alert) error {
	publicBase := strings.TrimSpace(os.Getenv("NETSPEC_PUBLIC_URL"))
	payload := buildWebhookPayload(alert, publicBase)
	doc := elasticDocument{
		Timestamp:      alert.FiredAt.UTC().Format(time.RFC3339Nano),
		webhookPayload: payload,
	}
	jsonData, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal elasticsearch payload: %w", err)
	}

	index := ch.ResolvedSIEMIndex()
	var errs []error
	for _, ep := range ch.ResolvedSIEMEndpoints() {
		if err := n.postElasticsearchDoc(channelName, ep, index, payload.Event, alert.ID, jsonData); err != nil {
			n.logger.Error().Err(err).Str("channel", channelName).Str("endpoint", ep.Name).Msg("failed to send elasticsearch notification")
			errs = append(errs, fmt.Errorf("endpoint %s: %w", ep.Name, err))
			continue
		}
		n.logger.Info().Str("channel", channelName).Str("endpoint", ep.Name).Str("alert_id", alert.ID).Msg("elasticsearch notification sent")
	}
	return errors.Join(errs...)
}

func (n *Notifier) postElasticsearchDoc(channelName string, ep config.WebhookEndpoint, index, event, alertID string, jsonData []byte) error {
	baseURL, token, err := resolveURLAndToken(ep)
	if err != nil {
		return err
	}
	docURL, err := resolveElasticDocURL(baseURL, index)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, docURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if err := applyElasticsearchAuth(req, ep.Auth, ep, token); err != nil {
		return err
	}

	n.logger.Debug().
		Str("channel", channelName).
		Str("endpoint", ep.Name).
		Str("event", event).
		Str("alert_id", alertID).
		Str("url", scrubServiceURL(docURL)).
		Msg("elasticsearch notify")

	return n.doSIEMRequest(req)
}

func applyElasticsearchAuth(req *http.Request, auth string, ep config.WebhookEndpoint, token string) error {
	switch auth {
	case "none", "":
		return nil
	case "api_key":
		if token == "" {
			return fmt.Errorf("environment variable %s is not set or empty", ep.TokenEnv)
		}
		req.Header.Set("Authorization", "ApiKey "+token)
	case "bearer":
		if token == "" {
			return fmt.Errorf("environment variable %s is not set or empty", ep.TokenEnv)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	default:
		return fmt.Errorf("unsupported auth %q", auth)
	}
	return nil
}

func resolveElasticDocURL(baseURL, index string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("elasticsearch URL is not an http(s) URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		if index == "" {
			return "", fmt.Errorf("index is required when URL has no path (set index or index_env, or use a full .../{index}/_doc URL)")
		}
		parsed.Path = "/" + strings.Trim(index, "/") + "/_doc"
	}
	return parsed.String(), nil
}

func resolveURLAndToken(ep config.WebhookEndpoint) (endpointURL, token string, err error) {
	endpointURL = strings.TrimSpace(os.Getenv(ep.URLEnv))
	if endpointURL == "" {
		return "", "", fmt.Errorf("environment variable %s is not set or empty", ep.URLEnv)
	}
	parsed, err := url.Parse(endpointURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", fmt.Errorf("environment variable %s is not an http(s) URL", ep.URLEnv)
	}
	if ep.TokenEnv != "" {
		token = strings.TrimSpace(os.Getenv(ep.TokenEnv))
		if token != "" && strings.ContainsAny(token, "\r\n") {
			return "", "", fmt.Errorf("environment variable %s contains invalid header characters", ep.TokenEnv)
		}
	}
	return endpointURL, token, nil
}

func (n *Notifier) doSIEMRequest(req *http.Request) error {
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s", summarizeHTTPError(resp.StatusCode, respBody))
	}
	return nil
}
