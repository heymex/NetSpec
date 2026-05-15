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

func TestLldpCapabilityU16FromOctets_prefersBitmaskInLowByte(t *testing.T) {
	t.Parallel()
	// Some agents send IEEE bitmap in second octet [0x00, 0x0C] (B+W) — LE would read 0x0C00.
	if got := lldpCapabilityU16FromOctets([]byte{0x00, 0x0C}); got != 0x0C {
		t.Fatalf("got 0x%x want 0x0c", got)
	}
	// Primary encoding: low octet first [0x0C, 0x00].
	if got := lldpCapabilityU16FromOctets([]byte{0x0C, 0x00}); got != 0x0C {
		t.Fatalf("got 0x%x want 0x0c", got)
	}
}

func TestNormalizeLLDPCapEnabledForSNMP_ap030ToBridgeWlan(t *testing.T) {
	t.Parallel()
	// Raw 0x30 is Router+Telephone in IEEE LSB decoding; IOS-XE CLI shows B,W for these APs.
	raw := uint16(0x30)
	got := normalizeLLDPCapEnabledForSNMP(raw, "ap-ha1-14")
	if got != lldpCapBridge|lldpCapWLANAP {
		t.Fatalf("got 0x%x want B+W (0x%x)", got, lldpCapBridge|lldpCapWLANAP)
	}
	if c := formatLLDPCapCodes(got); c != "B,W" {
		t.Fatalf("codes=%q want B,W", c)
	}
}

func TestNormalizeLLDPCapEnabledForSNMP_nonApKeepsRouterTel(t *testing.T) {
	t.Parallel()
	raw := uint16(0x30)
	got := normalizeLLDPCapEnabledForSNMP(raw, "some-router")
	if got != raw {
		t.Fatalf("got 0x%x want 0x30", got)
	}
}

func TestNormalizeLLDPCapEnabledForSNMP_polyPhoneUnchanged(t *testing.T) {
	t.Parallel()
	raw := uint16(0x24) // B,T
	got := normalizeLLDPCapEnabledForSNMP(raw, "dvf9918")
	if got != raw {
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
