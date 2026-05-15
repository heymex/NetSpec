package config

// RulesConfig defines business rules for port monitoring defaults at discovery time.
type RulesConfig struct {
	DeviceRoles []DeviceRole `yaml:"device_roles"`
}

// DeviceRole matches devices by hostname prefix and defines port rules for that role.
type DeviceRole struct {
	Name          string         `yaml:"name"`
	Prefix        string         `yaml:"prefix"` // hostname prefix, e.g. "dsw", "asw"
	PortRules     []PortRule     `yaml:"port_rules"`
	NeighborRules []NeighborRule `yaml:"neighbor_rules,omitempty"`
}

// NeighborRule classifies LLDP/CDP peers and optionally checks port alias patterns.
type NeighborRule struct {
	Label               string `yaml:"label"`
	MatchLLDPCapability string `yaml:"match_lldp_capability,omitempty"` // e.g. telephone, bridge, wlan_ap
	MatchSysName        string `yaml:"match_sys_name,omitempty"`
	MatchSysDesc        string `yaml:"match_sys_desc,omitempty"`
	MatchPlatform       string `yaml:"match_platform,omitempty"`
	ExpectAliasGlob     string `yaml:"expect_alias_glob,omitempty"`
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
