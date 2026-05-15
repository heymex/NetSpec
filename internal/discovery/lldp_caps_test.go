package discovery

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netspec/netspec/internal/config"
)

func TestLldpCapabilityEnabled_telephone(t *testing.T) {
	t.Parallel()
	// Cisco phone example: Bridge + Telephone (B,T) → 0x24
	bits := lldpCapBridge | lldpCapPhone
	for _, name := range []string{"telephone", "phone", "t"} {
		if !lldpCapabilityEnabled(bits, name) {
			t.Fatalf("%q not enabled in 0x%x", name, bits)
		}
	}
	if lldpCapabilityEnabled(bits, "router") {
		t.Fatal("router should not match")
	}
}

func TestFormatLLDPCapCodes(t *testing.T) {
	t.Parallel()
	got := formatLLDPCapCodes(lldpCapBridge | lldpCapPhone)
	if got != "B,T" {
		t.Fatalf("got %q want B,T", got)
	}
}

func TestPduCapabilityBits(t *testing.T) {
	t.Parallel()
	pdu := gosnmp.SnmpPDU{Value: []byte{0x24}}
	if got := pduCapabilityBits(pdu); got != 0x24 {
		t.Fatalf("got 0x%x", got)
	}
}

func TestNeighborRuleMatches_telephoneCapability(t *testing.T) {
	t.Parallel()
	r := &config.NeighborRule{Label: "IP Phone", MatchLLDPCapability: "telephone"}
	nb := PortNeighbor{
		Protocol:            "lldp",
		RemoteSysName:       "dvf9918",
		RemoteSysCapEnabled: lldpCapBridge | lldpCapPhone,
	}
	if !neighborRuleMatches(nb, r) {
		t.Fatal("expected telephone capability match")
	}
	nb.RemoteSysCapEnabled = lldpCapBridge
	if neighborRuleMatches(nb, r) {
		t.Fatal("bridge-only should not match telephone rule")
	}
}
