package collector

import (
	"testing"
	"time"
)

func TestIngestStale(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	threshold := 5 * time.Minute

	tests := []struct {
		name        string
		lastEventAt time.Time
		startedAt   time.Time
		now         time.Time
		threshold   time.Duration
		want        bool
	}{
		{
			name:      "fresh start is not stale",
			startedAt: started,
			now:       started.Add(30 * time.Second),
			threshold: threshold,
			want:      false,
		},
		{
			name:      "never received past threshold is stale",
			startedAt: started,
			now:       started.Add(threshold),
			threshold: threshold,
			want:      true,
		},
		{
			name:        "recent event is not stale",
			lastEventAt: started.Add(10 * time.Minute),
			startedAt:   started,
			now:         started.Add(12 * time.Minute),
			threshold:   threshold,
			want:        false,
		},
		{
			name:        "last event older than threshold is stale",
			lastEventAt: started,
			startedAt:   started,
			now:         started.Add(threshold),
			threshold:   threshold,
			want:        true,
		},
		{
			name:        "zero threshold disables",
			lastEventAt: started,
			startedAt:   started,
			now:         started.Add(time.Hour),
			threshold:   0,
			want:        false,
		},
		{
			name:      "zero startedAt and lastEventAt is not stale",
			now:       started,
			threshold: threshold,
			want:      false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IngestStale(tc.lastEventAt, tc.startedAt, tc.now, tc.threshold)
			if got != tc.want {
				t.Fatalf("IngestStale() = %v, want %v", got, tc.want)
			}
		})
	}
}
