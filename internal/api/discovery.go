package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/netspec/netspec/internal/config"
	"github.com/netspec/netspec/internal/discovery"
	"github.com/netspec/netspec/internal/webui"
)

type discoveryRequest struct {
	Address   string `json:"address"`
	Community string `json:"community"`
	Port      uint16 `json:"port"`
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

	data := struct {
		Version      string
		Commit       string
		BuildDate    string
		SNMPWarnings []SNMPUIWarning
	}{
		Version:      version,
		Commit:       commit,
		BuildDate:    buildDate,
		SNMPWarnings: snmpUIWarnings(wizCfg),
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
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
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
