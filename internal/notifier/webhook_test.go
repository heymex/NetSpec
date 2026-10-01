package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

func TestBuildWebhookPayload_Firing(t *testing.T) {
	fired := time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC)
	alert := &types.Alert{
		ID:        "core-sw-01|Port-channel10|port_channel_degraded-1723312800000",
		Device:    "core-sw-01",
		Entity:    "Port-channel10",
		AlertType: "port_channel_degraded",
		Severity:  "critical",
		State:     "firing",
		FiredAt:   fired,
		Message:   "Port-channel degraded",
	}

	payload := buildWebhookPayload(alert, "https://netspec.example")
	if payload.Event != "alert.firing" {
		t.Fatalf("event: %q", payload.Event)
	}
	if payload.Alert.AlertType != "port_channel_degraded" {
		t.Fatalf("alert_type: %q", payload.Alert.AlertType)
	}
	if payload.Alert.RelatedState == nil {
		t.Fatal("related_state should be non-nil empty map")
	}
	if payload.Links == nil {
		t.Fatal("expected links")
	}
	if payload.Links.Alert != "https://netspec.example/alerts" {
		t.Fatalf("links.alert: %q", payload.Links.Alert)
	}
	if payload.Links.Device != "https://netspec.example/device/core-sw-01" {
		t.Fatalf("links.device: %q", payload.Links.Device)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	alertObj, ok := decoded["alert"].(map[string]any)
	if !ok {
		t.Fatalf("alert object: %#v", decoded["alert"])
	}
	for _, key := range []string{"id", "device", "entity", "alert_type", "severity", "state", "fired_at", "message", "related_state"} {
		if _, ok := alertObj[key]; !ok {
			t.Fatalf("missing alert.%s in JSON", key)
		}
	}
}

func TestBuildWebhookPayload_ResolvedOmitsLinksWithoutPublicURL(t *testing.T) {
	alert := &types.Alert{
		ID:        "d|e|t-1",
		Device:    "d",
		Entity:    "e",
		AlertType: "t",
		Severity:  "warning",
		State:     "resolved",
		FiredAt:   time.Now().UTC(),
		Message:   "ok",
		RelatedState: map[string]string{
			"oper": "up",
		},
	}
	payload := buildWebhookPayload(alert, "")
	if payload.Event != "alert.resolved" {
		t.Fatalf("event: %q", payload.Event)
	}
	if payload.Links != nil {
		t.Fatalf("expected nil links, got %#v", payload.Links)
	}
	if payload.Alert.RelatedState["oper"] != "up" {
		t.Fatalf("related_state: %#v", payload.Alert.RelatedState)
	}
}

func TestSendAlert_OpenClawAlias(t *testing.T) {
	var (
		gotAuth   string
		gotToken  string
		gotBody   webhookPayload
		gotMethod string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotToken = r.Header.Get("x-openclaw-token")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	t.Setenv("OPENCLAW_WEBHOOK_URL", srv.URL+"/hooks/netspec")
	t.Setenv("OPENCLAW_HOOK_TOKEN", "shared-secret")
	t.Setenv("NETSPEC_PUBLIC_URL", "https://netspec.example")
	_ = os.Unsetenv("APPRISE_API_URL")

	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-openclaw": {
			Type:           "openclaw",
			URLEnv:         "OPENCLAW_WEBHOOK_URL",
			TokenEnv:       "OPENCLAW_HOOK_TOKEN",
			SeverityFilter: []string{"critical"},
		},
	})
	n.client = srv.Client()

	alert := firingAlert()
	if err := n.SendAlert(alert, []string{"ops-openclaw"}); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method: %q", gotMethod)
	}
	if gotAuth != "Bearer shared-secret" {
		t.Fatalf("Authorization: %q", gotAuth)
	}
	if gotToken != "shared-secret" {
		t.Fatalf("x-openclaw-token: %q", gotToken)
	}
	if gotBody.Event != "alert.firing" {
		t.Fatalf("event: %q", gotBody.Event)
	}
	if gotBody.Alert.Device != "core-sw-01" {
		t.Fatalf("device: %q", gotBody.Alert.Device)
	}
	if gotBody.Links == nil || gotBody.Links.Device != "https://netspec.example/device/core-sw-01" {
		t.Fatalf("links: %#v", gotBody.Links)
	}
}

func TestSendAlert_WebhookHeaderAuth(t *testing.T) {
	var gotAuth, gotRouting string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotRouting = r.Header.Get("X-Routing-Key")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	t.Setenv("PAGER_WEBHOOK_URL", srv.URL)
	t.Setenv("PAGER_ROUTING_KEY", "route-key")

	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-pager": {
			Type:       "webhook",
			URLEnv:     "PAGER_WEBHOOK_URL",
			TokenEnv:   "PAGER_ROUTING_KEY",
			Auth:       "header",
			HeaderName: "X-Routing-Key",
		},
	})
	n.client = srv.Client()

	if err := n.SendAlert(firingAlert(), []string{"ops-pager"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization: %q", gotAuth)
	}
	if gotRouting != "route-key" {
		t.Fatalf("X-Routing-Key: %q", gotRouting)
	}
}

func TestSendAlert_WebhookMultipleEndpoints(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	record := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method", http.StatusMethodNotAllowed)
				return
			}
			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				http.Error(w, "content-type", http.StatusBadRequest)
				return
			}
			body, _ := io.ReadAll(r.Body)
			var payload webhookPayload
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if payload.Event != "alert.firing" || payload.Alert.ID == "" {
				http.Error(w, "payload", http.StatusBadRequest)
				return
			}
			mu.Lock()
			hits[name]++
			mu.Unlock()
			if name == "broken" {
				http.Error(w, "nope", http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusOK)
		}
	}

	openclawSrv := httptest.NewServer(record("openclaw"))
	defer openclawSrv.Close()
	internalSrv := httptest.NewServer(record("internal"))
	defer internalSrv.Close()
	brokenSrv := httptest.NewServer(record("broken"))
	defer brokenSrv.Close()

	t.Setenv("OPENCLAW_WEBHOOK_URL", openclawSrv.URL)
	t.Setenv("OPENCLAW_HOOK_TOKEN", "claw-token")
	t.Setenv("INTERNAL_WEBHOOK_URL", internalSrv.URL)
	t.Setenv("BROKEN_WEBHOOK_URL", brokenSrv.URL)

	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-hooks": {
			Type: "webhook",
			Endpoints: []config.WebhookEndpoint{
				{
					Name:       "openclaw",
					URLEnv:     "OPENCLAW_WEBHOOK_URL",
					TokenEnv:   "OPENCLAW_HOOK_TOKEN",
					Auth:       "bearer_and_header",
					HeaderName: "x-openclaw-token",
				},
				{
					Name:   "internal",
					URLEnv: "INTERNAL_WEBHOOK_URL",
				},
				{
					Name:   "broken",
					URLEnv: "BROKEN_WEBHOOK_URL",
				},
			},
		},
	})

	err := n.SendAlert(firingAlert(), []string{"ops-hooks"})
	if err == nil {
		t.Fatal("expected partial failure")
	}
	if !strings.Contains(err.Error(), "endpoint broken") {
		t.Fatalf("error should name the failed endpoint: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"openclaw", "internal", "broken"} {
		if hits[name] != 1 {
			t.Fatalf("endpoint %s hits: %d", name, hits[name])
		}
	}
}

func TestSendAlert_WebhookSeverityFilter(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("HOOK_URL", srv.URL)
	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-hook": {
			Type:           "webhook",
			URLEnv:         "HOOK_URL",
			SeverityFilter: []string{"critical"},
		},
	})
	n.client = srv.Client()

	alert := firingAlert()
	alert.Severity = "warning"
	if err := n.SendAlert(alert, []string{"ops-hook"}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("expected severity_filter to skip delivery")
	}
}

func firingAlert() *types.Alert {
	return &types.Alert{
		ID:        "core-sw-01|Port-channel10|port_channel_degraded-1",
		Device:    "core-sw-01",
		Entity:    "Port-channel10",
		AlertType: "port_channel_degraded",
		Severity:  "critical",
		State:     "firing",
		FiredAt:   time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC),
		Message:   "members down",
	}
}
