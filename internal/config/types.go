package config

import "time"

// Config represents the complete NetSpec configuration
type Config struct {
	DesiredState DesiredStateConfig `yaml:"desired_state"`
	Alerts       AlertsConfig       `yaml:"alerts"`
	Credentials  CredentialsConfig  `yaml:"credentials"`
	Maintenance  MaintenanceConfig  `yaml:"maintenance"`
	// Device source stats are runtime-only metadata for observability.
	MonolithicDeviceCount int `yaml:"-"`
	SplitDeviceCount      int `yaml:"-"`
}

// TotalDeviceCount returns total configured devices across all sources.
func (c *Config) TotalDeviceCount() int {
	return c.MonolithicDeviceCount + c.SplitDeviceCount
}

// DesiredStateConfig contains device and interface monitoring configuration
type DesiredStateConfig struct {
	Global  GlobalConfig            `yaml:"global"`
	Devices map[string]DeviceConfig `yaml:"devices"`
}

// AlertsConfig defines alert routing and behavior
type AlertsConfig struct {
	Channels      map[string]ChannelConfig `yaml:"channels"`
	AlertRules    map[string]AlertRule     `yaml:"alert_rules"`
	AlertBehavior AlertBehavior            `yaml:"alert_behavior"`
}

// CredentialsConfig defines credential storage
type CredentialsConfig struct {
	Credentials map[string]CredentialEntry `yaml:"credentials"`
}

// CredentialEntry defines a credential set
type CredentialEntry struct {
	Username      string `yaml:"username"`
	PasswordEnv   string `yaml:"password_env,omitempty"`
	PasswordVault string `yaml:"password_vault,omitempty"`
}

// MaintenanceConfig defines maintenance windows
type MaintenanceConfig struct {
	MaintenanceWindows []MaintenanceWindow `yaml:"maintenance_windows,omitempty"`
}

// GlobalConfig contains global settings
type GlobalConfig struct {
	DefaultCredentials string        `yaml:"default_credentials,omitempty"`
	CollectionInterval time.Duration `yaml:"collection_interval,omitempty"`
	TelemetryMode      string        `yaml:"telemetry_mode,omitempty"` // "snmp_validate_only" or "telemetry_ingest_push"
	SNMP               SNMPConfig    `yaml:"snmp,omitempty"`
	Ingest             IngestConfig  `yaml:"ingest,omitempty"`
}

// SNMPConfig contains SNMP polling configuration.
type SNMPConfig struct {
	Port               uint16        `yaml:"port,omitempty"`
	Version            string        `yaml:"version,omitempty"` // currently supports "2c"
	CommunityEnv       string        `yaml:"community_env,omitempty"`
	ValidationInterval time.Duration `yaml:"validation_interval,omitempty"`
	Timeout            time.Duration `yaml:"timeout,omitempty"`
	Retries            int           `yaml:"retries,omitempty"`
}

// IngestConfig contains push telemetry ingest listener configuration.
type IngestConfig struct {
	ListenAddress string `yaml:"listen_address,omitempty"`
	Port          uint16 `yaml:"port,omitempty"`
	TokenEnv      string `yaml:"token_env,omitempty"`
}

// DeviceConfig defines a device to monitor
type DeviceConfig struct {
	Address        string                     `yaml:"address"`
	Description    string                     `yaml:"description,omitempty"`
	CredentialsRef string                     `yaml:"credentials_ref,omitempty"`
	Interfaces     map[string]InterfaceConfig `yaml:"interfaces,omitempty"`
}

// InterfaceConfig defines interface monitoring requirements
type InterfaceConfig struct {
	Description  string        `yaml:"description,omitempty"`
	DesiredState string        `yaml:"desired_state"`         // "up" or "down"
	AdminState   string        `yaml:"admin_state,omitempty"` // "enabled" or "disabled"
	Monitor      bool          `yaml:"monitor"`
	SNMPIfIndex  int           `yaml:"snmp_ifindex,omitempty"`
	Members      *MemberConfig `yaml:"members,omitempty"`
	MemberPolicy *MemberPolicy `yaml:"member_policy,omitempty"`
	Alerts       AlertSeverity `yaml:"alerts,omitempty"`
}

// UnmarshalYAML ensures monitor defaults to true when omitted.
func (i *InterfaceConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawInterfaceConfig struct {
		Description  string        `yaml:"description,omitempty"`
		DesiredState string        `yaml:"desired_state"`
		AdminState   string        `yaml:"admin_state,omitempty"`
		Monitor      *bool         `yaml:"monitor"`
		SNMPIfIndex  int           `yaml:"snmp_ifindex,omitempty"`
		Members      *MemberConfig `yaml:"members,omitempty"`
		MemberPolicy *MemberPolicy `yaml:"member_policy,omitempty"`
		Alerts       AlertSeverity `yaml:"alerts,omitempty"`
	}

	var raw rawInterfaceConfig
	if err := unmarshal(&raw); err != nil {
		return err
	}

	i.Description = raw.Description
	i.DesiredState = raw.DesiredState
	i.AdminState = raw.AdminState
	i.SNMPIfIndex = raw.SNMPIfIndex
	i.Members = raw.Members
	i.MemberPolicy = raw.MemberPolicy
	i.Alerts = raw.Alerts
	i.Monitor = true
	if raw.Monitor != nil {
		i.Monitor = *raw.Monitor
	}
	return nil
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
	Type            string   `yaml:"type"`
	URLEnv          string   `yaml:"url_env"`
	SeverityFilter  []string `yaml:"severity_filter,omitempty"`
	EscalationDelay int      `yaml:"escalation_delay,omitempty"`
}

// AlertRule defines routing rules for alerts
type AlertRule struct {
	Channels []string `yaml:"channels"`
}

// AlertBehavior defines alert behavior settings
type AlertBehavior struct {
	DeduplicationWindow time.Duration    `yaml:"deduplication_window"`
	FlapDetection       FlapDetection    `yaml:"flap_detection,omitempty"`
	StatePersistence    StatePersistence `yaml:"state_persistence,omitempty"`
}

// FlapDetection defines flap detection settings
type FlapDetection struct {
	Enabled   bool          `yaml:"enabled"`
	Threshold int           `yaml:"threshold"`
	Window    time.Duration `yaml:"window"`
}

// StatePersistence defines state persistence settings
type StatePersistence struct {
	Enabled   bool   `yaml:"enabled"`
	Path      string `yaml:"path"`
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
