package discovery

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// IEEE 802.1AB lldpSystemCapabilities bit positions (enabled/supported TLV).
const (
	lldpCapOther    uint16 = 1 << 0
	lldpCapRepeater uint16 = 1 << 1
	lldpCapBridge   uint16 = 1 << 2
	lldpCapWLANAP   uint16 = 1 << 3
	lldpCapRouter   uint16 = 1 << 4
	lldpCapPhone    uint16 = 1 << 5
	lldpCapDOCSIS   uint16 = 1 << 6
	lldpCapStation  uint16 = 1 << 7
)

var lldpCapNames = []struct {
	mask uint16
	name string
	code string // Cisco-style single-letter hint
}{
	{lldpCapOther, "other", "O"},
	{lldpCapRepeater, "repeater", "P"},
	{lldpCapBridge, "bridge", "B"},
	{lldpCapWLANAP, "wlan_ap", "W"},
	{lldpCapRouter, "router", "R"},
	{lldpCapPhone, "telephone", "T"},
	{lldpCapDOCSIS, "docsis", "C"},
	{lldpCapStation, "station", "S"},
}

func pduCapabilityBits(pdu gosnmp.SnmpPDU) uint16 {
	switch v := pdu.Value.(type) {
	case []byte:
		if len(v) == 0 {
			return 0
		}
		if len(v) == 1 {
			return uint16(v[0])
		}
		return uint16(v[0]) | uint16(v[1])<<8
	case string:
		if len(v) == 0 {
			return 0
		}
		if len(v) == 1 {
			return uint16(v[0])
		}
		return uint16(v[0]) | uint16(v[1])<<8
	case int:
		return uint16(v)
	case uint:
		return uint16(v)
	case int64:
		return uint16(v)
	case uint32:
		return uint16(v)
	case uint64:
		return uint16(v)
	default:
		return 0
	}
}

// formatLLDPCaps returns enabled capability names (e.g. bridge, telephone).
func formatLLDPCaps(bits uint16) []string {
	if bits == 0 {
		return nil
	}
	out := make([]string, 0, 4)
	for _, c := range lldpCapNames {
		if bits&c.mask != 0 {
			out = append(out, c.name)
		}
	}
	return out
}

// formatLLDPCapCodes returns Cisco-style capability letters (e.g. "B,T").
func formatLLDPCapCodes(bits uint16) string {
	if bits == 0 {
		return ""
	}
	var codes []string
	for _, c := range lldpCapNames {
		if bits&c.mask != 0 {
			codes = append(codes, c.code)
		}
	}
	return strings.Join(codes, ",")
}

// lldpCapabilityEnabled reports whether the named capability is set in the bitmask.
// Names: telephone, phone, t, bridge, b, router, r, wlan_ap, w, repeater, p, etc.
func lldpCapabilityEnabled(bits uint16, name string) bool {
	if bits == 0 || strings.TrimSpace(name) == "" {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(name))
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.ReplaceAll(key, " ", "_")
	for _, c := range lldpCapNames {
		if key == c.name || key == strings.ToLower(c.code) {
			return bits&c.mask != 0
		}
	}
	switch key {
	case "phone", "ip_phone", "ipphone":
		return bits&lldpCapPhone != 0
	case "ap", "wlan", "wifi", "wireless":
		return bits&lldpCapWLANAP != 0
	default:
		return false
	}
}

// lldpCapsSummary is a short display string for the wizard (sysname + cap codes).
func lldpCapsSummary(nb PortNeighbor) string {
	if nb.RemoteSysCapEnabled == 0 {
		return ""
	}
	codes := formatLLDPCapCodes(nb.RemoteSysCapEnabled)
	if codes == "" {
		return ""
	}
	return fmt.Sprintf("[%s]", codes)
}
