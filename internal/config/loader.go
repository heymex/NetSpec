package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads configuration from a single file (legacy method)
func LoadConfig(path string) (*Config, error) {
	return LoadConfigDir(filepath.Dir(path))
}

// LoadConfigDir loads all configuration files from a directory
func LoadConfigDir(dir string) (*Config, error) {
	cfg := &Config{}

	// Load desired-state.yaml
	if err := loadYAML(filepath.Join(dir, "desired-state.yaml"), &cfg.DesiredState); err != nil {
		return nil, fmt.Errorf("loading desired-state.yaml: %w", err)
	}
	if err := MergeMonolithicDeviceOverlay(dir, &cfg.DesiredState); err != nil {
		return nil, err
	}
	if cfg.DesiredState.Devices == nil {
		cfg.DesiredState.Devices = make(map[string]DeviceConfig)
	}
	cfg.MonolithicDeviceCount = len(cfg.DesiredState.Devices)
	// Optionally load additional device definitions from split YAML directories:
	// <configDir>/devices (legacy) and <configDir>/../data/devices (writable in Docker).
	splitCount := 0
	for _, d := range SplitDeviceReadDirs(dir) {
		n, err := loadDeviceFiles(d, cfg)
		if err != nil {
			return nil, fmt.Errorf("loading devices directory %s: %w", d, err)
		}
		splitCount += n
	}
	cfg.SplitDeviceCount = splitCount

	// Load alerts.yaml (optional)
	alertsPath := filepath.Join(dir, "alerts.yaml")
	if _, err := os.Stat(alertsPath); err == nil {
		if err := loadYAML(alertsPath, &cfg.Alerts); err != nil {
			return nil, fmt.Errorf("loading alerts.yaml: %w", err)
		}
	}

	// Load credentials.yaml (optional)
	credentialsPath := filepath.Join(dir, "credentials.yaml")
	if _, err := os.Stat(credentialsPath); err == nil {
		if err := loadYAML(credentialsPath, &cfg.Credentials); err != nil {
			return nil, fmt.Errorf("loading credentials.yaml: %w", err)
		}
	}

	// Load maintenance.yaml (optional).
	// TODO: not yet wired into runtime alert suppression logic.
	maintenancePath := filepath.Join(dir, "maintenance.yaml")
	if _, err := os.Stat(maintenancePath); err == nil {
		if err := loadYAML(maintenancePath, &cfg.Maintenance); err != nil {
			return nil, fmt.Errorf("loading maintenance.yaml: %w", err)
		}
	}

	// Load rules.yaml (optional) — business rules for discovery defaults.
	rulesPath := filepath.Join(dir, "rules.yaml")
	if _, err := os.Stat(rulesPath); err == nil {
		if err := loadYAML(rulesPath, &cfg.Rules); err != nil {
			return nil, fmt.Errorf("loading rules.yaml: %w", err)
		}
	}

	// Set defaults
	if cfg.DesiredState.Global.CollectionInterval == 0 {
		cfg.DesiredState.Global.CollectionInterval = 10 * time.Second
	}
	if cfg.DesiredState.Global.TelemetryMode == "" {
		cfg.DesiredState.Global.TelemetryMode = "snmp_validate_only"
	}
	if cfg.DesiredState.Global.SNMP.Port == 0 {
		cfg.DesiredState.Global.SNMP.Port = 161
	}
	if cfg.DesiredState.Global.SNMP.Version == "" {
		cfg.DesiredState.Global.SNMP.Version = "2c"
	}
	if cfg.DesiredState.Global.SNMP.CommunityEnv == "" {
		cfg.DesiredState.Global.SNMP.CommunityEnv = "SNMP_COMMUNITY"
	}
	if cfg.DesiredState.Global.SNMP.ValidationInterval == 0 {
		cfg.DesiredState.Global.SNMP.ValidationInterval = cfg.DesiredState.Global.CollectionInterval
	}
	if cfg.DesiredState.Global.SNMP.TelemetryFallbackEnabled &&
		cfg.DesiredState.Global.SNMP.TelemetryFallbackInterval == 0 {
		cfg.DesiredState.Global.SNMP.TelemetryFallbackInterval = 5 * time.Minute
	}
	if cfg.DesiredState.Global.SNMP.Timeout == 0 {
		cfg.DesiredState.Global.SNMP.Timeout = 3 * time.Second
	}
	if cfg.DesiredState.Global.SNMP.Retries == 0 {
		cfg.DesiredState.Global.SNMP.Retries = 1
	}
	if cfg.DesiredState.Global.Ingest.ListenAddress == "" {
		cfg.DesiredState.Global.Ingest.ListenAddress = "0.0.0.0"
	}
	if cfg.DesiredState.Global.Ingest.Port == 0 {
		cfg.DesiredState.Global.Ingest.Port = 57500
	}
	if cfg.DesiredState.Global.TelemetryMode == "telemetry_ingest_push" &&
		cfg.DesiredState.Global.Ingest.StaleAfter == 0 {
		cfg.DesiredState.Global.Ingest.StaleAfter = DefaultIngestStaleAfter
	}
	if cfg.Alerts.AlertBehavior.DeduplicationWindow == 0 {
		cfg.Alerts.AlertBehavior.DeduplicationWindow = 5 * time.Minute
	}

	// Validate configuration
	if err := ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// loadYAML loads a YAML file into a struct
func loadYAML(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}

// deviceFileWrapper supports files that define devices under a top-level
// "devices" key.
type deviceFileWrapper struct {
	Devices map[string]DeviceConfig `yaml:"devices"`
}

// loadDeviceFiles merges device definitions from *.yaml/*.yml files under
// config/devices. Per-file formats supported:
//   - top-level "devices:" map
//   - top-level map of "<device_name>: <DeviceConfig>"
func loadDeviceFiles(devicesDir string, cfg *Config) (int, error) {
	entries, err := os.ReadDir(devicesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	if cfg.DesiredState.Devices == nil {
		cfg.DesiredState.Devices = make(map[string]DeviceConfig)
	}

	fileNames := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			fileNames = append(fileNames, name)
		}
	}
	sort.Strings(fileNames)
	added := 0

	for _, name := range fileNames {
		fullPath := filepath.Join(devicesDir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return 0, fmt.Errorf("reading %s: %w", fullPath, err)
		}

		devices, err := parseDeviceFile(data)
		if err != nil {
			return 0, fmt.Errorf("parsing %s: %w", fullPath, err)
		}

		for deviceName, deviceCfg := range devices {
			if _, exists := cfg.DesiredState.Devices[deviceName]; exists {
				return 0, fmt.Errorf("duplicate device %q found in %s", deviceName, fullPath)
			}
			cfg.DesiredState.Devices[deviceName] = deviceCfg
			added++
		}
	}

	return added, nil
}

func parseDeviceFile(data []byte) (map[string]DeviceConfig, error) {
	var wrapped deviceFileWrapper
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}
	if len(wrapped.Devices) > 0 {
		return wrapped.Devices, nil
	}

	var direct map[string]DeviceConfig
	if err := yaml.Unmarshal(data, &direct); err != nil {
		return nil, err
	}
	if len(direct) > 0 {
		return direct, nil
	}

	return nil, fmt.Errorf("no devices defined; expected either top-level devices: map or direct device map")
}

// ResolveCredentials resolves credentials for a device.
// TODO: not yet wired into SNMP/gNMI collectors.
func (c *Config) ResolveCredentials(deviceName string) CredentialEntry {
	dev, ok := c.DesiredState.Devices[deviceName]
	if !ok {
		// Return default if available
		if c.DesiredState.Global.DefaultCredentials != "" {
			if cred, ok := c.Credentials.Credentials[c.DesiredState.Global.DefaultCredentials]; ok {
				return cred
			}
		}
		return CredentialEntry{}
	}

	// Check device-specific credential reference
	if dev.CredentialsRef != "" {
		if cred, ok := c.Credentials.Credentials[dev.CredentialsRef]; ok {
			return cred
		}
	}

	// Fall back to default
	if c.DesiredState.Global.DefaultCredentials != "" {
		if cred, ok := c.Credentials.Credentials[c.DesiredState.Global.DefaultCredentials]; ok {
			return cred
		}
	}

	return CredentialEntry{}
}

// ValidateConfig validates the configuration
func ValidateConfig(cfg *Config) error {
	for name, device := range cfg.DesiredState.Devices {
		if device.Address == "" {
			return fmt.Errorf("device %s: address is required", name)
		}

		// Validate credential references
		if device.CredentialsRef != "" {
			if _, ok := cfg.Credentials.Credentials[device.CredentialsRef]; !ok {
				return fmt.Errorf("device %s: references unknown credential %s", name, device.CredentialsRef)
			}
		}

		// Validate interfaces
		for ifName, ifCfg := range device.Interfaces {
			if ifCfg.DesiredState == "" {
				return fmt.Errorf("device %s, interface %s: desired_state is required", name, ifName)
			}
			if ifCfg.DesiredState != "up" && ifCfg.DesiredState != "down" {
				return fmt.Errorf("device %s, interface %s: desired_state must be 'up' or 'down'", name, ifName)
			}

			if ifCfg.AdminState != "" && ifCfg.AdminState != "enabled" && ifCfg.AdminState != "disabled" {
				return fmt.Errorf("device %s, interface %s: admin_state must be 'enabled' or 'disabled'", name, ifName)
			}
			if ifCfg.SNMPIfIndex < 0 {
				return fmt.Errorf("device %s, interface %s: snmp_ifindex must be >= 0", name, ifName)
			}

			// Validate member policy if members are defined
			if ifCfg.Members != nil && len(ifCfg.Members.Required) > 0 {
				if ifCfg.MemberPolicy == nil {
					return fmt.Errorf("device %s, interface %s: has members but no member_policy", name, ifName)
				}
				seenMembers := make(map[string]struct{}, len(ifCfg.Members.Required))
				for _, member := range ifCfg.Members.Required {
					member = strings.TrimSpace(member)
					if member == "" {
						return fmt.Errorf("device %s, interface %s: members.required cannot contain empty entries", name, ifName)
					}
					if member == ifName {
						return fmt.Errorf("device %s, interface %s: cannot reference itself in members.required", name, ifName)
					}
					if _, exists := seenMembers[member]; exists {
						return fmt.Errorf("device %s, interface %s: duplicate member %s", name, ifName, member)
					}
					seenMembers[member] = struct{}{}
				}
				if ifCfg.MemberPolicy.Mode != "all_active" &&
					ifCfg.MemberPolicy.Mode != "min_active" &&
					ifCfg.MemberPolicy.Mode != "per_stack_minimum" {
					return fmt.Errorf("device %s, interface %s: member_policy.mode must be 'all_active', 'min_active', or 'per_stack_minimum'", name, ifName)
				}
				if ifCfg.MemberPolicy.Mode == "min_active" && ifCfg.MemberPolicy.Minimum <= 0 {
					return fmt.Errorf("device %s, interface %s: member_policy.minimum must be > 0 for min_active mode", name, ifName)
				}
				if ifCfg.MemberPolicy.Mode == "min_active" && ifCfg.MemberPolicy.Minimum > len(ifCfg.Members.Required) {
					return fmt.Errorf("device %s, interface %s: member_policy.minimum cannot exceed required member count", name, ifName)
				}
				if ifCfg.MemberPolicy.CriticalThresholdPct != nil {
					if *ifCfg.MemberPolicy.CriticalThresholdPct <= 0 || *ifCfg.MemberPolicy.CriticalThresholdPct > 100 {
						return fmt.Errorf("device %s, interface %s: member_policy.critical_threshold_pct must be > 0 and <= 100", name, ifName)
					}
				}
				if ifCfg.MemberPolicy.WarningThresholdPct != nil {
					if *ifCfg.MemberPolicy.WarningThresholdPct <= 0 || *ifCfg.MemberPolicy.WarningThresholdPct >= 100 {
						return fmt.Errorf("device %s, interface %s: member_policy.warning_threshold_pct must be > 0 and < 100", name, ifName)
					}
				}
				if ifCfg.MemberPolicy.CriticalThresholdPct != nil && ifCfg.MemberPolicy.WarningThresholdPct != nil &&
					*ifCfg.MemberPolicy.WarningThresholdPct >= *ifCfg.MemberPolicy.CriticalThresholdPct {
					return fmt.Errorf("device %s, interface %s: member_policy.warning_threshold_pct must be less than critical_threshold_pct", name, ifName)
				}
			}
		}
	}

	if cfg.DesiredState.Global.TelemetryMode != "snmp_validate_only" &&
		cfg.DesiredState.Global.TelemetryMode != "telemetry_ingest_push" {
		return fmt.Errorf("global.telemetry_mode must be 'snmp_validate_only' or 'telemetry_ingest_push'")
	}

	if cfg.DesiredState.Global.SNMP.Version != "2c" {
		return fmt.Errorf("global.snmp.version currently only supports '2c'")
	}
	if cfg.DesiredState.Global.SNMP.TelemetryFallbackEnabled &&
		cfg.DesiredState.Global.SNMP.TelemetryFallbackInterval <= 0 {
		return fmt.Errorf("global.snmp.telemetry_fallback_interval must be > 0 when telemetry_fallback_enabled=true")
	}

	if err := validateIngestListeners(&cfg.DesiredState.Global.Ingest); err != nil {
		return err
	}
	if cfg.DesiredState.Global.Ingest.StaleAfter < 0 {
		return fmt.Errorf("global.ingest.stale_after must be >= 0")
	}

	// Validate alert channels
	for name, channel := range cfg.Alerts.Channels {
		switch channel.Type {
		case "apprise", "":
			if channel.URLEnv == "" {
				return fmt.Errorf("channel %s: url_env is required for apprise channels", name)
			}
		case "openclaw":
			if channel.URLEnv == "" {
				return fmt.Errorf("channel %s: url_env is required for openclaw channels", name)
			}
		case "slack_chatops":
			if channel.ChannelEnv == "" {
				return fmt.Errorf("channel %s: channel_env is required for slack_chatops channels", name)
			}
		default:
			return fmt.Errorf("channel %s: unsupported type %q (valid: apprise, openclaw, slack_chatops)", name, channel.Type)
		}
		// Note: We don't validate env var values here as they may be set at runtime
	}

	// Validate alert rules reference valid channels
	for ruleName, rule := range cfg.Alerts.AlertRules {
		for _, chName := range rule.Channels {
			if _, ok := cfg.Alerts.Channels[chName]; !ok {
				return fmt.Errorf("alert rule %s: references unknown channel %s", ruleName, chName)
			}
		}
	}

	return nil
}

func validateIngestListeners(ing *IngestConfig) error {
	if ing == nil {
		return nil
	}
	seen := make(map[uint16]struct{})
	primary := ing.Port
	if primary == 0 {
		primary = 57500
	}
	ports := []uint16{primary}
	for _, add := range ing.AdditionalListeners {
		ports = append(ports, add.Port)
	}
	for _, p := range ports {
		if p == 0 {
			return fmt.Errorf("global.ingest: listener port cannot be 0")
		}
		if _, dup := seen[p]; dup {
			return fmt.Errorf("global.ingest: duplicate listener port %d", p)
		}
		seen[p] = struct{}{}
	}
	return nil
}
