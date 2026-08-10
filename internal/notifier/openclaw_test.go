package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

func TestBuildOpenClawPayload_Firing(t *testing.T) {
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

	payload := buildOpenClawPayload(alert, "https://netspec.example")
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

func TestBuildOpenClawPayload_ResolvedOmitsLinksWithoutPublicURL(t *testing.T) {
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
	payload := buildOpenClawPayload(alert, "")
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

func TestSendAlert_OpenClaw(t *testing.T) {
	var (
		gotAuth   string
		gotToken  string
		gotBody   openClawWebhookPayload
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
	// Ensure Apprise is not required for openclaw-only delivery.
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

	alert := &types.Alert{
		ID:        "core-sw-01|Port-channel10|port_channel_degraded-1",
		Device:    "core-sw-01",
		Entity:    "Port-channel10",
		AlertType: "port_channel_degraded",
		Severity:  "critical",
		State:     "firing",
		FiredAt:   time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC),
		Message:   "members down",
	}
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

func TestSendAlert_OpenClawSeverityFilter(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("OPENCLAW_WEBHOOK_URL", srv.URL)
	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-openclaw": {
			Type:           "openclaw",
			URLEnv:         "OPENCLAW_WEBHOOK_URL",
			SeverityFilter: []string{"critical"},
		},
	})
	n.client = srv.Client()

	alert := &types.Alert{
		ID: "x", Device: "d", Entity: "e", AlertType: "t",
		Severity: "warning", State: "firing", FiredAt: time.Now().UTC(), Message: "m",
	}
	if err := n.SendAlert(alert, []string{"ops-openclaw"}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("expected severity_filter to skip delivery")
	}
}
