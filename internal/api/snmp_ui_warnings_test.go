package api

import (
	"testing"
	"time"

	"github.com/netspec/netspec/internal/config"
)

func TestSnmpUIWarningsNilConfig(t *testing.T) {
	t.Parallel()
	if w := snmpUIWarnings(nil); len(w) != 0 {
		t.Fatalf("expected empty, got %+v", w)
	}
}

func TestSnmpUIWarningsFallbackShowsWarning(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		DesiredState: config.DesiredStateConfig{
			Global: config.GlobalConfig{
				TelemetryMode:      "telemetry_ingest_push",
				CollectionInterval: 30 * time.Second,
				SNMP: config.SNMPConfig{
					Version:                   "2c",
					ValidationInterval:        10 * time.Second,
					TelemetryFallbackEnabled:   true,
					TelemetryFallbackInterval:  2 * time.Minute,
				},
			},
		},
	}
	w := snmpUIWarnings(cfg)
	if len(w) != 1 || w[0].Class != "warning" || w[0].Title == "" {
		t.Fatalf("unexpected warnings: %+v", w)
	}
}
