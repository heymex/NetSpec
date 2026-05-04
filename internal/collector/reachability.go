package collector

import (
	"sync"
	"time"
)

// SNMP reachability states for UI / honeycomb augmentation.
const (
	SNMPReachUnknown = "unknown"
	SNMPReachOK      = "ok"
	SNMPReachFail    = "fail"
)

// DeviceSNMPReachStatus is JSON-friendly SNMP contact metadata for a device.
type DeviceSNMPReachStatus struct {
	Reachability   string    `json:"snmp_reachability"`
	LastAttemptAt  time.Time `json:"snmp_last_attempt_at,omitempty"`
	LastOKAt       time.Time `json:"snmp_last_ok_at,omitempty"`
	LastError      string    `json:"snmp_last_error,omitempty"`
}

// ReachabilityTracker records the outcome of recent SNMP attempts per device.
type ReachabilityTracker struct {
	mu sync.RWMutex
	m  map[string]DeviceSNMPReachStatus
}

func NewReachabilityTracker() *ReachabilityTracker {
	return &ReachabilityTracker{m: make(map[string]DeviceSNMPReachStatus)}
}

// Prune removes devices not in allowed (typically after config reload).
func (t *ReachabilityTracker) Prune(allowed map[string]struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for name := range t.m {
		if _, ok := allowed[name]; !ok {
			delete(t.m, name)
		}
	}
}

// Status returns reachability for a device (unknown if never recorded).
func (t *ReachabilityTracker) Status(device string) DeviceSNMPReachStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if st, ok := t.m[device]; ok {
		return st
	}
	return DeviceSNMPReachStatus{Reachability: SNMPReachUnknown}
}

// RecordPoll records a full-device SNMP poll outcome (snmp_validate_only loop).
func (t *ReachabilityTracker) RecordPoll(device string, err error, snapshotCount, monitoredIfaceCount int) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	st := DeviceSNMPReachStatus{LastAttemptAt: now}
	switch {
	case err != nil:
		st.Reachability = SNMPReachFail
		st.LastError = err.Error()
	case monitoredIfaceCount == 0:
		st.Reachability = SNMPReachOK
		st.LastOKAt = now
	case snapshotCount == 0:
		st.Reachability = SNMPReachFail
		st.LastError = "no SNMP data for monitored interfaces"
	default:
		st.Reachability = SNMPReachOK
		st.LastOKAt = now
	}
	t.m[device] = st
}

// RecordPing records a lightweight SNMP reachability check (telemetry mode).
func (t *ReachabilityTracker) RecordPing(device string, err error) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	st := DeviceSNMPReachStatus{LastAttemptAt: now}
	if err != nil {
		st.Reachability = SNMPReachFail
		st.LastError = err.Error()
	} else {
		st.Reachability = SNMPReachOK
		st.LastOKAt = now
	}
	t.m[device] = st
}

// RecordInterfaceSNMPSuccess marks the device OK after a successful per-interface SNMP read.
func (t *ReachabilityTracker) RecordInterfaceSNMPSuccess(device string) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[device] = DeviceSNMPReachStatus{
		Reachability:  SNMPReachOK,
		LastAttemptAt: now,
		LastOKAt:      now,
	}
}
