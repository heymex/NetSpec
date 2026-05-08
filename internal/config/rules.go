package config

// RulesConfig defines business rules for port monitoring defaults at discovery time.
type RulesConfig struct {
	DeviceRoles []DeviceRole `yaml:"device_roles"`
}

// DeviceRole matches devices by hostname prefix and defines port rules for that role.
type DeviceRole struct {
	Name      string     `yaml:"name"`
	Prefix    string     `yaml:"prefix"` // hostname prefix, e.g. "dsw", "asw"
	PortRules []PortRule `yaml:"port_rules"`
}

// PortRule matches ports by alias glob pattern and defines monitoring defaults.
// Rules are evaluated in order; the first match wins.
type PortRule struct {
	Label        string        `yaml:"label,omitempty"`
	Match        string        `yaml:"match"`              // glob pattern matched against interface alias/description
	Monitor      *bool         `yaml:"monitor,omitempty"`  // nil = user decides; false = exclude from YAML
	DesiredState string        `yaml:"desired_state,omitempty"`
	Alerts       AlertSeverity `yaml:"alerts,omitempty"`
}
