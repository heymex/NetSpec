package config

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportImportConfigArchiveRoundTripReplace(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "cfg")
	dataDir := DataDir(cfgDir)
	if err := os.MkdirAll(filepath.Join(cfgDir, "devices"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "devices"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(cfgDir, "desired-state.yaml"), []byte(`global:
  telemetry_mode: snmp_validate_only
  snmp:
    version: "2c"
devices: {}
`))
	writeTestFile(t, filepath.Join(cfgDir, "alerts.yaml"), []byte(`channels: {}
alert_rules: {}
alert_behavior:
  deduplication_window: 5m
`))
	writeTestFile(t, filepath.Join(cfgDir, "devices", "keep-sw.yaml"), []byte(`devices:
  keep-sw:
    address: 10.0.0.1
    interfaces:
      Gi1:
        desired_state: up
        monitor: true
`))
	writeTestFile(t, filepath.Join(dataDir, "devices", "data-sw.yaml"), []byte(`devices:
  data-sw:
    address: 10.0.0.2
    interfaces:
      Gi1:
        desired_state: up
        monitor: true
`))

	archive, _, err := ExportConfigArchive(cfgDir, "test", "abc123")
	if err != nil {
		t.Fatalf("ExportConfigArchive: %v", err)
	}

	writeTestFile(t, filepath.Join(dataDir, "devices", "drop-sw.yaml"), []byte(`devices:
  drop-sw:
    address: 10.0.0.9
    interfaces:
      Gi1:
        desired_state: up
        monitor: true
`))

	result, err := ImportConfigArchive(cfgDir, bytes.NewReader(archive), int64(len(archive)), ImportModeReplace)
	if err != nil {
		t.Fatalf("ImportConfigArchive: %v", err)
	}
	if len(result.FilesWritten) == 0 {
		t.Fatal("expected files written")
	}
	foundRemoved := false
	for _, removed := range result.FilesRemoved {
		if removed == "data/devices/drop-sw.yaml" {
			foundRemoved = true
			break
		}
	}
	if !foundRemoved {
		t.Fatalf("expected drop-sw.yaml removed, got %v", result.FilesRemoved)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "devices", "drop-sw.yaml")); !os.IsNotExist(err) {
		t.Fatalf("drop-sw.yaml should be removed, stat err=%v", err)
	}

	cfg, err := LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatalf("LoadConfigDir after import: %v", err)
	}
	if cfg.TotalDeviceCount() != 2 {
		t.Fatalf("device count: want 2, got %d", cfg.TotalDeviceCount())
	}
}

func TestImportConfigArchiveRejectsUnsupportedPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "cfg")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("secrets.env")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("SNMP_COMMUNITY=secret")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	archive := buf.Bytes()

	_, err = ImportConfigArchive(cfgDir, bytes.NewReader(archive), int64(len(archive)), ImportModeMerge)
	if err == nil {
		t.Fatal("expected unsupported path import to fail")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
