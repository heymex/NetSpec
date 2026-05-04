package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestOpenAPIJSONValidAndServed(t *testing.T) {
	var doc map[string]interface{}
	if err := json.Unmarshal(openAPISpec, &doc); err != nil {
		t.Fatalf("openapi.json: %v", err)
	}
	if doc["openapi"] != "3.0.3" {
		t.Fatalf("expected openapi 3.0.3, got %v", doc["openapi"])
	}

	s := NewServer(nil, zerolog.Nop(), "8080")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	s.handleOpenAPIJSON(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("content-type: %q", ct)
	}
}

func TestAPIBrowserServed(t *testing.T) {
	s := NewServer(nil, zerolog.Nop(), "8080")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api-browser", nil)
	s.handleAPIBrowser(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if len(body) < 500 || !strings.Contains(body, "SwaggerUIBundle") {
		t.Fatal("expected swagger shell HTML")
	}
}
