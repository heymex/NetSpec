package egress

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netspec/netspec/internal/collector"
	"github.com/rs/zerolog"
)

// Target is a NetSpec NDJSON ingest host:port.
type Target struct {
	Host string
	Port int
}

func (t Target) addr() string {
	return net.JoinHostPort(t.Host, fmt.Sprintf("%d", t.Port))
}

// Client writes PushTelemetryEvent as newline-delimited JSON, matching
// collector.PushIngestor and the Python mdt-translator contract.
type Client struct {
	targets []Target
	token   string
	log     zerolog.Logger

	mu    sync.Mutex
	conns map[string]net.Conn

	sendOK   atomic.Uint64
	sendFail atomic.Uint64
}

// Stats is a point-in-time copy of NDJSON forward counters.
type Stats struct {
	ForwardOK     uint64   `json:"forward_ok"`
	ForwardFailed uint64   `json:"forward_failed"`
	ForwardConns  int      `json:"forward_conns"`
	IngestTargets []string `json:"ingest_targets,omitempty"`
}

func NewClient(targets []Target, token string, log zerolog.Logger) *Client {
	return &Client{
		targets: targets,
		token:   token,
		log:     log,
		conns:   map[string]net.Conn{},
	}
}

func ParseTargets(csv string, fallbackHost string, fallbackPort int) ([]Target, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		if strings.TrimSpace(fallbackHost) == "" || fallbackPort <= 0 {
			return nil, fmt.Errorf("no ingest targets")
		}
		return []Target{{Host: fallbackHost, Port: fallbackPort}}, nil
	}
	var out []Target
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(csv, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		host, portS, err := net.SplitHostPort(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid target %q: %w", entry, err)
		}
		var port int
		if _, err := fmt.Sscanf(portS, "%d", &port); err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("invalid target port in %q", entry)
		}
		if strings.TrimSpace(host) == "" {
			return nil, fmt.Errorf("invalid target %q: empty host", entry)
		}
		key := net.JoinHostPort(host, portS)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Target{Host: host, Port: port})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no ingest targets")
	}
	return out, nil
}

func (c *Client) Stats() Stats {
	if c == nil {
		return Stats{}
	}
	c.mu.Lock()
	conns := len(c.conns)
	c.mu.Unlock()
	targets := make([]string, 0, len(c.targets))
	for _, t := range c.targets {
		targets = append(targets, t.addr())
	}
	return Stats{
		ForwardOK:     c.sendOK.Load(),
		ForwardFailed: c.sendFail.Load(),
		ForwardConns:  conns,
		IngestTargets: targets,
	}
}

func (c *Client) Send(ctx context.Context, ev collector.PushTelemetryEvent) error {
	if c.token != "" {
		ev.Token = c.token
	}
	line, err := json.Marshal(ev)
	if err != nil {
		c.sendFail.Add(1)
		return err
	}
	payload := append(line, '\n')
	var last error
	for _, t := range c.targets {
		if err := c.sendOne(ctx, t, payload); err != nil {
			last = err
			c.log.Warn().Err(err).Str("target", t.addr()).Msg("NDJSON ingest send failed")
		}
	}
	if last != nil {
		c.sendFail.Add(1)
		return last
	}
	c.sendOK.Add(1)
	return nil
}

func (c *Client) sendOne(ctx context.Context, t Target, payload []byte) error {
	addr := t.addr()
	c.mu.Lock()
	conn := c.conns[addr]
	c.mu.Unlock()

	if conn == nil {
		var err error
		conn, err = dial(ctx, addr)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.conns[addr] = conn
		c.mu.Unlock()
	}
	if err := writeAll(conn, payload); err != nil {
		_ = conn.Close()
		c.mu.Lock()
		delete(c.conns, addr)
		c.mu.Unlock()
		conn, err = dial(ctx, addr)
		if err != nil {
			return err
		}
		c.mu.Lock()
		c.conns[addr] = conn
		c.mu.Unlock()
		return writeAll(conn, payload)
	}
	return nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for addr, conn := range c.conns {
		_ = conn.Close()
		delete(c.conns, addr)
	}
}

func dial(ctx context.Context, addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func writeAll(conn net.Conn, payload []byte) error {
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(payload)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}
