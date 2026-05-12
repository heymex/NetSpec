package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// PushTelemetryEvent represents a single normalized interface state update.
type PushTelemetryEvent struct {
	Device      string `json:"device"`
	Interface   string `json:"interface"`
	OperStatus  string `json:"oper_status,omitempty"`
	AdminStatus string `json:"admin_status,omitempty"`
	Token       string `json:"token,omitempty"`
	Source      string `json:"source,omitempty"`
	// RemoteAddr is the TCP peer address (host:port) of the connection that delivered this event.
	// Not part of the wire format — populated by the ingestor, used for address hinting.
	RemoteAddr string `json:"-"`
}

// PushIngestor receives line-delimited JSON telemetry over TCP.
type PushIngestor struct {
	listenAddress string
	port          uint16
	sourceTag     string // when non-empty, sets PushTelemetryEvent.Source (pipeline / "sourcetype")
	authToken     string
	logger        zerolog.Logger
	onEvent       func(PushTelemetryEvent)
	statsMu       sync.Mutex
	stats         PushIngestorStats
	eventsBySec   map[int64]uint64
}

type PushIngestorStats struct {
	Port                uint16            `json:"port,omitempty"`
	Source              string            `json:"source,omitempty"`
	Received            uint64            `json:"received"`
	Accepted            uint64            `json:"accepted"`
	RejectedInvalidJSON uint64            `json:"rejected_invalid_json"`
	RejectedAuth        uint64            `json:"rejected_auth"`
	RejectedMissing     uint64            `json:"rejected_missing"`
	ByDevice            map[string]uint64 `json:"by_device,omitempty"`
	LastEventAt         time.Time         `json:"last_event_at"`
	EventsPerSecond     float64           `json:"events_per_second"`
	RecentPerSecond     []EventRatePoint  `json:"recent_per_second,omitempty"`
}

type EventRatePoint struct {
	Timestamp time.Time `json:"timestamp"`
	Count     uint64    `json:"count"`
}

type DeviceTelemetryStat struct {
	Device string `json:"device"`
	Count  uint64 `json:"count"`
}

func NewPushIngestor(listenAddress string, port uint16, sourceTag string, authToken string, logger zerolog.Logger, onEvent func(PushTelemetryEvent)) *PushIngestor {
	return &PushIngestor{
		listenAddress: listenAddress,
		port:          port,
		sourceTag:     strings.TrimSpace(sourceTag),
		authToken:     authToken,
		logger:        logger,
		onEvent:       onEvent,
		stats: PushIngestorStats{
			ByDevice: make(map[string]uint64),
		},
		eventsBySec: make(map[int64]uint64),
	}
}

// AggregatePushIngestorStats merges counters across listeners for dashboard totals.
func AggregatePushIngestorStats(parts []PushIngestorStats) PushIngestorStats {
	if len(parts) == 0 {
		return PushIngestorStats{}
	}
	out := PushIngestorStats{
		ByDevice: make(map[string]uint64),
	}
	seriesBySec := make(map[int64]uint64)
	for _, p := range parts {
		out.Received += p.Received
		out.Accepted += p.Accepted
		out.RejectedInvalidJSON += p.RejectedInvalidJSON
		out.RejectedAuth += p.RejectedAuth
		out.RejectedMissing += p.RejectedMissing
		out.EventsPerSecond += p.EventsPerSecond
		if p.LastEventAt.After(out.LastEventAt) {
			out.LastEventAt = p.LastEventAt
		}
		for d, n := range p.ByDevice {
			out.ByDevice[d] += n
		}
		for _, pt := range p.RecentPerSecond {
			sec := pt.Timestamp.Unix()
			seriesBySec[sec] += pt.Count
		}
	}
	if len(seriesBySec) > 0 {
		secs := make([]int64, 0, len(seriesBySec))
		for sec := range seriesBySec {
			secs = append(secs, sec)
		}
		sort.Slice(secs, func(i, j int) bool { return secs[i] < secs[j] })
		out.RecentPerSecond = make([]EventRatePoint, 0, len(secs))
		for _, sec := range secs {
			out.RecentPerSecond = append(out.RecentPerSecond, EventRatePoint{
				Timestamp: time.Unix(sec, 0).UTC(),
				Count:     seriesBySec[sec],
			})
		}
	}
	return out
}

func (i *PushIngestor) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", i.listenAddress, i.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("start push ingestor listener: %w", err)
	}
	defer ln.Close()

	i.logger.Info().Str("address", addr).Msg("Push ingestor listening for telemetry")

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				i.logger.Warn().Err(err).Msg("Failed to accept push telemetry connection")
				continue
			}
		}
		go i.handleConn(ctx, conn)
	}
}

func (i *PushIngestor) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	i.logger.Debug().Str("remote", remote).Msg("Push telemetry client connected")

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		i.incReceived()

		var event PushTelemetryEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			i.incRejectedInvalidJSON()
			i.logger.Warn().Err(err).Str("remote", remote).Msg("Invalid push telemetry JSON payload")
			continue
		}

		if i.authToken != "" && event.Token != i.authToken {
			i.incRejectedAuth()
			i.logger.Warn().Str("remote", remote).Msg("Rejected push telemetry payload due to invalid token")
			continue
		}
		if event.Device == "" || event.Interface == "" {
			i.incRejectedMissing()
			i.logger.Warn().Str("remote", remote).Msg("Rejected push telemetry payload missing device/interface")
			continue
		}
		if event.OperStatus == "" && event.AdminStatus == "" {
			i.incRejectedMissing()
			i.logger.Warn().Str("remote", remote).Msg("Rejected push telemetry payload missing status fields")
			continue
		}
		if i.sourceTag != "" {
			event.Source = i.sourceTag
		}
		event.RemoteAddr = remote
		i.incAccepted(event.Device)
		i.onEvent(event)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		i.logger.Warn().Err(err).Str("remote", remote).Msg("Push telemetry connection read error")
	}

	i.logger.Debug().Str("remote", remote).Time("closed_at", time.Now()).Msg("Push telemetry client disconnected")
}

func (i *PushIngestor) incReceived() {
	i.statsMu.Lock()
	defer i.statsMu.Unlock()
	i.stats.Received++
}

func (i *PushIngestor) incRejectedInvalidJSON() {
	i.statsMu.Lock()
	defer i.statsMu.Unlock()
	i.stats.RejectedInvalidJSON++
}

func (i *PushIngestor) incRejectedAuth() {
	i.statsMu.Lock()
	defer i.statsMu.Unlock()
	i.stats.RejectedAuth++
}

func (i *PushIngestor) incRejectedMissing() {
	i.statsMu.Lock()
	defer i.statsMu.Unlock()
	i.stats.RejectedMissing++
}

func (i *PushIngestor) incAccepted(device string) {
	i.statsMu.Lock()
	defer i.statsMu.Unlock()
	i.stats.Accepted++
	now := time.Now()
	i.stats.LastEventAt = now
	i.stats.ByDevice[device]++
	i.eventsBySec[now.Unix()]++
}

func (i *PushIngestor) Stats() PushIngestorStats {
	i.statsMu.Lock()
	defer i.statsMu.Unlock()

	nowSec := time.Now().Unix()
	var totalRecent uint64
	for sec, count := range i.eventsBySec {
		age := nowSec - sec
		if age >= 0 && age < 10 {
			totalRecent += count
		}
		if age > 1200 {
			delete(i.eventsBySec, sec)
		}
	}

	series := make([]EventRatePoint, 0, 600)
	startSec := nowSec - 599
	for sec := startSec; sec <= nowSec; sec++ {
		series = append(series, EventRatePoint{
			Timestamp: time.Unix(sec, 0).UTC(),
			Count:     i.eventsBySec[sec],
		})
	}

	out := PushIngestorStats{
		Port:                i.port,
		Source:              i.sourceTag,
		Received:            i.stats.Received,
		Accepted:            i.stats.Accepted,
		RejectedInvalidJSON: i.stats.RejectedInvalidJSON,
		RejectedAuth:        i.stats.RejectedAuth,
		RejectedMissing:     i.stats.RejectedMissing,
		ByDevice:            make(map[string]uint64, len(i.stats.ByDevice)),
		LastEventAt:         i.stats.LastEventAt,
		EventsPerSecond:     float64(totalRecent) / 10.0,
		RecentPerSecond:     series,
	}
	for k, v := range i.stats.ByDevice {
		out.ByDevice[k] = v
	}
	return out
}

func TopDeviceStats(byDevice map[string]uint64, limit int) []DeviceTelemetryStat {
	out := make([]DeviceTelemetryStat, 0, len(byDevice))
	for k, v := range byDevice {
		out = append(out, DeviceTelemetryStat{Device: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Device < out[j].Device
		}
		return out[i].Count > out[j].Count
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
