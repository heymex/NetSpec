package graph

import (
	"math"
	"testing"
	"time"
)

func TestHourOfWeekMonday(t *testing.T) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-08-10 is a Monday.
	mon9 := time.Date(2026, 8, 10, 9, 30, 0, 0, loc)
	if got := HourOfWeekMonday(mon9, loc); got != 9 {
		t.Fatalf("Monday 09:30 → %d, want 9", got)
	}
	// Sunday 23:00 → weekday index 6, hour 23 → 6*24+23 = 167
	sun23 := time.Date(2026, 8, 16, 23, 0, 0, 0, loc)
	if got := HourOfWeekMonday(sun23, loc); got != 167 {
		t.Fatalf("Sunday 23:00 → %d, want 167", got)
	}
	// DST spring forward 2026-03-08 02:00 → 03:00 America/Chicago
	before := time.Date(2026, 3, 8, 1, 30, 0, 0, loc)
	after := time.Date(2026, 3, 8, 3, 30, 0, 0, loc)
	if HourOfWeekMonday(before, loc) == HourOfWeekMonday(after, loc) {
		t.Fatal("DST should place 1:30 and 3:30 in different local hour buckets")
	}
}

func TestPercentile(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := Percentile(vals, 0); got != 1 {
		t.Fatalf("p0 = %v", got)
	}
	if got := Percentile(vals, 100); got != 10 {
		t.Fatalf("p100 = %v", got)
	}
	// p50 of 1..10 → rank 4.5 → 5.5
	if got := Percentile(vals, 50); math.Abs(got-5.5) > 1e-9 {
		t.Fatalf("p50 = %v, want 5.5", got)
	}
	if math.IsNaN(Percentile(nil, 50)) == false {
		t.Fatal("empty should be NaN")
	}
}

func TestBuildAndProjectBand(t *testing.T) {
	loc := time.FixedZone("test", -6*3600)
	// Two Mondays at 10:00 with values 100 and 300 → p10/p90 between them.
	t1 := time.Date(2026, 8, 3, 10, 0, 0, 0, loc)  // Mon
	t2 := time.Date(2026, 8, 10, 10, 0, 0, 0, loc) // Mon
	v1, v2 := 100.0, 300.0
	samples := []Point{
		{T: t1.Unix(), V: &v1},
		{T: t2.Unix(), V: &v2},
		{T: t1.Add(time.Hour).Unix(), V: &v1}, // Mon 11:00 — different bucket
	}
	buckets := BuildHourOfWeekBand(samples, loc)
	h := HourOfWeekMonday(t1, loc) // Monday 10 → 10
	if buckets[h].N != 2 {
		t.Fatalf("bucket %d N=%d, want 2", h, buckets[h].N)
	}
	if buckets[h].P10 >= buckets[h].P90 && buckets[h].P10 != buckets[h].P90 {
		t.Fatalf("p10=%v p90=%v", buckets[h].P10, buckets[h].P90)
	}
	// With 2 points, p10 and p90 are interpolated toward the ends.
	if buckets[h].P10 < 100 || buckets[h].P90 > 300 {
		t.Fatalf("unexpected envelope %v–%v", buckets[h].P10, buckets[h].P90)
	}
	if !BandHasData(buckets) {
		t.Fatal("expected band data")
	}

	start := time.Date(2026, 8, 17, 9, 0, 0, 0, loc) // next Monday 09:00
	end := start.Add(3 * time.Hour)
	p10, p90 := ProjectBand(buckets, start, end, time.Hour, loc)
	if len(p10) != 4 || len(p90) != 4 {
		t.Fatalf("projected len p10=%d p90=%d", len(p10), len(p90))
	}
	// 10:00 slot should have values
	found := false
	for i := range p10 {
		if time.Unix(p10[i].T, 0).In(loc).Hour() == 10 && p10[i].V != nil && p90[i].V != nil {
			found = true
			if *p10[i].V != buckets[h].P10 || *p90[i].V != buckets[h].P90 {
				t.Fatalf("projected mismatch")
			}
		}
	}
	if !found {
		t.Fatal("expected projected Monday 10:00 band point")
	}
}

func TestShiftPoints(t *testing.T) {
	v := 1.5
	pts := []Point{{T: 1000, V: &v}}
	got := ShiftPoints(pts, 7*24*time.Hour)
	if got[0].T != 1000+7*24*3600 {
		t.Fatalf("shift T = %d", got[0].T)
	}
	if got[0].V == nil || *got[0].V != 1.5 {
		t.Fatalf("value changed")
	}
}

func TestBuildBandIgnoresNilAndUsesAbs(t *testing.T) {
	loc := time.UTC
	neg := -50.0
	pos := 50.0
	t0 := time.Date(2026, 8, 10, 12, 0, 0, 0, loc)
	buckets := BuildHourOfWeekBand([]Point{
		{T: t0.Unix(), V: &neg},
		{T: t0.Add(7 * 24 * time.Hour).Unix(), V: &pos},
	}, loc)
	h := HourOfWeekMonday(t0, loc)
	if buckets[h].N != 2 {
		t.Fatalf("N=%d", buckets[h].N)
	}
	if buckets[h].P10 != 50 || buckets[h].P90 != 50 {
		t.Fatalf("abs envelope = %v–%v, want 50–50", buckets[h].P10, buckets[h].P90)
	}
}
