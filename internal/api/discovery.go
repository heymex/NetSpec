package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/discovery"
	"github.com/netspec/netspec/internal/rules"
	"github.com/netspec/netspec/internal/topology"
	"github.com/netspec/netspec/internal/webui"
)

type discoveryRequest struct {
	Address   string `json:"address"`
	Community string `json:"community"`
	Port      uint16 `json:"port"`
	SysName   string `json:"sys_name"` // optional; passed from probe result for rule matching
}

func (s *Server) handleWizardPage(w http.ResponseWriter, r *http.Request) {
	s.versionMu.RLock()
	version := s.version
	commit := s.commit
	buildDate := s.buildDate
	s.versionMu.RUnlock()

	s.reloadMu.RLock()
	wizCfg := s.config
	s.reloadMu.RUnlock()

	type roleInfo struct {
		Name   string `json:"name"`
		Prefix string `json:"prefix"`
	}
	var roleInfos []roleInfo
	if wizCfg != nil {
		for _, r := range wizCfg.Rules.DeviceRoles {
			roleInfos = append(roleInfos, roleInfo{Name: r.Name, Prefix: r.Prefix})
		}
	}

	data := struct {
		Version      string
		Commit       string
		BuildDate    string
		SNMPWarnings []SNMPUIWarning
		HasRules     bool
	}{
		Version:      version,
		Commit:       commit,
		BuildDate:    buildDate,
		SNMPWarnings: snmpUIWarnings(wizCfg),
		HasRules:     wizCfg != nil && len(wizCfg.Rules.DeviceRoles) > 0,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := webui.Templates.ExecuteTemplate(w, "wizard", data); err != nil {
		s.logger.Error().Err(err).Msg("Failed to render wizard template")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleDiscoveryProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req discoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDiscoveryError(w, http.StatusBadRequest, err)
		return
	}
	req = s.applyDiscoveryDefaults(req)
	result, err := discovery.ProbeDevice(req.Address, req.Port, req.Community, 3*time.Second)
	if err != nil {
		writeDiscoverySNMPError(w, err)
		return
	}

	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()
	if cfg != nil {
		for key, dev := range cfg.DesiredState.Devices {
			if strings.EqualFold(strings.TrimSpace(dev.Address), strings.TrimSpace(req.Address)) {
				result.AlreadyConfigured = true
				result.ExistingDeviceKey = key
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) handleDiscoveryWalk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req discoveryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDiscoveryError(w, http.StatusBadRequest, err)
		return
	}
	req = s.applyDiscoveryDefaults(req)
	result, err := discovery.WalkInterfaces(req.Address, req.Port, req.Community, 15*time.Second)
	if err != nil {
		writeDiscoverySNMPError(w, err)
		return
	}

	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()

	if cfg != nil {
		var match *config.DeviceConfig
		for _, dev := range cfg.DesiredState.Devices {
			if strings.EqualFold(strings.TrimSpace(dev.Address), strings.TrimSpace(req.Address)) {
				d := dev
				match = &d
				break
			}
		}
		if match != nil {
			for i := range result.Interfaces {
				it := &result.Interfaces[i]
				ic, ok := match.Interfaces[it.Name]
				if !ok {
					it.AlreadyConfigured = false
					continue
				}
				it.AlreadyConfigured = true
				sev := strings.TrimSpace(ic.Alerts.StateMismatch)
				if sev == "" {
					sev = "warning"
				}
				admin := strings.TrimSpace(ic.AdminState)
				if admin == "" {
					admin = "enabled"
				}
				var members []string
				isPC := ic.Members != nil && len(ic.Members.Required) > 0
				if ic.Members != nil {
					members = append(members, ic.Members.Required...)
				}
				it.ExistingConfig = &discovery.InterfaceConfigWish{
					Monitor:       ic.Monitor,
					Description:   ic.Description,
					DesiredState:  ic.DesiredState,
					AdminState:    admin,
					AlertSeverity: sev,
					IsPortChannel: isPC || it.IsPortChannel,
					Members:       members,
				}
			}
		}

		// Apply business rules when a hostname is provided.
		if req.SysName != "" && len(cfg.Rules.DeviceRoles) > 0 {
			applyRulesToWalk(req.SysName, result, cfg.Rules)
		}
	}

	// Rebuild topology with hostname label and Graphviz DOT for reporting.
	if req.SysName != "" && len(result.TopologyEdges) > 0 {
		host := req.SysName
		for i := range result.TopologyEdges {
			result.TopologyEdges[i].LocalDevice = host
		}
		result.TopologyDOT = topology.RenderDOT(host, result.TopologyEdges)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// applyRulesToWalk annotates each interface in result with the matching rule defaults.
func applyRulesToWalk(hostname string, result *discovery.WalkResult, rulesConfig config.RulesConfig) {
	role := rules.MatchDevice(hostname, rulesConfig.DeviceRoles)
	for i := range result.Interfaces {
		iface := &result.Interfaces[i]
		mr := rules.MatchPort(iface.Alias, role)
		if mr != nil {
			iface.RuleName = mr.RuleLabel
			iface.RuleMonitor = mr.Monitor
			iface.RuleDesiredState = mr.DesiredState
			iface.RuleSeverity = mr.Alerts.StateMismatch
		}
		// Parse trunk description regardless of role match.
		if tl := rules.ParseTrunkDescription(iface.Alias); tl != nil {
			iface.TrunkLink = tl
		}
	}
	discovery.ApplyNeighborRules(hostname, result, rulesConfig.DeviceRoles)
}

func (s *Server) handleDiscoveryCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req discovery.CommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDiscoveryError(w, http.StatusBadRequest, err)
		return
	}

	s.reloadMu.RLock()
	configPath := s.configPath
	s.reloadMu.RUnlock()
	desiredPath := discovery.DesiredStatePathFrom(configPath)

	result, err := discovery.PatchDesiredState(desiredPath, &req)
	if err != nil {
		status := discovery.StatusCodeFor(err)
		if status < 400 {
			status = http.StatusInternalServerError
		}
		writeDiscoveryError(w, status, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// handleRulesAPI returns the loaded rules configuration.
func (s *Server) handleRulesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.reloadMu.RLock()
	cfg := s.config
	s.reloadMu.RUnlock()

	if cfg == nil || len(cfg.Rules.DeviceRoles) == 0 {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_roles":[]}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cfg.Rules)
}

func (s *Server) applyDiscoveryDefaults(req discoveryRequest) discoveryRequest {
	req.Address = strings.TrimSpace(req.Address)
	req.Community = strings.TrimSpace(req.Community)
	if req.Community == "" {
		envName := "SNMP_COMMUNITY"
		s.reloadMu.RLock()
		if s.config != nil {
			if configured := strings.TrimSpace(s.config.DesiredState.Global.SNMP.CommunityEnv); configured != "" {
				envName = configured
			}
		}
		s.reloadMu.RUnlock()
		req.Community = strings.TrimSpace(os.Getenv(envName))
		if req.Community == "" && envName != "SNMP_COMMUNITY" {
			req.Community = strings.TrimSpace(os.Getenv("SNMP_COMMUNITY"))
		}
	}
	if req.Port == 0 {
		req.Port = 161
	}
	return req
}

func writeDiscoverySNMPError(w http.ResponseWriter, err error) {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"):
		writeDiscoveryError(w, http.StatusGatewayTimeout, err)
	default:
		writeDiscoveryError(w, http.StatusBadGateway, err)
	}
}

func writeDiscoveryError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
