// Package ifname normalizes interface naming for matching telemetry/SNMP names to config keys.
package ifname

import (
	"strings"
)

// Canonical returns a compact lowercase form used to compare IOS-XE-style names.
func Canonical(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"gigabitethernet", "gi",
		"tengigabitethernet", "te",
		"twentyfivegigabitethernet", "tw",
		"twentyfivegige", "tw",
		"twentyfivegigabite", "tw",
		"hundredgigabitethernet", "hu",
		"hundredgige", "hu",
		"fortygigabitethernet", "fo",
		"fortygige", "fo",
		"port-channel", "po",
		"portchannel", "po",
		" ", "",
		"twe", "tw",
	)
	return replacer.Replace(s)
}

// Match reports whether two interface names refer to the same interface by exact or canonical match.
func Match(a, b string) bool {
	if a == b {
		return true
	}
	ca, cb := Canonical(a), Canonical(b)
	return ca != "" && ca == cb
}

// ResolveConfigKey returns the entry in ifaceKeys that matches query by exact or canonical
// equality. If no key matches, query is returned unchanged (callers may use it as a cache key).
func ResolveConfigKey(ifaceKeys []string, query string) string {
	for _, k := range ifaceKeys {
		if k == query {
			return k
		}
	}
	t := Canonical(query)
	for _, k := range ifaceKeys {
		if Canonical(k) == t {
			return k
		}
	}
	return query
}
