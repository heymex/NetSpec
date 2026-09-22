package metrics

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/mdt/egress"
	"github.com/netspec/netspec/internal/mdt/receiver"
	"github.com/netspec/netspec/internal/mdt/transformer"
	"github.com/rs/zerolog"
)

func sample() Snapshot {
	return Snapshot{
		Status:  "healthy",
		Time:    time.Date(2026, 9, 22, 19, 0, 0, 0, time.UTC),
		Version: "dev",
		Uptime:  "1h",
		Receiver: receiver.Stats{
			ListenAddr:      "0.0.0.0:57500",
			QueueLen:        3,
			QueueCap:        1024,
			QueueHighWater:  12,
			QueueBlocked:    2,
			PacketsReceived: 100,
			PacketsEmpty:    4,
			DecodeFailed:    1,
			RecordsUnpacked: 80,
			StreamsActive:   34,
			StreamsOpened:   40,
			StreamsClosed:   6,
			Transformer: transformer.Snapshot{
				Emitted:            50,
				SkippedUnknownPath: 10,
				SkippedNoStatus:    5,
				SkippedAllowlist:   1,
				SkippedDedup:       20,
				TrackedInterfaces:  400,
			},
		},
		Forward: egress.Stats{
			ForwardOK:     49,
			ForwardFailed: 1,
			ForwardConns:  1,
		},
	}
}

func TestStatsAndMetricsHandlers(t *testing.T) {
	t.Parallel()
	s := NewServer("127.0.0.1:0", func() Snapshot { return sample() }, zerolog.Nop())

	rec := httptest.NewRecorder()
	s.handleStats(rec, httptest.NewRequest(http.MethodGet, "/stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status %d", rec.Code)
	}
	var got Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Receiver.PacketsReceived != 100 || got.Forward.ForwardFailed != 1 {
		t.Fatalf("stats: %+v", got)
	}

	rec = httptest.NewRecorder()
	s.handleMetrics(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	for _, want := range []string{
		"netspec_mdt_queue_len 3",
		"netspec_mdt_packets_received_total 100",
		`netspec_mdt_events_skipped_total{reason="dedup"} 20`,
		"netspec_mdt_forward_failed_total 1",
		"netspec_mdt_tracked_interfaces 400",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing %q\n%s", want, text)
		}
	}
	if rec.Header().Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("content-type %q", rec.Header().Get("Content-Type"))
	}
}
