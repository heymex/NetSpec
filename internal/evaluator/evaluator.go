package evaluator

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/config"
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
	Device      string
	Interface   string
	OperStatus  string
	AdminStatus string
	Members     []string
	UpdatedAt   time.Time
}

var (
	alertTypeInterfaceMismatch = "interface_state_mismatch"
	alertTypeInterfaceAdminDown = "interface_admin_down"
	alertTypeChannelDown       = "port_channel_down"
	alertTypeMemberDown        = "port_channel_member_down"
)

var supportedOperStates = map[string]struct{}{
	"up":   {},
	"down": {},
}

var supportedAdminStates = map[string]struct{}{
	"enabled":  {},
	"disabled": {},
}

// StateChange represents a detected state change
type StateChange struct {
	Device      string
	Interface   string
	AlertType   string
	Severity    string
	Message     string
	RelatedState map[string]string
}

// NewEvaluator creates a new state evaluator
func NewEvaluator(cfg *config.Config, logger zerolog.Logger) *Evaluator {
	return &Evaluator{
		config:     cfg,
		logger:     logger,
		stateCache: make(map[string]interfaceState),
	}
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
		_, hasInterfaceConfig := deviceCfg.Interfaces[ifaceName]
		if !hasInterfaceConfig {
			// Interface not in desired state config, skip
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
		case "admin-status":
			state.AdminStatus = normalizeState(stateValue)
		}

		e.stateCache[cacheKey] = state
		prevState := state
		e.mu.Unlock()

		// Evaluate state against desired state
		if ifCfg, ok := deviceCfg.Interfaces[ifaceName]; ok {
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
		}

		// Evaluate port-channel membership if this is an oper-status change
		if stateType == "oper-status" {
			pcChanges := e.evaluatePortChannel(deviceName, ifaceName, deviceCfg, state)
			changes = append(changes, pcChanges...)
		}
	}

	return changes
}

// parseInterfacePath extracts interface name and state type from gNMI path
func (e *Evaluator) parseInterfacePath(path *gnmi.Path) (ifaceName string, stateType string, err error) {
	if len(path.Elem) < 4 {
		return "", "", fmt.Errorf("path too short")
	}

	// Expected: /interfaces/interface[name="X"]/state/oper-status or admin-status
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

	// Check if we're in state subtree
	if len(path.Elem) < 3 || path.Elem[2].Name != "state" {
		return "", "", fmt.Errorf("not in state subtree")
	}

	// Get state type (should be 4th element: oper-status or admin-status)
	if len(path.Elem) < 4 {
		return "", "", fmt.Errorf("state type not found in path")
	}
	
	stateType = path.Elem[3].Name
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
func (e *Evaluator) evaluatePortChannel(deviceName, ifaceName string, deviceCfg config.DeviceConfig, ifaceState interfaceState) []StateChange {
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
		channelAlerts := e.evaluateChannelMembers(deviceName, channelName, channelCfg, ifaceState)
		changes = append(changes, channelAlerts...)
	}
	return changes
}

// evaluateChannelMembers evaluates port-channel member policies
func (e *Evaluator) evaluateChannelMembers(deviceName, channelName string, ifaceCfg config.InterfaceConfig, ifaceState interfaceState) []StateChange {
	if ifaceCfg.Members == nil || len(ifaceCfg.Members.Required) == 0 {
		return nil
	}
	memberPolicy := ifaceCfg.MemberPolicy
	mode := "all_active"
	minimum := len(ifaceCfg.Members.Required)
	if memberPolicy != nil {
		if memberPolicy.Mode != "" {
			mode = memberPolicy.Mode
		}
		if mode == "min_active" && memberPolicy.Minimum > 0 {
			minimum = memberPolicy.Minimum
		}
	}

	e.mu.RLock()
	active := 0
	var downMembers []string
	for _, member := range ifaceCfg.Members.Required {
		cacheKey := fmt.Sprintf("%s:%s", deviceName, member)
		memberState := e.stateCache[cacheKey]
		if normalizeState(memberState.OperStatus) == "up" {
			active++
		} else {
			downMembers = append(downMembers, member)
		}
	}
	e.mu.RUnlock()

	if mode == "all_active" && len(downMembers) > 0 {
		severity := severityForAlert(ifaceCfg, "member_down", "critical")
		return []StateChange{{
			Device:    deviceName,
			Interface: channelName,
			AlertType: alertTypeMemberDown,
			Severity:  severity,
			Message:   fmt.Sprintf("port-channel %s members down: %s", channelName, strings.Join(downMembers, ", ")),
			RelatedState: map[string]string{
				"down_members": strings.Join(downMembers, ","),
			},
		}}
	}

	if mode == "min_active" && active < minimum {
		severity := severityForAlert(ifaceCfg, "channel_down", "critical")
		return []StateChange{{
			Device:    deviceName,
			Interface: channelName,
			AlertType: alertTypeChannelDown,
			Severity:  severity,
			Message:   fmt.Sprintf("port-channel %s active members %d below minimum %d", channelName, active, minimum),
			RelatedState: map[string]string{
				"active_members": fmt.Sprintf("%d", active),
				"minimum":        fmt.Sprintf("%d", minimum),
			},
		}}
	}

	return nil
}

// channelNamesForMember finds port-channels that include a given member interface
func (e *Evaluator) channelNamesForMember(deviceCfg config.DeviceConfig, member string) []string {
	var channels []string
	for ifaceName, ifaceCfg := range deviceCfg.Interfaces {
		if ifaceCfg.Members == nil || len(ifaceCfg.Members.Required) == 0 {
			continue
		}
		for _, required := range ifaceCfg.Members.Required {
			if required == member {
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
