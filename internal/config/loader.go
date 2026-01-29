package config

import (
	"fmt"
	"os"
	"path/filepath"
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

	// Load alerts.yaml
	if err := loadYAML(filepath.Join(dir, "alerts.yaml"), &cfg.Alerts); err != nil {
		return nil, fmt.Errorf("loading alerts.yaml: %w", err)
	}

	// Load credentials.yaml (optional)
	credentialsPath := filepath.Join(dir, "credentials.yaml")
	if _, err := os.Stat(credentialsPath); err == nil {
		if err := loadYAML(credentialsPath, &cfg.Credentials); err != nil {
			return nil, fmt.Errorf("loading credentials.yaml: %w", err)
		}
	}

	// Load maintenance.yaml (optional)
	maintenancePath := filepath.Join(dir, "maintenance.yaml")
	if _, err := os.Stat(maintenancePath); err == nil {
		if err := loadYAML(maintenancePath, &cfg.Maintenance); err != nil {
			return nil, fmt.Errorf("loading maintenance.yaml: %w", err)
		}
	}

	// Set defaults
	if cfg.DesiredState.Global.GNMIPort == 0 {
		cfg.DesiredState.Global.GNMIPort = 9339
	}
	if cfg.DesiredState.Global.CollectionInterval == 0 {
		cfg.DesiredState.Global.CollectionInterval = 10 * time.Second
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

// ResolveCredentials resolves credentials for a device
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
	if len(cfg.DesiredState.Devices) == 0 {
		return fmt.Errorf("no devices configured")
	}

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

			// Validate member policy if members are defined
			if ifCfg.Members != nil && len(ifCfg.Members.Required) > 0 {
				if ifCfg.MemberPolicy == nil {
					return fmt.Errorf("device %s, interface %s: has members but no member_policy", name, ifName)
				}
				if ifCfg.MemberPolicy.Mode != "all_active" &&
					ifCfg.MemberPolicy.Mode != "min_active" &&
					ifCfg.MemberPolicy.Mode != "per_stack_minimum" {
					return fmt.Errorf("device %s, interface %s: member_policy.mode must be 'all_active', 'min_active', or 'per_stack_minimum'", name, ifName)
				}
				if ifCfg.MemberPolicy.Mode == "min_active" && ifCfg.MemberPolicy.Minimum <= 0 {
					return fmt.Errorf("device %s, interface %s: member_policy.minimum must be > 0 for min_active mode", name, ifName)
				}
			}
		}
	}

	// Validate alert channels
	for name, channel := range cfg.Alerts.Channels {
		if channel.Type != "apprise" {
			return fmt.Errorf("channel %s: only 'apprise' type is supported", name)
		}
		if channel.URLEnv == "" {
			return fmt.Errorf("channel %s: url_env is required", name)
		}
		// Note: We don't validate env var exists here as it may be set at runtime
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
