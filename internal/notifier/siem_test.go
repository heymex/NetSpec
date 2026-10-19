package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/rs/zerolog"
)

func TestSendAlert_SplunkHEC(t *testing.T) {
	var (
		gotAuth   string
		gotMethod string
		gotBody   splunkHECPayload
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"text":"Success","code":0}`))
	}))
	defer srv.Close()

	t.Setenv("SPLUNK_HEC_URL", srv.URL+"/services/collector/event")
	t.Setenv("SPLUNK_HEC_TOKEN", "hec-secret")
	t.Setenv("NETSPEC_PUBLIC_URL", "https://netspec.example")

	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-splunk": {
			Type:       "splunk",
			URLEnv:     "SPLUNK_HEC_URL",
			TokenEnv:   "SPLUNK_HEC_TOKEN",
			Index:      "netops",
			Sourcetype: "netspec:alert",
			Source:     "netspec",
		},
	})
	n.client = srv.Client()

	if err := n.SendAlert(firingAlert(), []string{"ops-splunk"}); err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method: %q", gotMethod)
	}
	if gotAuth != "Splunk hec-secret" {
		t.Fatalf("Authorization: %q", gotAuth)
	}
	if gotBody.Event.Event != "alert.firing" {
		t.Fatalf("event.event: %q", gotBody.Event.Event)
	}
	if gotBody.Host != "core-sw-01" {
		t.Fatalf("host: %q", gotBody.Host)
	}
	if gotBody.Index != "netops" {
		t.Fatalf("index: %q", gotBody.Index)
	}
	if gotBody.SourceType != "netspec:alert" {
		t.Fatalf("sourcetype: %q", gotBody.SourceType)
	}
	if gotBody.Event.Links == nil || gotBody.Event.Links.Alert != "https://netspec.example/alerts" {
		t.Fatalf("links: %#v", gotBody.Event.Links)
	}
	wantTime := float64(time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC).UnixNano()) / 1e9
	if gotBody.Time != wantTime {
		t.Fatalf("time: got %v want %v", gotBody.Time, wantTime)
	}
}

func TestSendAlert_ElasticsearchAPIKey(t *testing.T) {
	var (
		gotAuth string
		gotPath string
		gotDoc  map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotDoc); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"result":"created"}`))
	}))
	defer srv.Close()

	t.Setenv("ELASTIC_URL", srv.URL)
	t.Setenv("ELASTIC_API_KEY", "es-key")

	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-elastic": {
			Type:     "elasticsearch",
			URLEnv:   "ELASTIC_URL",
			TokenEnv: "ELASTIC_API_KEY",
			Index:    "netspec-alerts",
		},
	})
	n.client = srv.Client()

	if err := n.SendAlert(firingAlert(), []string{"ops-elastic"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "ApiKey es-key" {
		t.Fatalf("Authorization: %q", gotAuth)
	}
	if gotPath != "/netspec-alerts/_doc" {
		t.Fatalf("path: %q", gotPath)
	}
	if gotDoc["event"] != "alert.firing" {
		t.Fatalf("event: %#v", gotDoc["event"])
	}
	if _, ok := gotDoc["@timestamp"]; !ok {
		t.Fatal("missing @timestamp")
	}
	alertObj, ok := gotDoc["alert"].(map[string]any)
	if !ok || alertObj["device"] != "core-sw-01" {
		t.Fatalf("alert: %#v", gotDoc["alert"])
	}
}

func TestSendAlert_ElasticAliasBearerFullURL(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("ELASTIC_URL", srv.URL+"/custom-alerts/_create")
	t.Setenv("ELASTIC_TOKEN", "bearer-tok")

	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-elastic": {
			Type:     "elastic",
			URLEnv:   "ELASTIC_URL",
			TokenEnv: "ELASTIC_TOKEN",
			Auth:     "bearer",
		},
	})
	n.client = srv.Client()

	if err := n.SendAlert(firingAlert(), []string{"ops-elastic"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer bearer-tok" {
		t.Fatalf("Authorization: %q", gotAuth)
	}
	if gotPath != "/custom-alerts/_create" {
		t.Fatalf("path should be unchanged: %q", gotPath)
	}
}

func TestResolveElasticDocURL(t *testing.T) {
	got, err := resolveElasticDocURL("https://es.example:9200", "netspec-alerts")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://es.example:9200/netspec-alerts/_doc" {
		t.Fatalf("got %q", got)
	}

	got, err = resolveElasticDocURL("https://es.example:9200/netspec-alerts/_doc", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://es.example:9200/netspec-alerts/_doc" {
		t.Fatalf("got %q", got)
	}

	_, err = resolveElasticDocURL("https://es.example:9200", "")
	if err == nil || !strings.Contains(err.Error(), "index is required") {
		t.Fatalf("expected index required, got %v", err)
	}
}

func TestSendAlert_SplunkMissingToken(t *testing.T) {
	t.Setenv("SPLUNK_HEC_URL", "https://splunk.example:8088/services/collector")
	t.Setenv("SPLUNK_HEC_TOKEN", "")

	n := NewNotifier(zerolog.Nop(), map[string]config.ChannelConfig{
		"ops-splunk": {
			Type:     "splunk",
			URLEnv:   "SPLUNK_HEC_URL",
			TokenEnv: "SPLUNK_HEC_TOKEN",
		},
	})
	err := n.SendAlert(firingAlert(), []string{"ops-splunk"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "SPLUNK_HEC_TOKEN") {
		t.Fatalf("error: %v", err)
	}
}
