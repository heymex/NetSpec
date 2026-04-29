package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/netspec/netspec/internal/config"
	"gopkg.in/yaml.v3"
)

var deviceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func PatchDesiredState(configPath string, req *CommitRequest) (*CommitResult, error) {
	if err := validateCommitRequest(req); err != nil {
		return nil, err
	}

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

	targetKey := req.DeviceKey
	switch req.Action {
	case "add":
		if _, exists := desired.Devices[targetKey]; exists {
			return nil, errConflict("device key already exists")
		}
		desired.Devices[targetKey] = config.DeviceConfig{
			Address:     req.Address,
			Description: req.DeviceDescription,
			Interfaces:  make(map[string]config.InterfaceConfig),
		}
	case "patch":
		if req.ExistingDeviceKey == "" {
			req.ExistingDeviceKey = targetKey
		}
		dev, exists := desired.Devices[req.ExistingDeviceKey]
		if !exists {
			return nil, errNotFound("device key not found")
		}
		targetKey = req.ExistingDeviceKey
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
	default:
		return nil, fmt.Errorf("action must be add or patch")
	}

	dev := desired.Devices[targetKey]
	if dev.Interfaces == nil {
		dev.Interfaces = make(map[string]config.InterfaceConfig)
	}

	monitored := 0
	for _, iface := range req.Interfaces {
		ifaceName := strings.TrimSpace(iface.Name)
		if ifaceName == "" {
			continue
		}
		if iface.Monitor {
			monitored++
		}
		dev.Interfaces[ifaceName] = config.InterfaceConfig{
			Description:  iface.Alias,
			DesiredState: iface.DesiredState,
			AdminState:   iface.AdminState,
			Monitor:      iface.Monitor,
			Alerts: config.AlertSeverity{
				StateMismatch: iface.AlertSeverity,
			},
		}
	}
	desired.Devices[targetKey] = dev

	out, err := yaml.Marshal(&desired)
	if err != nil {
		return nil, fmt.Errorf("marshal desired-state: %w", err)
	}
	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return nil, fmt.Errorf("write temp desired-state: %w", err)
	}
	if err := os.Rename(tmp, configPath); err != nil {
		return nil, fmt.Errorf("replace desired-state: %w", err)
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
