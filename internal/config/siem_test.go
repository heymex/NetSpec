package config

import (
	"strings"
	"testing"
)

func TestValidateConfigSIEMChannels(t *testing.T) {
	t.Parallel()

	cfg := webhookValidationConfig(ChannelConfig{Type: "splunk"})
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected url_env required for splunk")
	}

	cfg = webhookValidationConfig(ChannelConfig{
		Type:   "splunk",
		URLEnv: "SPLUNK_HEC_URL",
	})
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "token_env") {
		t.Fatalf("expected token_env required, got %v", err)
	}

	cfg = webhookValidationConfig(ChannelConfig{
		Type:     "splunk",
		URLEnv:   "SPLUNK_HEC_URL",
		TokenEnv: "SPLUNK_HEC_TOKEN",
		Index:    "netops",
	})
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("valid splunk: %v", err)
	}

	cfg = webhookValidationConfig(ChannelConfig{
		Type:     "splunk",
		URLEnv:   "SPLUNK_HEC_URL",
		TokenEnv: "SPLUNK_HEC_TOKEN",
		Auth:     "bearer",
	})
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "auth is not used") {
		t.Fatalf("expected auth rejected for splunk, got %v", err)
	}

	cfg = webhookValidationConfig(ChannelConfig{
		Type:   "elasticsearch",
		URLEnv: "ELASTIC_URL",
		Index:  "netspec-alerts",
	})
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("elastic without token: %v", err)
	}

	cfg = webhookValidationConfig(ChannelConfig{
		Type:     "elastic",
		URLEnv:   "ELASTIC_URL",
		TokenEnv: "ELASTIC_API_KEY",
	})
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("elastic alias: %v", err)
	}

	cfg = webhookValidationConfig(ChannelConfig{
		Type:     "elasticsearch",
		URLEnv:   "ELASTIC_URL",
		TokenEnv: "ELASTIC_API_KEY",
		Auth:     "header",
	})
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "unsupported auth") {
		t.Fatalf("expected unsupported auth, got %v", err)
	}

	cfg = webhookValidationConfig(ChannelConfig{
		Type: "elasticsearch",
		Endpoints: []WebhookEndpoint{
			{Name: "primary", URLEnv: "ELASTIC_URL", TokenEnv: "ELASTIC_API_KEY"},
			{Name: "primary", URLEnv: "ELASTIC_URL_2", TokenEnv: "ELASTIC_API_KEY_2"},
		},
	})
	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate endpoint, got %v", err)
	}
}

func TestResolvedSIEMIndexEnvOverride(t *testing.T) {
	ch := ChannelConfig{Index: "yaml-index", IndexEnv: "SIEM_INDEX"}
	t.Setenv("SIEM_INDEX", "env-index")
	if got := ch.ResolvedSIEMIndex(); got != "env-index" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("SIEM_INDEX", "")
	if got := ch.ResolvedSIEMIndex(); got != "yaml-index" {
		t.Fatalf("got %q", got)
	}
}

func TestResolvedSIEMEndpointsElasticAuthDefault(t *testing.T) {
	ch := ChannelConfig{
		Type:     "elasticsearch",
		URLEnv:   "ELASTIC_URL",
		TokenEnv: "ELASTIC_API_KEY",
	}
	eps := ch.ResolvedSIEMEndpoints()
	if len(eps) != 1 {
		t.Fatalf("len: %d", len(eps))
	}
	if eps[0].Auth != siemAuthAPIKey {
		t.Fatalf("auth: %q", eps[0].Auth)
	}
	if eps[0].Name != "default" {
		t.Fatalf("name: %q", eps[0].Name)
	}
}
