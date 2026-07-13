package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/netspec/netspec/internal/config"
)

func (s *Server) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.reloadMu.RLock()
	configPath := s.configPath
	s.reloadMu.RUnlock()
	if strings.TrimSpace(configPath) == "" {
		http.Error(w, "configuration path not set", http.StatusServiceUnavailable)
		return
	}
	configDir := filepath.Dir(configPath)

	s.versionMu.RLock()
	version := s.version
	commit := s.commit
	s.versionMu.RUnlock()

	archive, filename, err := config.ExportConfigArchive(configDir, version, commit)
	if err != nil {
		s.logger.Error().Err(err).Msg("Config export failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(archive)))
	_, _ = w.Write(archive)
}

func (s *Server) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mode := config.ImportMode(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = config.ImportModeReplace
	}

	s.reloadMu.RLock()
	configPath := s.configPath
	reloadFn := s.reloadFunc
	s.reloadMu.RUnlock()
	if strings.TrimSpace(configPath) == "" {
		http.Error(w, "configuration path not set", http.StatusServiceUnavailable)
		return
	}
	configDir := filepath.Dir(configPath)

	r.Body = http.MaxBytesReader(w, r.Body, config.MaxBackupArchiveBytes+1<<20)
	if err := r.ParseMultipartForm(config.MaxBackupArchiveBytes + 1<<20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, config.MaxBackupArchiveBytes+1))
	if err != nil {
		http.Error(w, "failed to read upload", http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, "empty upload", http.StatusBadRequest)
		return
	}
	if int64(len(data)) > config.MaxBackupArchiveBytes {
		http.Error(w, fmt.Sprintf("upload exceeds maximum size of %d bytes", config.MaxBackupArchiveBytes), http.StatusRequestEntityTooLarge)
		return
	}

	result, err := config.ImportConfigArchive(configDir, bytes.NewReader(data), int64(len(data)), mode)
	if err != nil {
		s.logger.Error().Err(err).Str("upload", header.Filename).Msg("Config import failed")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var deviceCount int
	if reloadFn != nil {
		newCfg, reloadErr := reloadFn()
		if reloadErr != nil {
			s.logger.Error().Err(reloadErr).Msg("Config import reload failed")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   reloadErr.Error(),
				"import":  result,
			})
			return
		}
		s.reloadMu.Lock()
		s.config = newCfg
		s.reloadMu.Unlock()
		deviceCount = newCfg.TotalDeviceCount()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"mode":          result.Mode,
		"files_written": result.FilesWritten,
		"files_removed": result.FilesRemoved,
		"device_count":  deviceCount,
	})
}
