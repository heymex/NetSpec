package api

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/netspec/netspec/internal/alerter"
	"github.com/netspec/netspec/internal/config"
	"github.com/rs/zerolog"
)

func TestHandleConfigExportImport(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "cfg")
	dataDir := config.DataDir(cfgDir)
	if err := os.MkdirAll(filepath.Join(cfgDir, "devices"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "devices"), 0o755); err != nil {
		t.Fatal(err)
	}
	desired := []byte(`global:
  telemetry_mode: snmp_validate_only
  snmp:
    version: "2c"
devices: {}
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "desired-state.yaml"), desired, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "alerts.yaml"), []byte("channels: {}\nalert_rules: {}\nalert_behavior:\n  deduplication_window: 5m\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := alerter.NewEngine(&config.Config{}, nil, zerolog.Nop(), config.DataDir(cfgDir))
	srv := NewServer(engine, zerolog.Nop(), "8088")
	srv.SetConfig(&config.Config{}, filepath.Join(cfgDir, "desired-state.yaml"))
	srv.SetVersion("test", "abc", "now")
	srv.SetReloadFunc(func() (*config.Config, error) {
		return config.LoadConfigDir(cfgDir)
	})

	exportReq := httptest.NewRequest(http.MethodGet, "/api/config/export", nil)
	exportRec := httptest.NewRecorder()
	srv.handleConfigExport(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("export status: want 200, got %d body=%s", exportRec.Code, exportRec.Body.String())
	}
	if ct := exportRec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type: want application/zip, got %q", ct)
	}
	archive := exportRec.Body.Bytes()
	if len(archive) == 0 {
		t.Fatal("expected non-empty export")
	}

	var importBody bytes.Buffer
	mw := multipart.NewWriter(&importBody)
	part, err := mw.CreateFormFile("file", "backup.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	importReq := httptest.NewRequest(http.MethodPost, "/api/config/import?mode=merge", &importBody)
	importReq.Header.Set("Content-Type", mw.FormDataContentType())
	importRec := httptest.NewRecorder()
	srv.handleConfigImport(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("import status: want 200, got %d body=%s", importRec.Code, importRec.Body.String())
	}
}

func TestExportArchiveContainsManifest(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "cfg")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "desired-state.yaml"), []byte(`global:
  telemetry_mode: snmp_validate_only
  snmp:
    version: "2c"
devices: {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	archive, _, err := config.ExportConfigArchive(cfgDir, "v", "c")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	foundManifest := false
	for _, f := range zr.File {
		if f.Name == config.BackupManifestName {
			foundManifest = true
			break
		}
	}
	if !foundManifest {
		t.Fatal("expected manifest.json in export")
	}
}
