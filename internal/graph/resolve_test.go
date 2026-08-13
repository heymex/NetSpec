package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/netspec/netspec/internal/graph/vm"
)

func TestResolveTelemetryInterfaceShortToLong(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/label/interface/values", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   []string{"GigabitEthernet1/0/1", "TenGigabitEthernet1/1/1", "Port-channel20"},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Bypass package cache pollution across tests by using a fresh device name.
	client := vm.NewClient(srv.URL)
	got, err := ResolveTelemetryInterface(context.Background(), client, "asw-resolve-test-01", "Gi1/0/1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "GigabitEthernet1/0/1" {
		t.Fatalf("got %q, want GigabitEthernet1/0/1", got)
	}
	got, err = ResolveTelemetryInterface(context.Background(), client, "asw-resolve-test-01", "Te1/1/1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "TenGigabitEthernet1/1/1" {
		t.Fatalf("got %q, want TenGigabitEthernet1/1/1", got)
	}
}

func TestResolveTelemetryInterfacePassthrough(t *testing.T) {
	got, err := ResolveTelemetryInterface(context.Background(), nil, "x", "Gi1/0/1")
	if err != nil || got != "Gi1/0/1" {
		t.Fatalf("nil client: got %q err=%v", got, err)
	}
}
