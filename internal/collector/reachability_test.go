package collector

import "testing"

func TestRecordPollNoMonitoredInterfacesIsUnknown(t *testing.T) {
	t.Parallel()
	tr := NewReachabilityTracker()
	tr.RecordPoll("sw1", nil, 0, 0)
	st := tr.Status("sw1")
	if st.Reachability != SNMPReachUnknown {
		t.Fatalf("reachability: want %q, got %q", SNMPReachUnknown, st.Reachability)
	}
	if st.LastError == "" {
		t.Fatal("expected explanatory last_error for unknown reachability")
	}
}

