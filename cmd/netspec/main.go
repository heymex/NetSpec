package main

import (
	"context"
	"flag"
	"io"
	"os"
	"os/signal"
	"path/filepath"
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
	"sync"
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

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Fatal().
			Err(err).
			Str("config_path", *configPath).
			Msg("Failed to load configuration")
	}

	logger.Info().
		Int("device_count", len(cfg.DesiredState.Devices)).
		Msg("Configuration loaded")

	// Create notifier
	notifier := notifier.NewNotifier(logger)

	// Create alert engine
	alertEngine := alerter.NewEngine(cfg, notifier, logger)

	// Start alert engine
	go alertEngine.Run()

	// Create evaluator
	eval := evaluator.NewEvaluator(cfg, logger)

	// Create collectors for each device
	collectors := make(map[string]*collector.Collector)
	collectorsMu := sync.RWMutex{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get credentials (simplified for MVP - in production, use vault integration)
	username := os.Getenv("GNMI_USERNAME")
	if username == "" {
		username = "gnmi-monitor"
	}
	password := os.Getenv("GNMI_PASSWORD")
	if password == "" {
		logger.Fatal().Msg("GNMI_PASSWORD environment variable is required")
	}

	// Helper function to start a collector (defined before first use).
	// Launches both the connection-management goroutine and the
	// update-processing goroutine so that reloaded collectors also
	// have their updates consumed.
	startCollector := func(deviceName string, deviceCfg config.DeviceConfig, cfg *config.Config, username, password string) {
		collectorsMu.Lock()
		defer collectorsMu.Unlock()

		// Close old collector if one exists for this device
		if existing, ok := collectors[deviceName]; ok && existing != nil {
			existing.Close()
		}

		logger.Info().
			Str("device", deviceName).
			Str("address", deviceCfg.Address).
			Int("port", cfg.DesiredState.Global.GNMIPort).
			Msg("Creating collector")

		cred := cfg.ResolveCredentials(deviceName)
		credUsername := cred.Username
		credPassword := ""
		if cred.PasswordEnv != "" {
			credPassword = os.Getenv(cred.PasswordEnv)
		}
		if credUsername == "" {
			credUsername = username
		}
		if credPassword == "" {
			credPassword = password
		}

		col := collector.NewCollector(
			deviceCfg.Address,
			credUsername,
			credPassword,
			cfg.DesiredState.Global.GNMIPort,
			logger.With().Str("device", deviceName).Logger(),
		)

		collectors[deviceName] = col

		// Connection goroutine: connect with retry and auto-reconnect.
		// Exits when either the main ctx or the collector's own ctx is
		// cancelled (the latter happens on Close() during reload).
		go func(name string, addr string, c *collector.Collector) {
			logger.Info().
				Str("device", name).
				Str("address", addr).
				Msg("Starting connection goroutine")

			reconnectDelay := 5 * time.Second
			const maxReconnectDelay = 120 * time.Second

			for {
				if err := c.Connect(); err != nil {
					// If the collector was intentionally closed, exit silently
					if c.Done() != nil {
						select {
						case <-c.Done():
							logger.Debug().Str("device", name).Msg("Collector closed, exiting connection goroutine")
							return
						default:
						}
					}

					logger.Error().
						Err(err).
						Str("device", name).
						Dur("retry_in", reconnectDelay).
						Msg("Failed to connect, will retry")

					select {
					case <-ctx.Done():
						return
					case <-c.Done():
						logger.Debug().Str("device", name).Msg("Collector closed during backoff, exiting")
						return
					case <-time.After(reconnectDelay):
					}

					reconnectDelay = reconnectDelay * 2
					if reconnectDelay > maxReconnectDelay {
						reconnectDelay = maxReconnectDelay
					}
					continue
				}

				// Connection succeeded, reset reconnect delay
				reconnectDelay = 5 * time.Second

				logger.Info().
					Str("device", name).
					Msg("Connection established, monitoring for errors")

				// Monitor connection health and reconnect if lost
				select {
				case <-ctx.Done():
					return
				case <-c.Done():
					logger.Debug().Str("device", name).Msg("Collector closed while connected, exiting")
					return
				case err := <-c.Errors():
					if err != nil {
						// Check if this error is from an intentional close
						select {
						case <-c.Done():
							logger.Debug().Str("device", name).Msg("Collector closed (error during shutdown), exiting")
							return
						default:
						}

						logger.Warn().
							Err(err).
							Str("device", name).
							Msg("Connection lost, will reconnect after cooldown")

						select {
						case <-ctx.Done():
							return
						case <-c.Done():
							return
						case <-time.After(5 * time.Second):
						}
					}
				}
			}
		}(deviceName, deviceCfg.Address, col)

		// Update-processing goroutine: evaluates telemetry against desired
		// state and feeds changes into the alert engine.
		go func(name string, c *collector.Collector) {
			for {
				select {
				case <-ctx.Done():
					return
				case <-c.Done():
					return
				case notification := <-c.Updates():
					changes := eval.EvaluateNotification(name, notification)
					for _, change := range changes {
						alertEngine.ProcessStateChange(change)
					}
				}
			}
		}(deviceName, col)
	}

	// Start collectors
	logger.Info().
		Int("device_count", len(cfg.DesiredState.Devices)).
		Msg("Starting collectors for devices")
	
	for deviceName, deviceCfg := range cfg.DesiredState.Devices {
		startCollector(deviceName, deviceCfg, cfg, username, password)
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
	apiServer.SetCollectorGetter(func(deviceName string) *collector.Collector {
		collectorsMu.RLock()
		defer collectorsMu.RUnlock()
		return collectors[deviceName]
	})

	// Set up config reload function
	apiServer.SetReloadFunc(func() (*config.Config, error) {
		logger.Info().Str("config_dir", configDir).Msg("Reloading configuration")
		newCfg, err := config.LoadConfigDir(configDir)
		if err != nil {
			return nil, err
		}
		
		// Note: We can't easily update evaluator and alert engine without
		// more complex state management. For now, collectors are restarted
		// which is the main issue (IP address changes).
		go alertEngine.Run()
		
		// Stop collectors for removed devices
		collectorsMu.Lock()
		for name, col := range collectors {
			if _, exists := newCfg.DesiredState.Devices[name]; !exists {
				logger.Info().Str("device", name).Msg("Device removed from config, stopping collector")
				if col != nil {
					col.Close()
				}
				delete(collectors, name)
			}
		}
		collectorsMu.Unlock()
		
		// Start/restart collectors for all devices (handles new devices and IP changes)
		for deviceName, deviceCfg := range newCfg.DesiredState.Devices {
			collectorsMu.RLock()
			existing := collectors[deviceName]
			collectorsMu.RUnlock()
			
			// Check if device is new or address changed
			needsRestart := existing == nil
			if existing != nil {
				// For existing collectors, always restart to pick up any config changes
				// (we can't easily compare addresses, so restart is safer)
				logger.Info().Str("device", deviceName).Msg("Restarting collector for device")
				existing.Close()
				needsRestart = true
			}
			
			if needsRestart {
				startCollector(deviceName, deviceCfg, newCfg, username, password)
			}
		}
		
		logger.Info().
			Int("device_count", len(newCfg.DesiredState.Devices)).
			Msg("Configuration reloaded and collectors updated")
		
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

	// Close all collectors
	for name, col := range collectors {
		if err := col.Close(); err != nil {
			logger.Error().
				Err(err).
				Str("device", name).
				Msg("Error closing collector")
		}
	}

	cancel()
	logger.Info().Msg("NetSpec stopped")
}
