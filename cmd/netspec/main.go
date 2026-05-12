package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/netspec/netspec/internal/api"
	"github.com/netspec/netspec/internal/auth"
	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/evaluator"
	"github.com/netspec/netspec/internal/ifname"
	"github.com/netspec/netspec/internal/notifier"
	"github.com/netspec/netspec/internal/version"
	"github.com/netspec/netspec/internal/webhook"
	"github.com/netspec/netspec/internal/webui"
	"github.com/rs/zerolog"
)

func main() {
	// Subcommand: netspec hash-password [password]
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		var password string
		if len(os.Args) > 2 {
			password = os.Args[2]
		} else {
			fmt.Print("Password: ")
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				password = scanner.Text()
			}
		}
		if password == "" {
			fmt.Fprintln(os.Stderr, "error: password must not be empty")
			os.Exit(1)
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(hash)
		return
	}

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

	var runningCfg atomic.Pointer[config.Config]
	runningCfg.Store(cfg)
	reachTracker := collector.NewReachabilityTracker()

	// Create notifier (channel URLs come from env vars named in config/alerts.yaml channels.*.url_env)
	appriseNotifier := notifier.NewNotifier(logger, cfg.Alerts.Channels)

	// Create alert engine
	alertEngine := alerter.NewEngine(cfg, appriseNotifier, logger, config.DataDir(configDir))

	// Wire up Slack ChatOps if configured.
	var slackNotifier *notifier.SlackNotifier
	if cfg.Alerts.Slack.Enabled && cfg.Alerts.Slack.BotTokenEnv != "" {
		slackNotifier = notifier.NewSlackNotifier(cfg.Alerts.Slack.BotTokenEnv, logger)
		if slackNotifier != nil {
			alertEngine.SetSlackNotifier(slackNotifier)
			logger.Info().Msg("Slack ChatOps enabled")
		} else {
			logger.Warn().
				Str("bot_token_env", cfg.Alerts.Slack.BotTokenEnv).
				Msg("Slack ChatOps configured but bot token env var is empty — disabled")
		}
	}

	// Start alert engine
	go alertEngine.Run()

	// Create evaluator
	eval := evaluator.NewEvaluator(cfg, logger)

	var pushIngestors []*collector.PushIngestor
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
					monitored := 0
					for _, ic := range dc.Interfaces {
						if ic.Monitor {
							monitored++
						}
					}
					snapshots, err := validator.PollDevice(name, dc)
					reachTracker.RecordPoll(name, err, len(snapshots), monitored)
					syncSNMPReachAlerts(alertEngine, reachTracker, name)
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
		inger := cfg.DesiredState.Global.Ingest
		listenerDefs := []struct {
			port   uint16
			source string
		}{{inger.Port, strings.TrimSpace(inger.Source)}}
		for _, al := range inger.AdditionalListeners {
			listenerDefs = append(listenerDefs, struct {
				port   uint16
				source string
			}{al.Port, strings.TrimSpace(al.Source)})
		}
		portsLog := make([]uint16, len(listenerDefs))
		for i, d := range listenerDefs {
			portsLog[i] = d.port
		}
		logger.Info().
			Str("listen_address", inger.ListenAddress).
			Interface("listen_ports", portsLog).
			Msg("Starting push telemetry ingest listeners")

		validator := collector.NewSNMPValidator(
			cfg.DesiredState.Global.SNMP,
			snmpCommunity,
			logger.With().Str("component", "snmp-validator").Logger(),
		)

		ingestToken := ""
		if inger.TokenEnv != "" {
			ingestToken = os.Getenv(inger.TokenEnv)
		}

		onEvent := func(event collector.PushTelemetryEvent) {
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
			} else {
				reachTracker.RecordInterfaceSNMPSuccess(deviceName)
				alertEngine.SyncSNMPReachability(deviceName, true, "")
			}

			var validationSource string
			if err != nil {
				// Could not SNMP-confirm this push-driven update; evaluate from telemetry snapshot only.
				validationSource = "telemetry"
			} else {
				// Normal push path: both telemetry (event) and SNMP confirmation succeeded — record both in the UI/runtime cache.
				validationSource = "push_snmp"
			}
			changes := eval.EvaluateInterfaceSnapshotWithSource(deviceName, snapshot.Interface, snapshot.OperStatus, snapshot.AdminStatus, validationSource)
			for _, change := range changes {
				alertEngine.ProcessStateChange(change)
			}
		}

		for _, def := range listenerDefs {
			ing := collector.NewPushIngestor(
				inger.ListenAddress,
				def.port,
				def.source,
				ingestToken,
				logger.With().Str("component", "push-ingestor").Uint16("listen_port", def.port).Logger(),
				onEvent,
			)
			pushIngestors = append(pushIngestors, ing)
			go func(inst *collector.PushIngestor, listenPort uint16) {
				if err := inst.Start(ctx); err != nil {
					logger.Error().Err(err).Uint16("listen_port", listenPort).Msg("Push telemetry ingestor stopped")
					cancel()
				}
			}(ing, def.port)
		}

		// Periodic SNMP reachability so configured devices do not appear healthy when no telemetry arrives.
		go func() {
			iv := cfg.DesiredState.Global.SNMP.ValidationInterval
			if iv <= 0 {
				iv = cfg.DesiredState.Global.CollectionInterval
			}
			if iv <= 0 {
				iv = 10 * time.Second
			}
			ticker := time.NewTicker(iv)
			defer ticker.Stop()
			for {
				cur := runningCfg.Load()
				if cur != nil {
					for name, dc := range cur.DesiredState.Devices {
						addr := strings.TrimSpace(dc.Address)
						if addr == "" {
							reachTracker.RecordPing(name, fmt.Errorf("device has no address"))
						} else {
							reachTracker.RecordPing(name, validator.SNMPPing(addr))
						}
						syncSNMPReachAlerts(alertEngine, reachTracker, name)
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()

		// Optional heavy fallback: full SNMP polling in telemetry mode.
		if cfg.DesiredState.Global.SNMP.TelemetryFallbackEnabled {
			fallbackInterval := cfg.DesiredState.Global.SNMP.TelemetryFallbackInterval
			if fallbackInterval <= 0 {
				fallbackInterval = 5 * time.Minute
			}
			logger.Warn().
				Dur("fallback_interval", fallbackInterval).
				Msg("SNMP telemetry fallback polling ENABLED: this can significantly increase SNMP load and reduce performance on larger fleets")
			go func() {
				ticker := time.NewTicker(fallbackInterval)
				defer ticker.Stop()
				for {
					cur := runningCfg.Load()
					if cur != nil {
						for name, dc := range cur.DesiredState.Devices {
							monitored := 0
							for _, ic := range dc.Interfaces {
								if ic.Monitor {
									monitored++
								}
							}
							snapshots, err := validator.PollDevice(name, dc)
							reachTracker.RecordPoll(name, err, len(snapshots), monitored)
							syncSNMPReachAlerts(alertEngine, reachTracker, name)
							if err != nil {
								logger.Warn().
									Err(err).
									Str("device", name).
									Msg("Telemetry fallback SNMP poll failed")
								continue
							}
							for _, snap := range snapshots {
								changes := eval.EvaluateInterfaceSnapshotWithSource(name, snap.Interface, snap.OperStatus, snap.AdminStatus, "snmp")
								for _, change := range changes {
									alertEngine.ProcessStateChange(change)
								}
							}
						}
					}
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
			}()
		}
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

	// Register Slack interaction webhook if ChatOps is enabled.
	if slackNotifier != nil {
		signingSecret := ""
		if cfg.Alerts.Slack.SigningSecretEnv != "" {
			signingSecret = os.Getenv(cfg.Alerts.Slack.SigningSecretEnv)
		}
		if signingSecret == "" {
			logger.Warn().Msg("Slack ChatOps: SLACK_SIGNING_SECRET not set — webhook signature validation disabled")
		}
		slackWebhookHandler := webhook.NewSlackHandler(signingSecret, alertEngine, slackNotifier, logger)
		apiServer.SetSlackWebhookHandler(slackWebhookHandler)
		logger.Info().
			Str("path", "/webhook/slack/interactions").
			Str("port", apiPort).
			Msg("Slack interaction webhook registered")
	}

	// Configure auth (disabled when NETSPEC_ADMIN_PASSWORD_HASH is unset).
	authManager := auth.NewManager(
		os.Getenv("NETSPEC_ADMIN_PASSWORD_HASH"),
		os.Getenv("NETSPEC_API_TOKEN"),
	)
	if authManager.Enabled() {
		logger.Info().Msg("Authentication enabled")
	} else {
		logger.Warn().Msg("Authentication disabled: set NETSPEC_ADMIN_PASSWORD_HASH to enable")
	}

	// Configure the API server with log buffer, config, version, and collector getter
	apiServer.SetAuthManager(authManager)
	apiServer.SetLogBuffer(logBuffer)
	apiServer.SetConfig(cfg, *configPath)
	apiServer.SetSNMPReachabilityTracker(reachTracker)
	apiServer.SetVersion(version.GetVersion(), version.GetCommit(), version.GetBuildDate())
	apiServer.SetEvaluatorGetter(func() *evaluator.Evaluator {
		return eval
	})
	apiServer.SetTelemetryStatsGetter(func() api.TelemetryStats {
		stats := api.TelemetryStats{}
		if len(pushIngestors) > 0 {
			perListener := make([]collector.PushIngestorStats, 0, len(pushIngestors))
			for _, ing := range pushIngestors {
				perListener = append(perListener, ing.Stats())
			}
			merged := collector.AggregatePushIngestorStats(perListener)
			stats.Received = merged.Received
			stats.Accepted = merged.Accepted
			stats.RejectedInvalidJSON = merged.RejectedInvalidJSON
			stats.RejectedAuth = merged.RejectedAuth
			stats.RejectedMissing = merged.RejectedMissing
			stats.LastEventAt = merged.LastEventAt
			stats.EventsPerSecond = merged.EventsPerSecond
			stats.RecentPerSecond = merged.RecentPerSecond
			stats.TopDevices = collector.TopDeviceStats(merged.ByDevice, 10)
			stats.Listeners = perListener
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
		runningCfg.Store(newCfg)
		allowed := make(map[string]struct{}, len(newCfg.DesiredState.Devices))
		for n := range newCfg.DesiredState.Devices {
			allowed[n] = struct{}{}
		}
		reachTracker.Prune(allowed)
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
	keys := make([]string, 0, len(ifaces))
	for k := range ifaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	matched := ifname.ResolveConfigKey(keys, eventIface)
	if cfg, ok := ifaces[matched]; ok {
		return matched, cfg, true
	}
	return "", config.InterfaceConfig{}, false
}

func syncSNMPReachAlerts(engine *alerter.Engine, tr *collector.ReachabilityTracker, device string) {
	if engine == nil || tr == nil {
		return
	}
	st := tr.Status(device)
	ok := st.Reachability == collector.SNMPReachOK
	errMsg := ""
	if st.Reachability == collector.SNMPReachFail {
		errMsg = st.LastError
	}
	engine.SyncSNMPReachability(device, ok, errMsg)
}
