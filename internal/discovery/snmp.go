package discovery

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	sysDescrOID    = "1.3.6.1.2.1.1.1.0"
	sysObjectIDOID = "1.3.6.1.2.1.1.2.0"
	sysNameOID     = "1.3.6.1.2.1.1.5.0"
	sysLocationOID = "1.3.6.1.2.1.1.6.0"

	ifIndexOID       = "1.3.6.1.2.1.2.2.1.1"
	ifDescrOID       = "1.3.6.1.2.1.2.2.1.2"
	ifTypeOID        = "1.3.6.1.2.1.2.2.1.3"
	ifAdminStatusOID = "1.3.6.1.2.1.2.2.1.7"
	ifOperStatusOID  = "1.3.6.1.2.1.2.2.1.8"
	ifNameOID        = "1.3.6.1.2.1.31.1.1.1.1"
	ifAliasOID       = "1.3.6.1.2.1.31.1.1.1.18"
	ifStackStatusOID = "1.3.6.1.2.1.31.1.2.1.3"
)

var ifTypeLabels = map[int]string{
	6:   "ethernetCsmacd",
	24:  "softwareLoopback",
	53:  "propVirtual",
	131: "tunnel",
	161: "ieee8023adLag",
}

func ProbeDevice(address string, port uint16, community string, timeout time.Duration) (*ProbeResult, error) {
	if err := validateAddress(address); err != nil {
		return nil, err
	}
	client := newSNMPClient(address, port, community, timeout, 2)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("snmp connect failed: %w", err)
	}
	defer client.Conn.Close()

	resp, err := client.Get([]string{sysDescrOID, sysObjectIDOID, sysNameOID, sysLocationOID})
	if err != nil {
		return nil, fmt.Errorf("snmp get failed: %w", err)
	}
	result := &ProbeResult{Address: address, VendorHint: "Unknown"}
	for _, pdu := range resp.Variables {
		switch pdu.Name {
		case "." + sysDescrOID, sysDescrOID:
			result.SysDescr = pduString(pdu)
		case "." + sysObjectIDOID, sysObjectIDOID:
			result.SysObjectID = pduString(pdu)
		case "." + sysNameOID, sysNameOID:
			result.SysName = pduString(pdu)
		case "." + sysLocationOID, sysLocationOID:
			result.SysLocation = pduString(pdu)
		}
	}
	result.VendorHint = vendorHint(result.SysObjectID, result.SysDescr)
	return result, nil
}

func WalkInterfaces(address string, port uint16, community string, timeout time.Duration) (*WalkResult, error) {
	if err := validateAddress(address); err != nil {
		return nil, err
	}
	client := newSNMPClient(address, port, community, timeout, 1)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("snmp connect failed: %w", err)
	}
	defer client.Conn.Close()

	indexes := map[int]*Interface{}
	ensure := func(idx int) *Interface {
		if it, ok := indexes[idx]; ok {
			return it
		}
		it := &Interface{Index: idx}
		indexes[idx] = it
		return it
	}

	walk := func(oid string, fn func(idx int, pdu gosnmp.SnmpPDU)) error {
		return client.BulkWalk(oid, func(pdu gosnmp.SnmpPDU) error {
			idx, err := oidIndex(pdu.Name)
			if err != nil {
				return nil
			}
			fn(idx, pdu)
			return nil
		})
	}

	if err := walk(ifIndexOID, func(idx int, _ gosnmp.SnmpPDU) { _ = ensure(idx) }); err != nil {
		return nil, fmt.Errorf("ifIndex walk failed: %w", err)
	}
	if err := walk(ifDescrOID, func(idx int, pdu gosnmp.SnmpPDU) { ensure(idx).Name = pduString(pdu) }); err != nil {
		return nil, fmt.Errorf("ifDescr walk failed: %w", err)
	}
	if err := walk(ifTypeOID, func(idx int, pdu gosnmp.SnmpPDU) { ensure(idx).Type = pduInt(pdu) }); err != nil {
		return nil, fmt.Errorf("ifType walk failed: %w", err)
	}
	if err := walk(ifAdminStatusOID, func(idx int, pdu gosnmp.SnmpPDU) { ensure(idx).AdminStatus = mapIfStatus(pduInt(pdu)) }); err != nil {
		return nil, fmt.Errorf("ifAdminStatus walk failed: %w", err)
	}
	if err := walk(ifOperStatusOID, func(idx int, pdu gosnmp.SnmpPDU) { ensure(idx).OperStatus = mapIfStatus(pduInt(pdu)) }); err != nil {
		return nil, fmt.Errorf("ifOperStatus walk failed: %w", err)
	}
	_ = walk(ifNameOID, func(idx int, pdu gosnmp.SnmpPDU) {
		name := pduString(pdu)
		if name != "" {
			ensure(idx).Name = name
		}
	})
	_ = walk(ifAliasOID, func(idx int, pdu gosnmp.SnmpPDU) { ensure(idx).Alias = pduString(pdu) })

	channelMembersByIndex := map[int][]int{}
	_ = client.BulkWalk(ifStackStatusOID, func(pdu gosnmp.SnmpPDU) error {
		if pduInt(pdu) != 1 {
			return nil
		}
		higher, lower, err := ifStackIndexes(pdu.Name)
		if err != nil {
			return nil
		}
		channelMembersByIndex[higher] = append(channelMembersByIndex[higher], lower)
		return nil
	})

	out := make([]Interface, 0, len(indexes))
	filtered := 0
	for _, it := range indexes {
		it.TypeLabel = typeLabel(it.Type)
		it.IsPortChannel = it.Type == 161 || strings.HasPrefix(strings.ToLower(it.Name), "port-channel") || strings.HasPrefix(strings.ToLower(it.Name), "po")
		for _, memberIdx := range channelMembersByIndex[it.Index] {
			if member, ok := indexes[memberIdx]; ok && member.Name != "" {
				it.ChannelMembers = append(it.ChannelMembers, member.Name)
			}
		}
		sort.Strings(it.ChannelMembers)
		if it.Type == 24 || it.Type == 53 {
			filtered++
			continue
		}
		out = append(out, *it)
	}

	sort.Slice(out, func(i, j int) bool {
		wi := typeWeight(out[i].Type)
		wj := typeWeight(out[j].Type)
		if wi != wj {
			return wi < wj
		}
		return out[i].Name < out[j].Name
	})

	byIndex, _ := WalkNeighbors(address, port, community, timeout)
	var edges []TopologyEdge
	if len(byIndex) > 0 {
		out, edges = AttachNeighbors("", out, byIndex)
	}

	return &WalkResult{
		Address:       address,
		Interfaces:    out,
		FilteredCount: filtered,
		TopologyEdges: edges,
	}, nil
}

func newSNMPClient(address string, port uint16, community string, timeout time.Duration, retries int) *gosnmp.GoSNMP {
	return &gosnmp.GoSNMP{
		Target:    address,
		Port:      port,
		Version:   gosnmp.Version2c,
		Community: community,
		Timeout:   timeout,
		Retries:   retries,
		MaxOids:   gosnmp.MaxOids,
	}
}

func validateAddress(address string) error {
	if strings.Contains(address, "://") {
		return fmt.Errorf("address must not include URL scheme")
	}
	if net.ParseIP(address) != nil {
		return nil
	}
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("address is required")
	}
	if strings.Contains(address, "/") {
		return fmt.Errorf("invalid hostname")
	}
	return nil
}

func oidIndex(oid string) (int, error) {
	parts := strings.Split(oid, ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid oid")
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func ifStackIndexes(oid string) (int, int, error) {
	trimmed := strings.TrimPrefix(oid, ".")
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid ifStack oid")
	}
	higher, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return 0, 0, err
	}
	lower, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0, 0, err
	}
	return higher, lower, nil
}

func pduString(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", pdu.Value)
	}
}

func pduInt(pdu gosnmp.SnmpPDU) int {
	switch v := pdu.Value.(type) {
	case int:
		return v
	case uint:
		return int(v)
	case int64:
		return int(v)
	case uint64:
		return int(v)
	case uint32:
		return int(v)
	default:
		return 0
	}
}

func mapIfStatus(v int) string {
	switch v {
	case 1:
		return "up"
	case 2:
		return "down"
	default:
		return "unknown"
	}
}

func vendorHint(objectID, descr string) string {
	switch {
	case strings.HasPrefix(objectID, "1.3.6.1.4.1.9"):
		return "Cisco"
	case strings.HasPrefix(objectID, "1.3.6.1.4.1.11"):
		return "HP/Aruba"
	case strings.HasPrefix(objectID, "1.3.6.1.4.1.25461"):
		return "Palo Alto"
	}
	s := strings.ToLower(descr)
	switch {
	case strings.Contains(s, "cisco"):
		return "Cisco"
	case strings.Contains(s, "aruba") || strings.Contains(s, "procurve") || strings.Contains(s, "hewlett"):
		return "HP/Aruba"
	case strings.Contains(s, "palo alto"):
		return "Palo Alto"
	}
	return "Unknown"
}

func typeLabel(ifType int) string {
	if label, ok := ifTypeLabels[ifType]; ok {
		return label
	}
	return "other"
}

func typeWeight(ifType int) int {
	switch ifType {
	case 6:
		return 0
	case 161:
		return 1
	default:
		return 2
	}
}
