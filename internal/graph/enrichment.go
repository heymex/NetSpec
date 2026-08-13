package graph

import (
	"sort"
	"strings"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/ifname"
	"github.com/netspec/netspec/internal/rules"
)

// InterfaceIdentity is the query-time join of a desired-state interface to
// NetSpec device/port rules. Labels like role/alias/monitored are never written
// to VictoriaMetrics; this index is how Graph recovers them.
type InterfaceIdentity struct {
	Device           string `json:"device"`
	Interface        string `json:"interface"` // config map key (usually SNMP ifName)
	Canonical        string `json:"canonical"`
	Alias            string `json:"alias"`
	DeviceRole       string `json:"device_role"`
	DeviceRolePrefix string `json:"device_role_prefix"`
	PortRole         string `json:"port_role"` // port_rules label; empty if no match
	Monitored        bool   `json:"monitored"`
	DesiredState     string `json:"desired_state"`
	GraphPath        string `json:"graph_path"`
}

// RoleInfo summarizes a device_roles entry for filter UIs.
type RoleInfo struct {
	Name           string   `json:"name"`
	Prefix         string   `json:"prefix"`
	PortRuleLabels []string `json:"port_rule_labels"`
}

// Filter selects identities from an Index. Empty fields are ignored.
type Filter struct {
	Device         string // exact hostname (case-insensitive)
	DevicePrefix   string // hostname prefix (case-insensitive), same idea as /noc
	DeviceRole     string // MatchDevice role name
	PortRole       string // MatchPort rule label (e.g. "Wireless APs")
	Monitored      *bool
	DesiredState   string
	Query          string // substring on device, interface, alias, or port role
}

// Index is an in-memory (device, interface) → identity map built from config + rules.
type Index struct {
	byKey   map[string]InterfaceIdentity // device\x00canonical → identity
	devices map[string][]string          // device → config interface keys
	list    []InterfaceIdentity
	roles   []RoleInfo
}

// BuildIndex walks desired-state devices/interfaces and re-applies rules.MatchDevice /
// MatchPort so Graph agrees with discovery/wizard classification by construction.
func BuildIndex(cfg *config.Config) *Index {
	idx := &Index{
		byKey:   make(map[string]InterfaceIdentity),
		devices: make(map[string][]string),
	}
	if cfg == nil {
		return idx
	}

	var roles []config.DeviceRole
	if len(cfg.Rules.DeviceRoles) > 0 {
		roles = cfg.Rules.DeviceRoles
		idx.roles = roleInfos(roles)
	}

	if cfg.DesiredState.Devices == nil {
		return idx
	}

	for deviceName, dev := range cfg.DesiredState.Devices {
		deviceRole := rules.MatchDevice(deviceName, roles)
		roleName, rolePrefix := "", ""
		if deviceRole != nil {
			roleName = deviceRole.Name
			rolePrefix = deviceRole.Prefix
		}

		keys := make([]string, 0, len(dev.Interfaces))
		for ifaceKey, ifaceCfg := range dev.Interfaces {
			keys = append(keys, ifaceKey)
			portRole := matchPortRole(deviceRole, ifaceCfg.Description, ifaceKey)
			id := InterfaceIdentity{
				Device:           deviceName,
				Interface:        ifaceKey,
				Canonical:        ifname.Canonical(ifaceKey),
				Alias:            ifaceCfg.Description,
				DeviceRole:       roleName,
				DeviceRolePrefix: rolePrefix,
				PortRole:         portRole,
				Monitored:        ifaceCfg.Monitor,
				DesiredState:     ifaceCfg.DesiredState,
				GraphPath:        interfacePagePath(deviceName, ifaceKey),
			}
			idx.byKey[indexKey(deviceName, id.Canonical)] = id
			idx.list = append(idx.list, id)
		}
		sort.Strings(keys)
		idx.devices[deviceName] = keys
	}

	sort.Slice(idx.list, func(i, j int) bool {
		if idx.list[i].Device != idx.list[j].Device {
			return idx.list[i].Device < idx.list[j].Device
		}
		return idx.list[i].Interface < idx.list[j].Interface
	})
	return idx
}

func indexKey(device, canonical string) string {
	return strings.ToLower(device) + "\x00" + canonical
}

// matchPortRole applies rules.MatchPort like discovery (alias first). Many live
// aliases do not follow the rule globs (e.g. Po aliases "peer:po31" vs match
// "po*"), so fall back to the config interface name and its canonical form.
func matchPortRole(role *config.DeviceRole, alias, ifaceKey string) string {
	for _, candidate := range []string{alias, ifaceKey, ifname.Canonical(ifaceKey)} {
		if candidate == "" {
			continue
		}
		if m := rules.MatchPort(candidate, role); m != nil {
			return m.RuleLabel
		}
	}
	return ""
}

func roleInfos(roles []config.DeviceRole) []RoleInfo {
	out := make([]RoleInfo, 0, len(roles))
	for _, r := range roles {
		labels := make([]string, 0, len(r.PortRules))
		seen := make(map[string]struct{}, len(r.PortRules))
		for _, pr := range r.PortRules {
			if pr.Label == "" {
				continue
			}
			if _, ok := seen[pr.Label]; ok {
				continue
			}
			seen[pr.Label] = struct{}{}
			labels = append(labels, pr.Label)
		}
		out = append(out, RoleInfo{
			Name:           r.Name,
			Prefix:         r.Prefix,
			PortRuleLabels: labels,
		})
	}
	return out
}

// Roles returns device_roles summaries for filter UIs.
func (idx *Index) Roles() []RoleInfo {
	if idx == nil {
		return nil
	}
	return idx.roles
}

// Len returns how many interfaces are indexed.
func (idx *Index) Len() int {
	if idx == nil {
		return 0
	}
	return len(idx.list)
}

// DeviceCount returns how many devices appear in the index.
func (idx *Index) DeviceCount() int {
	if idx == nil {
		return 0
	}
	return len(idx.devices)
}

// Lookup resolves a device + interface name (config key or telemetry-native)
// to an identity. ok is false when the device/interface is not in desired-state.
func (idx *Index) Lookup(device, iface string) (InterfaceIdentity, bool) {
	if idx == nil || device == "" || iface == "" {
		return InterfaceIdentity{}, false
	}
	keys := idx.devices[device]
	if keys == nil {
		// Case-insensitive device fallback.
		lower := strings.ToLower(device)
		for name, k := range idx.devices {
			if strings.ToLower(name) == lower {
				device = name
				keys = k
				break
			}
		}
	}
	if keys == nil {
		return InterfaceIdentity{}, false
	}
	cfgKey := ifname.ResolveConfigKey(keys, iface)
	id, ok := idx.byKey[indexKey(device, ifname.Canonical(cfgKey))]
	return id, ok
}

// Filter returns identities matching f, in stable device/interface order.
func (idx *Index) Filter(f Filter) []InterfaceIdentity {
	if idx == nil {
		return nil
	}
	out := make([]InterfaceIdentity, 0, len(idx.list))
	devExact := strings.ToLower(strings.TrimSpace(f.Device))
	devPrefix := strings.ToLower(strings.TrimSpace(f.DevicePrefix))
	devRole := strings.TrimSpace(f.DeviceRole)
	portRole := strings.TrimSpace(f.PortRole)
	desired := strings.TrimSpace(f.DesiredState)
	q := strings.ToLower(strings.TrimSpace(f.Query))

	for _, id := range idx.list {
		if devExact != "" && strings.ToLower(id.Device) != devExact {
			continue
		}
		if devPrefix != "" && !strings.HasPrefix(strings.ToLower(id.Device), devPrefix) {
			continue
		}
		if devRole != "" && !strings.EqualFold(id.DeviceRole, devRole) {
			continue
		}
		if portRole != "" && !strings.EqualFold(id.PortRole, portRole) {
			continue
		}
		if f.Monitored != nil && id.Monitored != *f.Monitored {
			continue
		}
		if desired != "" && !strings.EqualFold(id.DesiredState, desired) {
			continue
		}
		if q != "" {
			hay := strings.ToLower(id.Device + " " + id.Interface + " " + id.Alias + " " + id.PortRole + " " + id.DeviceRole)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		out = append(out, id)
	}
	return out
}

// PortRoleLabels returns the sorted unique port_rules labels across all roles.
func (idx *Index) PortRoleLabels() []string {
	if idx == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, r := range idx.roles {
		for _, l := range r.PortRuleLabels {
			seen[l] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// PortRoleCount is a port-rule label with how many indexed interfaces match it.
type PortRoleCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// PortRoleCounts returns labels that appear on at least one indexed interface,
// sorted by count descending then label. Prefer this for filter UIs so empty
// roles (e.g. APs excluded from desired-state) are not offered as primary choices.
func (idx *Index) PortRoleCounts() []PortRoleCount {
	if idx == nil {
		return nil
	}
	counts := make(map[string]int)
	for _, id := range idx.list {
		if id.PortRole == "" {
			continue
		}
		counts[id.PortRole]++
	}
	out := make([]PortRoleCount, 0, len(counts))
	for label, n := range counts {
		out = append(out, PortRoleCount{Label: label, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}
