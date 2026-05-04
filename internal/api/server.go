package api

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/discovery"
	"github.com/netspec/netspec/internal/evaluator"
	"github.com/netspec/netspec/internal/webui"
	"github.com/rs/zerolog"
)

// ConfigReloadFunc is called when config reload is requested
type ConfigReloadFunc func() (*config.Config, error)

type EvaluatorGetter func() *evaluator.Evaluator
type TelemetryStatsGetter func() TelemetryStats

type TelemetryStats struct {
	Received            uint64                          `json:"received"`
	Accepted            uint64                          `json:"accepted"`
	RejectedInvalidJSON uint64                          `json:"rejected_invalid_json"`
	RejectedAuth        uint64                          `json:"rejected_auth"`
	RejectedMissing     uint64                          `json:"rejected_missing"`
	LastEventAt         time.Time                       `json:"last_event_at"`
	EventsPerSecond     float64                         `json:"events_per_second"`
	TopDevices          []collector.DeviceTelemetryStat `json:"top_devices"`
	UnknownDevices      []UnknownTelemetryDevice        `json:"unknown_devices"`
}

type UnknownTelemetryDevice struct {
	Device     string    `json:"device"`
	Count      uint64    `json:"count"`
	LastSeenAt time.Time `json:"last_seen_at"`
	WizardURL  string    `json:"wizard_url"`
}

// Server provides HTTP API endpoints and web UI
type Server struct {
	alertEngine     *alerter.Engine
	logger          zerolog.Logger
	port            string
	logBuffer       *webui.LogBuffer
	config          *config.Config
	configPath      string
	startTime       time.Time
	reloadFunc      ConfigReloadFunc
	reloadMu        sync.RWMutex
	version         string
	commit          string
	buildDate       string
	versionMu       sync.RWMutex
	evaluatorGetter EvaluatorGetter
	evaluatorMu     sync.RWMutex
	telemetryGetter TelemetryStatsGetter
	telemetryMu     sync.RWMutex
	reachMu        sync.RWMutex
	reachTracker   *collector.ReachabilityTracker
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

// SetEvaluatorGetter sets function to access evaluator runtime state.
func (s *Server) SetEvaluatorGetter(getter EvaluatorGetter) {
	s.evaluatorMu.Lock()
	defer s.evaluatorMu.Unlock()
	s.evaluatorGetter = getter
}

func (s *Server) SetTelemetryStatsGetter(getter TelemetryStatsGetter) {
	s.telemetryMu.Lock()
	defer s.telemetryMu.Unlock()
	s.telemetryGetter = getter
}

// SetSNMPReachabilityTracker supplies per-device SNMP contact state for the API and honeycomb.
func (s *Server) SetSNMPReachabilityTracker(t *collector.ReachabilityTracker) {
	s.reachMu.Lock()
	defer s.reachMu.Unlock()
	s.reachTracker = t
}

func (s *Server) snmpReachTracker() *collector.ReachabilityTracker {
	s.reachMu.RLock()
	defer s.reachMu.RUnlock()
	return s.reachTracker
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
	mux.HandleFunc("/api/telemetry/stats", s.handleTelemetryStatsAPI)
	mux.HandleFunc("/api/discovery/probe", s.handleDiscoveryProbe)
	mux.HandleFunc("/api/discovery/walk", s.handleDiscoveryWalk)
	mux.HandleFunc("/api/discovery/commit", s.handleDiscoveryCommit)
	mux.HandleFunc("/openapi.json", s.handleOpenAPIJSON)
	mux.HandleFunc("/api-browser", s.handleAPIBrowser)

	// Web UI routes
	mux.HandleFunc("/device/", s.handleDevicePage)
	mux.HandleFunc("/wizard", s.handleWizardPage)

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
	sort.SliceStable(alerts, func(i, j int) bool {
		iSeverity := alertSeverityRank(alerts[i].Severity)
		jSeverity := alertSeverityRank(alerts[j].Severity)
		if iSeverity != jSeverity {
			return iSeverity < jSeverity
		}
		if alerts[i].Device != alerts[j].Device {
			return alerts[i].Device < alerts[j].Device
		}
		if alerts[i].Entity != alerts[j].Entity {
			return alerts[i].Entity < alerts[j].Entity
		}
		if alerts[i].Severity != alerts[j].Severity {
			return alerts[i].Severity < alerts[j].Severity
		}
		return alerts[i].Message < alerts[j].Message
	})
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
		reverseLogEntries(entries)
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

	deviceNames := make([]string, 0, len(cfg.DesiredState.Devices))
	for name := range cfg.DesiredState.Devices {
		deviceNames = append(deviceNames, name)
	}
	sort.Strings(deviceNames)

	tr := s.snmpReachTracker()
	devices := make([]map[string]interface{}, 0, len(deviceNames))
	for _, name := range deviceNames {
		dev := cfg.DesiredState.Devices[name]
		row := map[string]interface{}{
			"name":            name,
			"address":         dev.Address,
			"description":     dev.Description,
			"interface_count": len(dev.Interfaces),
		}
		if tr != nil {
			rs := tr.Status(name)
			row["snmp_reachability"] = rs.Reachability
			if !rs.LastAttemptAt.IsZero() {
				row["snmp_last_attempt_at"] = rs.LastAttemptAt.UTC().Format(time.RFC3339Nano)
			}
			if !rs.LastOKAt.IsZero() {
				row["snmp_last_ok_at"] = rs.LastOKAt.UTC().Format(time.RFC3339Nano)
			}
			if rs.LastError != "" {
				row["snmp_last_error"] = rs.LastError
			}
		}
		devices = append(devices, row)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices": devices,
	})
}

// handleDeviceDetailAPI returns detailed information about a specific device
func (s *Server) handleDeviceDetailAPI(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/interfaces/") {
		s.handleInterfacePatch(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		s.handleDeviceDelete(w, r)
		return
	}
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

	// Build interface list
	ifaceNames := make([]string, 0, len(deviceCfg.Interfaces))
	for ifaceName := range deviceCfg.Interfaces {
		ifaceNames = append(ifaceNames, ifaceName)
	}
	sort.Strings(ifaceNames)

	interfaces := make([]map[string]interface{}, 0, len(ifaceNames))
	for _, ifaceName := range ifaceNames {
		ifaceCfg := deviceCfg.Interfaces[ifaceName]
		var runtime map[string]interface{}
		s.evaluatorMu.RLock()
		evalGetter := s.evaluatorGetter
		s.evaluatorMu.RUnlock()
		if evalGetter != nil {
			if eval := evalGetter(); eval != nil {
				if state, ok := eval.GetInterfaceState(deviceName, ifaceName); ok {
					runtime = map[string]interface{}{
						"oper_status":                  state.OperStatus,
						"admin_status":                 state.AdminStatus,
						"last_snmp_validation_at":      state.LastSNMPValidation,
						"last_telemetry_validation_at": state.LastTelemetryValidation,
					}
				}
			}
		}
		interfaces = append(interfaces, map[string]interface{}{
			"name":          ifaceName,
			"description":   ifaceCfg.Description,
			"desired_state": ifaceCfg.DesiredState,
			"admin_state":   ifaceCfg.AdminState,
			"alerts":        ifaceCfg.Alerts,
			"runtime":       runtime,
		})
	}

	// Get device-specific logs
	var deviceLogs []webui.LogEntry
	if s.logBuffer != nil {
		allLogs := s.logBuffer.GetRecentEntries(500)
		reverseLogEntries(allLogs)
		for _, entry := range allLogs {
			// Check if log entry is for this device
			if strings.Contains(strings.ToLower(entry.Message), strings.ToLower(deviceName)) ||
				strings.Contains(strings.ToLower(entry.Message), deviceCfg.Address) {
				deviceLogs = append(deviceLogs, entry)
			}
		}
		// Limit to most recent 100
		if len(deviceLogs) > 100 {
			deviceLogs = deviceLogs[:100]
		}
	}

	response := map[string]interface{}{
		"name":        deviceName,
		"address":     deviceCfg.Address,
		"description": deviceCfg.Description,
		"interfaces": interfaces,
		"logs":       deviceLogs,
	}

	json.NewEncoder(w).Encode(response)
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
	Telemetry      TelemetryStats
	HexMapSVG      template.HTML `json:"-"`
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

	s.telemetryMu.RLock()
	tg := s.telemetryGetter
	s.telemetryMu.RUnlock()
	if tg != nil {
		data.Telemetry = tg()
	}

	// Add config details
	if cfg != nil {
		data.DeviceCount = len(cfg.DesiredState.Devices)
		data.Config.CollectionInterval = cfg.DesiredState.Global.CollectionInterval.String()
		data.Config.DedupWindow = cfg.Alerts.AlertBehavior.DeduplicationWindow.String()

		// Build device list
		deviceNames := make([]string, 0, len(cfg.DesiredState.Devices))
		for name := range cfg.DesiredState.Devices {
			deviceNames = append(deviceNames, name)
		}
		sort.Strings(deviceNames)

		for _, name := range deviceNames {
			dev := cfg.DesiredState.Devices[name]
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
	sort.SliceStable(alerts, func(i, j int) bool {
		iSeverity := alertSeverityRank(alerts[i].Severity)
		jSeverity := alertSeverityRank(alerts[j].Severity)
		if iSeverity != jSeverity {
			return iSeverity < jSeverity
		}
		if alerts[i].Device != alerts[j].Device {
			return alerts[i].Device < alerts[j].Device
		}
		if alerts[i].Entity != alerts[j].Entity {
			return alerts[i].Entity < alerts[j].Entity
		}
		if alerts[i].Severity != alerts[j].Severity {
			return alerts[i].Severity < alerts[j].Severity
		}
		return alerts[i].Message < alerts[j].Message
	})
	for _, alert := range alerts {
		data.Alerts = append(data.Alerts, AlertInfo{
			Device:   alert.Device,
			Entity:   alert.Entity,
			Severity: alert.Severity,
			Message:  alert.Message,
		})
	}

	deviceNames := make([]string, 0, len(data.Devices))
	for _, d := range data.Devices {
		deviceNames = append(deviceNames, d.Name)
	}
	hexAlerts := make([]webui.HexAlert, 0, len(alerts))
	for _, a := range alerts {
		hexAlerts = append(hexAlerts, webui.HexAlert{Device: a.Device, Severity: a.Severity})
	}
	worstByDev := webui.WorstSeverityPerDevice(hexAlerts)
	reachAugment := snmpReachForHex(s.snmpReachTracker(), deviceNames)
	mergedHex := webui.MergeHexSeverityWithSNMP(worstByDev, reachAugment)
	hexLayout := webui.BuildHexMapLayout(deviceNames, mergedHex, webui.DefaultHexRadius)
	data.HexMapSVG = webui.RenderHexMapSVG(hexLayout)

	// Get recent logs
	if s.logBuffer != nil {
		data.Logs = s.logBuffer.GetRecentEntries(100)
		reverseLogEntries(data.Logs)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := webui.Templates.ExecuteTemplate(w, "base", data); err != nil {
		s.logger.Error().Err(err).Msg("Failed to render template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleTelemetryStatsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	s.telemetryMu.RLock()
	tg := s.telemetryGetter
	s.telemetryMu.RUnlock()
	if tg == nil {
		_ = json.NewEncoder(w).Encode(TelemetryStats{})
		return
	}
	_ = json.NewEncoder(w).Encode(tg())
}

// DevicePageData holds data for the device detail page
type DevicePageData struct {
	Device    DeviceDetailInfo
	Version   string
	Commit    string
	BuildDate string
}

// DeviceDetailInfo holds detailed device information
type DeviceDetailInfo struct {
	Name           string
	Address        string
	Description    string
	LastSNMPValidationAt      time.Time
	LastTelemetryValidationAt time.Time
	Interfaces     []InterfaceInfo
	Logs           []webui.LogEntry
}

// InterfaceInfo holds interface configuration
type InterfaceInfo struct {
	Name                      string
	Description               string
	DesiredState              string
	AdminState                string
	Monitor                   bool
	Alerts                    config.AlertSeverity
	OperStatus                string
	LastSNMPValidationAt      time.Time
	LastTelemetryValidationAt time.Time
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

	// Build interface list
	ifaceNames := make([]string, 0, len(deviceCfg.Interfaces))
	for ifaceName := range deviceCfg.Interfaces {
		ifaceNames = append(ifaceNames, ifaceName)
	}
	sort.Strings(ifaceNames)

	interfaces := make([]InterfaceInfo, 0, len(ifaceNames))
	var lastSNMPValidationAt time.Time
	var lastTelemetryValidationAt time.Time
	for _, ifaceName := range ifaceNames {
		ifaceCfg := deviceCfg.Interfaces[ifaceName]
		var operStatus string
		var lastSNMP time.Time
		var lastTelemetry time.Time
		s.evaluatorMu.RLock()
		evalGetter := s.evaluatorGetter
		s.evaluatorMu.RUnlock()
		if evalGetter != nil {
			if eval := evalGetter(); eval != nil {
				if state, ok := eval.GetInterfaceState(deviceName, ifaceName); ok {
					operStatus = state.OperStatus
					lastSNMP = state.LastSNMPValidation
					lastTelemetry = state.LastTelemetryValidation
					if state.LastSNMPValidation.After(lastSNMPValidationAt) {
						lastSNMPValidationAt = state.LastSNMPValidation
					}
					if state.LastTelemetryValidation.After(lastTelemetryValidationAt) {
						lastTelemetryValidationAt = state.LastTelemetryValidation
					}
				}
			}
		}
		interfaces = append(interfaces, InterfaceInfo{
			Name:                      ifaceName,
			Description:               ifaceCfg.Description,
			DesiredState:              ifaceCfg.DesiredState,
			AdminState:                ifaceCfg.AdminState,
			Monitor:                   ifaceCfg.Monitor,
			Alerts:                    ifaceCfg.Alerts,
			OperStatus:                operStatus,
			LastSNMPValidationAt:      lastSNMP,
			LastTelemetryValidationAt: lastTelemetry,
		})
	}

	// Get device-specific logs
	var deviceLogs []webui.LogEntry
	if s.logBuffer != nil {
		allLogs := s.logBuffer.GetRecentEntries(500)
		reverseLogEntries(allLogs)
		for _, entry := range allLogs {
			// Check if log entry is for this device
			if strings.Contains(strings.ToLower(entry.Message), strings.ToLower(deviceName)) ||
				strings.Contains(strings.ToLower(entry.Message), deviceCfg.Address) {
				deviceLogs = append(deviceLogs, entry)
			}
		}
		// Limit to most recent 100
		if len(deviceLogs) > 100 {
			deviceLogs = deviceLogs[:100]
		}
	}

	deviceDetail := DeviceDetailInfo{
		Name:                      deviceName,
		Address:                   deviceCfg.Address,
		Description:               deviceCfg.Description,
		LastSNMPValidationAt:      lastSNMPValidationAt,
		LastTelemetryValidationAt: lastTelemetryValidationAt,
		Interfaces:                interfaces,
		Logs:                      deviceLogs,
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

// snmpReachForHex maps device names to reachability strings for honeycomb merge (ok omitted).
func snmpReachForHex(tracker *collector.ReachabilityTracker, deviceNames []string) map[string]string {
	if tracker == nil || len(deviceNames) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, dn := range deviceNames {
		st := tracker.Status(dn)
		if st.Reachability == collector.SNMPReachOK {
			continue
		}
		out[dn] = st.Reachability
	}
	return out
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
		return strconv.Itoa(days) + "d"
	}
	return strconv.Itoa(days) + "d " + strconv.Itoa(hours) + "h"
}

func reverseLogEntries(entries []webui.LogEntry) {
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
}

func alertSeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

type interfacePatchRequest struct {
	Description   *string `json:"description"`
	DesiredState  *string `json:"desired_state"`
	AdminState    *string `json:"admin_state"`
	Monitor       *bool   `json:"monitor"`
	AlertSeverity *string `json:"alert_severity"`
}

func (s *Server) handleInterfacePatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[1] != "interfaces" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	deviceName, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(deviceName) == "" {
		http.Error(w, "invalid device", http.StatusBadRequest)
		return
	}
	ifaceName, err := url.PathUnescape(strings.Join(parts[2:], "/"))
	if err != nil || strings.TrimSpace(ifaceName) == "" {
		http.Error(w, "invalid interface", http.StatusBadRequest)
		return
	}

	var req interfacePatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	s.reloadMu.RLock()
	configPath := s.configPath
	reloadFn := s.reloadFunc
	s.reloadMu.RUnlock()
	desiredPath := discovery.DesiredStatePathFrom(configPath)

	err = discovery.UpdateDeviceInterface(desiredPath, deviceName, ifaceName, discovery.InterfaceUpdate{
		Description:   req.Description,
		DesiredState:  req.DesiredState,
		AdminState:    req.AdminState,
		Monitor:       req.Monitor,
		AlertSeverity: req.AlertSeverity,
	})
	if err != nil {
		status := discovery.StatusCodeFor(err)
		if status < 400 {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	if reloadFn != nil {
		if newCfg, err := reloadFn(); err == nil {
			s.reloadMu.Lock()
			s.config = newCfg
			s.reloadMu.Unlock()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"device":    deviceName,
		"interface": ifaceName,
	})
}

func (s *Server) handleDeviceDelete(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	if path == "" || path == "/api/devices" {
		http.Error(w, "device name required", http.StatusBadRequest)
		return
	}
	deviceName, err := url.PathUnescape(path)
	if err != nil || strings.TrimSpace(deviceName) == "" {
		http.Error(w, "invalid device name", http.StatusBadRequest)
		return
	}

	s.reloadMu.RLock()
	configPath := s.configPath
	reloadFn := s.reloadFunc
	s.reloadMu.RUnlock()
	desiredPath := discovery.DesiredStatePathFrom(configPath)
	if err := discovery.DeleteDevice(desiredPath, deviceName); err != nil {
		status := discovery.StatusCodeFor(err)
		if status < 400 {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}

	if reloadFn != nil {
		if newCfg, err := reloadFn(); err == nil {
			s.reloadMu.Lock()
			s.config = newCfg
			s.reloadMu.Unlock()
		}
	}
	cleared := s.alertEngine.ClearAlertsForDevice(deviceName)
	if cleared > 0 {
		s.logger.Info().
			Str("device", deviceName).
			Int("cleared_alerts", cleared).
			Msg("Cleared active alerts for deleted device")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"device":         deviceName,
		"cleared_alerts": cleared,
	})
}
