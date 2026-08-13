// Package graph implements the NetSpecGraph HTTP UI and query API.
package graph

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
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
	timezone string
}

// NewServer builds a Server from Options.
func NewServer(opts Options) *Server {
	return &Server{
		log:      opts.Logger,
		auth:     opts.Auth,
		vm:       opts.VM,
		cfg:      opts.Config,
		timezone: opts.Timezone,
	}
}

// Handler returns the root HTTP handler (auth gate + routes).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/vm/health", s.handleVMHealth)
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
	loginErr := r.URL.Query().Get("error") == "1"
	data := map[string]any{
		"Error": loginErr,
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
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	deviceCount := 0
	if s.cfg != nil {
		deviceCount = len(s.cfg.DesiredState.Devices)
	}
	data := map[string]any{
		"Version":    version.GetVersion(),
		"Timezone":   s.timezone,
		"DeviceCount": deviceCount,
		"VMURL":      s.vm.BaseURL(),
	}
	if err := indexTemplate.Execute(w, data); err != nil {
		s.log.Error().Err(err).Msg("render index")
	}
}

var indexTemplate = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>NetSpecGraph</title>
  <style>
    :root { color-scheme: dark; --bg:#0d1117; --fg:#e6edf3; --muted:#8b949e; --accent:#58a6ff; --bd:#30363d; }
    body { margin:0; font-family: ui-sans-serif, system-ui, sans-serif; background:var(--bg); color:var(--fg); }
    main { max-width: 42rem; margin: 3rem auto; padding: 0 1.25rem; }
    h1 { font-size: 1.75rem; margin: 0 0 0.5rem; letter-spacing: -0.02em; }
    p { color: var(--muted); line-height: 1.5; }
    code { background:#161b22; padding:0.1em 0.35em; border-radius:4px; border:1px solid var(--bd); }
    .meta { margin-top: 2rem; font-size: 0.85rem; color: var(--muted); }
    a { color: var(--accent); }
  </style>
</head>
<body>
  <main>
    <h1>NetSpecGraph</h1>
    <p>Metrics companion to NetSpec. Vertical-slice UI (per-interface uPlot) lands after VictoriaMetrics shows live counters and the Telegraf rename contract is frozen.</p>
    <p>Loaded <strong>{{.DeviceCount}}</strong> devices from NetSpec config · timezone <code>{{.Timezone}}</code></p>
    <p class="meta">version {{.Version}} · VM <code>{{.VMURL}}</code> · <a href="/health">/health</a> · <a href="/api/vm/health">/api/vm/health</a></p>
  </main>
</body>
</html>
`))
