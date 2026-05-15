package discovery

import (
	"testing"

	"github.com/netspec/netspec/internal/config"
)

func TestApplyNeighborRules_aliasMismatchHint(t *testing.T) {
	t.Parallel()
	roles := []config.DeviceRole{{
		Prefix: "asw",
		NeighborRules: []config.NeighborRule{
			{Label: "IP Phone", MatchLLDPCapability: "telephone", ExpectAliasGlob: "phone*"},
		},
	}}
	result := &WalkResult{
		Interfaces: []Interface{{
			Name:  "Gi1/0/41",
			Alias: "user-port",
			Neighbors: []PortNeighbor{{
				Protocol:            "lldp",
				RemoteSysName:       "dvf9918",
				RemoteSysCapEnabled: lldpCapBridge | lldpCapPhone,
				RemoteLLDPCaps:      []string{"bridge", "telephone"},
			}},
		}},
	}
	ApplyNeighborRules("asw-hcd-01", result, roles)
	iface := result.Interfaces[0]
	if iface.NeighborRuleLabel != "IP Phone" {
		t.Fatalf("label=%q", iface.NeighborRuleLabel)
	}
	if iface.NeighborHint == "" {
		t.Fatal("expected neighbor_hint for alias mismatch")
	}
}
