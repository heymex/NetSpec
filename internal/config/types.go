package config

import "time"

// Config represents the complete NetSpec configuration
type Config struct {
	Global    GlobalConfig            `yaml:"global"`
	Devices   map[string]DeviceConfig `yaml:"devices"`
	Alerts    AlertConfig             `yaml:"alerts"`
	Maintenance []MaintenanceWindow   `yaml:"maintenance_windows,omitempty"`
}

// GlobalConfig contains global settings
type GlobalConfig struct {
	DefaultCredentials string        `yaml:"default_credentials"`
	GNMIPort           int           `yaml:"gnmi_port"`
	CollectionInterval time.Duration `yaml:"collection_interval"`
}

// DeviceConfig defines a device to monitor
type DeviceConfig struct {
	Address       string                 `yaml:"address"`
	Description   string                 `yaml:"description,omitempty"`
	CredentialsRef string                `yaml:"credentials_ref,omitempty"`
	Interfaces    map[string]InterfaceConfig `yaml:"interfaces,omitempty"`
}

// InterfaceConfig defines interface monitoring requirements
type InterfaceConfig struct {
	Description   string            `yaml:"description,omitempty"`
	DesiredState  string            `yaml:"desired_state"` // "up" or "down"
	AdminState    string            `yaml:"admin_state,omitempty"` // "enabled" or "disabled"
	Members       *MemberConfig     `yaml:"members,omitempty"`
	MemberPolicy  *MemberPolicy     `yaml:"member_policy,omitempty"`
	Alerts        AlertSeverity     `yaml:"alerts,omitempty"`
}

// MemberConfig defines port-channel member requirements
type MemberConfig struct {
	Required []string `yaml:"required,omitempty"`
}

// MemberPolicy defines port-channel member policies
type MemberPolicy struct {
	Mode            string `yaml:"mode"` // "all_active", "min_active", "per_stack_minimum"
	Minimum         int    `yaml:"minimum,omitempty"`
	PerStackMinimum int    `yaml:"per_stack_minimum,omitempty"`
}

// AlertSeverity defines alert severities for different conditions
type AlertSeverity struct {
	StateMismatch string `yaml:"state_mismatch,omitempty"`
	MemberDown    string `yaml:"member_down,omitempty"`
	ChannelDown   string `yaml:"channel_down,omitempty"`
	AdminDown     string `yaml:"admin_down,omitempty"`
}

// AlertConfig defines alert routing and behavior
type AlertConfig struct {
	Channels      map[string]ChannelConfig `yaml:"channels"`
	AlertRules    map[string]AlertRule     `yaml:"alert_rules"`
	AlertBehavior AlertBehavior            `yaml:"alert_behavior"`
}

// ChannelConfig defines a notification channel
type ChannelConfig struct {
	Type           string   `yaml:"type"`
	URLEnv         string   `yaml:"url_env"`
	SeverityFilter []string `yaml:"severity_filter,omitempty"`
	EscalationDelay int     `yaml:"escalation_delay,omitempty"`
}

// AlertRule defines routing rules for alerts
type AlertRule struct {
	Channels []string `yaml:"channels"`
}

// AlertBehavior defines alert behavior settings
type AlertBehavior struct {
	DeduplicationWindow time.Duration `yaml:"deduplication_window"`
	StatePersistence    StatePersistence `yaml:"state_persistence,omitempty"`
}

// StatePersistence defines state persistence settings
type StatePersistence struct {
	Enabled  bool   `yaml:"enabled"`
	Path     string `yaml:"path"`
	OnRestart string `yaml:"on_restart"` // "warn_unknown" or "silent"
}

// MaintenanceWindow defines maintenance window configuration
type MaintenanceWindow struct {
	Name           string   `yaml:"name"`
	Devices        []string `yaml:"devices"`
	Schedule       Schedule `yaml:"schedule"`
	SuppressAlerts bool     `yaml:"suppress_alerts"`
}

// Schedule defines maintenance window schedule
type Schedule struct {
	Type     string `yaml:"type"` // "recurring" or "one-time"
	Day      string `yaml:"day,omitempty"`
	Start    string `yaml:"start"`
	End      string `yaml:"end"`
	Timezone string `yaml:"timezone,omitempty"`
}
