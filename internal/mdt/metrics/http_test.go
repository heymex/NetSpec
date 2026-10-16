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
	"github.com/netspec/netspec/internal/mdt/egress/influx"
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
			ListenAddr:             "0.0.0.0:57500",
			QueueLen:               3,
			QueueCap:               1024,
			QueueHighWater:         12,
			QueueBlocked:           2,
			PacketsReceived:        100,
			PacketsEmpty:           4,
			DecodeFailed:           1,
			RecordsUnpacked:        80,
			StreamsActive:          34,
			StreamsOpened:          40,
			StreamsClosed:          6,
			BytesReceived:          1_024_000,
			BytesReceivedPerSecond: 1200.5,
			ByDevice: []receiver.DeviceBandwidthStat{
				{Device: "csw-01", Bytes: 900_000, Packets: 80},
			},
			ByGroup: []receiver.GroupBandwidthStat{
				{Group: "campus_core", Bytes: 900_000, Packets: 80},
				{Group: "ungrouped", Bytes: 124_000, Packets: 20},
			},
			ByKind: []receiver.PathStat{
				{Kind: "interface", Count: 70},
				{Kind: "optics", Count: 10},
			},
			ByEncodingPath: []receiver.PathStat{
				{Kind: "interface", EncodingPath: "openconfig-interfaces:interfaces/interface", Count: 70},
			},
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
			ForwardOK:               49,
			ForwardFailed:           1,
			ForwardConns:            1,
			BytesForwarded:          12_345,
			BytesForwardedPerSecond: 45.0,
			ByGroup: []egress.GroupBandwidthStat{
				{Group: "campus_core", Bytes: 10_000, Events: 40},
				{Group: "ungrouped", Bytes: 2_345, Events: 9},
			},
		},
		VM: influx.Stats{
			Enabled:         true,
			Samples:         200,
			WriteOK:         4,
			WriteFailed:     1,
			SkippedCopper:   8,
			SkippedUnmapped: 3,
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
		"netspec_mdt_bytes_received_total 1024000",
		"netspec_mdt_bytes_received_per_second 1200.5",
		`netspec_mdt_bytes_received_by_group_total{group="campus_core"} 900000`,
		`netspec_mdt_events_skipped_total{reason="dedup"} 20`,
		"netspec_mdt_forward_failed_total 1",
		"netspec_mdt_bytes_forwarded_total 12345",
		"netspec_mdt_bytes_forwarded_per_second 45",
		`netspec_mdt_bytes_forwarded_by_group_total{group="campus_core"} 10000`,
		"netspec_mdt_tracked_interfaces 400",
		`netspec_mdt_records_by_kind_total{kind="interface"} 70`,
		`netspec_mdt_records_by_kind_total{kind="optics"} 10`,
		"netspec_mdt_vm_samples_total 200",
		`netspec_mdt_vm_skipped_total{reason="copper"} 8`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing %q\n%s", want, text)
		}
	}
	if rec.Header().Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("content-type %q", rec.Header().Get("Content-Type"))
	}
}
