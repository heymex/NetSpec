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
	"github.com/netspec/netspec/internal/webui"
	"github.com/rs/zerolog"
)

var (
	version = "dev"
	commit  = "unknown"
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
		Str("version", version).
		Str("commit", commit).
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

	// Start collectors
	for deviceName, deviceCfg := range cfg.DesiredState.Devices {
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

		// Connect in goroutine with retry
		go func(name string, c *collector.Collector) {
			for {
				if err := c.Connect(); err != nil {
					logger.Error().
						Err(err).
						Str("device", name).
						Msg("Failed to connect, retrying in 10s")
					time.Sleep(10 * time.Second)
					continue
				}
				break
			}
		}(deviceName, col)
	}

	// Start API server with Web UI
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "8088"
	}
	apiServer := api.NewServer(alertEngine, logger, apiPort)

	// Configure the API server with log buffer and config
	apiServer.SetLogBuffer(logBuffer)
	apiServer.SetConfig(cfg, *configPath)

	// Set up config reload function
	apiServer.SetReloadFunc(func() (*config.Config, error) {
		logger.Info().Str("config_dir", configDir).Msg("Reloading configuration")
		newCfg, err := config.LoadConfigDir(configDir)
		if err != nil {
			return nil, err
		}
		// Note: In a full implementation, you would also:
		// - Update the evaluator with new config
		// - Restart collectors for new devices
		// - Stop collectors for removed devices
		// For MVP, we just reload the config for display purposes
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

	// Process updates from collectors
	for deviceName, col := range collectors {
		go func(name string, c *collector.Collector) {
			for {
				select {
				case <-ctx.Done():
					return
				case notification := <-c.Updates():
					// Evaluate state changes
					changes := eval.EvaluateNotification(name, notification)

					// Process each change
					for _, change := range changes {
						// Send to alert engine via event channel
						alertEngine.ProcessStateChange(change)
					}
				}
			}
		}(deviceName, col)
	}

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
