package transformer

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/mdt/decoder"
)

const DefaultResendInterval = 60 * time.Second
const DefaultSource = "netspec-mdt"

// Config controls allowlist, resend, and event source tagging.
type Config struct {
	ResendInterval time.Duration
	AllowedDevices map[string]struct{}
	Source         string
	Now            func() time.Time
}

// Transformer maps kvGPB records to PushTelemetryEvent with translator-parity
// status normalize, allowlist, and unchanged-state resend.
type Transformer struct {
	cfg Config
	mu  sync.Mutex
	// lastState/lastSent keyed by device\x1finterface
	lastState map[string]string
	lastSent  map[string]time.Time

	SkippedUnknownPath atomic.Uint64
	SkippedNoStatus    atomic.Uint64
	SkippedAllowlist   atomic.Uint64
	SkippedDedup       atomic.Uint64
	Emitted            atomic.Uint64
}

func New(cfg Config) *Transformer {
	if cfg.ResendInterval <= 0 {
		cfg.ResendInterval = DefaultResendInterval
	}
	if cfg.Source == "" {
		cfg.Source = DefaultSource
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Transformer{
		cfg:       cfg,
		lastState: map[string]string{},
		lastSent:  map[string]time.Time{},
	}
}

func (t *Transformer) Events(recs []*decoder.Record) []collector.PushTelemetryEvent {
	if len(recs) == 0 {
		return nil
	}
	out := make([]collector.PushTelemetryEvent, 0, len(recs))
	now := t.cfg.Now()
	for _, rec := range recs {
		if ev, ok := t.event(rec, now); ok {
			out = append(out, ev)
		}
	}
	return out
}

func (t *Transformer) event(rec *decoder.Record, now time.Time) (collector.PushTelemetryEvent, bool) {
	if rec == nil {
		return collector.PushTelemetryEvent{}, false
	}
	if !isInterfacePath(rec.EncodingPath) {
		t.SkippedUnknownPath.Add(1)
		return collector.PushTelemetryEvent{}, false
	}
	device := strings.TrimSpace(rec.NodeID)
	iface := strings.TrimSpace(rec.Interface)
	if device == "" || iface == "" {
		t.SkippedNoStatus.Add(1)
		return collector.PushTelemetryEvent{}, false
	}
	if len(t.cfg.AllowedDevices) > 0 {
		if _, ok := t.cfg.AllowedDevices[device]; !ok {
			t.SkippedAllowlist.Add(1)
			return collector.PushTelemetryEvent{}, false
		}
	}
	oper := NormOper(lookupStatus(rec.Fields, "oper_status"))
	admin := NormAdmin(lookupStatus(rec.Fields, "admin_status"))
	if oper == "" && admin == "" {
		t.SkippedNoStatus.Add(1)
		return collector.PushTelemetryEvent{}, false
	}

	key := device + "\x1f" + iface
	state := oper + "\x1f" + admin
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastState[key] == state {
		if now.Sub(t.lastSent[key]) < t.cfg.ResendInterval {
			t.SkippedDedup.Add(1)
			return collector.PushTelemetryEvent{}, false
		}
	}
	t.lastState[key] = state
	t.lastSent[key] = now
	t.Emitted.Add(1)
	return collector.PushTelemetryEvent{
		Device:      device,
		Interface:   iface,
		OperStatus:  oper,
		AdminStatus: admin,
		Source:      t.cfg.Source,
		RemoteAddr:  rec.Peer,
	}, true
}

func isInterfacePath(path string) bool {
	p := strings.ToLower(strings.TrimSpace(path))
	if p == "" {
		return false
	}
	return strings.Contains(p, "interface")
}
