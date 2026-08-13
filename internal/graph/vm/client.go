// Package vm is a thin VictoriaMetrics / MetricsQL HTTP client.
package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to a single-node VictoriaMetrics instance.
type Client struct {
	base   string
	client *http.Client
}

// NewClient returns a Client for baseURL (e.g. http://netspec-victoriametrics:8428).
func NewClient(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// BaseURL returns the configured VictoriaMetrics base URL.
func (c *Client) BaseURL() string {
	return c.base
}

// Ping checks that VM's /health endpoint responds OK.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("victoria metrics health: status %d", resp.StatusCode)
	}
	return nil
}

// Sample is one (timestamp, value) point. Value is nil when the sample is missing/NaN/Inf
// (empty series must not be rendered as a flat zero line).
type Sample struct {
	T int64    `json:"t"` // unix seconds
	V *float64 `json:"v"`
}

// Series is a time-aligned list of samples for one MetricsQL expression.
type Series struct {
	Metric map[string]string `json:"metric,omitempty"`
	Points []Sample          `json:"points"`
}

type queryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]any           `json:"values"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

// QueryRange runs a MetricsQL range query and returns typed series.
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error) {
	if step <= 0 {
		step = 30 * time.Second
	}
	u, err := url.Parse(c.base + "/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("start", fmt.Sprintf("%d", start.Unix()))
	q.Set("end", fmt.Sprintf("%d", end.Unix()))
	q.Set("step", fmt.Sprintf("%ds", int(step.Seconds())))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query_range status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var parsed queryRangeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode query_range: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("query_range: %s", parsed.Error)
	}

	out := make([]Series, 0, len(parsed.Data.Result))
	for _, r := range parsed.Data.Result {
		s := Series{Metric: r.Metric, Points: make([]Sample, 0, len(r.Values))}
		for _, pair := range r.Values {
			if len(pair) < 2 {
				continue
			}
			ts, ok := asInt64(pair[0])
			if !ok {
				continue
			}
			s.Points = append(s.Points, Sample{T: ts, V: asFloatPtr(pair[1])})
		}
		out = append(out, s)
	}
	return out, nil
}

// LabelValues returns distinct values for label (e.g. "interface"), optionally
// filtered by Prometheus-style match[] selectors.
func (c *Client) LabelValues(ctx context.Context, label string, matches ...string) ([]string, error) {
	if label == "" {
		return nil, fmt.Errorf("label is required")
	}
	u, err := url.Parse(c.base + "/api/v1/label/" + url.PathEscape(label) + "/values")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	for _, m := range matches {
		if m != "" {
			q.Add("match[]", m)
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("label values status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var parsed struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
		Error  string   `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode label values: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("label values: %s", parsed.Error)
	}
	return parsed.Data, nil
}

// EscapeLabel escapes a value for use inside a MetricsQL double-quoted label matcher.
func EscapeLabel(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// Selector returns {device="…",interface="…"} for contract-series matchers.
func Selector(device, iface string) string {
	return fmt.Sprintf(`{device="%s",interface="%s"}`, EscapeLabel(device), EscapeLabel(iface))
}

func asInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case json.Number:
		i, err := t.Int64()
		return i, err == nil
	case string:
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return 0, false
		}
		return int64(f), true
	default:
		return 0, false
	}
}

func asFloatPtr(v any) *float64 {
	var f float64
	switch t := v.(type) {
	case float64:
		f = t
	case json.Number:
		x, err := t.Float64()
		if err != nil {
			return nil
		}
		f = x
	case string:
		if t == "NaN" || t == "Inf" || t == "+Inf" || t == "-Inf" {
			return nil
		}
		x, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil
		}
		f = x
	default:
		return nil
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil
	}
	return &f
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
