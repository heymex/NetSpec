package notifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestBuildNotifyRequest_StatelessWithURLs(t *testing.T) {
	u, payload, err := buildNotifyRequest(
		"http://localhost:8086",
		"slack://tokenA/tokenB/tokenC",
		"NetSpec: warning",
		"body",
		"warning",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(u, "/notify") && !strings.HasSuffix(u, "/notify/") {
		t.Fatalf("unexpected URL %q", u)
	}
	if payload["urls"] != "slack://tokenA/tokenB/tokenC" {
		t.Fatalf("urls: %q", payload["urls"])
	}
	if payload["type"] != "warning" {
		t.Fatalf("type: %q", payload["type"])
	}
}

func TestBuildNotifyRequest_KeyedNotify(t *testing.T) {
	u, payload, err := buildNotifyRequest(
		"http://apprise:8000/",
		"ops-alerts",
		"t",
		"b",
		"info",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, "/notify/ops-alerts") {
		t.Fatalf("expected keyed path, got %q", u)
	}
	if _, ok := payload["urls"]; ok {
		t.Fatal("keyed notify should not set urls")
	}
}

func TestBuildNotifyRequest_InvalidBase(t *testing.T) {
	_, _, err := buildNotifyRequest("not-a-url", "slack://x", "t", "b", "info")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildNotifyRequest_InvalidServiceValue(t *testing.T) {
	_, _, err := buildNotifyRequest("http://localhost:8086/", "bad key!", "t", "b", "info")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeliver_StatelessIntegration(t *testing.T) {
	var gotPath string
	var got map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":null}`))
	}))
	defer srv.Close()

	n := &Notifier{
		logger: testLogger(),
		client: srv.Client(),
	}
	if err := n.deliver(srv.URL, "https://example.com/hook", "Title", "Body", "failure", "ops-slack", "https://(redacted)"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/notify" && gotPath != "/notify/" {
		t.Fatalf("path %q", gotPath)
	}
	if got["urls"] != "https://example.com/hook" {
		t.Fatalf("urls %q", got["urls"])
	}
	if got["type"] != "failure" {
		t.Fatalf("type %q", got["type"])
	}
}

func testLogger() zerolog.Logger {
	return zerolog.Nop()
}

func TestScrubServiceURL(t *testing.T) {
	if g := scrubServiceURL("slack://a/b/c"); g != "slack://(redacted)" {
		t.Fatalf("got %q", g)
	}
	if g := scrubServiceURL("ops-key"); g != "ops-key" {
		t.Fatalf("got %q", g)
	}
}

func TestSummarizeAppriseAPIError_JSON(t *testing.T) {
	body := []byte(`{"error":"One or more notifications could not be sent","details":[["WARN","t","msg"]]}`)
	s := summarizeAppriseAPIError(424, body)
	if !strings.Contains(s, "424") || !strings.Contains(s, "One or more") {
		t.Fatalf("got %q", s)
	}
}

func TestSummarizeAppriseAPIError_Raw(t *testing.T) {
	s := summarizeAppriseAPIError(500, []byte("plain"))
	if !strings.Contains(s, "500") || !strings.Contains(s, "plain") {
		t.Fatalf("got %q", s)
	}
}
