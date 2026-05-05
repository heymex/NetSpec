package config

import "path/filepath"

// DataDir is the writable tree next to the config directory (e.g. ../data when config is ./config).
func DataDir(configDir string) string {
	return filepath.Clean(filepath.Join(configDir, "..", "data"))
}

// MonolithicDeviceOverlayPath is the YAML file that replaces devices: from desired-state.yaml
// when /config is read-only; edits are persisted here and merged at load time.
func MonolithicDeviceOverlayPath(configDir string) string {
	return filepath.Join(DataDir(configDir), "desired-state-devices.yaml")
}

// SplitDeviceReadDirs returns directories scanned for split per-device YAML, in order.
// The first path is the legacy/config-side tree; the second is under ../data/devices
// so Docker stacks can mount /config read-only while the wizard still persists devices.
func SplitDeviceReadDirs(configDir string) []string {
	return []string{
		filepath.Join(configDir, "devices"),
		SplitDeviceWriteDir(configDir),
	}
}

// SplitDeviceWriteDir is where discovery and API writes place new per-device YAML files.
func SplitDeviceWriteDir(configDir string) string {
	return filepath.Join(DataDir(configDir), "devices")
}
