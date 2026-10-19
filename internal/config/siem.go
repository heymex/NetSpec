package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	siemAuthNone   = "none"
	siemAuthBearer = "bearer"
	siemAuthAPIKey = "api_key"
)

// ResolvedSIEMIndex returns the configured index, preferring a non-empty
// environment value from IndexEnv when that env var is set.
func (c ChannelConfig) ResolvedSIEMIndex() string {
	if envName := strings.TrimSpace(c.IndexEnv); envName != "" {
		if v := strings.TrimSpace(os.Getenv(envName)); v != "" {
			return v
		}
	}
	return strings.TrimSpace(c.Index)
}

// ResolvedSIEMEndpoints returns destinations for splunk / elasticsearch channels.
// Elasticsearch defaults auth to api_key when token_env is set (not bearer).
func (c ChannelConfig) ResolvedSIEMEndpoints() []WebhookEndpoint {
	raw := c.Endpoints
	if len(raw) == 0 {
		raw = []WebhookEndpoint{{
			URLEnv:   c.URLEnv,
			TokenEnv: c.TokenEnv,
			Auth:     c.Auth,
		}}
	}

	elastic := c.Type == "elasticsearch" || c.Type == "elastic"
	out := make([]WebhookEndpoint, len(raw))
	for i, ep := range raw {
		ep.Name = strings.TrimSpace(ep.Name)
		ep.URLEnv = strings.TrimSpace(ep.URLEnv)
		ep.TokenEnv = strings.TrimSpace(ep.TokenEnv)
		ep.Auth = strings.ToLower(strings.TrimSpace(ep.Auth))
		ep.HeaderName = ""

		if ep.Auth == "" {
			switch {
			case elastic && ep.TokenEnv != "":
				ep.Auth = siemAuthAPIKey
			case elastic:
				ep.Auth = siemAuthNone
			default:
				// splunk ignores Auth at send time
				ep.Auth = siemAuthNone
			}
		}
		if ep.Name == "" {
			if len(c.Endpoints) == 0 {
				ep.Name = "default"
			} else {
				ep.Name = fmt.Sprintf("endpoint-%d", i+1)
			}
		}
		out[i] = ep
	}
	return out
}

func validateSIEMChannel(name string, channel ChannelConfig) error {
	topLevel := channel.URLEnv != "" || channel.TokenEnv != "" || channel.Auth != "" || channel.HeaderName != ""
	if len(channel.Endpoints) > 0 && topLevel {
		return fmt.Errorf("channel %s: set either url_env or endpoints, not both", name)
	}
	if len(channel.Endpoints) == 0 && strings.TrimSpace(channel.URLEnv) == "" {
		return fmt.Errorf("channel %s: url_env or endpoints is required for %s channels", name, channel.Type)
	}
	if strings.TrimSpace(channel.HeaderName) != "" {
		return fmt.Errorf("channel %s: header_name is not used for %s channels", name, channel.Type)
	}

	switch channel.Type {
	case "splunk":
		if strings.TrimSpace(channel.Auth) != "" {
			return fmt.Errorf("channel %s: auth is not used for splunk channels (HEC always sends Authorization: Splunk)", name)
		}
	case "elasticsearch", "elastic":
		auth := strings.ToLower(strings.TrimSpace(channel.Auth))
		if auth != "" && auth != siemAuthNone && auth != siemAuthBearer && auth != siemAuthAPIKey {
			return fmt.Errorf("channel %s: unsupported auth %q (valid: api_key, bearer, none)", name, channel.Auth)
		}
	}

	raw := channel.Endpoints
	if len(raw) == 0 {
		raw = []WebhookEndpoint{{
			Name:     "default",
			URLEnv:   channel.URLEnv,
			TokenEnv: channel.TokenEnv,
			Auth:     channel.Auth,
		}}
	}

	seen := make(map[string]struct{})
	for i, ep := range raw {
		label := strings.TrimSpace(ep.Name)
		if label == "" {
			if len(channel.Endpoints) == 0 {
				label = "default"
			} else {
				label = fmt.Sprintf("endpoint-%d", i+1)
			}
		} else if len(channel.Endpoints) > 0 {
			if _, ok := seen[label]; ok {
				return fmt.Errorf("channel %s: duplicate endpoint name %q", name, label)
			}
			seen[label] = struct{}{}
		}

		if strings.TrimSpace(ep.URLEnv) == "" {
			return fmt.Errorf("channel %s: endpoint %s: url_env is required", name, label)
		}
		if strings.TrimSpace(ep.HeaderName) != "" {
			return fmt.Errorf("channel %s: endpoint %s: header_name is not used for %s endpoints", name, label, channel.Type)
		}

		switch channel.Type {
		case "splunk":
			if strings.TrimSpace(ep.Auth) != "" {
				return fmt.Errorf("channel %s: endpoint %s: auth is not used for splunk endpoints", name, label)
			}
			if strings.TrimSpace(ep.TokenEnv) == "" {
				return fmt.Errorf("channel %s: endpoint %s: token_env is required for splunk HEC", name, label)
			}
		case "elasticsearch", "elastic":
			auth := strings.ToLower(strings.TrimSpace(ep.Auth))
			if auth == "" {
				if strings.TrimSpace(ep.TokenEnv) != "" {
					auth = siemAuthAPIKey
				} else {
					auth = siemAuthNone
				}
			}
			switch auth {
			case siemAuthNone:
				if strings.TrimSpace(ep.TokenEnv) != "" && strings.EqualFold(strings.TrimSpace(ep.Auth), siemAuthNone) {
					return fmt.Errorf("channel %s: endpoint %s: token_env is not used when auth is none", name, label)
				}
			case siemAuthBearer, siemAuthAPIKey:
				if strings.TrimSpace(ep.TokenEnv) == "" {
					return fmt.Errorf("channel %s: endpoint %s: token_env is required for %s auth", name, label, auth)
				}
			default:
				return fmt.Errorf("channel %s: endpoint %s: unsupported auth %q (valid: api_key, bearer, none)", name, label, ep.Auth)
			}
		}
	}
	return nil
}
