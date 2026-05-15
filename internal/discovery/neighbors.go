package discovery

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// LLDP remote table (IEEE 802.1AB).
const (
	lldpRemLocalPortNumOID = "1.0.8802.1.1.2.1.4.1.1.2"
	lldpRemPortIdOID       = "1.0.8802.1.1.2.1.4.1.1.7"
	lldpRemPortDescOID     = "1.0.8802.1.1.2.1.4.1.1.8"
	lldpRemSysNameOID      = "1.0.8802.1.1.2.1.4.1.1.9"
	lldpRemSysDescOID      = "1.0.8802.1.1.2.1.4.1.1.10"
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

// WalkNeighbors collects LLDP and CDP neighbor rows keyed by local ifIndex.
// Errors are ignored per protocol so a partial result still returns.
func WalkNeighbors(address string, port uint16, community string, timeout time.Duration) (map[int][]PortNeighbor, error) {
	if err := validateAddress(address); err != nil {
		return nil, err
	}
	client := newSNMPClient(address, port, community, timeout, 1)
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("snmp connect failed: %w", err)
	}
	defer client.Conn.Close()

	out := map[int][]PortNeighbor{}
	merge := func(localIdx int, n PortNeighbor) {
		if localIdx <= 0 {
			return
		}
		out[localIdx] = append(out[localIdx], n)
	}

	_ = walkLLDP(client, merge)
	_ = walkCDP(client, merge)
	return out, nil
}

func walkLLDP(client *gosnmp.GoSNMP, merge func(int, PortNeighbor)) error {
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
		row.LocalIfIndex = pduInt(pdu)
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

	for key, row := range rows {
		local := row.LocalIfIndex
		if local <= 0 {
			local = key.localPortNum
		}
		merge(local, *row)
	}
	return nil
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
