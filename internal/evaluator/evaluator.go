package evaluator

import (
	"fmt"
	"strings"

	"github.com/netspec/netspec/internal/config"
	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/rs/zerolog"
)

// Evaluator compares telemetry data against desired state
type Evaluator struct {
	config     *config.Config
	logger     zerolog.Logger
	stateCache map[string]interfaceState
}

// interfaceState represents the current state of an interface
type interfaceState struct {
	Device      string
	Interface   string
	OperStatus  string
	AdminStatus string
	Members     []string
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
		deviceCfg, ok := e.config.Devices[deviceName]
		if !ok {
			continue
		}

		ifCfg, ok := deviceCfg.Interfaces[ifaceName]
		if !ok {
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
		cacheKey := fmt.Sprintf("%s:%s", deviceName, ifaceName)
		state := e.stateCache[cacheKey]
		state.Device = deviceName
		state.Interface = ifaceName

		// Update appropriate state field
		switch stateType {
		case "oper-status":
			state.OperStatus = stateValue
		case "admin-status":
			state.AdminStatus = stateValue
		}

		e.stateCache[cacheKey] = state

		// Evaluate state against desired state
		if change := e.evaluateInterfaceState(deviceName, ifaceName, ifCfg, state); change != nil {
			changes = append(changes, *change)
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

// evaluateInterfaceState compares current state against desired state
func (e *Evaluator) evaluateInterfaceState(deviceName, ifaceName string, ifCfg config.InterfaceConfig, state interfaceState) *StateChange {
	// Check admin status first
	if ifCfg.AdminState != "" {
		expectedAdmin := "UP"
		if ifCfg.AdminState == "disabled" {
			expectedAdmin = "DOWN"
		}

		if state.AdminStatus != "" && state.AdminStatus != expectedAdmin {
			severity := "warning"
			if ifCfg.Alerts.AdminDown != "" {
				severity = ifCfg.Alerts.AdminDown
			}

			return &StateChange{
				Device:    deviceName,
				Interface: ifaceName,
				AlertType: "admin_down",
				Severity:  severity,
				Message:   fmt.Sprintf("Interface %s on %s is administratively %s (expected %s)", 
					ifaceName, deviceName, state.AdminStatus, expectedAdmin),
			}
		}
	}

	// Check operational status
	expectedOper := strings.ToUpper(ifCfg.DesiredState)
	actualOper := strings.ToUpper(state.OperStatus)

	if actualOper != expectedOper {
		// If admin is down, oper down is expected - don't alert
		if state.AdminStatus == "DOWN" {
			return nil
		}

		severity := "warning"
		if ifCfg.Alerts.StateMismatch != "" {
			severity = ifCfg.Alerts.StateMismatch
		}

		return &StateChange{
			Device:    deviceName,
			Interface: ifaceName,
			AlertType: "state_mismatch",
			Severity:  severity,
			Message:   fmt.Sprintf("Interface %s on %s is %s (expected %s)", 
				ifaceName, deviceName, actualOper, expectedOper),
		}
	}

	return nil
}
