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
	Logger           zerolog.Logger
	Auth             *auth.Manager
	VM               *vm.Client
	Config           *config.Config
	Timezone         string
	Location         *time.Location
	BandWindow       time.Duration // trailing seasonality window; default 21d
	NetSpecPublicURL string        // optional browser-reachable NetSpec origin for deep-links
}

// Server is the NetSpecGraph HTTP front-end.
type Server struct {
	log              zerolog.Logger
	auth             *auth.Manager
	vm               *vm.Client
	cfg              *config.Config
	index            *Index
	timezone         string
	location         *time.Location
	bandWindow       time.Duration
	netspecPublicURL string
}

// NewServer builds a Server from Options.
func NewServer(opts Options) *Server {
	loc := opts.Location
	if loc == nil {
		loc = time.UTC
		if opts.Timezone != "" {
			if l, err := time.LoadLocation(opts.Timezone); err == nil {
				loc = l
			}
		}
	}
	bw := opts.BandWindow
	if bw == 0 {
		bw = 21 * 24 * time.Hour
	}
	return &Server{
		log:              opts.Logger,
		auth:             opts.Auth,
		vm:               opts.VM,
		cfg:              opts.Config,
		index:            BuildIndex(opts.Config),
		timezone:         opts.Timezone,
		location:         loc,
		bandWindow:       bw,
		netspecPublicURL: strings.TrimRight(strings.TrimSpace(opts.NetSpecPublicURL), "/"),
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
	mux.HandleFunc("/api/fleet/top", s.handleFleetTopAPI)
	mux.HandleFunc("/api/device/", s.handleDeviceAPI)
	mux.HandleFunc("/fleet", s.handleFleetPage)
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
	portRoles := []PortRoleCount{}
	deviceRoles := []RoleInfo{}
	if s.index != nil {
		deviceCount = s.index.DeviceCount()
		ifaceCount = s.index.Len()
		portRoles = s.index.PortRoleCounts()
		deviceRoles = s.index.Roles()
	}
	data := map[string]any{
		"Version":          version.GetVersion(),
		"Timezone":         s.timezone,
		"DeviceCount":      deviceCount,
		"IfaceCount":       ifaceCount,
		"PortRoles":        portRoles,
		"DeviceRoles":      deviceRoles,
		"ExamplePath":      interfacePagePath("csw-mcd-01", "Port-channel20"),
		"NetSpecPublicURL": s.netspecPublicURL,
	}
	if err := indexTemplate.Execute(w, data); err != nil {
		s.log.Error().Err(err).Msg("render index")
	}
}

func (s *Server) fleetOptsFromRequest(r *http.Request) FleetOptions {
	q := r.URL.Query()
	opts := FleetOptions{
		PortRole:       firstNonEmpty(q.Get("port_role"), q.Get("role")),
		DevicePrefix:   firstNonEmpty(q.Get("device_prefix"), q.Get("role_prefix")),
		Device:         q.Get("device"),
		NetSpecBaseURL: s.netspecPublicURL,
		Limit:          25,
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			opts.Limit = n
		}
	}
	switch q.Get("monitored") {
	case "1", "true", "yes":
		opts.MonitoredOnly = true
	}
	if win := q.Get("rate_window"); win != "" {
		if d, err := time.ParseDuration(win); err == nil && d >= time.Minute && d <= time.Hour {
			opts.RateWindow = d
		}
	}
	return opts
}

func (s *Server) handleFleetTopAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snap, err := FetchFleetSnapshot(r.Context(), s.vm, s.index, s.fleetOptsFromRequest(r))
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		s.log.Error().Err(err).Msg("fleet snapshot failed")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(snap)
}

func (s *Server) handleFleetPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	opts := s.fleetOptsFromRequest(r)
	// HTML default: Port-Channel uplinks (NOC uplink heat), unless caller cleared with port_role=all.
	if r.URL.Query().Get("port_role") == "" && r.URL.Query().Get("role") == "" {
		opts.PortRole = "Port-Channel Uplinks"
	}
	if r.URL.Query().Get("port_role") == "all" || r.URL.Query().Get("role") == "all" {
		opts.PortRole = ""
	}
	snap, err := FetchFleetSnapshot(r.Context(), s.vm, s.index, opts)
	if err != nil {
		s.log.Error().Err(err).Msg("fleet page snapshot failed")
		http.Error(w, "fleet query failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	portRoles := []PortRoleCount{}
	deviceRoles := []RoleInfo{}
	if s.index != nil {
		portRoles = s.index.PortRoleCounts()
		deviceRoles = s.index.Roles()
	}
	hex := ""
	if snap != nil {
		q := url.Values{}
		if opts.PortRole != "" {
			q.Set("port_role", opts.PortRole)
		} else {
			q.Set("port_role", "all")
		}
		if opts.DevicePrefix != "" {
			q.Set("device_prefix", opts.DevicePrefix)
		}
		if opts.Limit != 25 {
			q.Set("limit", strconv.Itoa(opts.Limit))
		}
		linkBase := "/fleet"
		if enc := q.Encode(); enc != "" {
			linkBase += "?" + enc
		}
		hex = RenderFleetHexSVG(snap.Devices, linkBase)
	}
	data := map[string]any{
		"Version":          version.GetVersion(),
		"Timezone":         s.timezone,
		"PortRole":         opts.PortRole,
		"Device":           opts.Device,
		"DevicePrefix":     opts.DevicePrefix,
		"Limit":            opts.Limit,
		"PortRoles":        portRoles,
		"DeviceRoles":      deviceRoles,
		"Snapshot":         snap,
		"HexSVG":           template.HTML(hex),
		"NetSpecPublicURL": s.netspecPublicURL,
		"Error":            "",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := fleetTemplate.Execute(w, data); err != nil {
		s.log.Error().Err(err).Msg("render fleet page")
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
	if pathIsOptics(escaped) {
		data := map[string]any{
			"Device":           device,
			"Interface":        iface,
			"SeriesURLJS":      template.JS(strconv.Quote(opticsSeriesAPIPath(device, iface))),
			"TrafficPath":      interfacePagePath(device, iface),
			"Timezone":         s.timezone,
			"Version":          version.GetVersion(),
			"NetSpecPublicURL": s.netspecPublicURL,
			"NetSpecDeviceURL": netspecDevicePath(s.netspecPublicURL, device),
		}
		if err := opticsTemplate.Execute(w, data); err != nil {
			s.log.Error().Err(err).Msg("render optics page")
		}
		return
	}
	data := map[string]any{
		"Device":           device,
		"Interface":        iface,
		"SeriesURLJS":      template.JS(strconv.Quote(interfaceSeriesAPIPath(device, iface))),
		"OpticsPath":       opticsPagePath(device, iface),
		"Timezone":         s.timezone,
		"Version":          version.GetVersion(),
		"DefaultRange":     "6h",
		"PortRole":         "",
		"DeviceRole":       "",
		"Alias":            "",
		"Monitored":        "",
		"DesiredState":     "",
		"InConfig":         false,
		"NetSpecPublicURL": s.netspecPublicURL,
		"NetSpecDeviceURL": netspecDevicePath(s.netspecPublicURL, device),
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
	counts := []PortRoleCount{}
	if s.index != nil {
		roles = s.index.Roles()
		labels = s.index.PortRoleLabels()
		counts = s.index.PortRoleCounts()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"device_roles":     roles,
		"port_role_labels": labels,
		"port_role_counts": counts,
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
	if pathIsOptics(escaped) {
		s.handleOpticsSeriesAPI(w, r)
		return
	}
	s.handleInterfaceSeriesAPI(w, r)
}

func (s *Server) handleOpticsSeriesAPI(w http.ResponseWriter, r *http.Request) {
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
	payload, err := FetchOpticsSeries(ctx, s.vm, device, iface, time.Now().UTC(), window, step)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		s.log.Error().Err(err).Str("device", device).Str("interface", iface).Msg("optics series query failed")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
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

	opts := SeriesOptions{
		Location:   s.location,
		BandWindow: s.bandWindow,
	}
	switch r.URL.Query().Get("band") {
	case "0", "false", "off", "no":
		opts.BandWindow = -1
	}
	switch r.URL.Query().Get("baseline") {
	case "1w", "week":
		opts.Baseline = 7 * 24 * time.Hour
		opts.BaselineLabel = "1 week ago"
	case "52w", "year":
		opts.Baseline = 52 * 7 * 24 * time.Hour // weekday-aligned year
		opts.BaselineLabel = "same week last year"
	}

	ctx := r.Context()
	payload, err := FetchInterfaceSeriesOpts(ctx, s.vm, device, iface, time.Now().UTC(), window, step, opts)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		s.log.Error().Err(err).Str("device", device).Str("interface", iface).Msg("series query failed")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}
