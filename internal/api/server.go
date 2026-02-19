package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/webui"
	"github.com/rs/zerolog"
)

// ConfigReloadFunc is called when config reload is requested
type ConfigReloadFunc func() (*config.Config, error)

// CollectorGetter is a function that returns a collector by device name
type CollectorGetter func(deviceName string) *collector.Collector

// Server provides HTTP API endpoints and web UI
type Server struct {
	alertEngine    *alerter.Engine
	logger         zerolog.Logger
	port           string
	logBuffer      *webui.LogBuffer
	config         *config.Config
	configPath     string
	startTime      time.Time
	reloadFunc     ConfigReloadFunc
	reloadMu       sync.RWMutex
	version        string
	commit         string
	buildDate      string
	versionMu      sync.RWMutex
	collectorGetter CollectorGetter
	collectorMu     sync.RWMutex
}

// NewServer creates a new API server
func NewServer(alertEngine *alerter.Engine, logger zerolog.Logger, port string) *Server {
	return &Server{
		alertEngine: alertEngine,
		logger:      logger,
		port:        port,
		startTime:   time.Now(),
	}
}

// SetLogBuffer sets the log buffer for the web UI
func (s *Server) SetLogBuffer(lb *webui.LogBuffer) {
	s.logBuffer = lb
}

// SetConfig sets the current configuration
func (s *Server) SetConfig(cfg *config.Config, configPath string) {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	s.config = cfg
	s.configPath = configPath
}

// SetReloadFunc sets the function to call when config reload is requested
func (s *Server) SetReloadFunc(fn ConfigReloadFunc) {
	s.reloadFunc = fn
}

// SetVersion sets the version information
func (s *Server) SetVersion(version, commit, buildDate string) {
	s.versionMu.Lock()
	defer s.versionMu.Unlock()
	s.version = version
	s.commit = commit
	s.buildDate = buildDate
}

// SetCollectorGetter sets the function to get collectors by device name
func (s *Server) SetCollectorGetter(getter CollectorGetter) {
	s.collectorMu.Lock()
	defer s.collectorMu.Unlock()
	s.collectorGetter = getter
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/alerts", s.handleAlerts)
	mux.HandleFunc("/api/logs", s.handleLogsAPI)
	mux.HandleFunc("/api/reload", s.handleReload)
	mux.HandleFunc("/api/devices", s.handleDevicesAPI)
	mux.HandleFunc("/api/devices/", s.handleDeviceDetailAPI)
	mux.HandleFunc("/api/test/", s.handleTestConnection)
	
	// Web UI routes
	mux.HandleFunc("/device/", s.handleDevicePage)

	// Web UI
	mux.HandleFunc("/", s.handleWebUI)

	addr := ":" + s.port
	s.logger.Info().
		Str("address", addr).
		Msg("Starting API server with Web UI")

	return http.ListenAndServe(addr, mux)
}

// handleHealth returns service health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// handleStatus returns current state summary
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	alerts := s.alertEngine.GetActiveAlerts()
	s.versionMu.RLock()
	version := s.version
	commit := s.commit
	buildDate := s.buildDate
	s.versionMu.RUnlock()

	status := map[string]interface{}{
		"active_alerts": len(alerts),
		"time":          time.Now().UTC().Format(time.RFC3339),
		"uptime":        time.Since(s.startTime).String(),
		"version":       version,
		"commit":        commit,
		"build_date":    buildDate,
	}

	json.NewEncoder(w).Encode(status)
}

// handleAlerts returns active alerts
func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	alerts := s.alertEngine.GetActiveAlerts()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts": alerts,
		"count":  len(alerts),
	})
}

// handleLogsAPI returns recent log entries as JSON
func (s *Server) handleLogsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var entries []webui.LogEntry
	if s.logBuffer != nil {
		entries = s.logBuffer.GetRecentEntries(200)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"count":   len(entries),
	})
}

// handleDevicesAPI returns device configuration as JSON
func (s *Server) handleDevicesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()

	if cfg == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"devices": []interface{}{},
		})
		return
	}

	devices := make([]map[string]interface{}, 0)
	for name, dev := range cfg.DesiredState.Devices {
		devices = append(devices, map[string]interface{}{
			"name":            name,
			"address":         dev.Address,
			"description":     dev.Description,
			"interface_count": len(dev.Interfaces),
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices": devices,
	})
}

// handleDeviceDetailAPI returns detailed information about a specific device
func (s *Server) handleDeviceDetailAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Extract device name from path: /api/devices/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	if path == "" || path == "/api/devices" {
		http.Error(w, "Device name required", http.StatusBadRequest)
		return
	}
	deviceName := path

	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()

	if cfg == nil {
		http.Error(w, "Configuration not loaded", http.StatusInternalServerError)
		return
	}

	// Get device config
	deviceCfg, exists := cfg.DesiredState.Devices[deviceName]
	if !exists {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Get collector health
	var health collector.DeviceHealth
	s.collectorMu.RLock()
	getter := s.collectorGetter
	s.collectorMu.RUnlock()

	if getter != nil {
		if col := getter(deviceName); col != nil {
			health = col.Health()
		}
	}

	// Build interface list
	interfaces := make([]map[string]interface{}, 0)
	for ifaceName, ifaceCfg := range deviceCfg.Interfaces {
		interfaces = append(interfaces, map[string]interface{}{
			"name":          ifaceName,
			"description":   ifaceCfg.Description,
			"desired_state": ifaceCfg.DesiredState,
			"admin_state":   ifaceCfg.AdminState,
			"alerts":        ifaceCfg.Alerts,
		})
	}

	// Get device-specific logs
	var deviceLogs []webui.LogEntry
	if s.logBuffer != nil {
		allLogs := s.logBuffer.GetRecentEntries(500)
		for _, entry := range allLogs {
			// Check if log entry is for this device
			if strings.Contains(strings.ToLower(entry.Message), strings.ToLower(deviceName)) ||
				strings.Contains(strings.ToLower(entry.Message), deviceCfg.Address) {
				deviceLogs = append(deviceLogs, entry)
			}
		}
		// Limit to most recent 100
		if len(deviceLogs) > 100 {
			deviceLogs = deviceLogs[len(deviceLogs)-100:]
		}
	}

	response := map[string]interface{}{
		"name":        deviceName,
		"address":     deviceCfg.Address,
		"description": deviceCfg.Description,
		"health": map[string]interface{}{
			"connected":        health.Connected,
			"last_update":       health.LastUpdate,
			"last_error":        health.LastError,
			"reconnect_count":   health.ReconnectCount,
			"update_count":      health.UpdateCount,
			"sync_received":     health.SyncReceived,
			"last_path":         health.LastPath,
			"last_value":        health.LastValue,
			"connected_since":   health.ConnectedSince,
		},
		"interfaces": interfaces,
		"logs":       deviceLogs,
	}

	json.NewEncoder(w).Encode(response)
}

// handleTestConnection performs a one-shot gNMI capabilities test
func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Extract device name from path: /api/test/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/test/")
	if path == "" {
		http.Error(w, "Device name required", http.StatusBadRequest)
		return
	}
	deviceName := path

	s.collectorMu.RLock()
	getter := s.collectorGetter
	s.collectorMu.RUnlock()

	if getter == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Collector not available",
		})
		return
	}

	col := getter(deviceName)
	if col == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Device not found or collector not running",
		})
		return
	}

	s.logger.Info().Str("device", deviceName).Msg("Testing gNMI connection")

	modelCount, gnmiVersion, err := col.TestConnection()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"gnmi_version": gnmiVersion,
		"model_count":  modelCount,
	})
}

// handleReload handles config reload requests
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if s.reloadFunc == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Config reload not configured",
		})
		return
	}

	s.logger.Info().Msg("Config reload requested via API")

	newCfg, err := s.reloadFunc()
	if err != nil {
		s.logger.Error().Err(err).Msg("Config reload failed")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	s.reloadMu.Lock()
	s.config = newCfg
	s.reloadMu.Unlock()

	s.logger.Info().
		Int("device_count", len(newCfg.DesiredState.Devices)).
		Msg("Config reloaded successfully")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"device_count": len(newCfg.DesiredState.Devices),
	})
}

// DeviceInfo holds device information for the web UI
type DeviceInfo struct {
	Name           string
	Address        string
	Description    string
	InterfaceCount int
}

// AlertInfo holds alert information for the web UI
type AlertInfo struct {
	Device   string
	Entity   string
	Severity string
	Message  string
}

// ConfigInfo holds configuration summary for the web UI
type ConfigInfo struct {
	GNMIPort           int
	CollectionInterval string
	DedupWindow        string
	ConfigPath         string
}

// PageData holds all data for the web UI template
type PageData struct {
	DeviceCount    int
	InterfaceCount int
	AlertCount     int
	Uptime         string
	Devices        []DeviceInfo
	Alerts         []AlertInfo
	Logs           []webui.LogEntry
	Config         ConfigInfo
	Version        string
	Commit         string
	BuildDate      string
}

// handleWebUI renders the main web interface
func (s *Server) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.reloadMu.RLock()
	cfg := s.config
	configPath := s.configPath
	s.reloadMu.RUnlock()

	// Get version info
	s.versionMu.RLock()
	version := s.version
	commit := s.commit
	buildDate := s.buildDate
	s.versionMu.RUnlock()

	// Build page data
	data := PageData{
		Uptime: formatDuration(time.Since(s.startTime)),
		Config: ConfigInfo{
			ConfigPath: configPath,
		},
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}

	// Add config details
	if cfg != nil {
		data.DeviceCount = len(cfg.DesiredState.Devices)
		data.Config.GNMIPort = cfg.DesiredState.Global.GNMIPort
		data.Config.CollectionInterval = cfg.DesiredState.Global.CollectionInterval.String()
		data.Config.DedupWindow = cfg.Alerts.AlertBehavior.DeduplicationWindow.String()

		// Build device list
		for name, dev := range cfg.DesiredState.Devices {
			data.Devices = append(data.Devices, DeviceInfo{
				Name:           name,
				Address:        dev.Address,
				Description:    dev.Description,
				InterfaceCount: len(dev.Interfaces),
			})
			data.InterfaceCount += len(dev.Interfaces)
		}
	}

	// Get active alerts
	alerts := s.alertEngine.GetActiveAlerts()
	data.AlertCount = len(alerts)
	for _, alert := range alerts {
		data.Alerts = append(data.Alerts, AlertInfo{
			Device:   alert.Device,
			Entity:   alert.Entity,
			Severity: alert.Severity,
			Message:  alert.Message,
		})
	}

	// Get recent logs
	if s.logBuffer != nil {
		data.Logs = s.logBuffer.GetRecentEntries(100)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := webui.Templates.ExecuteTemplate(w, "base", data); err != nil {
		s.logger.Error().Err(err).Msg("Failed to render template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// DevicePageData holds data for the device detail page
type DevicePageData struct {
	Device      DeviceDetailInfo
	Version     string
	Commit      string
	BuildDate   string
}

// DeviceDetailInfo holds detailed device information
type DeviceDetailInfo struct {
	Name           string
	Address        string
	Description    string
	Connected      bool
	LastUpdate     time.Time
	LastError      string
	ReconnectCount int
	UpdateCount    int64
	SyncReceived   bool
	LastPath       string
	LastValue      string
	ConnectedSince time.Time
	Interfaces     []InterfaceInfo
	Logs           []webui.LogEntry
}

// InterfaceInfo holds interface configuration
type InterfaceInfo struct {
	Name          string
	Description   string
	DesiredState  string
	AdminState    string
	Alerts        config.AlertSeverity
}

// handleDevicePage renders the device detail page
func (s *Server) handleDevicePage(w http.ResponseWriter, r *http.Request) {
	// Extract device name from path: /device/{name}
	path := strings.TrimPrefix(r.URL.Path, "/device/")
	if path == "" || path == "/device" {
		http.NotFound(w, r)
		return
	}
	deviceName := path

	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()

	if cfg == nil {
		http.Error(w, "Configuration not loaded", http.StatusInternalServerError)
		return
	}

	// Get device config
	deviceCfg, exists := cfg.DesiredState.Devices[deviceName]
	if !exists {
		http.NotFound(w, r)
		return
	}

	// Get version info
	s.versionMu.RLock()
	version := s.version
	commit := s.commit
	buildDate := s.buildDate
	s.versionMu.RUnlock()

	// Get collector health
	var health collector.DeviceHealth
	s.collectorMu.RLock()
	getter := s.collectorGetter
	s.collectorMu.RUnlock()

	if getter != nil {
		if col := getter(deviceName); col != nil {
			health = col.Health()
		}
	}

	// Build interface list
	interfaces := make([]InterfaceInfo, 0)
	for ifaceName, ifaceCfg := range deviceCfg.Interfaces {
		interfaces = append(interfaces, InterfaceInfo{
			Name:         ifaceName,
			Description:  ifaceCfg.Description,
			DesiredState: ifaceCfg.DesiredState,
			AdminState:   ifaceCfg.AdminState,
			Alerts:       ifaceCfg.Alerts,
		})
	}

	// Get device-specific logs
	var deviceLogs []webui.LogEntry
	if s.logBuffer != nil {
		allLogs := s.logBuffer.GetRecentEntries(500)
		for _, entry := range allLogs {
			// Check if log entry is for this device
			if strings.Contains(strings.ToLower(entry.Message), strings.ToLower(deviceName)) ||
				strings.Contains(strings.ToLower(entry.Message), deviceCfg.Address) {
				deviceLogs = append(deviceLogs, entry)
			}
		}
		// Limit to most recent 100
		if len(deviceLogs) > 100 {
			deviceLogs = deviceLogs[len(deviceLogs)-100:]
		}
	}

	deviceDetail := DeviceDetailInfo{
		Name:           deviceName,
		Address:        deviceCfg.Address,
		Description:    deviceCfg.Description,
		Connected:      health.Connected,
		LastUpdate:     health.LastUpdate,
		LastError:      health.LastError,
		ReconnectCount: health.ReconnectCount,
		UpdateCount:    health.UpdateCount,
		SyncReceived:   health.SyncReceived,
		LastPath:       health.LastPath,
		LastValue:      health.LastValue,
		ConnectedSince: health.ConnectedSince,
		Interfaces:     interfaces,
		Logs:           deviceLogs,
	}

	data := DevicePageData{
		Device:    deviceDetail,
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := webui.Templates.ExecuteTemplate(w, "device", data); err != nil {
		s.logger.Error().Err(err).Msg("Failed to render device template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		return d.Round(time.Minute).String()
	}
	hours := int(d.Hours())
	if hours < 24 {
		return d.Round(time.Minute).String()
	}
	days := hours / 24
	hours = hours % 24
	if hours == 0 {
		return string(rune('0'+days)) + "d"
	}
	return string(rune('0'+days)) + "d " + string(rune('0'+hours)) + "h"
}
