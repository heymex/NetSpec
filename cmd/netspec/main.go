package main

import (
	"bufio"
	"context"
	"flag"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/netspec/netspec/internal/api"
	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/evaluator"
	"github.com/netspec/netspec/internal/notifier"
	"github.com/netspec/netspec/internal/version"
	"github.com/netspec/netspec/internal/webui"
	"github.com/rs/zerolog"
)

func main() {
	configPath := flag.String("config", "/config/desired-state.yaml", "Path to desired state configuration")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	flag.Parse()

	// Create log buffer for web UI (captures last 1000 log entries)
	logBuffer := webui.NewLogBuffer(1000)

	// Setup logger with multi-writer (stdout + log buffer)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logLevelParsed, err := zerolog.ParseLevel(*logLevel)
	if err != nil {
		logLevelParsed = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(logLevelParsed)

	// Write to both stdout and the log buffer
	multiWriter := io.MultiWriter(os.Stdout, logBuffer)
	logger := zerolog.New(multiWriter).With().
		Timestamp().
		Str("version", version.GetVersion()).
		Str("commit", version.GetCommit()).
		Logger()

	logger.Info().Msg("Starting NetSpec")

	// Resolve config directory
	configDir := filepath.Dir(*configPath)
	loadDotEnvIfPresent(configDir)

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Fatal().
			Err(err).
			Str("config_path", *configPath).
			Msg("Failed to load configuration")
	}

	logger.Info().
		Int("device_count", cfg.TotalDeviceCount()).
		Int("monolithic_device_count", cfg.MonolithicDeviceCount).
		Int("split_device_count", cfg.SplitDeviceCount).
		Msg("Configuration loaded")

	// Create notifier (channel URLs come from env vars named in desired-state alerts.channels.*.url_env)
	notifier := notifier.NewNotifier(logger, cfg.Alerts.Channels)

	// Create alert engine
	alertEngine := alerter.NewEngine(cfg, notifier, logger)

	// Start alert engine
	go alertEngine.Run()

	// Create evaluator
	eval := evaluator.NewEvaluator(cfg, logger)

	var pushIngestor *collector.PushIngestor
	var unknownTelemetryMu sync.Mutex
	unknownTelemetryCount := map[string]uint64{}
	unknownTelemetryLast := map[string]time.Time{}
	unknownTelemetryAddress := map[string]string{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snmpCommunity := os.Getenv(cfg.DesiredState.Global.SNMP.CommunityEnv)
	if (cfg.DesiredState.Global.TelemetryMode == "snmp_validate_only" ||
		cfg.DesiredState.Global.TelemetryMode == "telemetry_ingest_push") && snmpCommunity == "" {
		logger.Fatal().
			Str("env_var", cfg.DesiredState.Global.SNMP.CommunityEnv).
			Msg("SNMP mode requires community environment variable")
	}

	switch strings.ToLower(cfg.DesiredState.Global.TelemetryMode) {
	case "snmp_validate_only":
		logger.Info().
			Int("device_count", len(cfg.DesiredState.Devices)).
			Msg("Starting SNMP validation loops for devices")
		validator := collector.NewSNMPValidator(
			cfg.DesiredState.Global.SNMP,
			snmpCommunity,
			logger.With().Str("component", "snmp-validator").Logger(),
		)
		for deviceName, deviceCfg := range cfg.DesiredState.Devices {
			go func(name string, dc config.DeviceConfig) {
				ticker := time.NewTicker(cfg.DesiredState.Global.SNMP.ValidationInterval)
				defer ticker.Stop()

				for {
					snapshots, err := validator.PollDevice(name, dc)
					if err != nil {
						logger.Warn().Err(err).Str("device", name).Msg("SNMP validation poll failed")
					} else {
						for _, snap := range snapshots {
							changes := eval.EvaluateInterfaceSnapshotWithSource(name, snap.Interface, snap.OperStatus, snap.AdminStatus, "snmp")
							for _, change := range changes {
								alertEngine.ProcessStateChange(change)
							}
						}
					}

					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
			}(deviceName, deviceCfg)
		}
	case "telemetry_ingest_push":
		logger.Info().
			Str("listen_address", cfg.DesiredState.Global.Ingest.ListenAddress).
			Uint16("listen_port", cfg.DesiredState.Global.Ingest.Port).
			Msg("Starting push telemetry ingest listener")

		validator := collector.NewSNMPValidator(
			cfg.DesiredState.Global.SNMP,
			snmpCommunity,
			logger.With().Str("component", "snmp-validator").Logger(),
		)

		ingestToken := ""
		if cfg.DesiredState.Global.Ingest.TokenEnv != "" {
			ingestToken = os.Getenv(cfg.DesiredState.Global.Ingest.TokenEnv)
		}

		ingestor := collector.NewPushIngestor(
			cfg.DesiredState.Global.Ingest.ListenAddress,
			cfg.DesiredState.Global.Ingest.Port,
			ingestToken,
			logger.With().Str("component", "push-ingestor").Logger(),
			func(event collector.PushTelemetryEvent) {
				deviceName, deviceCfg, ok := resolveDeviceForEvent(cfg, event)
				if !ok {
					unknownTelemetryMu.Lock()
					unknownTelemetryCount[event.Device]++
					unknownTelemetryLast[event.Device] = time.Now()
					if addr := eventAddressHint(event); addr != "" {
						unknownTelemetryAddress[event.Device] = addr
					}
					unknownTelemetryMu.Unlock()
					logger.Warn().Str("device", event.Device).Msg("Ignoring push telemetry for unknown device")
					return
				}
				unknownTelemetryMu.Lock()
				delete(unknownTelemetryCount, event.Device)
				delete(unknownTelemetryLast, event.Device)
				delete(unknownTelemetryAddress, event.Device)
				delete(unknownTelemetryCount, deviceName)
				delete(unknownTelemetryLast, deviceName)
				delete(unknownTelemetryAddress, deviceName)
				if addr := strings.TrimSpace(deviceCfg.Address); addr != "" {
					delete(unknownTelemetryCount, addr)
					delete(unknownTelemetryLast, addr)
					delete(unknownTelemetryAddress, addr)
				}
				unknownTelemetryMu.Unlock()

				matchedIface, ifaceCfg, ok := resolveInterfaceConfig(deviceCfg.Interfaces, event.Interface)
				if !ok {
					logger.Debug().
						Str("device", deviceName).
						Str("interface", event.Interface).
						Msg("Ignoring push telemetry for untracked interface")
					return
				}

				snapshot, err := validator.PollInterface(deviceName, deviceCfg, matchedIface, ifaceCfg)
				if err != nil {
					logger.Warn().
						Err(err).
						Str("device", deviceName).
						Str("interface", matchedIface).
						Msg("SNMP validation failed for push event; using ingested status")
					snapshot = collector.InterfaceSnapshot{
						Interface:   matchedIface,
						OperStatus:  event.OperStatus,
						AdminStatus: event.AdminStatus,
					}
				}

				validationSource := "snmp"
				if err != nil {
					validationSource = "telemetry"
				}
				changes := eval.EvaluateInterfaceSnapshotWithSource(deviceName, snapshot.Interface, snapshot.OperStatus, snapshot.AdminStatus, validationSource)
				for _, change := range changes {
					alertEngine.ProcessStateChange(change)
				}
			},
		)
		pushIngestor = ingestor

		go func() {
			if err := ingestor.Start(ctx); err != nil {
				logger.Error().Err(err).Msg("Push telemetry ingestor stopped")
				cancel()
			}
		}()
	default:
		logger.Fatal().
			Str("telemetry_mode", cfg.DesiredState.Global.TelemetryMode).
			Msg("Unsupported telemetry mode")
	}

	// Start API server with Web UI
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "8088"
	}
	apiServer := api.NewServer(alertEngine, logger, apiPort)

	// Configure the API server with log buffer, config, version, and collector getter
	apiServer.SetLogBuffer(logBuffer)
	apiServer.SetConfig(cfg, *configPath)
	apiServer.SetVersion(version.GetVersion(), version.GetCommit(), version.GetBuildDate())
	apiServer.SetEvaluatorGetter(func() *evaluator.Evaluator {
		return eval
	})
	apiServer.SetTelemetryStatsGetter(func() api.TelemetryStats {
		stats := api.TelemetryStats{}
		if pushIngestor != nil {
			s := pushIngestor.Stats()
			stats.Received = s.Received
			stats.Accepted = s.Accepted
			stats.RejectedInvalidJSON = s.RejectedInvalidJSON
			stats.RejectedAuth = s.RejectedAuth
			stats.RejectedMissing = s.RejectedMissing
			stats.LastEventAt = s.LastEventAt
			stats.EventsPerSecond = s.EventsPerSecond
			stats.TopDevices = collector.TopDeviceStats(s.ByDevice, 10)
		}

		unknownTelemetryMu.Lock()
		unknown := make([]api.UnknownTelemetryDevice, 0, len(unknownTelemetryCount))
		for dev, cnt := range unknownTelemetryCount {
			wizardURL := "/wizard?device_key=" + url.QueryEscape(dev)
			addr := strings.TrimSpace(unknownTelemetryAddress[dev])
			if addr == "" && net.ParseIP(dev) != nil {
				addr = dev
			}
			if addr != "" {
				wizardURL = "/wizard?address=" + url.QueryEscape(addr) + "&device_key=" + url.QueryEscape(dev)
			}
			unknown = append(unknown, api.UnknownTelemetryDevice{
				Device:     dev,
				Count:      cnt,
				LastSeenAt: unknownTelemetryLast[dev],
				WizardURL:  wizardURL,
			})
		}
		unknownTelemetryMu.Unlock()
		sort.Slice(unknown, func(i, j int) bool {
			if unknown[i].Count == unknown[j].Count {
				return unknown[i].Device < unknown[j].Device
			}
			return unknown[i].Count > unknown[j].Count
		})
		if len(unknown) > 10 {
			unknown = unknown[:10]
		}
		stats.UnknownDevices = unknown
		return stats
	})

	// Set up config reload function
	apiServer.SetReloadFunc(func() (*config.Config, error) {
		logger.Info().Str("config_dir", configDir).Msg("Reloading configuration")
		newCfg, err := config.LoadConfigDir(configDir)
		if err != nil {
			return nil, err
		}

		// Swap in the latest config/evaluator so newly added devices/interfaces
		// are evaluated immediately after reload.
		cfg = newCfg
		eval = evaluator.NewEvaluator(cfg, logger)

		logger.Info().
			Int("device_count", len(newCfg.DesiredState.Devices)).
			Msg("Configuration reloaded")

		return newCfg, nil
	})

	go func() {
		if err := apiServer.Start(); err != nil {
			logger.Error().
				Err(err).
				Msg("API server error")
		}
	}()

	logger.Info().
		Str("port", apiPort).
		Msg("Web UI available")

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Info().Msg("NetSpec running, press Ctrl+C to stop")

	// Wait for shutdown signal
	<-sigChan
	logger.Info().Msg("Shutting down...")

	cancel()
	logger.Info().Msg("NetSpec stopped")
}

func loadDotEnvIfPresent(configDir string) {
	candidates := []string{
		filepath.Join(configDir, ".env"),
		filepath.Join(configDir, "netspec.env"),
	}
	seen := map[string]struct{}{}
	for _, p := range candidates {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		loadEnvFileNoOverride(p)
	}
}

func loadEnvFileNoOverride(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		val := strings.TrimSpace(kv[1])
		if len(val) >= 2 {
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
		}
		_ = os.Setenv(key, val)
	}
}

func resolveDeviceForEvent(cfg *config.Config, event collector.PushTelemetryEvent) (string, config.DeviceConfig, bool) {
	if dev, ok := cfg.DesiredState.Devices[event.Device]; ok {
		return event.Device, dev, true
	}
	eventDevice := strings.TrimSpace(event.Device)
	for name, dev := range cfg.DesiredState.Devices {
		if strings.EqualFold(name, eventDevice) {
			return name, dev, true
		}
	}
	eventAddr := strings.TrimSpace(eventAddressHint(event))
	for name, dev := range cfg.DesiredState.Devices {
		addr := strings.TrimSpace(dev.Address)
		if addr == "" {
			continue
		}
		if strings.EqualFold(addr, eventDevice) || (eventAddr != "" && strings.EqualFold(addr, eventAddr)) {
			return name, dev, true
		}
	}
	return "", config.DeviceConfig{}, false
}

func eventAddressHint(event collector.PushTelemetryEvent) string {
	if ip := net.ParseIP(strings.TrimSpace(event.Device)); ip != nil {
		return ip.String()
	}
	source := strings.TrimSpace(event.Source)
	if source == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(source)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(source); ip != nil {
		return ip.String()
	}
	return ""
}

func resolveInterfaceConfig(
	ifaces map[string]config.InterfaceConfig,
	eventIface string,
) (string, config.InterfaceConfig, bool) {
	if cfg, ok := ifaces[eventIface]; ok {
		return eventIface, cfg, true
	}
	target := canonicalInterfaceName(eventIface)
	for name, cfg := range ifaces {
		if canonicalInterfaceName(name) == target {
			return name, cfg, true
		}
	}
	return "", config.InterfaceConfig{}, false
}

func canonicalInterfaceName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"gigabitethernet", "gi",
		"tengigabitethernet", "te",
		"twentyfivegige", "tw",
		"twentyfivegigabite", "tw",
		"hundredgige", "hu",
		"hundredgigabitethernet", "hu",
		"port-channel", "po",
		"portchannel", "po",
		" ", "",
	)
	return replacer.Replace(s)
}
