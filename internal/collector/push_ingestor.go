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
}

// PushIngestor receives line-delimited JSON telemetry over TCP.
type PushIngestor struct {
	listenAddress string
	port          uint16
	authToken     string
	logger        zerolog.Logger
	onEvent       func(PushTelemetryEvent)
	statsMu       sync.Mutex
	stats         PushIngestorStats
	eventsBySec   map[int64]uint64
}

type PushIngestorStats struct {
	Received            uint64
	Accepted            uint64
	RejectedInvalidJSON uint64
	RejectedAuth        uint64
	RejectedMissing     uint64
	ByDevice            map[string]uint64
	LastEventAt         time.Time
	EventsPerSecond     float64
}

type DeviceTelemetryStat struct {
	Device string `json:"device"`
	Count  uint64 `json:"count"`
}

func NewPushIngestor(listenAddress string, port uint16, authToken string, logger zerolog.Logger, onEvent func(PushTelemetryEvent)) *PushIngestor {
	return &PushIngestor{
		listenAddress: listenAddress,
		port:          port,
		authToken:     authToken,
		logger:        logger,
		onEvent:       onEvent,
		stats: PushIngestorStats{
			ByDevice: make(map[string]uint64),
		},
		eventsBySec: make(map[int64]uint64),
	}
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
		if age > 120 {
			delete(i.eventsBySec, sec)
		}
	}

	out := PushIngestorStats{
		Received:            i.stats.Received,
		Accepted:            i.stats.Accepted,
		RejectedInvalidJSON: i.stats.RejectedInvalidJSON,
		RejectedAuth:        i.stats.RejectedAuth,
		RejectedMissing:     i.stats.RejectedMissing,
		ByDevice:            make(map[string]uint64, len(i.stats.ByDevice)),
		LastEventAt:         i.stats.LastEventAt,
		EventsPerSecond:     float64(totalRecent) / 10.0,
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
