package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateConfigPortChannelMemberValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		members   []string
		policy    *MemberPolicy
		shouldErr bool
	}{
		{
			name:    "reject self member reference",
			members: []string{"Port-channel10", "Gi1/0/1"},
			policy: &MemberPolicy{
				Mode:    "min_active",
				Minimum: 1,
			},
			shouldErr: true,
		},
		{
			name:    "reject duplicate members",
			members: []string{"Gi1/0/1", "Gi1/0/1"},
			policy: &MemberPolicy{
				Mode:    "min_active",
				Minimum: 1,
			},
			shouldErr: true,
		},
		{
			name:    "reject warning threshold above critical",
			members: []string{"Gi1/0/1", "Gi1/0/2"},
			policy: &MemberPolicy{
				Mode:                 "min_active",
				Minimum:              1,
				WarningThresholdPct:  ptrFloat(60),
				CriticalThresholdPct: ptrFloat(50),
			},
			shouldErr: true,
		},
		{
			name:    "accept valid policy with thresholds",
			members: []string{"Gi1/0/1", "Gi1/0/2", "Gi1/0/3"},
			policy: &MemberPolicy{
				Mode:                 "min_active",
				Minimum:              2,
				WarningThresholdPct:  ptrFloat(10),
				CriticalThresholdPct: ptrFloat(50),
			},
			shouldErr: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validConfigWithMembers(tc.members, tc.policy)
			err := ValidateConfig(cfg)
			if tc.shouldErr && err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("expected valid config, got error: %v", err)
			}
		})
	}
}

func validConfigWithMembers(members []string, policy *MemberPolicy) *Config {
	return &Config{
		DesiredState: DesiredStateConfig{
			Global: GlobalConfig{
				TelemetryMode: "snmp_validate_only",
				SNMP: SNMPConfig{
					Version: "2c",
				},
			},
			Devices: map[string]DeviceConfig{
				"sw1": {
					Address: "10.0.0.1",
					Interfaces: map[string]InterfaceConfig{
						"Port-channel10": {
							DesiredState: "up",
							Monitor:      true,
							Members: &MemberConfig{
								Required: members,
							},
							MemberPolicy: policy,
						},
						"Gi1/0/1": {DesiredState: "up", Monitor: true},
						"Gi1/0/2": {DesiredState: "up", Monitor: true},
						"Gi1/0/3": {DesiredState: "up", Monitor: true},
					},
				},
			},
		},
		Alerts: AlertsConfig{
			Channels:   map[string]ChannelConfig{},
			AlertRules: map[string]AlertRule{},
			AlertBehavior: AlertBehavior{
				DeduplicationWindow: time.Minute,
			},
		},
		Credentials: CredentialsConfig{
			Credentials: map[string]CredentialEntry{},
		},
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}

func TestLoadConfigDirReadsDataDevicesDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "cfg")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	desired := []byte(`global:
  telemetry_mode: snmp_validate_only
  snmp:
    version: "2c"
devices:
  mono-sw:
    address: 10.0.0.1
    interfaces:
      Gi1:
        desired_state: up
        monitor: true
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "desired-state.yaml"), desired, 0o644); err != nil {
		t.Fatal(err)
	}
	dataDevDir := SplitDeviceWriteDir(cfgDir)
	if err := os.MkdirAll(dataDevDir, 0o755); err != nil {
		t.Fatal(err)
	}
	split := []byte(`devices:
  split-sw:
    address: 10.0.0.2
    interfaces:
      Gi1:
        desired_state: up
        monitor: true
`)
	if err := os.WriteFile(filepath.Join(dataDevDir, "split-sw.yaml"), split, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatalf("LoadConfigDir: %v", err)
	}
	if _, ok := cfg.DesiredState.Devices["mono-sw"]; !ok {
		t.Fatal("missing mono-sw")
	}
	if _, ok := cfg.DesiredState.Devices["split-sw"]; !ok {
		t.Fatal("missing split-sw from data/devices")
	}
	if cfg.TotalDeviceCount() != 2 {
		t.Fatalf("device count: want 2, got %d", cfg.TotalDeviceCount())
	}
}

func TestLoadConfigDirAllowsZeroDevices(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "cfg")
	if err := os.MkdirAll(filepath.Join(cfgDir, "devices"), 0o755); err != nil {
		t.Fatal(err)
	}
	desired := []byte(`global:
  telemetry_mode: telemetry_ingest_push
  snmp:
    version: "2c"
devices: {}
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "desired-state.yaml"), desired, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatalf("LoadConfigDir with zero devices: %v", err)
	}
	if cfg.TotalDeviceCount() != 0 {
		t.Fatalf("device count: want 0, got %d", cfg.TotalDeviceCount())
	}
}

func TestValidateIngestDuplicateListenerPorts(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DesiredState: DesiredStateConfig{
			Global: GlobalConfig{
				TelemetryMode: "telemetry_ingest_push",
				SNMP: SNMPConfig{
					Version: "2c",
				},
				Ingest: IngestConfig{
					ListenAddress: "0.0.0.0",
					Port:          57500,
					AdditionalListeners: []IngestListenerEntry{
						{Port: 57500, Source: "duplicate"},
					},
				},
			},
			Devices: map[string]DeviceConfig{
				"sw1": {
					Address: "10.0.0.1",
					Interfaces: map[string]InterfaceConfig{
						"Gi1/0/1": {DesiredState: "up", Monitor: true},
					},
				},
			},
		},
		Alerts: AlertsConfig{
			Channels:   map[string]ChannelConfig{},
			AlertRules: map[string]AlertRule{},
			AlertBehavior: AlertBehavior{
				DeduplicationWindow: time.Minute,
			},
		},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected duplicate ingest port validation error")
	}
}

func TestValidateConfigOpenClawChannelRequiresURLEnv(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DesiredState: DesiredStateConfig{
			Global: GlobalConfig{
				TelemetryMode: "snmp_validate_only",
				SNMP:          SNMPConfig{Version: "2c"},
			},
			Devices: map[string]DeviceConfig{
				"sw1": {
					Address: "10.0.0.1",
					Interfaces: map[string]InterfaceConfig{
						"Gi1/0/1": {DesiredState: "up", Monitor: true},
					},
				},
			},
		},
		Alerts: AlertsConfig{
			Channels: map[string]ChannelConfig{
				"ops-openclaw": {Type: "openclaw"},
			},
			AlertRules: map[string]AlertRule{},
			AlertBehavior: AlertBehavior{
				DeduplicationWindow: time.Minute,
			},
		},
	}
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected url_env required for openclaw")
	}

	cfg.Alerts.Channels["ops-openclaw"] = ChannelConfig{
		Type:   "openclaw",
		URLEnv: "OPENCLAW_WEBHOOK_URL",
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestLoadConfigDirMonolithicDeviceOverlay(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "cfg")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(DataDir(cfgDir), 0o755); err != nil {
		t.Fatal(err)
	}
	desired := []byte(`global:
  telemetry_mode: snmp_validate_only
  snmp:
    version: "2c"
devices:
  keep-sw:
    address: 10.0.0.1
    interfaces:
      Gi1:
        desired_state: up
        monitor: true
  drop-sw:
    address: 10.0.0.9
    interfaces:
      Gi1:
        desired_state: up
        monitor: true
`)
	if err := os.WriteFile(filepath.Join(cfgDir, "desired-state.yaml"), desired, 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := []byte(`devices:
  keep-sw:
    address: 10.0.0.1
    interfaces:
      Gi1:
        desired_state: up
        monitor: true
`)
	if err := os.WriteFile(MonolithicDeviceOverlayPath(cfgDir), overlay, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigDir(cfgDir)
	if err != nil {
		t.Fatalf("LoadConfigDir: %v", err)
	}
	if _, ok := cfg.DesiredState.Devices["keep-sw"]; !ok {
		t.Fatal("missing keep-sw")
	}
	if _, ok := cfg.DesiredState.Devices["drop-sw"]; ok {
		t.Fatal("drop-sw should be overridden by overlay")
	}
	if cfg.TotalDeviceCount() != 1 {
		t.Fatalf("device count: want 1, got %d", cfg.TotalDeviceCount())
	}
}
