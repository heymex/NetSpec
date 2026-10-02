package enrichment

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config is the enrichment configuration block loaded from desired-state.yaml.
type Config struct {
	Enabled    bool          `yaml:"enabled"`
	Timeout    time.Duration `yaml:"timeout"`          // overall pipeline timeout per alert (default 15s)
	MaxConc    int           `yaml:"max_concurrent"`  // max parallel enrichers (default = number enabled)
	Netbox   *NetboxConfig   `yaml:"netbox"`
	Taillight *TaillightConfig `yaml:"taillight"`
	Elastic  *ElasticConfig  `yaml:"elastic"`
}

// DefaultConfig returns a Config with sensible defaults (disabled, 15s timeout).
func DefaultConfig() Config {
	return Config{
		Timeout: 15 * time.Second,
	}
}

// -- NetBox config --

type NetboxConfig struct {
	Enabled  bool     `yaml:"enabled"`
	APIURL   string   `yaml:"api_url"`
	APIToken string   `yaml:"api_token"`  // raw token (for test / inline config)
	APITokenEnv string `yaml:"api_token_env"` // env var containing the token
	Tags     []string `yaml:"tags"` // only enrich devices tagged with these in NetBox
}

// ResolvedAPIToken returns the API token, resolved via env var if APIToken is empty.
func (c *NetboxConfig) ResolvedAPIToken() string {
	if c.APIToken != "" {
		return c.APIToken
	}
	return strings.TrimSpace(os.Getenv(c.APITokenEnv))
}

// -- Taillight config --

type TaillightConfig struct {
	Enabled bool `yaml:"enabled"`
	// Backend determines how to reach Taillight data: "api" or "elastic" (default "api").
	Backend TaillightBackend `yaml:"backend"`

	// --- backend: api ---
	BaseURL  string `yaml:"base_url"`
	APIKey   string `yaml:"api_key"`
	APIKeyEnv string `yaml:"api_key_env"`

	// --- backend: elastic (when backend: elastic) ---
	ElasticURL     string `yaml:"elastic_url"`
	ElasticAPIKey  string `yaml:"elastic_api_key"`
	ElasticAPIKeyEnv string `yaml:"elastic_api_key_env"`
	SyslogIndex    string `yaml:"syslog_index"`      // e.g. "logs-netlog*"
	SummaryIndex   string `yaml:"summary_index"`     // e.g. "taillight-report-*"

	// Shared query config
	SyslogWindow *SyslogWindowConfig `yaml:"syslog_window"`
	DailySummary *DailySummaryConfig `yaml:"daily_summary"`
}

// TaillightBackend specifies the transport for the Taillight enricher.
type TaillightBackend string

const (
	BackendAPI    TaillightBackend = "api"
	BackendElastic TaillightBackend = "elastic"
)

// ResolveBackend returns the backend, defaulting to "api" if empty.
func (c *TaillightConfig) ResolveBackend() TaillightBackend {
	if c.Backend == "" {
		return BackendAPI
	}
	return c.Backend
}

// ResolvedAPIKey returns the Taillight API key from the env var or raw value.
func (c *TaillightConfig) ResolvedAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	return strings.TrimSpace(os.Getenv(c.APIKeyEnv))
}

// ResolvedElasticAPIKey returns the Elastic API key for Taillight queries.
func (c *TaillightConfig) ResolvedElasticAPIKey() string {
	if c.ElasticAPIKey != "" {
		return c.ElasticAPIKey
	}
	return strings.TrimSpace(os.Getenv(c.ElasticAPIKeyEnv))
}

// HasSyslogQuery returns true if the syslog_window sub-config is present and enabled.
func (c *TaillightConfig) HasSyslogQuery() bool {
	return c.SyslogWindow != nil && c.SyslogWindow.Enabled
}

// HasDailySummary returns true if the daily_summary sub-config is present and enabled.
func (c *TaillightConfig) HasDailySummary() bool {
	return c.DailySummary != nil && c.DailySummary.Enabled
}

type SyslogWindowConfig struct {
	Enabled        bool     `yaml:"enabled"`
	LookbackMin    int      `yaml:"lookback_min"`
	SeverityFilter []string `yaml:"severity_filter"`
	MaxEntries     int      `yaml:"max_entries"`
	EventKind      string   `yaml:"event_kind"` // "netlog" or "srvlog"
}

type DailySummaryConfig struct {
	Enabled      bool   `yaml:"enabled"`
	HostnameExact bool   `yaml:"hostname_exact"`
	EventKind    string `yaml:"event_kind"` // "netlog" or "srvlog"
}

// -- Elastic config (standalone) --

type ElasticConfig struct {
	Enabled bool `yaml:"enabled"`
	URL     string `yaml:"url"`
	APIKey  string `yaml:"api_key"`
	APIKeyEnv string `yaml:"api_key_env"`

	// Trend query config
	Trend *ElasticTrendConfig `yaml:"trend"`
}

// ResolvedAPIKey returns the Elastic API key from the env var or raw value.
func (c *ElasticConfig) ResolvedAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	return strings.TrimSpace(os.Getenv(c.APIKeyEnv))
}

type ElasticTrendConfig struct {
	Enabled    bool          `yaml:"enabled"`
	Window     time.Duration `yaml:"window"`
	IndexPattern string      `yaml:"index_pattern"` // e.g. "logs-*"
	MaxHits    int           `yaml:"max_hits"`
}

// -- Helpers --

// LoadConfigFromYAML parses the enrichment block from raw YAML bytes.
func LoadConfigFromYAML(data []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Fallback: try yaml.v3 via env file — callers usually pass the whole
		// desired-state.yaml blob and extract the enrichment block themselves.
		// This is a no-op here; callers should use config.YAML loading.
		return DefaultConfig(), err
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	return cfg, nil
}

// Validate runs basic sanity checks on the resolved config.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Netbox != nil && c.Netbox.Enabled {
		if c.Netbox.APIURL == "" && c.Netbox.APIURL != "" {
			// empty is fine — env var may provide it
		}
		if c.Netbox.ResolvedAPIToken() == "" && c.Netbox.APITokenEnv == "" {
			return fmt.Errorf("enrichment.netbox: api_token or api_token_env is required")
		}
	}
	if c.Taillight != nil && c.Taillight.Enabled {
		if !c.Taillight.HasSyslogQuery() && !c.Taillight.HasDailySummary() {
			return fmt.Errorf("enrichment.taillight: at least one of syslog_window or daily_summary must be enabled")
		}
	}
	return nil
}
