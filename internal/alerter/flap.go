package alerter

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// FlapDetector tracks rapid state changes and suppresses flapping alerts.
type FlapDetector struct {
	log       zerolog.Logger
	threshold int           // number of state changes to trigger flap
	window    time.Duration // time window for threshold
	mu        sync.Mutex
	history   map[string][]time.Time // key: device|entity -> timestamps of changes
	flapping  map[string]bool        // key: device|entity -> currently flapping
}

// NewFlapDetector creates a new flap detector.
func NewFlapDetector(log zerolog.Logger, threshold int, window time.Duration) *FlapDetector {
	return &FlapDetector{
		log:       log.With().Str("component", "flap-detector").Logger(),
		threshold: threshold,
		window:    window,
		history:   make(map[string][]time.Time),
		flapping:  make(map[string]bool),
	}
}

// RecordChange records a state change and returns whether the entity is flapping.
// If flapping just started, returns (true, true). If already flapping, returns (true, false).
// If not flapping, returns (false, false).
func (f *FlapDetector) RecordChange(key string) (flapping bool, justStarted bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-f.window)

	// Append and prune old entries
	timestamps := f.history[key]
	pruned := make([]time.Time, 0, len(timestamps)+1)
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	pruned = append(pruned, now)
	f.history[key] = pruned

	if len(pruned) >= f.threshold {
		wasFlapping := f.flapping[key]
		f.flapping[key] = true
		if !wasFlapping {
			f.log.Warn().Str("key", key).Int("changes", len(pruned)).Msg("flapping detected")
			return true, true
		}
		return true, false
	}

	return false, false
}

// IsFlapping returns whether an entity is currently marked as flapping.
func (f *FlapDetector) IsFlapping(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.flapping[key]
}

// CheckStable checks if a flapping entity has stabilized (no changes within the window).
// Returns true if it was flapping and has now stopped.
func (f *FlapDetector) CheckStable(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.flapping[key] {
		return false
	}

	now := time.Now()
	cutoff := now.Add(-f.window)
	timestamps := f.history[key]
	recent := 0
	for _, ts := range timestamps {
		if ts.After(cutoff) {
			recent++
		}
	}

	if recent < f.threshold {
		delete(f.flapping, key)
		f.log.Info().Str("key", key).Msg("flapping stopped")
		return true
	}
	return false
}

// Cleanup removes stale entries older than the window. Call periodically.
func (f *FlapDetector) Cleanup() {
	f.mu.Lock()
	defer f.mu.Unlock()

	cutoff := time.Now().Add(-f.window)
	for key, timestamps := range f.history {
		pruned := make([]time.Time, 0, len(timestamps))
		for _, ts := range timestamps {
			if ts.After(cutoff) {
				pruned = append(pruned, ts)
			}
		}
		if len(pruned) == 0 {
			delete(f.history, key)
			delete(f.flapping, key)
		} else {
			f.history[key] = pruned
		}
	}
}
