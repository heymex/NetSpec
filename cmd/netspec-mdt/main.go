package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/mdt/decoder"
	"github.com/netspec/netspec/internal/mdt/egress"
	"github.com/netspec/netspec/internal/mdt/egress/influx"
	"github.com/netspec/netspec/internal/mdt/graphmap"
	"github.com/netspec/netspec/internal/mdt/metrics"
	"github.com/netspec/netspec/internal/mdt/receiver"
	"github.com/netspec/netspec/internal/mdt/transformer"
	"github.com/netspec/netspec/internal/version"
	"github.com/rs/zerolog"
)

func main() {
	listenAddr := flag.String("listen-addr", envOr("NETSPEC_MDT_LISTEN_ADDR", "0.0.0.0:57500"), "gRPC MDT dial-out listen address")
	ingestHost := flag.String("ingest-host", envOr("NETSPEC_INGEST_HOST", "127.0.0.1"), "NetSpec NDJSON ingest host (if targets unset)")
	ingestPort := flag.Int("ingest-port", envInt("NETSPEC_INGEST_PORT", 57500), "NetSpec NDJSON ingest port (if targets unset)")
	ingestTargets := flag.String("ingest-targets", os.Getenv("NETSPEC_INGEST_TARGETS"), "comma-separated host:port NDJSON sinks")
	ingestTokenEnv := flag.String("ingest-token-env", envOr("NETSPEC_INGEST_TOKEN_ENV", ""), "env var holding optional ingest token")
	allowed := flag.String("allowed-devices", os.Getenv("MDT_ALLOWED_DEVICES"), "comma-separated node_id allowlist")
	resend := flag.Duration("resend-interval", envDuration("MDT_RESEND_INTERVAL", 60*time.Second), "unchanged-state resend interval")
	logLevel := flag.String("log-level", envOr("LOG_LEVEL", "info"), "log level")
	workers := flag.Int("workers", envInt("NETSPEC_MDT_WORKERS", 4), "decode worker count")
	queueSize := flag.Int("queue-size", envInt("NETSPEC_MDT_QUEUE_SIZE", 1024), "inbound job queue size")
	keepaliveMin := flag.Duration("keepalive-min-time", envDuration("NETSPEC_MDT_KEEPALIVE_MIN_TIME", 5*time.Minute), "gRPC keepalive enforcement minimum")
	permitIdlePing := flag.Bool("permit-keepalive-without-calls", envBool("NETSPEC_MDT_PERMIT_KEEPALIVE_WITHOUT_CALLS"), "allow IOS-XE idle pings")
	metricsAddr := flag.String("metrics-addr", envOr("NETSPEC_MDT_METRICS_ADDR", "0.0.0.0:8089"), "HTTP /health /stats /metrics listen address (off/- disables)")
	vmURL := flag.String("vm-url", envOr("MDT_VM_URL", ""), "VictoriaMetrics base URL for Graph Influx LP (empty disables)")
	vmDB := flag.String("vm-database", envOr("MDT_VM_DATABASE", "netspecgraph"), "Influx database query param (ignored by VM; kept for Telegraf parity)")
	flag.Parse()

	lvl, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log := zerolog.New(os.Stdout).Level(lvl).With().
		Timestamp().
		Str("component", "netspec-mdt").
		Str("version", version.GetVersion()).
		Logger()

	targets, err := egress.ParseTargets(*ingestTargets, *ingestHost, *ingestPort)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid ingest targets")
	}
	token := ""
	if strings.TrimSpace(*ingestTokenEnv) != "" {
		token = os.Getenv(strings.TrimSpace(*ingestTokenEnv))
	}

	allow := map[string]struct{}{}
	for _, d := range strings.Split(*allowed, ",") {
		d = strings.TrimSpace(d)
		if d != "" {
			allow[d] = struct{}{}
		}
	}
	var allowSet map[string]struct{}
	if len(allow) > 0 {
		allowSet = allow
	}

	xf := transformer.New(transformer.Config{
		ResendInterval: *resend,
		AllowedDevices: allowSet,
		Source:         transformer.DefaultSource,
	})
	client := egress.NewClient(targets, token, log)
	defer client.Close()

	vm := influx.New(influx.Config{
		URL:      *vmURL,
		Database: *vmDB,
	}, log)
	defer vm.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	onEvent := func(ev collector.PushTelemetryEvent) {
		if err := client.Send(ctx, ev); err != nil {
			log.Warn().Err(err).Str("device", ev.Device).Str("interface", ev.Interface).Msg("Failed to forward telemetry event")
		}
	}

	log.Info().
		Str("listen", *listenAddr).
		Str("metrics", *metricsAddr).
		Interface("ingest_targets", targets).
		Str("vm_url", *vmURL).
		Dur("resend_interval", *resend).
		Msg("Starting MDT dial-out sidecar")

	srv := receiver.New(receiver.Config{
		ListenAddr:                  *listenAddr,
		Workers:                     *workers,
		QueueSize:                   *queueSize,
		KeepaliveMinTime:            *keepaliveMin,
		PermitKeepaliveWithoutCalls: *permitIdlePing,
	}, xf, onEvent, log)
	if vm != nil {
		srv.SetOnRecords(func(recs []*decoder.Record) {
			res := graphmap.Map(recs)
			vm.ObserveMap(res)
			vm.Write(res.Samples)
		})
	}

	started := time.Now()
	if addr := strings.TrimSpace(*metricsAddr); addr != "" && addr != "off" && addr != "-" {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatal().Err(err).Str("addr", addr).Msg("Failed to listen for metrics HTTP")
		}
		httpSrv := metrics.NewServer(addr, func() metrics.Snapshot {
			return metrics.Snapshot{
				Status:   "healthy",
				Time:     time.Now().UTC(),
				Version:  version.GetVersion(),
				Uptime:   time.Since(started).Round(time.Second).String(),
				Receiver: srv.Stats(),
				Forward:  client.Stats(),
				VM:       vm.Stats(),
			}
		}, log)
		go func() {
			if err := httpSrv.Serve(ctx, ln); err != nil {
				log.Error().Err(err).Msg("Metrics HTTP stopped")
			}
		}()
	}

	if err := srv.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("MDT receiver stopped")
	}
	log.Info().Msg("netspec-mdt stopped")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
