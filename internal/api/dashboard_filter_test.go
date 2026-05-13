package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/notifier"
	"github.com/netspec/netspec/internal/types"
	"github.com/rs/zerolog"
)

// TestHandleDevicesAPI_RoleTagging verifies that /api/devices stamps each
// device with role_name/role_prefix derived from rules.yaml longest-prefix
// matching. Devices that match no rule must still be present with empty
// role fields so the dashboard's "Other" filter bucket works.
func TestHandleDevicesAPI_RoleTagging(t *testing.T) {
	t.Parallel()
	log := zerolog.New(io.Discard)

	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			AlertBehavior: config.AlertBehavior{DeduplicationWindow: time.Minute},
		},
		Rules: config.RulesConfig{
			DeviceRoles: []config.DeviceRole{
				{Name: "Core Switch", Prefix: "csw"},
				{Name: "Access Switch", Prefix: "asw"},
				{Name: "Outdoor Switch", Prefix: "osw"},
			},
		},
	}
	cfg.DesiredState.Devices = map[string]config.DeviceConfig{
		"csw-hcd-01":   {Address: "10.0.0.1"},
		"asw-hcd-02":   {Address: "10.0.0.2"},
		"osw-ms1-01":   {Address: "10.0.0.3"},
		"mystery-host": {Address: "10.0.0.4"},
	}

	eng := alerter.NewEngine(cfg, notifier.NewNotifier(log, nil), log, "")
	srv := NewServer(eng, log, "8088")
	srv.SetConfig(cfg, "/tmp/desired-state.yaml")

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rec := httptest.NewRecorder()
	srv.handleDevicesAPI(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body struct {
		Devices []map[string]any `json:"devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]struct{ Name, Prefix string }{
		"csw-hcd-01":   {"Core Switch", "csw"},
		"asw-hcd-02":   {"Access Switch", "asw"},
		"osw-ms1-01":   {"Outdoor Switch", "osw"},
		"mystery-host": {"", ""},
	}
	if len(body.Devices) != len(want) {
		t.Fatalf("device count: want %d, got %d", len(want), len(body.Devices))
	}
	for _, row := range body.Devices {
		name, _ := row["name"].(string)
		exp, ok := want[name]
		if !ok {
			t.Errorf("unexpected device %q", name)
			continue
		}
		gotName, _ := row["role_name"].(string)
		gotPrefix, _ := row["role_prefix"].(string)
		if gotName != exp.Name {
			t.Errorf("%s: role_name want %q, got %q", name, exp.Name, gotName)
		}
		if gotPrefix != exp.Prefix {
			t.Errorf("%s: role_prefix want %q, got %q", name, exp.Prefix, gotPrefix)
		}
	}
}

// TestHandleAlerts_InterfaceDescription verifies /alerts surfaces the
// configured interface description on each alert row. Synthetic entities
// (e.g. __snmp__) and unknown interfaces must leave the field empty.
func TestHandleAlerts_InterfaceDescription(t *testing.T) {
	t.Parallel()
	log := zerolog.New(io.Discard)

	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			AlertBehavior: config.AlertBehavior{DeduplicationWindow: time.Minute},
		},
	}
	cfg.DesiredState.Devices = map[string]config.DeviceConfig{
		"csw-hcd-01": {
			Address: "10.0.0.1",
			Interfaces: map[string]config.InterfaceConfig{
				"Hu1/0/28": {
					Description:  "t|po32|csw-mcd-01:hu1/0/28|po32",
					DesiredState: "up",
					Monitor:      true,
				},
			},
		},
	}

	eng := alerter.NewEngine(cfg, notifier.NewNotifier(log, nil), log, "")
	// Inject three synthetic alerts: a real interface, a synthetic __snmp__
	// entity, and an unknown interface name.
	inject := []*types.Alert{
		{ID: "a1", Device: "csw-hcd-01", Entity: "Hu1/0/28", AlertType: "interface_state_mismatch", Severity: "critical", State: "firing", Message: "interface Hu1/0/28 expected up got down", FiredAt: time.Now()},
		{ID: "a2", Device: "csw-hcd-01", Entity: "__snmp__", AlertType: "snmp_unreachable", Severity: "critical", State: "firing", Message: "snmp unreachable", FiredAt: time.Now()},
		{ID: "a3", Device: "csw-hcd-01", Entity: "Gi1/0/99", AlertType: "interface_state_mismatch", Severity: "warning", State: "firing", Message: "no such iface", FiredAt: time.Now()},
	}
	for _, a := range inject {
		eng.UpsertActiveAlertForTest(a)
	}

	srv := NewServer(eng, log, "8088")
	srv.SetConfig(cfg, "/tmp/desired-state.yaml")

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	rec := httptest.NewRecorder()
	srv.handleAlerts(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body struct {
		Alerts []struct {
			ID                   string `json:"ID"`
			Entity               string `json:"Entity"`
			InterfaceDescription string `json:"InterfaceDescription"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	got := map[string]string{}
	for _, a := range body.Alerts {
		got[a.ID] = a.InterfaceDescription
	}
	if got["a1"] != "t|po32|csw-mcd-01:hu1/0/28|po32" {
		t.Errorf("a1 description: want trunk string, got %q", got["a1"])
	}
	if got["a2"] != "" {
		t.Errorf("a2 (__snmp__): want empty description, got %q", got["a2"])
	}
	if got["a3"] != "" {
		t.Errorf("a3 (unknown iface): want empty description, got %q", got["a3"])
	}
}
