// Package vm is a thin VictoriaMetrics / MetricsQL HTTP client.
package vm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
			Timeout: 15 * time.Second,
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

// QueryRange runs a MetricsQL range query. Returns the raw JSON body for now;
// typed decoding lands with the per-interface vertical slice.
func (c *Client) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]byte, error) {
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
	return body, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
