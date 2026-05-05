package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// MergeMonolithicDeviceOverlay replaces DesiredState.Devices when
// data/desired-state-devices.yaml exists (e.g. Docker with read-only /config).
func MergeMonolithicDeviceOverlay(configDir string, ds *DesiredStateConfig) error {
	if ds == nil {
		return nil
	}
	path := MonolithicDeviceOverlayPath(configDir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read desired-state-devices overlay: %w", err)
	}
	var wrap deviceFileWrapper
	if err := yaml.Unmarshal(data, &wrap); err != nil {
		return fmt.Errorf("parse desired-state-devices overlay: %w", err)
	}
	if wrap.Devices != nil {
		ds.Devices = wrap.Devices
	}
	return nil
}

// WriteMonolithicDeviceOverlay writes the full devices map for the overlay file.
func WriteMonolithicDeviceOverlay(configDir string, devices map[string]DeviceConfig) error {
	if devices == nil {
		devices = make(map[string]DeviceConfig)
	}
	dataDir := DataDir(configDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	path := MonolithicDeviceOverlayPath(configDir)
	out, err := yaml.Marshal(&deviceFileWrapper{Devices: devices})
	if err != nil {
		return fmt.Errorf("marshal desired-state-devices overlay: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("write temp desired-state-devices overlay: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace desired-state-devices overlay: %w", err)
	}
	return nil
}
