package rules

import (
	"strings"

	"github.com/netspec/netspec/internal/config"
)

// MatchResult holds the resolved defaults for an interface from a matched rule.
type MatchResult struct {
	RoleName  string
	RuleLabel string
	Monitor   *bool
	// DesiredState is empty string when the rule has no opinion.
	DesiredState string
	Alerts       config.AlertSeverity
}

// TrunkLink represents a parsed trunk port description.
// Format: t|localPO|remoteDevice:remotePort|remotePO.
type TrunkLink struct {
	LocalPortChannel  string `json:"local_port_channel,omitempty"`
	RemoteDevice      string `json:"remote_device,omitempty"`
	RemotePort        string `json:"remote_port,omitempty"`
	RemotePortChannel string `json:"remote_port_channel,omitempty"`
}

// MatchDevice returns the DeviceRole whose prefix is the longest case-insensitive
// prefix of hostname. Returns nil when no role matches.
func MatchDevice(hostname string, roles []config.DeviceRole) *config.DeviceRole {
	lower := strings.ToLower(hostname)
	var best *config.DeviceRole
	bestLen := 0
	for i := range roles {
		prefix := strings.ToLower(roles[i].Prefix)
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(lower, prefix) && len(prefix) > bestLen {
			best = &roles[i]
			bestLen = len(prefix)
		}
	}
	return best
}

// MatchPort returns the MatchResult for the first PortRule in role whose glob
// matches alias. Returns nil when no rule matches.
func MatchPort(alias string, role *config.DeviceRole) *MatchResult {
	if role == nil {
		return nil
	}
	for i := range role.PortRules {
		r := &role.PortRules[i]
		if GlobMatch(r.Match, alias) {
			return &MatchResult{
				RoleName:     role.Name,
				RuleLabel:    r.Label,
				Monitor:      r.Monitor,
				DesiredState: r.DesiredState,
				Alerts:       r.Alerts,
			}
		}
	}
	return nil
}

// Apply runs MatchDevice then MatchPort for the given hostname and alias.
// Returns nil when no role or no rule matches.
func Apply(hostname, alias string, roles []config.DeviceRole) *MatchResult {
	return MatchPort(alias, MatchDevice(hostname, roles))
}

// GlobMatch returns true if pattern matches s (case-insensitive). Only * is supported.
func GlobMatch(pattern, s string) bool {
	return globMatch(strings.ToLower(pattern), strings.ToLower(s))
}

func globMatch(p, s string) bool {
	for {
		if len(p) == 0 {
			return len(s) == 0
		}
		if p[0] == '*' {
			// Skip consecutive stars.
			for len(p) > 0 && p[0] == '*' {
				p = p[1:]
			}
			if len(p) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if globMatch(p, s[i:]) {
					return true
				}
			}
			return false
		}
		if len(s) == 0 || p[0] != s[0] {
			return false
		}
		p = p[1:]
		s = s[1:]
	}
}

// ParseTrunkDescription parses the convention "t|localPO|remoteDevice:remotePort|remotePO."
// Returns nil when the description does not match the format.
func ParseTrunkDescription(desc string) *TrunkLink {
	desc = strings.TrimSuffix(strings.TrimSpace(desc), ".")
	parts := strings.Split(desc, "|")
	if len(parts) != 4 {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(parts[0])) != "t" {
		return nil
	}
	localPO := strings.TrimSpace(parts[1])
	remoteRaw := strings.TrimSpace(parts[2])
	remotePO := strings.TrimSpace(parts[3])

	remoteDevice := remoteRaw
	remotePort := ""
	if idx := strings.LastIndex(remoteRaw, ":"); idx >= 0 {
		remoteDevice = remoteRaw[:idx]
		remotePort = remoteRaw[idx+1:]
	}

	return &TrunkLink{
		LocalPortChannel:  localPO,
		RemoteDevice:      remoteDevice,
		RemotePort:        remotePort,
		RemotePortChannel: remotePO,
	}
}
