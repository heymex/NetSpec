package evaluator

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/ifname"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/rs/zerolog"
)

// Evaluator compares telemetry data against desired state
type Evaluator struct {
	config     *config.Config
	logger     zerolog.Logger
	stateCache map[string]interfaceState
	mu         sync.RWMutex
}

// interfaceState represents the current state of an interface
type interfaceState struct {
	Device                  string
	Interface               string
	OperStatus              string
	AdminStatus             string
	Members                 []string
	UpdatedAt               time.Time
	LastSNMPValidation      time.Time
	LastTelemetryValidation time.Time
}

// InterfaceRuntimeState is the current runtime state for one interface.
type InterfaceRuntimeState struct {
	OperStatus              string
	AdminStatus             string
	UpdatedAt               time.Time
	LastSNMPValidation      time.Time
	LastTelemetryValidation time.Time
}

var (
	alertTypeInterfaceMismatch  = "interface_state_mismatch"
	alertTypeInterfaceAdminDown = "interface_admin_down"
	alertTypeChannelDown        = "port_channel_down"
	alertTypeMemberDown         = "port_channel_member_down"
)

var supportedOperStates = map[string]struct{}{
	"up":   {},
	"down": {},
}

var supportedAdminStates = map[string]struct{}{
	"enabled":  {},
	"disabled": {},
}

// StateChange represents a detected state change that should raise or clear an alert.
// Resolved=false (default) raises/updates a firing alert; Resolved=true clears it.
type StateChange struct {
	Device       string
	Interface    string
	AlertType    string
	Severity     string
	Message      string
	RelatedState map[string]string
	Resolved     bool
}

// NewEvaluator creates a new state evaluator
func NewEvaluator(cfg *config.Config, logger zerolog.Logger) *Evaluator {
	return &Evaluator{
		config:     cfg,
		logger:     logger,
		stateCache: make(map[string]interfaceState),
	}
}

// EvaluateInterfaceSnapshot evaluates interface state from non-gNMI sources
// (for example SNMP validation).
func (e *Evaluator) EvaluateInterfaceSnapshot(deviceName, ifaceName, operStatus, adminStatus string) []StateChange {
	return e.evaluateInterfaceSnapshotWithSource(deviceName, ifaceName, operStatus, adminStatus, "snmp")
}

// EvaluateInterfaceSnapshotWithSource evaluates interface state with explicit source.
// Supported sources:
//   - "snmp": periodic SNMP poll (updates LastSNMPValidation only)
//   - "telemetry": push telemetry path when SNMP confirmation failed for that event (telemetry-only snapshot)
//   - "push_snmp": push telemetry arrived and SNMP GET confirmed — updates BOTH LastSNMPValidation and LastTelemetryValidation
func (e *Evaluator) EvaluateInterfaceSnapshotWithSource(deviceName, ifaceName, operStatus, adminStatus, source string) []StateChange {
	return e.evaluateInterfaceSnapshotWithSource(deviceName, ifaceName, operStatus, adminStatus, source)
}

func (e *Evaluator) evaluateInterfaceSnapshotWithSource(deviceName, ifaceName, operStatus, adminStatus, source string) []StateChange {
	deviceCfg, ok := e.config.DesiredState.Devices[deviceName]
	if !ok {
		return nil
	}
	ifaceKeys := sortedIfaceKeys(deviceCfg.Interfaces)
	ifaceName = ifname.ResolveConfigKey(ifaceKeys, ifaceName)
	ifCfg, ok := deviceCfg.Interfaces[ifaceName]
	if !ok {
		return nil
	}
	if !ifCfg.Monitor {
		return nil
	}

	e.mu.Lock()
	cacheKey := fmt.Sprintf("%s:%s", deviceName, ifaceName)
	prevState := e.stateCache[cacheKey]
	newState := prevState
	newState.Device = deviceName
	newState.Interface = ifaceName
	newState.UpdatedAt = time.Now()
	if operStatus != "" {
		newState.OperStatus = normalizeState(operStatus)
	}
	if adminStatus != "" {
		newState.AdminStatus = normalizeAdminState(adminStatus)
	}
	switch normalizeState(source) {
	case "snmp":
		newState.LastSNMPValidation = time.Now()
	case "telemetry":
		newState.LastTelemetryValidation = time.Now()
	case "push_snmp":
		newState.LastSNMPValidation = time.Now()
		newState.LastTelemetryValidation = time.Now()
	}
	e.stateCache[cacheKey] = newState
	e.mu.Unlock()

	var changes []StateChange
	if adminChange := e.evaluateAdminChange(deviceName, ifaceName, ifCfg, prevState, newState); adminChange != nil {
		changes = append(changes, *adminChange)
	}
	if operChange := e.evaluateOperChange(deviceName, ifaceName, ifCfg, newState); operChange != nil {
		changes = append(changes, *operChange)
	}
	changes = append(changes, e.evaluatePortChannel(deviceName, ifaceName, deviceCfg)...)
	return changes
}

// GetInterfaceState returns the current runtime state for a device/interface.
func (e *Evaluator) GetInterfaceState(deviceName, ifaceName string) (InterfaceRuntimeState, bool) {
	cacheKey := fmt.Sprintf("%s:%s", deviceName, ifaceName)
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.stateCache[cacheKey]
	if !ok {
		return InterfaceRuntimeState{}, false
	}
	return InterfaceRuntimeState{
		OperStatus:              s.OperStatus,
		AdminStatus:             s.AdminStatus,
		UpdatedAt:               s.UpdatedAt,
		LastSNMPValidation:      s.LastSNMPValidation,
		LastTelemetryValidation: s.LastTelemetryValidation,
	}, true
}

// EvaluateNotification processes a gNMI notification and returns state changes
func (e *Evaluator) EvaluateNotification(deviceName string, notification *gnmi.Notification) []StateChange {
	var changes []StateChange

	// Extract interface information from notification
	for _, update := range notification.Update {
		path := update.Path

		// Parse interface path: /interfaces/interface[name="X"]/state/oper-status
		ifaceName, stateType, err := e.parseInterfacePath(path)
		if err != nil {
			// Try to extract interface name from the prefix path if available
			if notification.Prefix != nil {
				// Check if prefix contains interface name
				for _, elem := range notification.Prefix.Elem {
					if elem.Name == "interface" && len(elem.Key) > 0 {
						ifaceName = elem.Key["name"]
						// Re-parse with interface name from prefix
						if ifaceName != "" {
							_, stateType, err = e.parseInterfacePath(path)
						}
					}
				}
			}

			if err != nil || ifaceName == "" {
				e.logger.Debug().
					Err(err).
					Str("path", path.String()).
					Msg("Skipping non-interface path")
				continue
			}
		}

		// Get interface config for this device
		deviceCfg, ok := e.config.DesiredState.Devices[deviceName]
		if !ok {
			continue
		}

		// Check if interface is in desired state config
		ifaceKeys := sortedIfaceKeys(deviceCfg.Interfaces)
		ifaceName = ifname.ResolveConfigKey(ifaceKeys, ifaceName)
		ifCfg, hasInterfaceConfig := deviceCfg.Interfaces[ifaceName]
		if !hasInterfaceConfig {
			// Interface not in desired state config, skip
			continue
		}
		if !ifCfg.Monitor {
			continue
		}

		// Extract state value
		var stateValue string
		if update.Val != nil {
			if strVal := update.Val.GetStringVal(); strVal != "" {
				stateValue = strVal
			}
		}

		// Update state cache
		e.mu.Lock()
		cacheKey := fmt.Sprintf("%s:%s", deviceName, ifaceName)
		state := e.stateCache[cacheKey]
		state.Device = deviceName
		state.Interface = ifaceName
		state.UpdatedAt = time.Now()

		// Update appropriate state field
		switch stateType {
		case "oper-status":
			state.OperStatus = normalizeState(stateValue)
			state.LastTelemetryValidation = time.Now()
		case "admin-status":
			state.AdminStatus = normalizeAdminState(stateValue)
			state.LastTelemetryValidation = time.Now()
		}

		e.stateCache[cacheKey] = state
		prevState := state
		e.mu.Unlock()

		// Evaluate state against desired state
		if stateType == "admin-status" {
			if adminChange := e.evaluateAdminChange(deviceName, ifaceName, ifCfg, prevState, state); adminChange != nil {
				changes = append(changes, *adminChange)
			}
		}
		if stateType == "oper-status" {
			if operChange := e.evaluateOperChange(deviceName, ifaceName, ifCfg, state); operChange != nil {
				changes = append(changes, *operChange)
			}
		}

		// Evaluate port-channel membership if this is an oper-status change
		if stateType == "oper-status" {
			pcChanges := e.evaluatePortChannel(deviceName, ifaceName, deviceCfg)
			changes = append(changes, pcChanges...)
		}
	}

	return changes
}

// parseInterfacePath extracts interface name and state type from gNMI path
// Supports both OpenConfig format (/interfaces/interface[name="X"]/state/oper-status)
// and vendor-specific format (/interfaces/interface[name="X"]/oper-status)
func (e *Evaluator) parseInterfacePath(path *gnmi.Path) (ifaceName string, stateType string, err error) {
	if len(path.Elem) < 3 {
		return "", "", fmt.Errorf("path too short")
	}

	// Expected: /interfaces/interface[name="X"]/state/oper-status or /interfaces/interface[name="X"]/oper-status
	if path.Elem[0].Name != "interfaces" || path.Elem[1].Name != "interface" {
		return "", "", fmt.Errorf("not an interface path")
	}

	// Extract interface name from key
	ifaceName = path.Elem[1].Key["name"]
	if ifaceName == "" {
		// Try to extract from origin or other fields
		// For wildcard subscriptions, we need to get it from the update itself
		return "", "", fmt.Errorf("interface name not found in path")
	}

	// Check for OpenConfig format (with /state/) or vendor-specific format (without /state/)
	var stateTypeIndex int
	if len(path.Elem) >= 3 && path.Elem[2].Name == "state" {
		// OpenConfig format: /interfaces/interface[name="X"]/state/oper-status
		if len(path.Elem) < 4 {
			return "", "", fmt.Errorf("state type not found in path")
		}
		stateTypeIndex = 3
	} else {
		// Vendor-specific format: /interfaces/interface[name="X"]/oper-status
		if len(path.Elem) < 3 {
			return "", "", fmt.Errorf("state type not found in path")
		}
		stateTypeIndex = 2
	}

	stateType = path.Elem[stateTypeIndex].Name
	if stateType != "oper-status" && stateType != "admin-status" {
		return "", "", fmt.Errorf("unknown state type: %s", stateType)
	}

	return ifaceName, stateType, nil
}

// evaluateAdminChange evaluates admin status changes
func (e *Evaluator) evaluateAdminChange(deviceName, ifaceName string, ifCfg config.InterfaceConfig, prevState, ifaceState interfaceState) *StateChange {
	if ifCfg.AdminState == "" {
		return nil
	}
	desiredAdmin := normalizeState(ifCfg.AdminState)
	if _, ok := supportedAdminStates[desiredAdmin]; !ok {
		return nil
	}
	if prevState.AdminStatus == ifaceState.AdminStatus {
		return nil
	}
	if ifaceState.AdminStatus == "" || ifaceState.AdminStatus == desiredAdmin {
		return nil
	}
	severity := severityForAlert(ifCfg, "admin_down", "warning")
	return &StateChange{
		Device:    deviceName,
		Interface: ifaceName,
		AlertType: alertTypeInterfaceAdminDown,
		Severity:  severity,
		Message:   fmt.Sprintf("interface %s admin state %s", ifaceName, ifaceState.AdminStatus),
		RelatedState: map[string]string{
			"expected_admin": desiredAdmin,
			"actual_admin":   ifaceState.AdminStatus,
		},
	}
}

// evaluateOperChange evaluates operational status changes
func (e *Evaluator) evaluateOperChange(deviceName, ifaceName string, ifCfg config.InterfaceConfig, ifaceState interfaceState) *StateChange {
	if ifCfg.DesiredState == "" {
		return nil
	}
	desired := normalizeState(ifCfg.DesiredState)
	if _, ok := supportedOperStates[desired]; !ok {
		return nil
	}

	// Check admin status first - if admin is down, don't alert on oper down
	if ifCfg.AdminState != "" {
		desiredAdmin := normalizeState(ifCfg.AdminState)
		if _, ok := supportedAdminStates[desiredAdmin]; ok {
			if ifaceState.AdminStatus != "" && ifaceState.AdminStatus != desiredAdmin {
				return nil
			}
		}
	}

	if ifaceState.OperStatus == "" {
		return nil
	}

	if ifaceState.OperStatus != desired {
		severity := severityForAlert(ifCfg, "state_mismatch", "critical")
		return &StateChange{
			Device:    deviceName,
			Interface: ifaceName,
			AlertType: alertTypeInterfaceMismatch,
			Severity:  severity,
			Message:   fmt.Sprintf("interface %s expected %s got %s", ifaceName, desired, ifaceState.OperStatus),
			RelatedState: map[string]string{
				"expected_state": desired,
				"actual_state":   ifaceState.OperStatus,
			},
		}
	}

	return nil
}

// evaluatePortChannel evaluates port-channel member requirements
func (e *Evaluator) evaluatePortChannel(deviceName, ifaceName string, deviceCfg config.DeviceConfig) []StateChange {
	var changes []StateChange
	channelNames := e.channelNamesForMember(deviceCfg, ifaceName)
	if ifaceCfg, ok := deviceCfg.Interfaces[ifaceName]; ok && ifaceCfg.Members != nil && len(ifaceCfg.Members.Required) > 0 {
		channelNames = append(channelNames, ifaceName)
	}
	for _, channelName := range channelNames {
		channelCfg, ok := deviceCfg.Interfaces[channelName]
		if !ok {
			continue
		}
		channelAlerts := e.evaluateChannelMembers(deviceName, channelName, channelCfg, deviceCfg)
		changes = append(changes, channelAlerts...)
	}
	return changes
}

func sortedIfaceKeys(ifaces map[string]config.InterfaceConfig) []string {
	keys := make([]string, 0, len(ifaces))
	for k := range ifaces {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// classifyMemberOper returns "up", "down", or "unknown".
// Missing cache entries and non-up/down oper values (including "" and "unknown") are unknown.
func classifyMemberOper(present bool, operStatus string) string {
	if !present {
		return "unknown"
	}
	switch normalizeState(operStatus) {
	case "up":
		return "up"
	case "down":
		return "down"
	default:
		return "unknown"
	}
}

// evaluateChannelMembers evaluates port-channel member policies.
// Members are classified as up, down, or unknown. Unknown members are never counted
// as down; evaluation is deferred until every required member has a known oper state.
// When a previously failing member policy (or channel-down condition) is healthy,
// a Resolved StateChange is emitted so the alert engine can clear sticky alerts.
func (e *Evaluator) evaluateChannelMembers(deviceName, channelName string, ifaceCfg config.InterfaceConfig, deviceCfg config.DeviceConfig) []StateChange {
	if ifaceCfg.Members == nil || len(ifaceCfg.Members.Required) == 0 {
		return nil
	}

	ifaceKeys := sortedIfaceKeys(deviceCfg.Interfaces)
	channelKey := ifname.ResolveConfigKey(ifaceKeys, channelName)

	e.mu.RLock()
	chState, chOK := e.stateCache[fmt.Sprintf("%s:%s", deviceName, channelKey)]
	active := 0
	var downMembers []string
	var unknownMembers []string
	for _, member := range ifaceCfg.Members.Required {
		cacheKeyName := ifname.ResolveConfigKey(ifaceKeys, member)
		cacheKey := fmt.Sprintf("%s:%s", deviceName, cacheKeyName)
		memberState, ok := e.stateCache[cacheKey]
		switch classifyMemberOper(ok, memberState.OperStatus) {
		case "up":
			active++
		case "down":
			downMembers = append(downMembers, member)
		default:
			unknownMembers = append(unknownMembers, member)
		}
	}
	e.mu.RUnlock()

	totalMembers := len(ifaceCfg.Members.Required)
	chOper := ""
	if chOK {
		chOper = normalizeState(chState.OperStatus)
	}

	var changes []StateChange

	// Logical port-channel down should always be critical (use Po interface cache, not a member).
	switch chOper {
	case "down":
		return []StateChange{{
			Device:    deviceName,
			Interface: channelName,
			AlertType: alertTypeChannelDown,
			Severity:  "critical",
			Message:   fmt.Sprintf("port-channel %s is down", channelName),
			RelatedState: map[string]string{
				"actual_state": chOper,
			},
		}}
	case "up":
		changes = append(changes, StateChange{
			Device:    deviceName,
			Interface: channelName,
			AlertType: alertTypeChannelDown,
			Severity:  "critical",
			Message:   fmt.Sprintf("port-channel %s is up", channelName),
			RelatedState: map[string]string{
				"actual_state": chOper,
			},
			Resolved: true,
		})
	}

	// Delay member-policy evaluation until every required member has known oper state.
	if len(unknownMembers) > 0 {
		e.logger.Debug().
			Str("device", deviceName).
			Str("channel", channelName).
			Strs("unknown_members", unknownMembers).
			Msg("deferring port-channel member evaluation; member state not yet hydrated")
		return changes
	}

	downCount := len(downMembers)
	downPct := 0.0
	if totalMembers > 0 {
		downPct = (float64(downCount) / float64(totalMembers)) * 100.0
	}
	criticalThreshold := 50.0
	if ifaceCfg.MemberPolicy != nil && ifaceCfg.MemberPolicy.CriticalThresholdPct != nil {
		criticalThreshold = *ifaceCfg.MemberPolicy.CriticalThresholdPct
	}
	warningThreshold := 0.0
	if ifaceCfg.MemberPolicy != nil && ifaceCfg.MemberPolicy.WarningThresholdPct != nil {
		warningThreshold = *ifaceCfg.MemberPolicy.WarningThresholdPct
	}

	related := map[string]string{
		"active_members": fmt.Sprintf("%d", active),
		"total_members":  fmt.Sprintf("%d", totalMembers),
		"down_members":   strings.Join(downMembers, ","),
		"down_count":     fmt.Sprintf("%d", downCount),
		"down_pct":       fmt.Sprintf("%.1f", math.Round(downPct*10)/10),
	}

	// Healthy (or below warning threshold): clear any sticky member-down alert.
	if downCount == 0 || downPct <= warningThreshold {
		changes = append(changes, StateChange{
			Device:       deviceName,
			Interface:    channelName,
			AlertType:    alertTypeMemberDown,
			Severity:     "info",
			Message:      fmt.Sprintf("port-channel %s member policy healthy (%d/%d members up)", channelName, active, totalMembers),
			RelatedState: related,
			Resolved:     true,
		})
		return changes
	}

	severity := "warning"
	if downPct >= criticalThreshold {
		severity = "critical"
	}

	changes = append(changes, StateChange{
		Device:       deviceName,
		Interface:    channelName,
		AlertType:    alertTypeMemberDown,
		Severity:     severity,
		Message:      fmt.Sprintf("port-channel %s has %d/%d members down: %s", channelName, downCount, totalMembers, strings.Join(downMembers, ", ")),
		RelatedState: related,
	})
	return changes
}

func (e *Evaluator) channelNamesForMember(deviceCfg config.DeviceConfig, member string) []string {
	var channels []string
	for ifaceName, ifaceCfg := range deviceCfg.Interfaces {
		if ifaceCfg.Members == nil || len(ifaceCfg.Members.Required) == 0 {
			continue
		}
		for _, required := range ifaceCfg.Members.Required {
			if ifname.Match(required, member) {
				channels = append(channels, ifaceName)
				break
			}
		}
	}
	return channels
}

// normalizeState normalizes state values to lowercase
func normalizeState(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeAdminState(value string) string {
	switch normalizeState(value) {
	case "up", "enabled":
		return "enabled"
	case "down", "disabled", "administratively-down":
		return "disabled"
	default:
		return normalizeState(value)
	}
}

// severityForAlert gets severity from config or returns fallback
func severityForAlert(ifaceCfg config.InterfaceConfig, alertName, fallback string) string {
	if ifaceCfg.Alerts.StateMismatch != "" && alertName == "state_mismatch" {
		return ifaceCfg.Alerts.StateMismatch
	}
	if ifaceCfg.Alerts.MemberDown != "" && alertName == "member_down" {
		return ifaceCfg.Alerts.MemberDown
	}
	if ifaceCfg.Alerts.ChannelDown != "" && alertName == "channel_down" {
		return ifaceCfg.Alerts.ChannelDown
	}
	if ifaceCfg.Alerts.AdminDown != "" && alertName == "admin_down" {
		return ifaceCfg.Alerts.AdminDown
	}
	return fallback
}
