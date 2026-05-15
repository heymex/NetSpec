package discovery

import (
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// LLDP local/rem tables (IEEE 802.1AB).
// lldpLocPortDesc maps lldpLocPortNum -> interface name string; NOT ifIndex.
const (
	lldpLocPortDescOID     = "1.0.8802.1.1.2.1.3.7.1.4"
	lldpRemLocalPortNumOID = "1.0.8802.1.1.2.1.4.1.1.2"
	lldpRemPortIdOID       = "1.0.8802.1.1.2.1.4.1.1.7"
	lldpRemPortDescOID     = "1.0.8802.1.1.2.1.4.1.1.8"
	lldpRemSysNameOID      = "1.0.8802.1.1.2.1.4.1.1.9"
	lldpRemSysDescOID       = "1.0.8802.1.1.2.1.4.1.1.10"
	lldpRemSysCapEnabledOID = "1.0.8802.1.1.2.1.4.1.1.11"
)

// Cisco CDP cache (best-effort).
const (
	cdpCacheDeviceIdOID   = "1.3.6.1.4.1.9.9.23.1.2.1.1.6"
	cdpCacheDevicePortOID = "1.3.6.1.4.1.9.9.23.1.2.1.1.7"
	cdpCachePlatformOID   = "1.3.6.1.4.1.9.9.23.1.2.1.1.8"
)

// lldpRemKey identifies one row in lldpRemTable (timeMark.localPortNum.remIndex).
type lldpRemKey struct {
	timeMark      int
	localPortNum  int
	remIndex      int
}

// walkNeighbors collects LLDP and CDP neighbor rows keyed by resolved local ifIndex.
// Only interfaces that exist in ifaces and report oper_status "up" receive rows.
func walkNeighbors(client *gosnmp.GoSNMP, ifaces []Interface) map[int][]PortNeighbor {
	acceptIdx := map[int]bool{}
	for _, iface := range ifaces {
		if strings.EqualFold(strings.TrimSpace(iface.OperStatus), "up") {
			acceptIdx[iface.Index] = true
		}
	}

	portMap, lldpLocTablePresent := buildLLDPLocalPortMap(client, ifaces)

	out := map[int][]PortNeighbor{}
	merge := func(localIdx int, n PortNeighbor) {
		if localIdx <= 0 || !acceptIdx[localIdx] {
			return
		}
		out[localIdx] = append(out[localIdx], n)
	}

	_ = walkLLDP(client, ifaces, portMap, lldpLocTablePresent, merge)
	_ = walkCDP(client, merge)
	return out
}

// buildLLDPLocalPortMap walks lldpLocPortDesc. The second return is true when the
// LLDP local port table exists — then lldpRemLocalPortNum must be resolved only
// via the map (or dropped), never by guessing it equals ifIndex.
func buildLLDPLocalPortMap(client *gosnmp.GoSNMP, ifaces []Interface) (map[int]int, bool) {
	descByPortNum := map[int]string{}
	_ = client.BulkWalk(lldpLocPortDescOID, func(pdu gosnmp.SnmpPDU) error {
		portNum, ok := oidLastIndex(pdu.Name)
		if !ok {
			return nil
		}
		descByPortNum[portNum] = strings.TrimSpace(pduString(pdu))
		return nil
	})
	if len(descByPortNum) == 0 {
		return nil, false
	}

	ifIndexByPortNum := make(map[int]int)
	knownIf := map[int]bool{}
	for _, iface := range ifaces {
		knownIf[iface.Index] = true
	}

	for portNum, desc := range descByPortNum {
		ifIdx := matchPortDescToIfIndex(desc, ifaces)
		if ifIdx > 0 {
			ifIndexByPortNum[portNum] = ifIdx
			continue
		}
		if desc == "" && knownIf[portNum] {
			ifIndexByPortNum[portNum] = portNum
		}
	}
	return ifIndexByPortNum, true
}

func walkLLDP(client *gosnmp.GoSNMP, ifaces []Interface, portNumToIfIndex map[int]int, lldpLocTablePresent bool, merge func(int, PortNeighbor)) error {
	rows := map[lldpRemKey]*PortNeighbor{}

	setField := func(oid string, pdu gosnmp.SnmpPDU, apply func(*PortNeighbor, string)) {
		key, ok := parseLLDPRemOID(oid)
		if !ok {
			return
		}
		row := rows[key]
		if row == nil {
			row = &PortNeighbor{Protocol: "lldp"}
			rows[key] = row
		}
		apply(row, pduString(pdu))
	}

	// Create rows from rem table keys. lldpRemLocalPortNum is the LLDP local port
	// number (lldpLocPortNum), not SNMP ifIndex on many platforms.
	_ = client.BulkWalk(lldpRemLocalPortNumOID, func(pdu gosnmp.SnmpPDU) error {
		key, ok := parseLLDPRemOID(pdu.Name)
		if !ok {
			return nil
		}
		row := rows[key]
		if row == nil {
			row = &PortNeighbor{Protocol: "lldp"}
			rows[key] = row
		}
		_ = pduInt(pdu) // value is lldpRemLocalPortNum; do not treat as ifIndex
		return nil
	})
	_ = client.BulkWalk(lldpRemSysNameOID, func(pdu gosnmp.SnmpPDU) error {
		setField(pdu.Name, pdu, func(r *PortNeighbor, v string) { r.RemoteSysName = v })
		return nil
	})
	_ = client.BulkWalk(lldpRemSysDescOID, func(pdu gosnmp.SnmpPDU) error {
		setField(pdu.Name, pdu, func(r *PortNeighbor, v string) { r.RemoteSysDesc = v })
		return nil
	})
	_ = client.BulkWalk(lldpRemPortIdOID, func(pdu gosnmp.SnmpPDU) error {
		setField(pdu.Name, pdu, func(r *PortNeighbor, v string) { r.RemotePortID = v })
		return nil
	})
	_ = client.BulkWalk(lldpRemPortDescOID, func(pdu gosnmp.SnmpPDU) error {
		setField(pdu.Name, pdu, func(r *PortNeighbor, v string) { r.RemotePortDesc = v })
		return nil
	})
	_ = client.BulkWalk(lldpRemSysCapEnabledOID, func(pdu gosnmp.SnmpPDU) error {
		key, ok := parseLLDPRemOID(pdu.Name)
		if !ok {
			return nil
		}
		row := rows[key]
		if row == nil {
			row = &PortNeighbor{Protocol: "lldp"}
			rows[key] = row
		}
		row.RemoteSysCapEnabled = pduCapabilityBits(pdu)
		row.RemoteLLDPCaps = formatLLDPCaps(row.RemoteSysCapEnabled)
		row.RemoteLLDPCapCodes = formatLLDPCapCodes(row.RemoteSysCapEnabled)
		return nil
	})

	for key, row := range rows {
		local := resolveLLDPLocalIfIndex(portNumToIfIndex, lldpLocTablePresent, key.localPortNum, ifaces)
		if local <= 0 {
			continue
		}
		row.LocalIfIndex = local
		merge(local, *row)
	}
	return nil
}

func resolveLLDPLocalIfIndex(portNumToIfIndex map[int]int, lldpLocTablePresent bool, portNum int, ifaces []Interface) int {
	if lldpLocTablePresent {
		if portNumToIfIndex != nil {
			if ifIdx, ok := portNumToIfIndex[portNum]; ok && ifIdx > 0 {
				return ifIdx
			}
		}
		return 0
	}
	for _, iface := range ifaces {
		if iface.Index == portNum {
			return iface.Index
		}
	}
	return 0
}

func oidLastIndex(oid string) (int, bool) {
	parts := strings.Split(strings.TrimPrefix(oid, "."), ".")
	if len(parts) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(parts[len(parts)-1])
	return n, err == nil
}

func normalizePortLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	for _, rep := range []struct{ from, to string }{
		{"gigabitethernet", "gi"},
		{"tengigabitethernet", "te"},
		{"twentyfivegigabitethernet", "twe"},
		{"twogigabitethernet", "tw"},
		{"fortygigabitethernet", "fo"},
		{"hundredgigabitethernet", "hu"},
		{"fivegigabitethernet", "fi"},
		{"fastethernet", "fa"},
		{"mgmtethernet", "mgmt"},
		{"port-channel", "po"},
		{"bundle-ether", "be"},
	} {
		s = strings.ReplaceAll(s, rep.from, rep.to)
	}
	return s
}

func matchPortDescToIfIndex(desc string, ifaces []Interface) int {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return 0
	}
	want := normalizePortLabel(desc)
	for _, iface := range ifaces {
		name := strings.TrimSpace(iface.Name)
		if name == "" {
			continue
		}
		if normalizePortLabel(name) == want {
			return iface.Index
		}
	}
	return 0
}

func walkCDP(client *gosnmp.GoSNMP, merge func(int, PortNeighbor)) error {
	type cdpKey struct {
		ifIndex      int
		deviceIndex  int
	}
	rows := map[cdpKey]*PortNeighbor{}

	rowForOID := func(oid string) (*PortNeighbor, bool) {
		ifIdx, devIdx, ok := parseCDPCacheOID(oid)
		if !ok {
			return nil, false
		}
		k := cdpKey{ifIndex: ifIdx, deviceIndex: devIdx}
		r := rows[k]
		if r == nil {
			r = &PortNeighbor{Protocol: "cdp", LocalIfIndex: ifIdx}
			rows[k] = r
		}
		return r, true
	}

	_ = client.BulkWalk(cdpCacheDeviceIdOID, func(pdu gosnmp.SnmpPDU) error {
		r, ok := rowForOID(pdu.Name)
		if !ok {
			return nil
		}
		r.RemoteSysName = pduString(pdu)
		return nil
	})
	_ = client.BulkWalk(cdpCacheDevicePortOID, func(pdu gosnmp.SnmpPDU) error {
		r, ok := rowForOID(pdu.Name)
		if !ok {
			return nil
		}
		r.RemotePortID = pduString(pdu)
		return nil
	})
	_ = client.BulkWalk(cdpCachePlatformOID, func(pdu gosnmp.SnmpPDU) error {
		r, ok := rowForOID(pdu.Name)
		if !ok {
			return nil
		}
		r.RemotePlatform = pduString(pdu)
		return nil
	})

	for _, row := range rows {
		merge(row.LocalIfIndex, *row)
	}
	return nil
}

// parseLLDPRemOID extracts timeMark, localPortNum, remIndex from lldpRem* column OIDs.
func parseLLDPRemOID(oid string) (lldpRemKey, bool) {
	parts := strings.Split(strings.TrimPrefix(oid, "."), ".")
	if len(parts) < 3 {
		return lldpRemKey{}, false
	}
	n := len(parts)
	tm, err1 := strconv.Atoi(parts[n-3])
	lp, err2 := strconv.Atoi(parts[n-2])
	ri, err3 := strconv.Atoi(parts[n-1])
	if err1 != nil || err2 != nil || err3 != nil {
		return lldpRemKey{}, false
	}
	return lldpRemKey{timeMark: tm, localPortNum: lp, remIndex: ri}, true
}

// parseCDPCacheOID extracts ifIndex and deviceIndex from cdpCache* OIDs.
func parseCDPCacheOID(oid string) (ifIndex, deviceIndex int, ok bool) {
	parts := strings.Split(strings.TrimPrefix(oid, "."), ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	n := len(parts)
	di, err1 := strconv.Atoi(parts[n-1])
	ii, err2 := strconv.Atoi(parts[n-2])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return ii, di, true
}

// AttachNeighbors merges neighbor rows onto interfaces by ifIndex and builds topology edges.
func AttachNeighbors(hostname string, ifaces []Interface, byIndex map[int][]PortNeighbor) ([]Interface, []TopologyEdge) {
	edges := make([]TopologyEdge, 0)
	local := strings.TrimSpace(hostname)
	if local == "" {
		local = "local"
	}
	for i := range ifaces {
		if !strings.EqualFold(strings.TrimSpace(ifaces[i].OperStatus), "up") {
			continue
		}
		nbrs := byIndex[ifaces[i].Index]
		if len(nbrs) == 0 {
			continue
		}
		ifaces[i].Neighbors = append(ifaces[i].Neighbors, nbrs...)
		for _, nb := range nbrs {
			remote := strings.TrimSpace(nb.RemoteSysName)
			if remote == "" {
				remote = strings.TrimSpace(nb.RemotePortID)
			}
			if remote == "" {
				continue
			}
			edges = append(edges, TopologyEdge{
				LocalDevice:  local,
				LocalPort:    ifaces[i].Name,
				RemoteDevice: remote,
				RemotePort:   nb.RemotePortID,
				Protocol:     nb.Protocol,
			})
		}
	}
	return ifaces, edges
}
