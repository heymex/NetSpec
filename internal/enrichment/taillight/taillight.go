package taillight

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Backend specifies how the client reaches Taillight data.
type Backend string

const (
	BackendAPI     Backend = "api"
	BackendElastic Backend = "elastic"
)

// ClientConfig is a flat, self-contained config for the Taillight client.
type ClientConfig struct {
	Backend      Backend
	Enabled      bool
	BaseURL      string
	APIKey       string
	ElasticURL   string
	ElasticAPIKey string
	SyslogIndex  string
	SummaryIndex string
}

// Client communicates with Taillight, either via its REST API or through Elastic.
type Client struct {
	backend Backend
	http    *http.Client

	baseURL      string
	apiKey       string
	elasticURL   string
	elasticAPIKey string
	syslogIndex  string
	summaryIndex string
	logger       zerolog.Logger
}

// NewClient creates a Taillight client from the resolved config.
func NewClient(cfg ClientConfig, logger zerolog.Logger) *Client {
	if !cfg.Enabled {
		return nil
	}

	backend := cfg.Backend
	if backend == "" {
		backend = BackendAPI
	}
	c := &Client{
		backend: backend,
		http:    &http.Client{Timeout: 10 * time.Second},
		logger:  logger.With().Str("enricher", "taillight").Logger(),
	}

	switch backend {
	case BackendAPI:
		c.baseURL = strings.TrimRight(cfg.BaseURL, "/")
		c.apiKey = cfg.APIKey
	case BackendElastic:
		c.elasticURL = strings.TrimRight(cfg.ElasticURL, "/")
		c.elasticAPIKey = cfg.ElasticAPIKey
		c.syslogIndex = cfg.SyslogIndex
		if c.syslogIndex == "" {
			c.syslogIndex = "logs-netlog*"
		}
		c.summaryIndex = cfg.SummaryIndex
		if c.summaryIndex == "" {
			c.summaryIndex = "taillight-report-*"
		}
	default:
		c.logger.Warn().Str("backend", string(backend)).Msg("unknown taillight backend, defaulting to api")
		c.baseURL = strings.TrimRight(cfg.BaseURL, "/")
		c.apiKey = cfg.APIKey
		c.backend = BackendAPI
	}

	return c
}

// QuerySyslog fetches raw log entries for the given device in the lookback window.
func (c *Client) QuerySyslog(ctx context.Context, hostname string, window time.Duration, severityFilter []string, maxEntries int) ([]SyslogEntry, int, error) {
	switch c.backend {
	case BackendAPI:
		return c.querySyslogAPI(ctx, hostname, window, severityFilter, maxEntries)
	case BackendElastic:
		return c.querySyslogElastic(ctx, hostname, window, severityFilter, maxEntries)
	default:
		return nil, 0, fmt.Errorf("taillight: unknown backend %s", c.backend)
	}
}

// QueryDailySummary fetches the daily analysis report for the given device.
func (c *Client) QueryDailySummary(ctx context.Context, hostname string, date time.Time, eventKind string) (*DailySummary, error) {
	switch c.backend {
	case BackendAPI:
		return c.queryDailySummaryAPI(ctx, hostname, date, eventKind)
	case BackendElastic:
		return c.queryDailySummaryElastic(ctx, hostname, date, eventKind)
	default:
		return nil, fmt.Errorf("taillight: unknown backend %s", c.backend)
	}
}

// -- API Transport --

func (c *Client) querySyslogAPI(ctx context.Context, hostname string, window time.Duration, severityFilter []string, maxEntries int) ([]SyslogEntry, int, error) {
	params := url.Values{}
	params.Set("hostname", hostname)
	params.Set("start", time.Now().Add(-window).Format(time.RFC3339))
	params.Set("limit", fmt.Sprintf("%d", maxEntries))
	if len(severityFilter) > 0 {
		params.Set("severity", strings.Join(severityFilter, ","))
	}

	reqURL := c.baseURL + "/api/v1/netlog/events?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return c.querySyslogAPISrvlog(ctx, hostname, window, severityFilter, maxEntries)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("taillight netlog API %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Count int              `json:"count"`
		Items []taillightEvent `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	entries := make([]SyslogEntry, 0, len(result.Items))
	for _, item := range result.Items {
		entries = append(entries, SyslogEntry{
			Timestamp: item.Timestamp.Format(time.RFC3339),
			Hostname:  item.Hostname,
			Facility:  fmt.Sprintf("%s (%d)", item.Facility, item.FacilityNum),
			Severity:  fmt.Sprintf("%s (%d)", item.Severity, item.SeverityNum),
			Program:   item.Programname,
			Message:   item.Message,
		})
	}

	return entries, result.Count, nil
}

func (c *Client) querySyslogAPISrvlog(ctx context.Context, hostname string, window time.Duration, severityFilter []string, maxEntries int) ([]SyslogEntry, int, error) {
	params := url.Values{}
	params.Set("hostname", hostname)
	params.Set("start", time.Now().Add(-window).Format(time.RFC3339))
	params.Set("limit", fmt.Sprintf("%d", maxEntries))
	if len(severityFilter) > 0 {
		params.Set("severity", strings.Join(severityFilter, ","))
	}

	reqURL := c.baseURL + "/api/v1/srvlog/events?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, 0, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("taillight srvlog API %d: %s", resp.StatusCode, "no srvlog data")
	}

	var result struct {
		Count int              `json:"count"`
		Items []taillightEvent `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}

	entries := make([]SyslogEntry, 0, len(result.Items))
	for _, item := range result.Items {
		entries = append(entries, SyslogEntry{
			Timestamp: item.Timestamp.Format(time.RFC3339),
			Hostname:  item.Hostname,
			Facility:  fmt.Sprintf("%s (%d)", item.Facility, item.FacilityNum),
			Severity:  fmt.Sprintf("%s (%d)", item.Severity, item.SeverityNum),
			Program:   item.Programname,
			Message:   item.Message,
		})
	}

	return entries, result.Count, nil
}

func (c *Client) queryDailySummaryAPI(ctx context.Context, hostname string, date time.Time, eventKind string) (*DailySummary, error) {
	reqURL := c.baseURL + "/api/v1/analysis/reports/daily"
	body := map[string]interface{}{
		"date":       date.Format("2006-01-02"),
		"hostname":   hostname,
		"event_kind": eventKind,
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("taillight daily summary API %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var result struct {
		TotalEvents  int        `json:"total_events"`
		ErrorCount   int        `json:"error_count"`
		WarningCount int        `json:"warning_count"`
		TopMessages  []topMsg   `json:"top_messages"`
		HasAnomalies bool       `json:"has_anomalies"`
		SummaryText  string     `json:"summary_text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	top := make([]MessageCount, 0, len(result.TopMessages))
	for _, m := range result.TopMessages {
		top = append(top, MessageCount{
			Message: m.Message,
			Count:   m.Count,
		})
	}

	return &DailySummary{
		ReportDate:   date.Format("2006-01-02"),
		TotalEvents:  result.TotalEvents,
		ErrorCount:   result.ErrorCount,
		WarningCount: result.WarningCount,
		TopMessages:  top,
		HasAnomalies: result.HasAnomalies,
		SummaryText:  result.SummaryText,
	}, nil
}

// -- Elastic Transport --

func (c *Client) querySyslogElastic(ctx context.Context, hostname string, window time.Duration, severityFilter []string, maxEntries int) ([]SyslogEntry, int, error) {
	fromTime := time.Now().Add(-window).Format(time.RFC3339Nano)
	toTime := time.Now().Format(time.RFC3339Nano)

	var parts []string
	if len(severityFilter) > 0 {
		for _, s := range severityFilter {
			parts = append(parts, fmt.Sprintf("severity.keyword:%s", strings.ToLower(s)))
			parts = append(parts, fmt.Sprintf("severity_label:%s", strings.ToLower(s)))
		}
	}
	kql := fmt.Sprintf("hostname:%s AND @timestamp:[%s TO %s]", hostname, fromTime, toTime)
	if len(parts) > 0 {
		kql += " AND (" + strings.Join(parts, " OR ") + ")"
	}

	queryBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{"query_string": map[string]interface{}{
						"query":         kql,
						"default_field": "_all",
					}},
				},
			},
		},
		"size": maxEntries,
		"_source": []string{
			"timestamp", "hostname", "facility", "facility_number",
			"severity", "severity_number", "severity_label",
			"programname", "message",
		},
		"sort": []map[string]interface{}{
			{"@timestamp": "desc"},
		},
	}

	resp, err := c.doElasticQuery(ctx, c.syslogIndex, queryBody)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, 0, fmt.Errorf("taillight elastic syslog query %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var elasticResp struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []elasticHit `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&elasticResp); err != nil {
		return nil, 0, err
	}

	entries := make([]SyslogEntry, 0, len(elasticResp.Hits.Hits))
	for _, h := range elasticResp.Hits.Hits {
		src := h.Source
		entries = append(entries, SyslogEntry{
			Timestamp: formatElasticTimestamp(src["@timestamp"]),
			Hostname:  src["hostname"].(string),
			Facility:  fmt.Sprintf("%s (%d)", src["severity_label"], int(src["severity_number"].(float64))),
			Severity:  fmt.Sprintf("%s (%d)", src["severity_label"], int(src["severity_number"].(float64))),
			Program:   src["programname"].(string),
			Message:   src["message"].(string),
		})
	}

	return entries, elasticResp.Hits.Total.Value, nil
}

func (c *Client) queryDailySummaryElastic(ctx context.Context, hostname string, date time.Time, eventKind string) (*DailySummary, error) {
	startOfDay := date.Format("2006-01-02") + "T00:00:00Z"
	endOfDay := date.Format("2006-01-02") + "T23:59:59Z"

	queryBody := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{"term": map[string]interface{}{"hostname": hostname}},
					{"range": map[string]interface{}{
						"@timestamp": map[string]interface{}{
							"gte": startOfDay,
							"lte": endOfDay,
						},
					}},
				},
			},
		},
		"size": 1,
		"_source": []string{"total_events", "error_count", "warning_count", "top_messages", "has_anomalies", "summary_text"},
	}

	resp, err := c.doElasticQuery(ctx, c.summaryIndex, queryBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var elasticResp struct {
		Hits struct {
			Total struct{ Value int } `json:"total"`
			Hits []elasticHit         `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&elasticResp); err != nil {
		return nil, nil
	}
	if len(elasticResp.Hits.Hits) == 0 {
		return nil, nil
	}

	src := elasticResp.Hits.Hits[0].Source
	topMessages := make([]MessageCount, 0)
	if tm, ok := src["top_messages"].([]interface{}); ok {
		for _, tmItem := range tm {
			if m, ok := tmItem.(map[string]interface{}); ok {
				topMessages = append(topMessages, MessageCount{
					Message: m["message"].(string),
					Count:   int(m["count"].(float64)),
				})
			}
		}
	}

	return &DailySummary{
		ReportDate:   startOfDay[:10],
		TotalEvents:  int(src["total_events"].(float64)),
		ErrorCount:   int(src["error_count"].(float64)),
		WarningCount: int(src["warning_count"].(float64)),
		TopMessages:  topMessages,
		HasAnomalies: src["has_anomalies"] == true,
		SummaryText:  src["summary_text"].(string),
	}, nil
}

func (c *Client) doElasticQuery(ctx context.Context, index string, queryBody map[string]interface{}) (*http.Response, error) {
	index = strings.TrimSpace(index)
	if index == "" {
		index = "logs-netlog*"
	}

	reqURL := fmt.Sprintf("%s/%s/_search", c.elasticURL, url.PathEscape(index))
	reqBody, err := json.Marshal(queryBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.elasticAPIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+c.elasticAPIKey)
	}

	return c.http.Do(req)
}

// -- Internal types --

type taillightEvent struct {
	Timestamp   time.Time `json:"timestamp"`
	Hostname    string    `json:"hostname"`
	Facility    string    `json:"facility"`
	FacilityNum int       `json:"facility_number"`
	Severity    string    `json:"severity"`
	SeverityNum int       `json:"severity_number"`
	Programname string    `json:"programname"`
	Message     string    `json:"message"`
}

type topMsg struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

type elasticHit struct {
	Source map[string]interface{} `json:"_source"`
}

// SyslogEntry mirrors a Taillight netlog/srvlog event.
type SyslogEntry struct {
	Timestamp string `json:"timestamp"`
	Hostname  string `json:"hostname"`
	Facility  string `json:"facility,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Program   string `json:"program,omitempty"`
	Message   string `json:"message"`
}

// MessageCount is a {message, count} pair for top-N aggregations.
type MessageCount struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// DailySummary is the Taillight daily analysis report for a device.
type DailySummary struct {
	ReportDate   string         `json:"report_date"`
	TotalEvents  int            `json:"total_events"`
	ErrorCount   int            `json:"error_count"`
	WarningCount int            `json:"warning_count"`
	TopMessages  []MessageCount `json:"top_messages,omitempty"`
	HasAnomalies bool           `json:"has_anomalies"`
	SummaryText  string         `json:"summary_text,omitempty"`
}

// formatElasticTimestamp normalizes Elastic timestamps (can be string or number).
func formatElasticTimestamp(val interface{}) string {
	switch v := val.(type) {
	case string:
		return v
	case float64:
		return time.Unix(int64(v), 0).Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", val)
	}
}
