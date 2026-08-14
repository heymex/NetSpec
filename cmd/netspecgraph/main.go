// Package main is the NetSpecGraph companion binary (metrics UI + query API).
//
// It reuses NetSpec's config/rules/ifname/auth packages and never evaluates
// desired state or sends alerts — see upcoming_development/NetSpecGraph/NetSpecGraph-DevSpec.md.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/netspec/netspec/internal/auth"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/graph"
	"github.com/netspec/netspec/internal/graph/vm"
	"github.com/netspec/netspec/internal/version"
	"github.com/rs/zerolog"
)

func main() {
	listenAddr := flag.String("listen", envOr("GRAPH_LISTEN_ADDR", ":8090"), "UI/API listen address")
	vmURL := flag.String("vm-url", envOr("GRAPH_VM_URL", "http://netspec-victoriametrics:8428"), "VictoriaMetrics base URL")
	configDir := flag.String("config-dir", envOr("GRAPH_CONFIG_DIR", "/config"), "NetSpec config directory (read-only)")
	timezone := flag.String("timezone", envOr("GRAPH_TIMEZONE", "America/Chicago"), "IANA timezone for seasonality buckets")
	bandWindowStr := flag.String("band-window", envOr("GRAPH_BAND_WINDOW", "504h"), "trailing window for hour-of-week band (default 21d)")
	netspecPublic := flag.String("netspec-public-url", envOr("NETSPEC_PUBLIC_URL", ""), "browser-reachable NetSpec origin for deep-links (optional)")
	logLevel := flag.String("log-level", envOr("LOG_LEVEL", "info"), "Log level")
	flag.Parse()

	level, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger := zerolog.New(os.Stdout).With().
		Timestamp().
		Str("component", "netspecgraph").
		Str("version", version.GetVersion()).
		Logger().Level(level)

	loc, err := time.LoadLocation(*timezone)
	if err != nil {
		logger.Fatal().Err(err).Str("timezone", *timezone).Msg("invalid GRAPH_TIMEZONE")
	}
	bandWindow, err := time.ParseDuration(*bandWindowStr)
	if err != nil || bandWindow <= 0 {
		logger.Fatal().Err(err).Str("band_window", *bandWindowStr).Msg("invalid GRAPH_BAND_WINDOW")
	}

	cfg, err := config.LoadConfigDir(*configDir)
	if err != nil {
		logger.Fatal().Err(err).Str("config_dir", *configDir).Msg("failed to load NetSpec config")
	}
	idx := graph.BuildIndex(cfg)
	logger.Info().
		Int("devices", idx.DeviceCount()).
		Int("interfaces", idx.Len()).
		Int("port_roles", len(idx.PortRoleLabels())).
		Str("timezone", *timezone).
		Dur("band_window", bandWindow).
		Msg("loaded NetSpec config (read-only identity source)")

	authMgr := auth.NewManager(
		os.Getenv("NETSPEC_ADMIN_PASSWORD_HASH"),
		os.Getenv("NETSPEC_API_TOKEN"),
	)
	if authMgr.Enabled() {
		logger.Info().Msg("authentication enabled (shared NetSpec session/token)")
	} else {
		logger.Warn().Msg("authentication disabled (NETSPEC_ADMIN_PASSWORD_HASH unset)")
	}

	vmClient := vm.NewClient(*vmURL)
	srv := graph.NewServer(graph.Options{
		Logger:           logger,
		Auth:             authMgr,
		VM:               vmClient,
		Config:           cfg,
		Timezone:         *timezone,
		Location:         loc,
		BandWindow:       bandWindow,
		NetSpecPublicURL: *netspecPublic,
	})
	if *netspecPublic != "" {
		logger.Info().Str("netspec_public_url", *netspecPublic).Msg("NetSpec deep-links enabled")
	}

	httpServer := &http.Server{
		Addr:              *listenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info().Str("addr", *listenAddr).Str("vm", *vmURL).Msg("netspecgraph listening")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("listen failed")
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error().Err(err).Msg("shutdown error")
	}
	logger.Info().Msg("netspecgraph stopped")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
