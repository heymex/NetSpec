package egress

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/mdt/bandwidth"
	"github.com/rs/zerolog"
)

const byteRateWindowSec = 10

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

	sendOK         atomic.Uint64
	sendFail       atomic.Uint64
	bytesForwarded atomic.Uint64

	bwMu       sync.Mutex
	groups     *bandwidth.Registry
	byGroup    map[string]fwGroupCounters
	bytesBySec map[int64]uint64
}

type fwGroupCounters struct {
	Bytes  uint64
	Events uint64
}

// GroupBandwidthStat is NDJSON forward bytes rolled up by operator group.
type GroupBandwidthStat struct {
	Group  string `json:"group"`
	Bytes  uint64 `json:"bytes_forwarded"`
	Events uint64 `json:"events"`
}

// Stats is a point-in-time copy of NDJSON forward counters.
type Stats struct {
	ForwardOK               uint64               `json:"forward_ok"`
	ForwardFailed           uint64               `json:"forward_failed"`
	ForwardConns            int                  `json:"forward_conns"`
	BytesForwarded          uint64               `json:"bytes_forwarded_total"`
	BytesForwardedPerSecond float64              `json:"bytes_forwarded_per_second"`
	ByGroup                 []GroupBandwidthStat `json:"by_group,omitempty"`
	IngestTargets           []string             `json:"ingest_targets,omitempty"`
}

func NewClient(targets []Target, token string, log zerolog.Logger) *Client {
	return &Client{
		targets:    targets,
		token:      token,
		log:        log,
		conns:      map[string]net.Conn{},
		byGroup:    map[string]fwGroupCounters{},
		bytesBySec: map[int64]uint64{},
	}
}

// SetBandwidthGroups installs the device→group registry for forwarded byte
// rollups. Nil treats every device as bandwidth.Ungrouped.
func (c *Client) SetBandwidthGroups(r *bandwidth.Registry) {
	if c == nil {
		return
	}
	c.bwMu.Lock()
	defer c.bwMu.Unlock()
	c.groups = r
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
	byGroup, rate := c.snapshotBandwidth()
	return Stats{
		ForwardOK:               c.sendOK.Load(),
		ForwardFailed:           c.sendFail.Load(),
		ForwardConns:            conns,
		BytesForwarded:          c.bytesForwarded.Load(),
		BytesForwardedPerSecond: rate,
		ByGroup:                 byGroup,
		IngestTargets:           targets,
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
	n := uint64(len(payload))
	c.bytesForwarded.Add(n)
	c.noteForwarded(ev.Device, n)
	return nil
}

func (c *Client) noteForwarded(device string, n uint64) {
	if c == nil || n == 0 {
		return
	}
	now := time.Now().Unix()
	c.bwMu.Lock()
	defer c.bwMu.Unlock()
	c.bytesBySec[now] += n
	group := bandwidth.Ungrouped
	if c.groups != nil {
		group = c.groups.GroupOf(device)
	}
	g := c.byGroup[group]
	g.Bytes += n
	g.Events++
	c.byGroup[group] = g
}

func (c *Client) snapshotBandwidth() ([]GroupBandwidthStat, float64) {
	c.bwMu.Lock()
	defer c.bwMu.Unlock()

	nowSec := time.Now().Unix()
	var recent uint64
	for sec, n := range c.bytesBySec {
		age := nowSec - sec
		if age >= 0 && age < byteRateWindowSec {
			recent += n
		}
		if age > 1200 {
			delete(c.bytesBySec, sec)
		}
	}
	rate := float64(recent) / float64(byteRateWindowSec)

	if c.groups != nil {
		for _, name := range c.groups.Names() {
			if _, ok := c.byGroup[name]; !ok {
				c.byGroup[name] = fwGroupCounters{}
			}
		}
	}
	byGroup := make([]GroupBandwidthStat, 0, len(c.byGroup))
	for group, counters := range c.byGroup {
		byGroup = append(byGroup, GroupBandwidthStat{
			Group:  group,
			Bytes:  counters.Bytes,
			Events: counters.Events,
		})
	}
	sort.Slice(byGroup, func(i, j int) bool {
		if byGroup[i].Bytes != byGroup[j].Bytes {
			return byGroup[i].Bytes > byGroup[j].Bytes
		}
		return byGroup[i].Group < byGroup[j].Group
	})
	return byGroup, rate
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
