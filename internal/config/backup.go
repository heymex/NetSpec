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

	configDir, err = filepath.Abs(filepath.Clean(configDir))
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	dataDir, err := filepath.Abs(DataDir(configDir))
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}
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

		dest, err := resolveImportDestination(configDir, zipPath)
		if err != nil {
			return nil, err
		}
		if err := ensureWithinBackupRoots(configDir, dataDir, dest); err != nil {
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
	// Normalize separators first so Clean collapses ".." on all platforms.
	name = filepath.ToSlash(name)
	name = filepath.Clean(name)
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "./")
	if name == "." || name == "" {
		return "", fmt.Errorf("invalid zip entry path")
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("absolute zip entry path rejected: %s", name)
	}
	if name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") || strings.HasSuffix(name, "/..") {
		return "", fmt.Errorf("zip entry path traversal rejected: %s", name)
	}
	if strings.Contains(name, "..") {
		return "", fmt.Errorf("zip entry path contains '..': %s", name)
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
		_, err := sanitizeYAMLBasename(strings.TrimPrefix(zipPath, "config/devices/"))
		return err
	case strings.HasPrefix(zipPath, "data/devices/"):
		_, err := sanitizeYAMLBasename(strings.TrimPrefix(zipPath, "data/devices/"))
		return err
	default:
		return fmt.Errorf("unsupported zip entry path: %s", zipPath)
	}
}

// sanitizeYAMLBasename accepts only a single path component ending in .yaml.
func sanitizeYAMLBasename(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid device yaml basename")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid device yaml basename: %s", name)
	}
	if filepath.Base(name) != name {
		return "", fmt.Errorf("invalid device yaml basename: %s", name)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".yaml") {
		return "", fmt.Errorf("device file must end with .yaml: %s", name)
	}
	return name, nil
}

func isDeviceYAMLZipPath(zipPath string) bool {
	return strings.HasPrefix(zipPath, "config/devices/") || strings.HasPrefix(zipPath, "data/devices/")
}

// resolveImportDestination maps an allowlisted zip path to an absolute destination
// using only trusted roots and string literals (or a sanitized single-component basename).
func resolveImportDestination(configDir, zipPath string) (string, error) {
	configDir = filepath.Clean(configDir)
	switch zipPath {
	case "desired-state.yaml":
		return filepath.Join(configDir, "desired-state.yaml"), nil
	case "alerts.yaml":
		return filepath.Join(configDir, "alerts.yaml"), nil
	case "credentials.yaml":
		return filepath.Join(configDir, "credentials.yaml"), nil
	case "maintenance.yaml":
		return filepath.Join(configDir, "maintenance.yaml"), nil
	case "rules.yaml":
		return filepath.Join(configDir, "rules.yaml"), nil
	case "data/desired-state-devices.yaml":
		return filepath.Join(DataDir(configDir), "desired-state-devices.yaml"), nil
	}
	if strings.HasPrefix(zipPath, "config/devices/") {
		base, err := sanitizeYAMLBasename(strings.TrimPrefix(zipPath, "config/devices/"))
		if err != nil {
			return "", err
		}
		return filepath.Join(configDir, "devices", base), nil
	}
	if strings.HasPrefix(zipPath, "data/devices/") {
		base, err := sanitizeYAMLBasename(strings.TrimPrefix(zipPath, "data/devices/"))
		if err != nil {
			return "", err
		}
		return filepath.Join(SplitDeviceWriteDir(configDir), base), nil
	}
	return "", fmt.Errorf("unsupported zip entry path: %s", zipPath)
}

func ensureWithinBackupRoots(configDir, dataDir, dest string) error {
	absDest, err := filepath.Abs(filepath.Clean(dest))
	if err != nil {
		return fmt.Errorf("resolve destination: %w", err)
	}
	if pathWithinRoot(configDir, absDest) || pathWithinRoot(dataDir, absDest) {
		return nil
	}
	return fmt.Errorf("refusing to write outside config/data roots: %s", absDest)
}

func pathWithinRoot(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	if candidate == root {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(candidate, root+sep)
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
