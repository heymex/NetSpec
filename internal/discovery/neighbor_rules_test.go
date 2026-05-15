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

func TestApplyNeighborRules_apHostnameBeforeBogusTelephoneBit(t *testing.T) {
	t.Parallel()
	roles := []config.DeviceRole{{
		Prefix: "asw",
		NeighborRules: []config.NeighborRule{
			{Label: "Wireless AP (hostname)", MatchSysName: "ap*", ExpectAliasGlob: "ap*"},
			{Label: "Wireless AP (iap)", MatchSysName: "iap*", ExpectAliasGlob: "iap*"},
			{Label: "IP Phone", MatchLLDPCapability: "telephone", ExpectAliasGlob: "phone*"},
		},
	}}
	// Campus AP often advertises R,T with no W bit — T must not win over ap- sysName.
	rtCaps := lldpCapRouter | lldpCapPhone
	result := &WalkResult{
		Interfaces: []Interface{{
			Name:  "Gi1/0/45",
			Alias: "ap-hb1-14",
			Neighbors: []PortNeighbor{{
				Protocol:            "lldp",
				RemoteSysName:       "ap-hb1-14",
				RemoteSysCapEnabled: rtCaps,
			}},
		}, {
			Name:  "Gi3/0/47",
			Alias: "iap-h1142-hall",
			Neighbors: []PortNeighbor{{
				Protocol:            "lldp",
				RemoteSysName:       "iap-h1142-hall",
				RemoteSysCapEnabled: rtCaps,
			}},
		}},
	}
	ApplyNeighborRules("asw-test-01", result, roles)
	if got := result.Interfaces[0].NeighborRuleLabel; got != "Wireless AP (hostname)" {
		t.Fatalf("ap-hb1-14 label=%q", got)
	}
	if got := result.Interfaces[1].NeighborRuleLabel; got != "Wireless AP (iap)" {
		t.Fatalf("iap-h1142-hall label=%q", got)
	}
}
