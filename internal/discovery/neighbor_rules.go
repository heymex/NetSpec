package discovery

import (
	"fmt"
	"strings"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/rules"
)

// ApplyNeighborRules annotates interfaces using neighbor_rules on the matched device role.
func ApplyNeighborRules(hostname string, result *WalkResult, deviceRoles []config.DeviceRole) {
	role := rules.MatchDevice(hostname, deviceRoles)
	if role == nil || len(role.NeighborRules) == 0 {
		return
	}
	for i := range result.Interfaces {
		iface := &result.Interfaces[i]
		for _, nb := range iface.Neighbors {
			mr := matchNeighborRule(nb, role)
			if mr == nil {
				continue
			}
			if iface.NeighborRuleLabel == "" {
				iface.NeighborRuleLabel = mr.Label
			}
			if mr.ExpectAliasGlob != "" && !rules.GlobMatch(mr.ExpectAliasGlob, iface.Alias) {
				hint := fmt.Sprintf("LLDP/CDP suggests %q but alias %q does not match %q",
					mr.Label, iface.Alias, mr.ExpectAliasGlob)
				if iface.NeighborHint == "" {
					iface.NeighborHint = hint
				} else if !strings.Contains(iface.NeighborHint, hint) {
					iface.NeighborHint += "; " + hint
				}
			}
		}
	}
}

func matchNeighborRule(nb PortNeighbor, role *config.DeviceRole) *config.NeighborRule {
	for i := range role.NeighborRules {
		r := &role.NeighborRules[i]
		if neighborRuleMatches(nb, r) {
			return r
		}
	}
	return nil
}

func neighborRuleMatches(nb PortNeighbor, r *config.NeighborRule) bool {
	if r.MatchSysName != "" && !rules.GlobMatch(r.MatchSysName, nb.RemoteSysName) {
		return false
	}
	if r.MatchSysDesc != "" && !rules.GlobMatch(r.MatchSysDesc, nb.RemoteSysDesc) {
		return false
	}
	if r.MatchPlatform != "" && !rules.GlobMatch(r.MatchPlatform, nb.RemotePlatform) {
		return false
	}
	return r.MatchSysName != "" || r.MatchSysDesc != "" || r.MatchPlatform != ""
}
