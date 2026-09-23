// Package influx writes NetSpecGraph samples as Influx line protocol to
// VictoriaMetrics (/write, -influxSkipSingleField).
package influx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netspec/netspec/internal/mdt/graphmap"
	"github.com/rs/zerolog"
)

const (
	defaultFlush       = 2 * time.Second
	defaultMaxBatch    = 5000
	defaultHTTPTimeout = 5 * time.Second
)

// Config is the optional VM write path. Empty URL disables the client.
type Config struct {
	URL        string
	Database   string
	FlushEvery time.Duration
	MaxBatch   int
}

// Stats is a point-in-time copy of VM forward counters.
type Stats struct {
	Enabled         bool   `json:"enabled"`
	URL             string `json:"url,omitempty"`
	Database        string `json:"database,omitempty"`
	Samples         uint64 `json:"samples"`
	WriteOK         uint64 `json:"write_ok"`
	WriteFailed     uint64 `json:"write_failed"`
	SkippedCopper   uint64 `json:"skipped_copper"`
	SkippedUnmapped uint64 `json:"skipped_unmapped"`
	SkippedNoTags   uint64 `json:"skipped_no_tags"`
}

// Client batches line protocol and POSTs to VictoriaMetrics.
type Client struct {
	writeURL   string
	publicURL  string
	database   string
	flushEvery time.Duration
	maxBatch   int
	http       *http.Client
	log        zerolog.Logger

	mu  sync.Mutex
	buf bytes.Buffer
	n   int

	samples         atomic.Uint64
	writeOK         atomic.Uint64
	writeFail       atomic.Uint64
	skippedCopper   atomic.Uint64
	skippedUnmapped atomic.Uint64
	skippedNoTags   atomic.Uint64

	stop chan struct{}
	done chan struct{}
}

// New returns nil when URL is empty (main compose leaves VM unset).
func New(cfg Config, log zerolog.Logger) *Client {
	u := strings.TrimSpace(cfg.URL)
	if u == "" {
		return nil
	}
	db := strings.TrimSpace(cfg.Database)
	if db == "" {
		db = "netspecgraph"
	}
	flush := cfg.FlushEvery
	if flush <= 0 {
		flush = defaultFlush
	}
	maxBatch := cfg.MaxBatch
	if maxBatch <= 0 {
		maxBatch = defaultMaxBatch
	}
	c := &Client{
		writeURL:   writeEndpoint(u, db),
		publicURL:  strings.TrimRight(u, "/"),
		database:   db,
		flushEvery: flush,
		maxBatch:   maxBatch,
		http:       &http.Client{Timeout: defaultHTTPTimeout},
		log:        log,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go c.loop()
	return c
}

func writeEndpoint(base, db string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	return base + "/write?db=" + url.QueryEscape(db)
}

func (c *Client) Enabled() bool { return c != nil }

func (c *Client) Stats() Stats {
	if c == nil {
		return Stats{}
	}
	return Stats{
		Enabled:         true,
		URL:             c.publicURL,
		Database:        c.database,
		Samples:         c.samples.Load(),
		WriteOK:         c.writeOK.Load(),
		WriteFailed:     c.writeFail.Load(),
		SkippedCopper:   c.skippedCopper.Load(),
		SkippedUnmapped: c.skippedUnmapped.Load(),
		SkippedNoTags:   c.skippedNoTags.Load(),
	}
}

// ObserveMap records mapper skip counters (records, not samples).
func (c *Client) ObserveMap(res graphmap.Result) {
	if c == nil {
		return
	}
	c.skippedCopper.Add(res.SkippedCopper)
	c.skippedUnmapped.Add(res.SkippedUnmapped)
	c.skippedNoTags.Add(res.SkippedNoTags)
}

func (c *Client) Write(samples []graphmap.Sample) {
	if c == nil || len(samples) == 0 {
		return
	}
	c.samples.Add(uint64(len(samples)))
	c.mu.Lock()
	for i := range samples {
		writeLine(&c.buf, samples[i])
		c.n++
		if c.n >= c.maxBatch {
			payload, n := c.takeLocked()
			c.mu.Unlock()
			c.send(payload, n)
			c.mu.Lock()
		}
	}
	c.mu.Unlock()
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	close(c.stop)
	<-c.done
}

func (c *Client) loop() {
	t := time.NewTicker(c.flushEvery)
	defer t.Stop()
	defer close(c.done)
	for {
		select {
		case <-c.stop:
			c.mu.Lock()
			payload, n := c.takeLocked()
			c.mu.Unlock()
			c.send(payload, n)
			return
		case <-t.C:
			c.mu.Lock()
			payload, n := c.takeLocked()
			c.mu.Unlock()
			c.send(payload, n)
		}
	}
}

func (c *Client) takeLocked() ([]byte, int) {
	if c.n == 0 {
		return nil, 0
	}
	payload := append([]byte(nil), c.buf.Bytes()...)
	n := c.n
	c.buf.Reset()
	c.n = 0
	return payload, n
}

func (c *Client) send(payload []byte, n int) {
	if n == 0 {
		return
	}
	if err := c.post(payload); err != nil {
		c.writeFail.Add(1)
		c.log.Warn().Err(err).Int("lines", n).Msg("VictoriaMetrics Influx write failed")
		return
	}
	c.writeOK.Add(1)
}

func (c *Client) post(payload []byte) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.writeURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("vm write status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func writeLine(buf *bytes.Buffer, s graphmap.Sample) {
	buf.WriteString(s.Name)
	buf.WriteString(",device=")
	buf.WriteString(escapeTag(s.Device))
	buf.WriteString(",interface=")
	buf.WriteString(escapeTag(s.Interface))
	buf.WriteString(" value=")
	buf.WriteString(strconvValue(s.Value))
	if ns := lpNanos(s.Timestamp); ns > 0 {
		buf.WriteByte(' ')
		fmt.Fprintf(buf, "%d", ns)
	}
	buf.WriteByte('\n')
}

func strconvValue(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}

func escapeTag(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `,`, `\,`)
	s = strings.ReplaceAll(s, ` `, `\ `)
	s = strings.ReplaceAll(s, `=`, `\=`)
	return s
}

func lpNanos(ts uint64) int64 {
	switch {
	case ts == 0:
		return 0
	case ts < 1e12:
		return int64(ts) * 1e9
	case ts < 1e15:
		return int64(ts) * 1e6
	default:
		return int64(ts)
	}
}
