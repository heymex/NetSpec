package decoder

import "strings"

// ClassifyPath rolls a Cisco/OpenConfig encoding_path into a coarse kind
// for stats. Optics/LLDP/hardware are matched before "interface" because
// those YANG paths often nest under an interface name.
func ClassifyPath(path string) string {
	p := strings.ToLower(strings.TrimSpace(path))
	if p == "" {
		return "empty"
	}
	switch {
	case containsAny(p, "optic", "transceiver", "dwdm", "coherent"):
		return "optics"
	case strings.Contains(p, "lldp"):
		return "lldp"
	case containsAny(p, "hardware", "platform", "environment", "inventory", "cpu-oper", "memory-oper", "process-cpu"):
		return "hardware"
	case containsAny(p, "bgp", "ospf", "isis", "mpls"):
		return "routing"
	case containsAny(p, "qos", "diffserv", "access-list", "acl"):
		return "qos"
	case strings.Contains(p, "interface") && containsAny(p, "counter", "statistic", "octet", "error"):
		return "interface_counters"
	case strings.Contains(p, "interface"):
		return "interface"
	default:
		return "other"
	}
}

func containsAny(p string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(p, n) {
			return true
		}
	}
	return false
}
