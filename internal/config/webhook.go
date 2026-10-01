package config

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	webhookAuthNone            = "none"
	webhookAuthBearer          = "bearer"
	webhookAuthHeader          = "header"
	webhookAuthBearerAndHeader = "bearer_and_header"
	openClawTokenHeader        = "x-openclaw-token"
)

// ResolvedWebhookEndpoints returns the endpoints this channel will POST to,
// with auth filled in. A channel uses either the top-level url_env fields or
// the endpoints list.
func (c ChannelConfig) ResolvedWebhookEndpoints() []WebhookEndpoint {
	raw := c.Endpoints
	if len(raw) == 0 {
		raw = []WebhookEndpoint{{
			URLEnv:     c.URLEnv,
			TokenEnv:   c.TokenEnv,
			Auth:       c.Auth,
			HeaderName: c.HeaderName,
		}}
	}

	openclawAlias := c.Type == "openclaw"
	out := make([]WebhookEndpoint, len(raw))
	for i, ep := range raw {
		ep = normalizeWebhookEndpoint(ep, openclawAlias)
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

func normalizeWebhookEndpoint(ep WebhookEndpoint, openclawAlias bool) WebhookEndpoint {
	ep.Name = strings.TrimSpace(ep.Name)
	ep.URLEnv = strings.TrimSpace(ep.URLEnv)
	ep.TokenEnv = strings.TrimSpace(ep.TokenEnv)
	ep.Auth = strings.ToLower(strings.TrimSpace(ep.Auth))
	ep.HeaderName = strings.TrimSpace(ep.HeaderName)

	if ep.Auth != "" {
		return ep
	}
	switch {
	case openclawAlias && ep.TokenEnv != "":
		ep.Auth = webhookAuthBearerAndHeader
		if ep.HeaderName == "" {
			ep.HeaderName = openClawTokenHeader
		}
	case ep.TokenEnv != "":
		ep.Auth = webhookAuthBearer
	default:
		ep.Auth = webhookAuthNone
	}
	return ep
}

func validateWebhookChannel(name string, channel ChannelConfig) error {
	topLevel := channel.URLEnv != "" || channel.TokenEnv != "" || channel.Auth != "" || channel.HeaderName != ""
	if len(channel.Endpoints) > 0 && topLevel {
		return fmt.Errorf("channel %s: set either url_env or endpoints, not both", name)
	}
	if len(channel.Endpoints) == 0 && strings.TrimSpace(channel.URLEnv) == "" {
		return fmt.Errorf("channel %s: url_env or endpoints is required for %s channels", name, channel.Type)
	}

	seen := make(map[string]struct{})
	for i, ep := range channel.ResolvedWebhookEndpoints() {
		label := ep.Name
		if i < len(channel.Endpoints) {
			configuredName := strings.TrimSpace(channel.Endpoints[i].Name)
			if configuredName != "" {
				if _, ok := seen[configuredName]; ok {
					return fmt.Errorf("channel %s: duplicate endpoint name %q", name, configuredName)
				}
				seen[configuredName] = struct{}{}
			}
		}
		if ep.URLEnv == "" {
			return fmt.Errorf("channel %s: endpoint %s: url_env is required", name, label)
		}
		switch ep.Auth {
		case webhookAuthNone:
			if ep.TokenEnv != "" {
				return fmt.Errorf("channel %s: endpoint %s: token_env is not used when auth is none", name, label)
			}
			if ep.HeaderName != "" {
				return fmt.Errorf("channel %s: endpoint %s: header_name is not used when auth is none", name, label)
			}
		case webhookAuthBearer:
			if ep.TokenEnv == "" {
				return fmt.Errorf("channel %s: endpoint %s: token_env is required for bearer auth", name, label)
			}
			if ep.HeaderName != "" {
				return fmt.Errorf("channel %s: endpoint %s: header_name is not used for bearer auth", name, label)
			}
		case webhookAuthHeader, webhookAuthBearerAndHeader:
			if ep.TokenEnv == "" {
				return fmt.Errorf("channel %s: endpoint %s: token_env is required for %s auth", name, label, ep.Auth)
			}
			if !validHTTPHeaderName(ep.HeaderName) {
				return fmt.Errorf("channel %s: endpoint %s: header_name is required for %s auth", name, label, ep.Auth)
			}
		default:
			return fmt.Errorf("channel %s: endpoint %s: unsupported auth %q (valid: bearer, header, bearer_and_header, none)", name, label, ep.Auth)
		}
	}
	return nil
}

func validHTTPHeaderName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if r > unicode.MaxASCII || !isHTTPTokenRune(r) {
			return false
		}
	}
	return true
}

func isHTTPTokenRune(r rune) bool {
	switch r {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	}
}
