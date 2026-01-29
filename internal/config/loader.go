package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads and parses the configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Set defaults
	if cfg.Global.GNMIPort == 0 {
		cfg.Global.GNMIPort = 9339
	}
	if cfg.Global.CollectionInterval == 0 {
		cfg.Global.CollectionInterval = 10 * time.Second
	}
	if cfg.Alerts.AlertBehavior.DeduplicationWindow == 0 {
		cfg.Alerts.AlertBehavior.DeduplicationWindow = 5 * time.Minute
	}

	// Validate configuration
	if err := ValidateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// ValidateConfig validates the configuration
func ValidateConfig(cfg *Config) error {
	if len(cfg.Devices) == 0 {
		return fmt.Errorf("no devices configured")
	}

	for name, device := range cfg.Devices {
		if device.Address == "" {
			return fmt.Errorf("device %s: address is required", name)
		}

		// Validate interfaces
		for ifName, ifCfg := range device.Interfaces {
			if ifCfg.DesiredState != "up" && ifCfg.DesiredState != "down" {
				return fmt.Errorf("device %s, interface %s: desired_state must be 'up' or 'down'", name, ifName)
			}

			if ifCfg.AdminState != "" && ifCfg.AdminState != "enabled" && ifCfg.AdminState != "disabled" {
				return fmt.Errorf("device %s, interface %s: admin_state must be 'enabled' or 'disabled'", name, ifName)
			}

			// Validate member policy if members are defined
			if ifCfg.Members != nil && ifCfg.MemberPolicy != nil {
				if ifCfg.MemberPolicy.Mode != "all_active" && 
				   ifCfg.MemberPolicy.Mode != "min_active" && 
				   ifCfg.MemberPolicy.Mode != "per_stack_minimum" {
					return fmt.Errorf("device %s, interface %s: member_policy.mode must be 'all_active', 'min_active', or 'per_stack_minimum'", name, ifName)
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
		// Check if environment variable exists
		if os.Getenv(channel.URLEnv) == "" {
			return fmt.Errorf("channel %s: environment variable %s is not set", name, channel.URLEnv)
		}
	}

	return nil
}
