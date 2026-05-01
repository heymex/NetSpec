package config

import (
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
