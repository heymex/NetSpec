package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/netspec/netspec/internal/config"
	"gopkg.in/yaml.v3"
)

var deviceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func PatchDesiredState(configPath string, req *CommitRequest) (*CommitResult, error) {
	if err := validateCommitRequest(req); err != nil {
		return nil, err
	}

	desired, err := loadDesiredStateForEdit(configPath)
	if err != nil {
		return nil, err
	}
	configDir := filepath.Dir(configPath)
	writeDevicesDir := config.SplitDeviceWriteDir(configDir)
	splitIndex, err := indexMergedSplitDeviceFiles(configDir)
	if err != nil {
		return nil, err
	}

	targetKey := req.DeviceKey
	switch req.Action {
	case "add":
		if _, exists := desired.Devices[targetKey]; exists || splitIndex[targetKey] != "" {
			return nil, errConflict("device key already exists")
		}
		dev := config.DeviceConfig{
			Address:     req.Address,
			Description: req.DeviceDescription,
			Interfaces:  make(map[string]config.InterfaceConfig),
		}
		applyInterfaces(&dev, req.Interfaces)
		if err := writeSplitDeviceFile(writeDevicesDir, targetKey, dev); err != nil {
			return nil, err
		}
	case "patch":
		if req.ExistingDeviceKey == "" {
			req.ExistingDeviceKey = targetKey
		}
		targetKey = req.ExistingDeviceKey
		dev, exists := desired.Devices[targetKey]
		splitPath := splitIndex[targetKey]
		if !exists && splitPath == "" {
			return nil, errNotFound("device key not found")
		}
		if splitPath != "" {
			fileDevices, err := loadSplitFile(splitPath)
			if err != nil {
				return nil, err
			}
			dev = fileDevices[targetKey]
			if dev.Interfaces == nil {
				dev.Interfaces = make(map[string]config.InterfaceConfig)
			}
			if req.DeviceDescription != "" {
				dev.Description = req.DeviceDescription
			}
			if req.Address != "" {
				dev.Address = req.Address
			}
			applyInterfaces(&dev, req.Interfaces)
			fileDevices[targetKey] = dev
			if err := writeSplitFile(splitPath, fileDevices); err != nil {
				return nil, err
			}
			break
		}

		if req.DeviceDescription != "" {
			dev.Description = req.DeviceDescription
		}
		if req.Address != "" {
			dev.Address = req.Address
		}
		if dev.Interfaces == nil {
			dev.Interfaces = make(map[string]config.InterfaceConfig)
		}
		desired.Devices[targetKey] = dev
		applyInterfaces(&dev, req.Interfaces)
		desired.Devices[targetKey] = dev
		delete(desired.Devices, targetKey)
		if err := writeDesiredStateAdaptive(configPath, desired); err != nil {
			return nil, err
		}
		if err := writeSplitDeviceFile(writeDevicesDir, targetKey, dev); err != nil {
			return nil, err
		}
		break
	default:
		return nil, fmt.Errorf("action must be add or patch")
	}

	monitored := 0
	for _, iface := range req.Interfaces {
		if iface.Monitor {
			monitored++
		}
	}

	return &CommitResult{
		Success:             true,
		Action:              req.Action,
		DeviceKey:           targetKey,
		InterfacesWritten:   len(req.Interfaces),
		InterfacesMonitored: monitored,
		Message:             "Device updated successfully. Click 'Reload Config' to apply changes.",
	}, nil
}

func applyInterfaces(dev *config.DeviceConfig, interfaces []CommitInterface) {
	if dev.Interfaces == nil {
		dev.Interfaces = make(map[string]config.InterfaceConfig)
	}
	for _, iface := range interfaces {
		ifaceName := strings.TrimSpace(iface.Name)
		if ifaceName == "" {
			continue
		}
		cfgIface := config.InterfaceConfig{
			Description:  iface.Alias,
			DesiredState: iface.DesiredState,
			AdminState:   iface.AdminState,
			Monitor:      iface.Monitor,
			Alerts: config.AlertSeverity{
				StateMismatch: iface.AlertSeverity,
				MemberDown:    iface.MemberDownSeverity,
				ChannelDown:   iface.ChannelDownSeverity,
				AdminDown:     iface.AdminDownSeverity,
			},
		}
		if iface.IsPortChannel && len(iface.Members) > 0 {
			members := uniqueSorted(iface.Members)
			cfgIface.Members = &config.MemberConfig{Required: members}
			cfgIface.MemberPolicy = &config.MemberPolicy{
				Mode: "min_active",
				// >=50% members down is critical; this threshold makes <=50% active critical.
				Minimum: (len(members) + 1) / 2,
			}
		}
		dev.Interfaces[ifaceName] = cfgIface
	}
}

func uniqueSorted(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func loadDesiredState(configPath string) (*config.DesiredStateConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read desired-state: %w", err)
	}
	var desired config.DesiredStateConfig
	if err := yaml.Unmarshal(data, &desired); err != nil {
		return nil, fmt.Errorf("parse desired-state: %w", err)
	}
	if desired.Devices == nil {
		desired.Devices = make(map[string]config.DeviceConfig)
	}
	return &desired, nil
}

func loadDesiredStateForEdit(configPath string) (*config.DesiredStateConfig, error) {
	desired, err := loadDesiredState(configPath)
	if err != nil {
		return nil, err
	}
	if err := config.MergeMonolithicDeviceOverlay(filepath.Dir(configPath), desired); err != nil {
		return nil, err
	}
	if desired.Devices == nil {
		desired.Devices = make(map[string]config.DeviceConfig)
	}
	return desired, nil
}

func isDirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".netspec-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

func writeDesiredStateAdaptive(configPath string, desired *config.DesiredStateConfig) error {
	dir := filepath.Dir(configPath)
	if isDirWritable(dir) {
		if err := writeDesiredState(configPath, desired); err != nil {
			return err
		}
		_ = os.Remove(config.MonolithicDeviceOverlayPath(dir))
		return nil
	}
	if desired.Devices == nil {
		desired.Devices = make(map[string]config.DeviceConfig)
	}
	return config.WriteMonolithicDeviceOverlay(dir, desired.Devices)
}

func writeDesiredState(configPath string, desired *config.DesiredStateConfig) error {
	out, err := yaml.Marshal(desired)
	if err != nil {
		return fmt.Errorf("marshal desired-state: %w", err)
	}
	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("write temp desired-state: %w", err)
	}
	if err := os.Rename(tmp, configPath); err != nil {
		return fmt.Errorf("replace desired-state: %w", err)
	}
	return nil
}

type splitFile struct {
	Devices map[string]config.DeviceConfig `yaml:"devices"`
}

func indexMergedSplitDeviceFiles(configDir string) (map[string]string, error) {
	out := map[string]string{}
	for _, dir := range config.SplitDeviceReadDirs(configDir) {
		sub, err := indexSplitDeviceFiles(dir)
		if err != nil {
			return nil, err
		}
		for key, path := range sub {
			if prev, ok := out[key]; ok {
				return nil, fmt.Errorf("duplicate device %q in split YAML: %s and %s", key, prev, path)
			}
			out[key] = path
		}
	}
	return out, nil
}

func indexSplitDeviceFiles(devicesDir string) (map[string]string, error) {
	index := map[string]string{}
	entries, err := os.ReadDir(devicesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return index, nil
		}
		return nil, fmt.Errorf("read devices dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(devicesDir, name)
		devs, err := loadSplitFile(path)
		if err != nil {
			return nil, err
		}
		for key := range devs {
			if prev, ok := index[key]; ok {
				return nil, fmt.Errorf("duplicate device %q in split files: %s and %s", key, prev, path)
			}
			index[key] = path
		}
	}
	return index, nil
}

func loadSplitFile(path string) (map[string]config.DeviceConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read split device file %s: %w", path, err)
	}
	var wrapped splitFile
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("parse split device file %s: %w", path, err)
	}
	if wrapped.Devices == nil {
		wrapped.Devices = map[string]config.DeviceConfig{}
	}
	return wrapped.Devices, nil
}

func writeSplitFile(path string, devices map[string]config.DeviceConfig) error {
	payload, err := yaml.Marshal(&splitFile{Devices: devices})
	if err != nil {
		return fmt.Errorf("marshal split file: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o644); err != nil {
		return fmt.Errorf("write temp split file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace split file: %w", err)
	}
	return nil
}

func writeSplitDeviceFile(devicesDir, key string, dev config.DeviceConfig) error {
	if err := os.MkdirAll(devicesDir, 0o755); err != nil {
		return fmt.Errorf("create devices dir: %w", err)
	}
	path := filepath.Join(devicesDir, key+".yaml")
	return writeSplitFile(path, map[string]config.DeviceConfig{key: dev})
}

func validateCommitRequest(req *CommitRequest) error {
	if req == nil {
		return errors.New("request is required")
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	req.DeviceKey = strings.TrimSpace(req.DeviceKey)
	req.Address = strings.TrimSpace(req.Address)
	if req.Action != "add" && req.Action != "patch" {
		return errors.New("action must be add or patch")
	}
	if !deviceKeyPattern.MatchString(req.DeviceKey) {
		return errors.New("device_key must match [A-Za-z0-9_-]{1,64}")
	}
	if req.Address == "" {
		return errors.New("address is required")
	}
	if len(req.Interfaces) == 0 {
		return errors.New("at least one interface is required")
	}
	for _, iface := range req.Interfaces {
		if strings.TrimSpace(iface.Name) == "" {
			return errors.New("interface name is required")
		}
		if iface.DesiredState != "up" && iface.DesiredState != "down" {
			return errors.New("desired_state must be up or down")
		}
		if iface.AdminState != "enabled" && iface.AdminState != "disabled" {
			return errors.New("admin_state must be enabled or disabled")
		}
		if iface.AlertSeverity != "info" && iface.AlertSeverity != "warning" && iface.AlertSeverity != "critical" {
			return errors.New("alert_severity must be info, warning, or critical")
		}
		for _, sev := range []string{iface.MemberDownSeverity, iface.ChannelDownSeverity, iface.AdminDownSeverity} {
			if sev != "" && sev != "info" && sev != "warning" && sev != "critical" {
				return errors.New("member_down_severity, channel_down_severity, and admin_down_severity must be info, warning, or critical")
			}
		}
	}
	return nil
}

type apiError struct {
	msg        string
	statusCode int
}

func (e *apiError) Error() string { return e.msg }

func errConflict(msg string) error { return &apiError{msg: msg, statusCode: 409} }

func errNotFound(msg string) error { return &apiError{msg: msg, statusCode: 404} }

func StatusCodeFor(err error) int {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.statusCode
	}
	return 400
}

func DesiredStatePathFrom(configPath string) string {
	if filepath.Base(configPath) == "desired-state.yaml" {
		return configPath
	}
	return filepath.Join(filepath.Dir(configPath), "desired-state.yaml")
}

type InterfaceUpdate struct {
	Description   *string
	DesiredState  *string
	AdminState    *string
	Monitor       *bool
	AlertSeverity *string
}

func UpdateDeviceInterface(configPath, deviceKey, ifaceName string, patch InterfaceUpdate) error {
	deviceKey = strings.TrimSpace(deviceKey)
	ifaceName = strings.TrimSpace(ifaceName)
	if deviceKey == "" || ifaceName == "" {
		return errors.New("device key and interface name are required")
	}
	if patch.DesiredState != nil && *patch.DesiredState != "up" && *patch.DesiredState != "down" {
		return errors.New("desired_state must be up or down")
	}
	if patch.AdminState != nil && *patch.AdminState != "enabled" && *patch.AdminState != "disabled" {
		return errors.New("admin_state must be enabled or disabled")
	}
	if patch.AlertSeverity != nil {
		v := strings.TrimSpace(*patch.AlertSeverity)
		if v != "info" && v != "warning" && v != "critical" {
			return errors.New("alert_severity must be info, warning, or critical")
		}
	}

	desired, err := loadDesiredStateForEdit(configPath)
	if err != nil {
		return err
	}
	configDir := filepath.Dir(configPath)
	splitIndex, err := indexMergedSplitDeviceFiles(configDir)
	if err != nil {
		return err
	}

	if splitPath := splitIndex[deviceKey]; splitPath != "" {
		fileDevices, err := loadSplitFile(splitPath)
		if err != nil {
			return err
		}
		dev, ok := fileDevices[deviceKey]
		if !ok {
			return errNotFound("device key not found")
		}
		if err := applyInterfacePatch(&dev, ifaceName, patch); err != nil {
			return err
		}
		fileDevices[deviceKey] = dev
		return writeSplitFile(splitPath, fileDevices)
	}

	dev, ok := desired.Devices[deviceKey]
	if !ok {
		return errNotFound("device key not found")
	}
	if err := applyInterfacePatch(&dev, ifaceName, patch); err != nil {
		return err
	}
	desired.Devices[deviceKey] = dev
	return writeDesiredStateAdaptive(configPath, desired)
}

func DeleteDevice(configPath, deviceKey string) error {
	deviceKey = strings.TrimSpace(deviceKey)
	if deviceKey == "" {
		return errors.New("device key is required")
	}

	desired, err := loadDesiredStateForEdit(configPath)
	if err != nil {
		return err
	}
	configDir := filepath.Dir(configPath)
	splitIndex, err := indexMergedSplitDeviceFiles(configDir)
	if err != nil {
		return err
	}

	if splitPath := splitIndex[deviceKey]; splitPath != "" {
		fileDevices, err := loadSplitFile(splitPath)
		if err != nil {
			return err
		}
		if _, ok := fileDevices[deviceKey]; !ok {
			return errNotFound("device key not found")
		}
		delete(fileDevices, deviceKey)
		if len(fileDevices) == 0 {
			if err := os.Remove(splitPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove empty split device file: %w", err)
			}
			return nil
		}
		return writeSplitFile(splitPath, fileDevices)
	}

	if _, ok := desired.Devices[deviceKey]; !ok {
		return errNotFound("device key not found")
	}
	delete(desired.Devices, deviceKey)
	return writeDesiredStateAdaptive(configPath, desired)
}

func applyInterfacePatch(dev *config.DeviceConfig, ifaceName string, patch InterfaceUpdate) error {
	if dev.Interfaces == nil {
		return errNotFound("interface not found")
	}
	iface, ok := dev.Interfaces[ifaceName]
	if !ok {
		return errNotFound("interface not found")
	}
	if patch.Description != nil {
		iface.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.DesiredState != nil {
		iface.DesiredState = *patch.DesiredState
	}
	if patch.AdminState != nil {
		iface.AdminState = *patch.AdminState
	}
	if patch.Monitor != nil {
		iface.Monitor = *patch.Monitor
	}
	if patch.AlertSeverity != nil {
		iface.Alerts.StateMismatch = strings.TrimSpace(*patch.AlertSeverity)
	}
	dev.Interfaces[ifaceName] = iface
	return nil
}
