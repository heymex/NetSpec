package config

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	BackupFormatVersion   = 1
	BackupManifestName    = "manifest.json"
	MaxBackupArchiveBytes = 32 << 20 // 32 MiB
)

// ImportMode controls how import applies files from an archive.
type ImportMode string

const (
	ImportModeMerge   ImportMode = "merge"   // overwrite/add files present in the archive only
	ImportModeReplace ImportMode = "replace" // merge plus remove device YAML not present in the archive
)

// BackupManifest describes a NetSpec configuration export archive.
type BackupManifest struct {
	FormatVersion  int      `json:"format_version"`
	ExportedAt     string   `json:"exported_at"`
	NetSpecVersion string   `json:"netspec_version,omitempty"`
	NetSpecCommit  string   `json:"netspec_commit,omitempty"`
	Files          []string `json:"files"`
}

// ImportResult summarizes a configuration import.
type ImportResult struct {
	Mode         ImportMode `json:"mode"`
	FilesWritten []string   `json:"files_written"`
	FilesRemoved []string   `json:"files_removed,omitempty"`
}

type backupEntry struct {
	zipPath string
	absPath string
}

// ExportConfigArchive builds a zip backup of the on-disk NetSpec config tree.
func ExportConfigArchive(configDir, netspecVersion, netspecCommit string) ([]byte, string, error) {
	configDir = filepath.Clean(configDir)
	entries, err := collectBackupEntries(configDir)
	if err != nil {
		return nil, "", err
	}
	if len(entries) == 0 {
		return nil, "", fmt.Errorf("no configuration files found to export")
	}

	manifestFiles := make([]string, 0, len(entries))
	for _, e := range entries {
		manifestFiles = append(manifestFiles, e.zipPath)
	}
	manifest := BackupManifest{
		FormatVersion:  BackupFormatVersion,
		ExportedAt:     time.Now().UTC().Format(time.RFC3339),
		NetSpecVersion: strings.TrimSpace(netspecVersion),
		NetSpecCommit:  strings.TrimSpace(netspecCommit),
		Files:          manifestFiles,
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("marshal manifest: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	mh, err := zw.Create(BackupManifestName)
	if err != nil {
		return nil, "", fmt.Errorf("create manifest entry: %w", err)
	}
	if _, err := mh.Write(manifestJSON); err != nil {
		return nil, "", fmt.Errorf("write manifest: %w", err)
	}

	for _, entry := range entries {
		if err := addFileToZip(zw, entry.zipPath, entry.absPath); err != nil {
			return nil, "", err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize zip: %w", err)
	}
	if buf.Len() > MaxBackupArchiveBytes {
		return nil, "", fmt.Errorf("export archive exceeds %d bytes", MaxBackupArchiveBytes)
	}

	filename := fmt.Sprintf("netspec-config-%s.zip", time.Now().UTC().Format("20060102-150405"))
	return buf.Bytes(), filename, nil
}

// ImportConfigArchive restores configuration files from a zip produced by ExportConfigArchive.
func ImportConfigArchive(configDir string, r io.ReaderAt, size int64, mode ImportMode) (*ImportResult, error) {
	if size < 0 {
		return nil, fmt.Errorf("invalid archive size")
	}
	if size > MaxBackupArchiveBytes {
		return nil, fmt.Errorf("archive exceeds maximum size of %d bytes", MaxBackupArchiveBytes)
	}
	if mode != ImportModeMerge && mode != ImportModeReplace {
		return nil, fmt.Errorf("invalid import mode %q (use merge or replace)", mode)
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("read zip archive: %w", err)
	}

	configDir = filepath.Clean(configDir)
	result := &ImportResult{Mode: mode}
	importedDevicePaths := map[string]bool{}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		zipPath, err := normalizeBackupZipPath(f.Name)
		if err != nil {
			return nil, err
		}
		if zipPath == BackupManifestName {
			continue
		}
		if err := validateBackupZipPath(zipPath); err != nil {
			return nil, err
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip entry %s: %w", zipPath, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, MaxBackupArchiveBytes+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read zip entry %s: %w", zipPath, err)
		}
		if int64(len(data)) > MaxBackupArchiveBytes {
			return nil, fmt.Errorf("zip entry %s exceeds maximum size", zipPath)
		}

		dest, err := backupZipPathToAbs(configDir, zipPath)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, fmt.Errorf("create directory for %s: %w", zipPath, err)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", zipPath, err)
		}
		result.FilesWritten = append(result.FilesWritten, zipPath)
		if isDeviceYAMLZipPath(zipPath) {
			importedDevicePaths[zipPath] = true
		}
	}

	if len(result.FilesWritten) == 0 {
		return nil, fmt.Errorf("archive contains no configuration files")
	}

	if mode == ImportModeReplace {
		removed, err := removeStaleBackupFiles(configDir, importedDevicePaths)
		if err != nil {
			return nil, err
		}
		result.FilesRemoved = removed
	}

	return result, nil
}

func collectBackupEntries(configDir string) ([]backupEntry, error) {
	var entries []backupEntry
	for _, name := range []string{
		"desired-state.yaml",
		"alerts.yaml",
		"credentials.yaml",
		"maintenance.yaml",
		"rules.yaml",
	} {
		path := filepath.Join(configDir, name)
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			entries = append(entries, backupEntry{zipPath: name, absPath: path})
		}
	}

	configDevices, err := listYAMLFiles(filepath.Join(configDir, "devices"))
	if err != nil {
		return nil, err
	}
	for _, abs := range configDevices {
		base := filepath.Base(abs)
		entries = append(entries, backupEntry{
			zipPath: filepath.ToSlash(filepath.Join("config", "devices", base)),
			absPath: abs,
		})
	}

	overlay := MonolithicDeviceOverlayPath(configDir)
	if st, err := os.Stat(overlay); err == nil && !st.IsDir() {
		entries = append(entries, backupEntry{
			zipPath: "data/desired-state-devices.yaml",
			absPath: overlay,
		})
	}

	dataDevices, err := listYAMLFiles(SplitDeviceWriteDir(configDir))
	if err != nil {
		return nil, err
	}
	for _, abs := range dataDevices {
		base := filepath.Base(abs)
		entries = append(entries, backupEntry{
			zipPath: filepath.ToSlash(filepath.Join("data", "devices", base)),
			absPath: abs,
		})
	}

	return entries, nil
}

func listYAMLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".yaml") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	return paths, nil
}

func addFileToZip(zw *zip.Writer, zipPath, absPath string) error {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", absPath, err)
	}
	w, err := zw.Create(zipPath)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", zipPath, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write zip entry %s: %w", zipPath, err)
	}
	return nil
}

func normalizeBackupZipPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty zip entry path")
	}
	name = filepath.ToSlash(filepath.Clean(name))
	name = strings.TrimPrefix(name, "./")
	if name == "." || name == "" {
		return "", fmt.Errorf("invalid zip entry path")
	}
	return name, nil
}

func validateBackupZipPath(zipPath string) error {
	switch {
	case zipPath == "desired-state.yaml",
		zipPath == "alerts.yaml",
		zipPath == "credentials.yaml",
		zipPath == "maintenance.yaml",
		zipPath == "rules.yaml",
		zipPath == "data/desired-state-devices.yaml":
		return nil
	case strings.HasPrefix(zipPath, "config/devices/"):
		return validateDeviceYAMLZipPath(zipPath, "config/devices/")
	case strings.HasPrefix(zipPath, "data/devices/"):
		return validateDeviceYAMLZipPath(zipPath, "data/devices/")
	default:
		return fmt.Errorf("unsupported zip entry path: %s", zipPath)
	}
}

func validateDeviceYAMLZipPath(zipPath, prefix string) error {
	name := strings.TrimPrefix(zipPath, prefix)
	if name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("invalid device yaml path: %s", zipPath)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".yaml") {
		return fmt.Errorf("device file must end with .yaml: %s", zipPath)
	}
	return nil
}

func isDeviceYAMLZipPath(zipPath string) bool {
	return strings.HasPrefix(zipPath, "config/devices/") || strings.HasPrefix(zipPath, "data/devices/")
}

func backupZipPathToAbs(configDir, zipPath string) (string, error) {
	switch zipPath {
	case "desired-state.yaml", "alerts.yaml", "credentials.yaml", "maintenance.yaml", "rules.yaml":
		return filepath.Join(configDir, zipPath), nil
	case "data/desired-state-devices.yaml":
		return MonolithicDeviceOverlayPath(configDir), nil
	}
	if strings.HasPrefix(zipPath, "config/devices/") && strings.HasSuffix(zipPath, ".yaml") {
		return filepath.Join(configDir, "devices", filepath.Base(zipPath)), nil
	}
	if strings.HasPrefix(zipPath, "data/devices/") && strings.HasSuffix(zipPath, ".yaml") {
		return filepath.Join(SplitDeviceWriteDir(configDir), filepath.Base(zipPath)), nil
	}
	return "", fmt.Errorf("unsupported zip entry path: %s", zipPath)
}

func removeStaleBackupFiles(configDir string, keep map[string]bool) ([]string, error) {
	var removed []string
	dirs := []struct {
		prefix string
		dir    string
	}{
		{prefix: "config/devices/", dir: filepath.Join(configDir, "devices")},
		{prefix: "data/devices/", dir: SplitDeviceWriteDir(configDir)},
	}
	for _, d := range dirs {
		files, err := listYAMLFiles(d.dir)
		if err != nil {
			return nil, err
		}
		for _, abs := range files {
			zipPath := filepath.ToSlash(filepath.Join(d.prefix, filepath.Base(abs)))
			if keep[zipPath] {
				continue
			}
			if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("remove stale %s: %w", abs, err)
			}
			removed = append(removed, zipPath)
		}
	}
	overlayZip := "data/desired-state-devices.yaml"
	if !keep[overlayZip] {
		overlay := MonolithicDeviceOverlayPath(configDir)
		if err := os.Remove(overlay); err == nil {
			removed = append(removed, overlayZip)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("remove stale overlay: %w", err)
		}
	}
	return removed, nil
}
