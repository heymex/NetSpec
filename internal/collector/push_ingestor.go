package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
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
}

func NewPushIngestor(listenAddress string, port uint16, authToken string, logger zerolog.Logger, onEvent func(PushTelemetryEvent)) *PushIngestor {
	return &PushIngestor{
		listenAddress: listenAddress,
		port:          port,
		authToken:     authToken,
		logger:        logger,
		onEvent:       onEvent,
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

		var event PushTelemetryEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			i.logger.Warn().Err(err).Str("remote", remote).Msg("Invalid push telemetry JSON payload")
			continue
		}

		if i.authToken != "" && event.Token != i.authToken {
			i.logger.Warn().Str("remote", remote).Msg("Rejected push telemetry payload due to invalid token")
			continue
		}
		if event.Device == "" || event.Interface == "" {
			i.logger.Warn().Str("remote", remote).Msg("Rejected push telemetry payload missing device/interface")
			continue
		}
		if event.OperStatus == "" && event.AdminStatus == "" {
			i.logger.Warn().Str("remote", remote).Msg("Rejected push telemetry payload missing status fields")
			continue
		}

		i.onEvent(event)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		i.logger.Warn().Err(err).Str("remote", remote).Msg("Push telemetry connection read error")
	}

	i.logger.Debug().Str("remote", remote).Time("closed_at", time.Now()).Msg("Push telemetry client disconnected")
}
