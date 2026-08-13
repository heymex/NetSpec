// Package graph implements the NetSpecGraph HTTP UI and query API.
package graph

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/auth"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/graph/vm"
	"github.com/netspec/netspec/internal/version"
	"github.com/netspec/netspec/internal/webui"
	"github.com/rs/zerolog"
)

// Options configures the NetSpecGraph HTTP server.
type Options struct {
	Logger   zerolog.Logger
	Auth     *auth.Manager
	VM       *vm.Client
	Config   *config.Config
	Timezone string
}

// Server is the NetSpecGraph HTTP front-end.
type Server struct {
	log      zerolog.Logger
	auth     *auth.Manager
	vm       *vm.Client
	cfg      *config.Config
	index    *Index
	timezone string
}

// NewServer builds a Server from Options.
func NewServer(opts Options) *Server {
	return &Server{
		log:      opts.Logger,
		auth:     opts.Auth,
		vm:       opts.VM,
		cfg:      opts.Config,
		index:    BuildIndex(opts.Config),
		timezone: opts.Timezone,
	}
}

// Handler returns the root HTTP handler (auth gate + routes).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/api/vm/health", s.handleVMHealth)
	mux.HandleFunc("/api/roles", s.handleRolesAPI)
	mux.HandleFunc("/api/interfaces", s.handleInterfacesAPI)
	mux.HandleFunc("/api/device/", s.handleDeviceAPI)
	mux.HandleFunc("/device/", s.handleInterfacePage)
	mux.HandleFunc("/", s.handleIndex)
	return s.authGate(mux)
}

func (s *Server) authGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil || !s.auth.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		if p == "/health" || p == "/login" || p == "/logout" {
			next.ServeHTTP(w, r)
			return
		}
		if s.auth.IsAuthenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		if len(p) >= 4 && p[:4] == "/api" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":  "ok",
		"service": "netspecgraph",
		"version": version.GetVersion(),
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleVMHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := s.vm.Ping(ctx)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || !s.auth.Enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if s.auth.ValidatePassword(r.FormValue("password")) {
			id, err := s.auth.CreateSession()
			if err != nil {
				http.Error(w, "session error", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, s.auth.SessionCookie(id))
			next := r.FormValue("next")
			if next == "" {
				next = "/"
			}
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
		return
	}
	data := map[string]any{
		"Error": r.URL.Query().Get("error") == "1",
		"Next":  r.URL.Query().Get("next"),
	}
	if err := webui.LoginTemplate.Execute(w, data); err != nil {
		s.log.Error().Err(err).Msg("render login")
	}
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.auth != nil {
		if c, err := r.Cookie("netspec_session"); err == nil {
			s.auth.DeleteSession(c.Value)
		}
		http.SetCookie(w, s.auth.ClearCookie())
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		device := r.FormValue("device")
		iface := r.FormValue("interface")
		if device == "" || iface == "" {
			http.Error(w, "device and interface required", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, interfacePagePath(device, iface), http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	deviceCount, ifaceCount := 0, 0
	portRoles := []string{}
	deviceRoles := []RoleInfo{}
	if s.index != nil {
		deviceCount = s.index.DeviceCount()
		ifaceCount = s.index.Len()
		portRoles = s.index.PortRoleLabels()
		deviceRoles = s.index.Roles()
	}
	data := map[string]any{
		"Version":     version.GetVersion(),
		"Timezone":    s.timezone,
		"DeviceCount": deviceCount,
		"IfaceCount":  ifaceCount,
		"PortRoles":   portRoles,
		"DeviceRoles": deviceRoles,
		"ExamplePath": interfacePagePath("csw-mcd-01", "Port-channel20"),
	}
	if err := indexTemplate.Execute(w, data); err != nil {
		s.log.Error().Err(err).Msg("render index")
	}
}

func (s *Server) handleInterfacePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	escaped := r.URL.EscapedPath()
	device, iface, ok := parseDeviceInterfacePath(escaped)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{
		"Device":       device,
		"Interface":    iface,
		"SeriesURLJS":  template.JS(strconv.Quote(interfaceSeriesAPIPath(device, iface))),
		"Timezone":     s.timezone,
		"Version":      version.GetVersion(),
		"DefaultRange": "6h",
		"PortRole":     "",
		"DeviceRole":   "",
		"Alias":        "",
		"Monitored":    "",
		"DesiredState": "",
		"InConfig":     false,
	}
	if s.index != nil {
		if id, ok := s.index.Lookup(device, iface); ok {
			data["PortRole"] = id.PortRole
			data["DeviceRole"] = id.DeviceRole
			data["Alias"] = id.Alias
			data["Monitored"] = strconv.FormatBool(id.Monitored)
			data["DesiredState"] = id.DesiredState
			data["InConfig"] = true
		}
	}
	if err := ifaceTemplate.Execute(w, data); err != nil {
		s.log.Error().Err(err).Msg("render interface page")
	}
}

func (s *Server) handleRolesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	roles := []RoleInfo{}
	labels := []string{}
	if s.index != nil {
		roles = s.index.Roles()
		labels = s.index.PortRoleLabels()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"device_roles":     roles,
		"port_role_labels": labels,
	})
}

func (s *Server) handleInterfacesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	f := Filter{
		Device:       q.Get("device"),
		DevicePrefix: firstNonEmpty(q.Get("device_prefix"), q.Get("role_prefix")),
		DeviceRole:   q.Get("device_role"),
		PortRole:     firstNonEmpty(q.Get("port_role"), q.Get("role")),
		DesiredState: q.Get("desired_state"),
		Query:        q.Get("q"),
	}
	if v := q.Get("monitored"); v != "" {
		b := v == "1" || v == "true" || v == "yes"
		f.Monitored = &b
	}
	items := []InterfaceIdentity{}
	if s.index != nil {
		items = s.index.Filter(f)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count":      len(items),
		"interfaces": items,
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *Server) handleDeviceAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	escaped := r.URL.EscapedPath()
	if strings.HasSuffix(escaped, "/meta") {
		s.handleInterfaceMetaAPI(w, r)
		return
	}
	s.handleInterfaceSeriesAPI(w, r)
}

func (s *Server) handleInterfaceMetaAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.EscapedPath(), "/meta")
	device, iface, ok := parseDeviceInterfacePath(path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if s.index == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "no identity index"})
		return
	}
	id, ok := s.index.Lookup(device, iface)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":     "interface not in desired-state",
			"device":    device,
			"interface": iface,
		})
		return
	}
	_ = json.NewEncoder(w).Encode(id)
}

func (s *Server) handleInterfaceSeriesAPI(w http.ResponseWriter, r *http.Request) {
	device, iface, ok := parseDeviceInterfacePath(r.URL.EscapedPath())
	if !ok {
		http.NotFound(w, r)
		return
	}
	window := 6 * time.Hour
	if v := r.URL.Query().Get("range"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 && d <= 48*time.Hour {
			window = d
		}
	}
	step := 30 * time.Second
	if v := r.URL.Query().Get("step"); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec >= 10 && sec <= 300 {
			step = time.Duration(sec) * time.Second
		}
	}

	ctx := r.Context()
	payload, err := FetchInterfaceSeries(ctx, s.vm, device, iface, time.Now().UTC(), window, step)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		s.log.Error().Err(err).Str("device", device).Str("interface", iface).Msg("series query failed")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}
