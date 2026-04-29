package collector

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/gosnmp/gosnmp"
	"github.com/netspec/netspec/internal/config"
	"github.com/rs/zerolog"
)

const (
	ifNameOID        = ".1.3.6.1.2.1.31.1.1.1.1"
	ifAdminStatusOID = ".1.3.6.1.2.1.2.2.1.7"
	ifOperStatusOID  = ".1.3.6.1.2.1.2.2.1.8"
)

// InterfaceSnapshot is normalized interface state from SNMP.
type InterfaceSnapshot struct {
	Interface   string
	OperStatus  string
	AdminStatus string
}

// SNMPValidator polls targeted interface OIDs and returns normalized state.
type SNMPValidator struct {
	logger       zerolog.Logger
	globalCfg    config.SNMPConfig
	community    string
	mu           sync.Mutex
	ifIndexByDev map[string]map[string]int
}

func NewSNMPValidator(globalCfg config.SNMPConfig, community string, logger zerolog.Logger) *SNMPValidator {
	return &SNMPValidator{
		logger:       logger,
		globalCfg:    globalCfg,
		community:    community,
		ifIndexByDev: make(map[string]map[string]int),
	}
}

func (v *SNMPValidator) PollDevice(deviceName string, deviceCfg config.DeviceConfig) ([]InterfaceSnapshot, error) {
	client := &gosnmp.GoSNMP{
		Target:    deviceCfg.Address,
		Port:      v.globalCfg.Port,
		Version:   gosnmp.Version2c,
		Community: v.community,
		Timeout:   v.globalCfg.Timeout,
		Retries:   v.globalCfg.Retries,
	}
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("snmp connect failed: %w", err)
	}
	defer client.Conn.Close()

	snapshots := make([]InterfaceSnapshot, 0, len(deviceCfg.Interfaces))
	for ifaceName, ifaceCfg := range deviceCfg.Interfaces {
		ifIndex, err := v.interfaceIndex(client, deviceName, ifaceName, ifaceCfg.SNMPIfIndex)
		if err != nil {
			v.logger.Warn().Err(err).Str("device", deviceName).Str("interface", ifaceName).Msg("Skipping SNMP validation for interface")
			continue
		}
		admin, oper, err := v.getStatuses(client, ifIndex)
		if err != nil {
			v.logger.Warn().Err(err).Str("device", deviceName).Str("interface", ifaceName).Int("ifindex", ifIndex).Msg("SNMP status read failed")
			continue
		}
		snapshots = append(snapshots, InterfaceSnapshot{
			Interface:   ifaceName,
			AdminStatus: admin,
			OperStatus:  oper,
		})
	}
	return snapshots, nil
}

// PollInterface performs a targeted SNMP GET for a single interface.
func (v *SNMPValidator) PollInterface(deviceName string, deviceCfg config.DeviceConfig, ifaceName string, ifaceCfg config.InterfaceConfig) (InterfaceSnapshot, error) {
	client := &gosnmp.GoSNMP{
		Target:    deviceCfg.Address,
		Port:      v.globalCfg.Port,
		Version:   gosnmp.Version2c,
		Community: v.community,
		Timeout:   v.globalCfg.Timeout,
		Retries:   v.globalCfg.Retries,
	}
	if err := client.Connect(); err != nil {
		return InterfaceSnapshot{}, fmt.Errorf("snmp connect failed: %w", err)
	}
	defer client.Conn.Close()

	ifIndex, err := v.interfaceIndex(client, deviceName, ifaceName, ifaceCfg.SNMPIfIndex)
	if err != nil {
		return InterfaceSnapshot{}, err
	}
	admin, oper, err := v.getStatuses(client, ifIndex)
	if err != nil {
		return InterfaceSnapshot{}, err
	}

	return InterfaceSnapshot{
		Interface:   ifaceName,
		OperStatus:  oper,
		AdminStatus: admin,
	}, nil
}

func (v *SNMPValidator) interfaceIndex(client *gosnmp.GoSNMP, deviceName, ifaceName string, configured int) (int, error) {
	if configured > 0 {
		return configured, nil
	}

	v.mu.Lock()
	if indexes, ok := v.ifIndexByDev[deviceName]; ok {
		if ifIndex, ok := indexes[ifaceName]; ok {
			v.mu.Unlock()
			return ifIndex, nil
		}
	}
	v.mu.Unlock()

	walk := make(map[string]int)
	if err := client.Walk(ifNameOID, func(pdu gosnmp.SnmpPDU) error {
		name := pduValueToString(pdu)
		idx, err := oidIndex(pdu.Name)
		if err == nil && name != "" {
			walk[name] = idx
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("ifName walk failed: %w", err)
	}

	v.mu.Lock()
	v.ifIndexByDev[deviceName] = walk
	v.mu.Unlock()

	if ifIndex, ok := walk[ifaceName]; ok {
		return ifIndex, nil
	}
	normalizedTarget := normalizeInterfaceName(ifaceName)
	if normalizedTarget != "" {
		for name, idx := range walk {
			if normalizeInterfaceName(name) == normalizedTarget {
				return idx, nil
			}
		}
	}
	return 0, fmt.Errorf("interface %s not found in ifName table", ifaceName)
}

func (v *SNMPValidator) getStatuses(client *gosnmp.GoSNMP, ifIndex int) (admin string, oper string, err error) {
	oids := []string{
		fmt.Sprintf("%s.%d", ifAdminStatusOID, ifIndex),
		fmt.Sprintf("%s.%d", ifOperStatusOID, ifIndex),
	}
	packet, err := client.Get(oids)
	if err != nil {
		return "", "", err
	}

	for _, pdu := range packet.Variables {
		switch {
		case strings.HasPrefix(pdu.Name, ifAdminStatusOID+"."):
			admin = mapIfStatus(intFromPDU(pdu))
		case strings.HasPrefix(pdu.Name, ifOperStatusOID+"."):
			oper = mapIfStatus(intFromPDU(pdu))
		}
	}
	if admin == "" || oper == "" {
		return "", "", fmt.Errorf("missing admin/oper status values")
	}
	return admin, oper, nil
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

func oidIndex(oid string) (int, error) {
	parts := strings.Split(oid, ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid oid %s", oid)
	}
	return strconv.Atoi(parts[len(parts)-1])
}

func pduValueToString(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case []byte:
		return string(v)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", pdu.Value)
	}
}

func intFromPDU(pdu gosnmp.SnmpPDU) int {
	switch v := pdu.Value.(type) {
	case int:
		return v
	case uint:
		return int(v)
	case uint32:
		return int(v)
	case int64:
		return int(v)
	case uint64:
		return int(v)
	default:
		return 0
	}
}

func normalizeInterfaceName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}

	replacements := map[string]string{
		"gigabitethernet":        "gi",
		"tengigabitethernet":     "te",
		"twentyfivegige":         "tw",
		"twentyfivegigabite":     "tw",
		"fortygigabitethernet":   "fo",
		"hundredgigabitethernet": "hu",
		"port-channel":           "po",
		"portchannel":            "po",
	}
	for old, newVal := range replacements {
		s = strings.ReplaceAll(s, old, newVal)
	}
	s = strings.ReplaceAll(s, " ", "")
	return s
}
