package config

import "time"

// Config represents the complete NetSpec configuration
type Config struct {
	DesiredState DesiredStateConfig `yaml:"desired_state"`
	Alerts       AlertsConfig       `yaml:"alerts"`
	Credentials  CredentialsConfig  `yaml:"credentials"`
	Maintenance  MaintenanceConfig  `yaml:"maintenance"`
	Rules        RulesConfig        `yaml:"-"` // loaded from rules.yaml, not inline
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
	Slack         SlackChatOpsConfig       `yaml:"slack_chatops,omitempty"`
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
	// TelemetryFallbackEnabled enables periodic full SNMP polling while in
	// telemetry_ingest_push mode. This is a safety net for missed telemetry and
	// increases SNMP/device load significantly.
	TelemetryFallbackEnabled bool `yaml:"telemetry_fallback_enabled,omitempty"`
	// TelemetryFallbackInterval controls how often fallback full-device SNMP
	// polls run when telemetry_fallback_enabled is true.
	TelemetryFallbackInterval time.Duration `yaml:"telemetry_fallback_interval,omitempty"`
	Timeout                   time.Duration `yaml:"timeout,omitempty"`
	Retries                   int           `yaml:"retries,omitempty"`
}

// DefaultIngestStaleAfter is used when telemetry_ingest_push is on and
// global.ingest.stale_after is omitted or zero.
const DefaultIngestStaleAfter = 5 * time.Minute

// IngestConfig contains push telemetry ingest listener configuration.
// Use one TCP port per upstream pipeline (e.g. Cribl destination) and optional
// source labels — same NDJSON body format on every port; only the listening
// port selects how events are tagged (similar to syslog sourcetype per input).
type IngestConfig struct {
	ListenAddress string `yaml:"listen_address,omitempty"`
	Port          uint16 `yaml:"port,omitempty"`
	// Source is applied to PushTelemetryEvent.Source for the primary port when non-empty.
	Source string `yaml:"source,omitempty"`
	// AdditionalListeners binds more TCP ports to optional source labels. Ports must be unique with Port.
	AdditionalListeners []IngestListenerEntry `yaml:"additional_listeners,omitempty"`
	TokenEnv            string                `yaml:"token_env,omitempty"`
	// StaleAfter is how long without an accepted ingest event before firing
	// telemetry_ingest_stale. Zero (or omitted) defaults to 5m when
	// telemetry_mode is telemetry_ingest_push.
	StaleAfter time.Duration `yaml:"stale_after,omitempty"`
}

// IngestListenerEntry is an extra TCP listener with the same auth and address as the primary ingest block.
type IngestListenerEntry struct {
	Port   uint16 `yaml:"port"`
	Source string `yaml:"source,omitempty"`
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
	Mode                 string   `yaml:"mode"` // "all_active", "min_active", "per_stack_minimum"
	Minimum              int      `yaml:"minimum,omitempty"`
	PerStackMinimum      int      `yaml:"per_stack_minimum,omitempty"`
	CriticalThresholdPct *float64 `yaml:"critical_threshold_pct,omitempty"`
	WarningThresholdPct  *float64 `yaml:"warning_threshold_pct,omitempty"`
}

// AlertSeverity defines alert severities for different conditions
type AlertSeverity struct {
	StateMismatch string `yaml:"state_mismatch,omitempty"`
	MemberDown    string `yaml:"member_down,omitempty"`
	ChannelDown   string `yaml:"channel_down,omitempty"`
	AdminDown     string `yaml:"admin_down,omitempty"`
}

// ChannelConfig defines a notification channel.
// type "apprise" (default) routes through Apprise-API via url_env.
// type "slack_chatops" uses the direct Slack API with interactive Block Kit messages;
// set channel_env to the env var holding the Slack channel ID (e.g. "C0123456789").
// type "openclaw" POSTs structured JSON to an OpenClaw Gateway webhook
// (e.g. POST /hooks/agent or a mapped /hooks/<name>); url_env holds the full webhook URL,
// token_env (optional) holds the shared hook token for Authorization: Bearer.
type ChannelConfig struct {
	Type            string   `yaml:"type"`
	URLEnv          string   `yaml:"url_env,omitempty"`
	TokenEnv        string   `yaml:"token_env,omitempty"`
	ChannelEnv      string   `yaml:"channel_env,omitempty"`
	SeverityFilter  []string `yaml:"severity_filter,omitempty"`
	EscalationDelay int      `yaml:"escalation_delay,omitempty"`
}

// SlackChatOpsConfig enables two-way Slack alerting via the Slack Web API.
type SlackChatOpsConfig struct {
	Enabled          bool   `yaml:"enabled"`
	BotTokenEnv      string `yaml:"bot_token_env"`      // env var: xoxb-… bot token
	SigningSecretEnv string `yaml:"signing_secret_env"` // env var: Slack signing secret for webhook validation
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

// MaintenanceWindow defines maintenance window configuration.
// TODO: not yet wired into runtime alert suppression logic.
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
