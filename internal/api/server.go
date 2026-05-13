package api

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/netspec/netspec/internal/auth"
	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/discovery"
	"github.com/netspec/netspec/internal/evaluator"
	"github.com/netspec/netspec/internal/notifier"
	"github.com/netspec/netspec/internal/rules"
	"github.com/netspec/netspec/internal/types"
	"github.com/netspec/netspec/internal/webhook"
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
	RecentPerSecond     []collector.EventRatePoint      `json:"recent_per_second,omitempty"`
	TopDevices          []collector.DeviceTelemetryStat `json:"top_devices"`
	UnknownDevices      []UnknownTelemetryDevice        `json:"unknown_devices"`
	// Listeners is per-TCP-port ingest (same wire format); use for pipeline / sourcetype mapping from Cribl, etc.
	Listeners []collector.PushIngestorStats `json:"listeners,omitempty"`
}

type UnknownTelemetryDevice struct {
	Device     string    `json:"device"`
	Count      uint64    `json:"count"`
	LastSeenAt time.Time `json:"last_seen_at"`
	WizardURL  string    `json:"wizard_url"`
}

type DeviceTelemetryDiagnostics struct {
	DeviceName                 string    `json:"device_name"`
	Address                    string    `json:"address"`
	ConfiguredInterfaces       int       `json:"configured_interfaces"`
	MonitoredInterfaces        int       `json:"monitored_interfaces"`
	RuntimeInterfaces          int       `json:"runtime_interfaces"`
	RuntimeTelemetryInterfaces int       `json:"runtime_telemetry_interfaces"`
	TelemetryEventsSeen        uint64    `json:"telemetry_events_seen"`
	LastTelemetryAt            time.Time `json:"last_telemetry_at"`
	CoveragePct                float64   `json:"coverage_pct"`
	SuspectedNameMismatch      bool      `json:"suspected_name_mismatch"`
}

type DiagnosticsCoverage struct {
	GeneratedAt      time.Time                    `json:"generated_at"`
	TelemetryMode    string                       `json:"telemetry_mode"`
	FallbackEnabled  bool                         `json:"fallback_enabled"`
	FallbackInterval string                       `json:"fallback_interval"`
	Devices          []DeviceTelemetryDiagnostics `json:"devices"`
}

// Server provides HTTP API endpoints and web UI
type Server struct {
	alertEngine     *alerter.Engine
	slackWebhook    *webhook.SlackHandler
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
	reachMu         sync.RWMutex
	reachTracker    *collector.ReachabilityTracker
	authManager     *auth.Manager
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

// SetAuthManager sets the authentication manager. If nil, auth is disabled.
func (s *Server) SetAuthManager(m *auth.Manager) {
	s.authManager = m
}

// SetSlackWebhookHandler registers the Slack interaction webhook handler.
func (s *Server) SetSlackWebhookHandler(h *webhook.SlackHandler) {
	s.slackWebhook = h
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

	// Auth routes (always accessible)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)

	// API endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/alerts", s.handleAlerts)
	mux.HandleFunc("/api/alerts/", s.handleAlertAction)
	mux.HandleFunc("/api/logs", s.handleLogsAPI)
	mux.HandleFunc("/api/reload", s.handleReload)
	mux.HandleFunc("/api/notifications/test", s.handleNotificationTest)
	mux.HandleFunc("/api/devices", s.handleDevicesAPI)
	mux.HandleFunc("/api/devices/", s.handleDeviceDetailAPI)
	mux.HandleFunc("/api/telemetry/stats", s.handleTelemetryStatsAPI)
	mux.HandleFunc("/api/diagnostics/coverage", s.handleDiagnosticsCoverageAPI)
	mux.HandleFunc("/api/discovery/probe", s.handleDiscoveryProbe)
	mux.HandleFunc("/api/discovery/walk", s.handleDiscoveryWalk)
	mux.HandleFunc("/api/discovery/commit", s.handleDiscoveryCommit)
	mux.HandleFunc("/api/rules", s.handleRulesAPI)
	mux.HandleFunc("/openapi.json", s.handleOpenAPIJSON)
	mux.HandleFunc("/api-browser", s.handleAPIBrowser)

	// ChatOps webhook routes (no auth — validated by provider signatures)
	if s.slackWebhook != nil {
		mux.HandleFunc("/webhook/slack/interactions", s.slackWebhook.HandleInteractions)
	}

	// Web UI routes
	mux.HandleFunc("/device/", s.handleDevicePage)
	mux.HandleFunc("/wizard", s.handleWizardPage)
	mux.HandleFunc("/diagnostics", s.handleDiagnosticsPage)

	// Web UI
	mux.HandleFunc("/", s.handleWebUI)

	addr := ":" + s.port
	s.logger.Info().
		Str("address", addr).
		Msg("Starting API server with Web UI")

	return http.ListenAndServe(addr, s.requireAuth(mux))
}

// requireAuth wraps a handler and enforces authentication when enabled.
// /health, /login, and /logout are always reachable without credentials.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authManager == nil || !s.authManager.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		// Always-open paths.
		p := r.URL.Path
		if p == "/health" || p == "/login" || p == "/logout" {
			next.ServeHTTP(w, r)
			return
		}
		if s.authManager.IsAuthenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		// API callers get 401; browser requests get a redirect.
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "application/json") || strings.HasPrefix(p, "/api/") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
	})
}

// handleLogin serves GET (login form) and POST (credential check).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.authManager == nil || !s.authManager.Enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if s.authManager.ValidatePassword(r.FormValue("password")) {
			id, err := s.authManager.CreateSession()
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, s.authManager.SessionCookie(id))
			next := r.FormValue("next")
			if next == "" || !strings.HasPrefix(next, "/") {
				next = "/"
			}
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		next := r.FormValue("next")
		redirectTo := "/login?error=1"
		if next != "" {
			redirectTo += "&next=" + url.QueryEscape(next)
		}
		http.Redirect(w, r, redirectTo, http.StatusSeeOther)
		return
	}
	// GET — render login form.
	loginErr := r.URL.Query().Get("error") == "1"
	next := r.URL.Query().Get("next")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := webui.LoginTemplate.Execute(w, map[string]interface{}{
		"Error": loginErr,
		"Next":  next,
	}); err != nil {
		s.logger.Error().Err(err).Msg("Failed to render login template")
	}
}

// handleLogout clears the session cookie and redirects to /login.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.authManager != nil {
		if c, err := r.Cookie("netspec_session"); err == nil {
			s.authManager.DeleteSession(c.Value)
		}
		http.SetCookie(w, s.authManager.ClearCookie())
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
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
	s.reloadMu.RLock()
	statusCfg := s.config
	s.reloadMu.RUnlock()
	if w := snmpUIWarnings(statusCfg); len(w) > 0 {
		list := make([]map[string]string, 0, len(w))
		for _, x := range w {
			list = append(list, map[string]string{
				"class": x.Class,
				"title": x.Title,
				"body":  x.Body,
			})
		}
		status["snmp_warnings"] = list
	}

	json.NewEncoder(w).Encode(status)
}

// handleAlerts returns active alerts. Each alert is enriched with the
// configured interface description (ifAlias) so the dashboard can show
// what each port is for without a second lookup. The persisted
// types.Alert struct is unchanged — InterfaceDescription is a
// response-only field.
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

	enriched := s.enrichAlertsForResponse(alerts)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"alerts": enriched,
		"count":  len(alerts),
	})
}

// alertResponse augments types.Alert with a render-time interface description
// lookup. Embedded pointer preserves every existing JSON field name on the
// /alerts response — only InterfaceDescription is new.
type alertResponse struct {
	*types.Alert
	InterfaceDescription string `json:"InterfaceDescription,omitempty"`
}

// enrichAlertsForResponse looks up each alert's interface description from
// the current desired-state config. Synthetic entities (e.g. "__snmp__"
// host-level reachability) and flap-detection IDs simply leave the field
// empty — the JS hides empty strings.
func (s *Server) enrichAlertsForResponse(alerts []*types.Alert) []alertResponse {
	out := make([]alertResponse, 0, len(alerts))
	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()

	for _, a := range alerts {
		row := alertResponse{Alert: a}
		if cfg != nil && a.Entity != "" && a.Entity != "__snmp__" {
			if dev, ok := cfg.DesiredState.Devices[a.Device]; ok {
				if iface, ok := dev.Interfaces[a.Entity]; ok {
					row.InterfaceDescription = iface.Description
				}
			}
		}
		out = append(out, row)
	}
	return out
}

// handleAlertAction handles POST /api/alerts/{id}/ack and /api/alerts/{id}/close.
// Path format: /api/alerts/<url-encoded-id>/<action>
func (s *Server) handleAlertAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse /api/alerts/{id}/{action}.
	// Use RawPath so that %2F in the alert ID is not decoded to '/' before we split.
	rawPath := r.URL.RawPath
	if rawPath == "" {
		rawPath = r.URL.Path
	}
	rest := strings.TrimPrefix(rawPath, "/api/alerts/")
	idx := strings.LastIndex(rest, "/")
	if idx < 0 {
		http.Error(w, "invalid path — expected /api/alerts/{id}/{ack|close}", http.StatusBadRequest)
		return
	}
	alertID, err := url.PathUnescape(rest[:idx])
	if err != nil || alertID == "" {
		http.Error(w, "invalid alert id", http.StatusBadRequest)
		return
	}
	action := rest[idx+1:]

	by := "web-ui"

	w.Header().Set("Content-Type", "application/json")

	switch action {
	case "ack":
		var body struct {
			Note string `json:"note"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		alert, err := s.alertEngine.AckAlert(alertID, by, body.Note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(alert)
	case "close":
		alert, err := s.alertEngine.CloseAlert(alertID, by)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(alert)
	default:
		http.Error(w, "unknown action "+action+" — use ack or close", http.StatusBadRequest)
	}
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
	roles := cfg.Rules.DeviceRoles
	devices := make([]map[string]interface{}, 0, len(deviceNames))
	for _, name := range deviceNames {
		dev := cfg.DesiredState.Devices[name]
		row := map[string]interface{}{
			"name":            name,
			"address":         dev.Address,
			"description":     dev.Description,
			"interface_count": len(dev.Interfaces),
			// Default to unknown so never-polled devices do not render as healthy.
			"snmp_reachability": collector.SNMPReachUnknown,
		}
		// Role classification from rules.yaml (longest-prefix match on hostname).
		// Empty role_prefix lands the device in the wizard's "Other" filter bucket.
		row["role_name"] = ""
		row["role_prefix"] = ""
		if role := rules.MatchDevice(name, roles); role != nil {
			row["role_name"] = role.Name
			row["role_prefix"] = role.Prefix
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
		"interfaces":  interfaces,
		"logs":        deviceLogs,
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

// handleNotificationTest sends a synthetic warning through Apprise for each configured alert channel (or a named subset).
func (s *Server) handleNotificationTest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Channels []string `json:"channels"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()
	if cfg == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "configuration not loaded"})
		return
	}

	n := notifier.NewNotifier(s.logger, cfg.Alerts.Channels)
	outcomes, err := n.NotifyAppriseTest(req.Channels)

	resp := map[string]interface{}{
		"outcomes": outcomes,
	}
	allOK := false
	if len(outcomes) > 0 {
		allOK = true
		for _, o := range outcomes {
			if !o.OK {
				allOK = false
				break
			}
		}
	}
	resp["all_ok"] = allOK
	if err != nil {
		resp["error"] = err.Error()
	}

	if outcomes == nil && err != nil {
		w.WriteHeader(http.StatusBadGateway)
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// DeviceInfo holds device information for the web UI
type DeviceInfo struct {
	Name           string
	Address        string
	Description    string
	InterfaceCount int
	// RoleName / RolePrefix come from rules.yaml device_roles via longest-prefix
	// matching on Name. Both are empty when no rule matches (renders as "Other").
	RoleName   string
	RolePrefix string
}

// RoleInfo is a flattened view of a rules.yaml device_role entry for the
// dashboard filter UI. Prefix is the stable filter key; Name is the label.
type RoleInfo struct {
	Name   string
	Prefix string
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
	DeviceCount        int
	InterfaceCount     int
	AlertCount         int
	Uptime             string
	Devices            []DeviceInfo
	Roles              []RoleInfo
	Alerts             []AlertInfo
	Logs               []webui.LogEntry
	Config             ConfigInfo
	Version            string
	Commit             string
	BuildDate          string
	Telemetry          TelemetryStats
	TelemetrySparkline template.HTML   `json:"-"`
	NOCView            bool            `json:"-"`
	NOCRows            []NOCDeviceRow  `json:"-"`
	HexMapSVG          template.HTML   `json:"-"`
	SNMPWarnings       []SNMPUIWarning `json:"-"`
}

type NOCDeviceRow struct {
	Name           string
	Address        string
	InterfaceCount int
	AlertCount     int
	WorstSeverity  string
	SNMPReach      string
	HexMapSVG      template.HTML   `json:"-"`
	SNMPWarnings   []SNMPUIWarning `json:"-"`
}

// handleWebUI renders the main web interface
func (s *Server) handleWebUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/noc" {
		http.NotFound(w, r)
		return
	}
	nocView := r.URL.Path == "/noc"

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
		NOCView:   nocView,
	}

	s.telemetryMu.RLock()
	tg := s.telemetryGetter
	s.telemetryMu.RUnlock()
	if tg != nil {
		data.Telemetry = tg()
		data.TelemetrySparkline = renderTelemetrySparkline(data.Telemetry.RecentPerSecond, 220, 46)
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

		// Surface device_roles from rules.yaml so the dashboard can render
		// per-role filter checkboxes. Order matches rules.yaml; "Other" is
		// added client-side when at least one device matched no rule.
		for _, role := range cfg.Rules.DeviceRoles {
			data.Roles = append(data.Roles, RoleInfo{Name: role.Name, Prefix: role.Prefix})
		}

		for _, name := range deviceNames {
			dev := cfg.DesiredState.Devices[name]
			info := DeviceInfo{
				Name:           name,
				Address:        dev.Address,
				Description:    dev.Description,
				InterfaceCount: len(dev.Interfaces),
			}
			if role := rules.MatchDevice(name, cfg.Rules.DeviceRoles); role != nil {
				info.RoleName = role.Name
				info.RolePrefix = role.Prefix
			}
			data.Devices = append(data.Devices, info)
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
	if nocView {
		alertCounts := make(map[string]int, len(data.Devices))
		worstSeverity := make(map[string]string, len(data.Devices))
		for _, a := range data.Alerts {
			alertCounts[a.Device]++
			cur, ok := worstSeverity[a.Device]
			if !ok || alertSeverityRank(a.Severity) < alertSeverityRank(cur) {
				worstSeverity[a.Device] = a.Severity
			}
		}
		tracker := s.snmpReachTracker()
		for _, d := range data.Devices {
			reach := collector.SNMPReachUnknown
			if tracker != nil {
				reach = tracker.Status(d.Name).Reachability
			}
			sev := worstSeverity[d.Name]
			if sev == "" {
				sev = "none"
			}
			data.NOCRows = append(data.NOCRows, NOCDeviceRow{
				Name:           d.Name,
				Address:        d.Address,
				InterfaceCount: d.InterfaceCount,
				AlertCount:     alertCounts[d.Name],
				WorstSeverity:  sev,
				SNMPReach:      reach,
			})
		}
		sort.Slice(data.NOCRows, func(i, j int) bool {
			ai := alertSeverityRank(data.NOCRows[i].WorstSeverity)
			aj := alertSeverityRank(data.NOCRows[j].WorstSeverity)
			if ai != aj {
				return ai < aj
			}
			if data.NOCRows[i].AlertCount != data.NOCRows[j].AlertCount {
				return data.NOCRows[i].AlertCount > data.NOCRows[j].AlertCount
			}
			return data.NOCRows[i].Name < data.NOCRows[j].Name
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
	hexSource := "dashboard"
	if nocView {
		hexSource = "noc"
	}
	data.HexMapSVG = webui.RenderHexMapSVGWithSource(hexLayout, hexSource)
	data.SNMPWarnings = snmpUIWarnings(cfg)

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

func (s *Server) buildDiagnosticsCoverage() DiagnosticsCoverage {
	out := DiagnosticsCoverage{
		GeneratedAt: time.Now().UTC(),
	}

	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()
	if cfg == nil {
		return out
	}

	out.TelemetryMode = cfg.DesiredState.Global.TelemetryMode
	out.FallbackEnabled = cfg.DesiredState.Global.SNMP.TelemetryFallbackEnabled
	out.FallbackInterval = cfg.DesiredState.Global.SNMP.TelemetryFallbackInterval.String()

	var topByDevice map[string]collector.DeviceTelemetryStat
	var lastEventAt time.Time
	s.telemetryMu.RLock()
	tg := s.telemetryGetter
	s.telemetryMu.RUnlock()
	if tg != nil {
		stats := tg()
		lastEventAt = stats.LastEventAt
		topByDevice = make(map[string]collector.DeviceTelemetryStat, len(stats.TopDevices))
		for _, td := range stats.TopDevices {
			topByDevice[td.Device] = td
		}
	}

	s.evaluatorMu.RLock()
	evalGetter := s.evaluatorGetter
	s.evaluatorMu.RUnlock()
	var eval *evaluator.Evaluator
	if evalGetter != nil {
		eval = evalGetter()
	}

	deviceNames := make([]string, 0, len(cfg.DesiredState.Devices))
	for dn := range cfg.DesiredState.Devices {
		deviceNames = append(deviceNames, dn)
	}
	sort.Strings(deviceNames)

	for _, dn := range deviceNames {
		dev := cfg.DesiredState.Devices[dn]
		item := DeviceTelemetryDiagnostics{
			DeviceName:           dn,
			Address:              dev.Address,
			ConfiguredInterfaces: len(dev.Interfaces),
			LastTelemetryAt:      lastEventAt,
		}

		for ifName, ifCfg := range dev.Interfaces {
			if ifCfg.Monitor {
				item.MonitoredInterfaces++
			}
			if eval == nil {
				continue
			}
			st, ok := eval.GetInterfaceState(dn, ifName)
			if !ok {
				continue
			}
			item.RuntimeInterfaces++
			if !st.LastTelemetryValidation.IsZero() {
				item.RuntimeTelemetryInterfaces++
			}
		}

		if item.MonitoredInterfaces > 0 {
			item.CoveragePct = float64(item.RuntimeTelemetryInterfaces) * 100.0 / float64(item.MonitoredInterfaces)
		}

		if td, ok := topByDevice[dn]; ok {
			item.TelemetryEventsSeen = td.Count
		}

		item.SuspectedNameMismatch = item.TelemetryEventsSeen > 0 &&
			item.MonitoredInterfaces > 0 &&
			item.RuntimeTelemetryInterfaces == 0

		out.Devices = append(out.Devices, item)
	}

	return out
}

func (s *Server) handleDiagnosticsCoverageAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.buildDiagnosticsCoverage())
}

func (s *Server) handleDiagnosticsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/diagnostics" {
		http.NotFound(w, r)
		return
	}
	d := s.buildDiagnosticsCoverage()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprint(w, "<!doctype html><html><head><meta charset=\"utf-8\"><title>NetSpec Diagnostics</title><style>body{font-family:system-ui,sans-serif;background:#0d1117;color:#e6edf3;padding:24px}table{border-collapse:collapse;width:100%}th,td{border:1px solid #30363d;padding:8px;text-align:left}th{background:#161b22}.warn{color:#d29922}.bad{color:#f85149}.ok{color:#3fb950}a{color:#58a6ff}code{background:#161b22;padding:2px 6px;border-radius:4px}</style></head><body>")
	_, _ = fmt.Fprintf(w, "<h1>Diagnostics Coverage</h1><p>Mode: <code>%s</code> | SNMP fallback: <code>%t</code> (%s) | Generated: %s</p>", d.TelemetryMode, d.FallbackEnabled, d.FallbackInterval, d.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprint(w, "<p><a href=\"/\">Back to Dashboard</a> | <a href=\"/api/diagnostics/coverage\">JSON API</a></p>")
	_, _ = fmt.Fprint(w, "<table><thead><tr><th>Device</th><th>Monitored</th><th>Telemetry Runtime</th><th>Coverage</th><th>Telemetry Events</th><th>Suspected Mapping Issue</th></tr></thead><tbody>")
	for _, x := range d.Devices {
		coverageClass := "ok"
		if x.CoveragePct < 50 {
			coverageClass = "bad"
		} else if x.CoveragePct < 90 {
			coverageClass = "warn"
		}
		issue := "no"
		issueClass := "ok"
		if x.SuspectedNameMismatch {
			issue = "yes"
			issueClass = "bad"
		}
		_, _ = fmt.Fprintf(w,
			"<tr><td><a href=\"/device/%s\">%s</a></td><td>%d</td><td>%d</td><td class=\"%s\">%.1f%%</td><td>%d</td><td class=\"%s\">%s</td></tr>",
			url.PathEscape(x.DeviceName), x.DeviceName, x.MonitoredInterfaces, x.RuntimeTelemetryInterfaces, coverageClass, x.CoveragePct, x.TelemetryEventsSeen, issueClass, issue)
	}
	_, _ = fmt.Fprint(w, "</tbody></table></body></html>")
}

// DevicePageData holds data for the device detail page
type DevicePageData struct {
	Device       DeviceDetailInfo
	Version      string
	Commit       string
	BuildDate    string
	BackURL      string
	BackLabel    string
	SNMPWarnings []SNMPUIWarning `json:"-"`
}

// DeviceDetailInfo holds detailed device information
type DeviceDetailInfo struct {
	Name                      string
	Address                   string
	Description               string
	LastSNMPValidationAt      time.Time
	LastTelemetryValidationAt time.Time
	Interfaces                []InterfaceInfo
	Logs                      []webui.LogEntry
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
	from := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("from")))
	backURL := "/"
	backLabel := "\u2190 Back to Dashboard"
	if from == "noc" {
		backURL = "/noc"
		backLabel = "\u2190 Back to NOC View"
	}

	data := DevicePageData{
		Device:       deviceDetail,
		Version:      version,
		Commit:       commit,
		BuildDate:    buildDate,
		BackURL:      backURL,
		BackLabel:    backLabel,
		SNMPWarnings: snmpUIWarnings(cfg),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := webui.Templates.ExecuteTemplate(w, "device", data); err != nil {
		s.logger.Error().Err(err).Msg("Failed to render device template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// snmpReachForHex maps device names to reachability strings for honeycomb merge (ok omitted).
func snmpReachForHex(tracker *collector.ReachabilityTracker, deviceNames []string) map[string]string {
	if len(deviceNames) == 0 {
		return nil
	}
	out := make(map[string]string)
	if tracker == nil {
		for _, dn := range deviceNames {
			out[dn] = collector.SNMPReachUnknown
		}
		return out
	}
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

func renderTelemetrySparkline(points []collector.EventRatePoint, width, height int) template.HTML {
	if len(points) == 0 || width <= 0 || height <= 0 {
		return template.HTML(`<svg viewBox="0 0 220 46" preserveAspectRatio="none" style="width:100%;height:46px;border:1px solid var(--border-color);border-radius:6px;background:var(--bg-primary)"><text x="50%" y="55%" text-anchor="middle" fill="var(--text-muted)" font-size="10">No telemetry in last 10m</text></svg>`)
	}
	var maxCount uint64
	for _, p := range points {
		if p.Count > maxCount {
			maxCount = p.Count
		}
	}
	if maxCount == 0 {
		return template.HTML(`<svg viewBox="0 0 220 46" preserveAspectRatio="none" style="width:100%;height:46px;border:1px solid var(--border-color);border-radius:6px;background:var(--bg-primary)"><text x="50%" y="55%" text-anchor="middle" fill="var(--text-muted)" font-size="10">No telemetry in last 10m</text></svg>`)
	}

	w := float64(width)
	h := float64(height)
	stepX := 0.0
	if len(points) > 1 {
		stepX = w / float64(len(points)-1)
	}

	var line strings.Builder
	for idx, p := range points {
		x := float64(idx) * stepX
		y := h - (float64(p.Count)/float64(maxCount))*h
		if y < 0 {
			y = 0
		}
		if y > h {
			y = h
		}
		if idx > 0 {
			line.WriteByte(' ')
		}
		line.WriteString(fmt.Sprintf("%.2f,%.2f", x, y))
	}

	return template.HTML(fmt.Sprintf(
		`<svg viewBox="0 0 %d %d" preserveAspectRatio="none" style="width:100%%;height:%dpx;border:1px solid var(--border-color);border-radius:6px;background:var(--bg-primary)"><polyline fill="none" stroke="var(--accent-blue)" stroke-width="1.8" points="%s" /></svg>`,
		width, height, height, line.String(),
	))
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
