package discovery

import "testing"

func TestValidateCommitRequest_syncAllowsUnmonitoredEntries(t *testing.T) {
	t.Parallel()
	req := &CommitRequest{
		Action:                     "patch",
		DeviceKey:                  "sw-a",
		Address:                    "10.0.0.1",
		SyncDiscoveredInterfaces:   true,
		Interfaces: []CommitInterface{
			{Name: "Gi1/0/1", Monitor: true, DesiredState: "up", AdminState: "enabled", AlertSeverity: "warning"},
			{Name: "Gi1/0/2", Monitor: false, DesiredState: "up", AdminState: "enabled", AlertSeverity: "warning"},
		},
	}
	if err := validateCommitRequest(req); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateCommitRequest_syncRequiresPatch(t *testing.T) {
	t.Parallel()
	req := &CommitRequest{
		Action:                     "add",
		DeviceKey:                  "sw-a",
		Address:                    "10.0.0.1",
		SyncDiscoveredInterfaces:   true,
		Interfaces: []CommitInterface{
			{Name: "Gi1/0/1", Monitor: true, DesiredState: "up", AdminState: "enabled", AlertSeverity: "warning"},
		},
	}
	if err := validateCommitRequest(req); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateCommitRequest_monitorFalseWithoutSyncRejected(t *testing.T) {
	t.Parallel()
	req := &CommitRequest{
		Action:   "patch",
		DeviceKey: "sw-a",
		Address:   "10.0.0.1",
		Interfaces: []CommitInterface{
			{Name: "Gi1/0/1", Monitor: true, DesiredState: "up", AdminState: "enabled", AlertSeverity: "warning"},
			{Name: "Gi1/0/2", Monitor: false, DesiredState: "up", AdminState: "enabled", AlertSeverity: "warning"},
		},
	}
	if err := validateCommitRequest(req); err == nil {
		t.Fatal("expected error")
	}
}
