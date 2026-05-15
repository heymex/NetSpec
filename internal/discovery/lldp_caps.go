package discovery

import (
	"fmt"
	"math/bits"
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
		return lldpCapabilityU16FromOctets(v)
	case string:
		return lldpCapabilityU16FromOctets([]byte(v))
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

// lldpCapabilityU16FromOctets decodes lldpRemSysCapSupported/Enabled (SIZE(2)).
// Agents differ on octet order: IEEE uses only the primary 8 capability bits; the second
// octet should be zero. Prefer the ordering that leaves the bitmap in the low 8 bits.
func lldpCapabilityU16FromOctets(b []byte) uint16 {
	if len(b) == 0 {
		return 0
	}
	if len(b) == 1 {
		return uint16(b[0])
	}
	le := uint16(b[0]) | uint16(b[1])<<8
	be := uint16(b[0])<<8 | uint16(b[1])
	if be <= 0xFF && le > 0xFF {
		return be
	}
	if le <= 0xFF && be > 0xFF {
		return le
	}
	return le
}

// normalizeLLDPCapEnabledForSNMP aligns IOS-XE LLDP-MIB values with `show lldp neighbors` caps.
//
// Some IOS-XE builds report the enabled-capability octet with a reversed bit order relative
// to IEEE 802.1AB LSB-first naming: raw byte 0x30 then reads as Router+Telephone, while the
// CLI shows Bridge+WLAN (0x0C); those differ by bits.Reverse8. When the remote sysName looks
// like an access point (ap-, iap-), remap that ambiguous pattern to match the CLI/AP interpretation.
func normalizeLLDPCapEnabledForSNMP(raw uint16, remoteSysName string) uint16 {
	n := strings.ToLower(strings.TrimSpace(remoteSysName))
	apLike := strings.HasPrefix(n, "ap-") || strings.HasPrefix(n, "iap-")
	if !apLike {
		return raw
	}
	lo := byte(raw & 0xff)
	hi := byte(raw >> 8)
	// Single non-zero octet 0x30: R+T in LSB decoding == B+W when bits are reversed within octet.
	if lo == 0x30 && hi == 0 {
		return uint16(bits.Reverse8(lo))
	}
	if hi == 0x30 && lo == 0 {
		return uint16(bits.Reverse8(hi))
	}
	return raw
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
