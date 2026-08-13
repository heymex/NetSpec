package graph

import (
	"math"
	"sort"
	"time"
)

const hoursPerWeek = 7 * 24 // 168

// BandBucket holds percentile envelope stats for one hour-of-week slot.
type BandBucket struct {
	P10 float64 `json:"p10"`
	P90 float64 `json:"p90"`
	N   int     `json:"n"`
}

// HourOfWeekMonday returns 0..167 for t in loc, with Monday 00:00–00:59 = 0.
// Site-local bucketing (not UTC) keeps “9am class hour” stable across DST.
func HourOfWeekMonday(t time.Time, loc *time.Location) int {
	if loc == nil {
		loc = time.UTC
	}
	lt := t.In(loc)
	wd := int(lt.Weekday()) // Sunday=0 … Saturday=6
	if wd == 0 {
		wd = 6 // Sunday → end of Mon-based week
	} else {
		wd-- // Monday=0
	}
	return wd*24 + lt.Hour()
}

// Percentile returns the p-th percentile (0–100) of sorted ascending values.
// Linear interpolation between ranks. Empty input returns NaN.
func Percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	if n == 1 {
		return sorted[0]
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[n-1]
	}
	rank := (p / 100) * float64(n-1)
	lo := int(math.Floor(rank))
	hi := int(math.Ceil(rank))
	if lo == hi {
		return sorted[lo]
	}
	w := rank - float64(lo)
	return sorted[lo]*(1-w) + sorted[hi]*w
}

// BuildHourOfWeekBand aggregates samples into 168 local hour-of-week buckets
// and computes p10/p90 per bucket. Buckets with no samples have N=0.
func BuildHourOfWeekBand(samples []Point, loc *time.Location) [hoursPerWeek]BandBucket {
	var vals [hoursPerWeek][]float64
	for _, p := range samples {
		if p.V == nil || math.IsNaN(*p.V) || math.IsInf(*p.V, 0) {
			continue
		}
		// Use absolute magnitude so In/Out share the same typical envelope scale.
		v := math.Abs(*p.V)
		h := HourOfWeekMonday(time.Unix(p.T, 0).UTC(), loc)
		if h < 0 || h >= hoursPerWeek {
			continue
		}
		vals[h] = append(vals[h], v)
	}

	var out [hoursPerWeek]BandBucket
	for i := 0; i < hoursPerWeek; i++ {
		if len(vals[i]) == 0 {
			continue
		}
		sort.Float64s(vals[i])
		out[i] = BandBucket{
			P10: Percentile(vals[i], 10),
			P90: Percentile(vals[i], 90),
			N:   len(vals[i]),
		}
	}
	return out
}

// ProjectBand maps hour-of-week buckets onto [start,end] at step, producing
// p10/p90 series aligned for chart overlay. Gaps where N==0 yield nulls.
func ProjectBand(buckets [hoursPerWeek]BandBucket, start, end time.Time, step time.Duration, loc *time.Location) (p10, p90 []Point) {
	if step <= 0 {
		step = time.Minute
	}
	if !end.After(start) {
		return nil, nil
	}
	n := int(end.Sub(start)/step) + 1
	if n < 1 {
		return nil, nil
	}
	if n > 20000 {
		n = 20000
	}
	times := make([]int64, 0, n)
	for t := start; !t.After(end) && len(times) < n; t = t.Add(step) {
		times = append(times, t.Unix())
	}
	return ProjectBandOnto(buckets, times, loc)
}

// ProjectBandOnto evaluates buckets at explicit unix timestamps (e.g. live series times).
func ProjectBandOnto(buckets [hoursPerWeek]BandBucket, times []int64, loc *time.Location) (p10, p90 []Point) {
	if len(times) == 0 {
		return nil, nil
	}
	p10 = make([]Point, len(times))
	p90 = make([]Point, len(times))
	for i, ts := range times {
		p10[i].T = ts
		p90[i].T = ts
		h := HourOfWeekMonday(time.Unix(ts, 0).UTC(), loc)
		if h < 0 || h >= hoursPerWeek {
			continue
		}
		b := buckets[h]
		if b.N == 0 {
			continue
		}
		p10[i].V = floatPtr(b.P10)
		p90[i].V = floatPtr(b.P90)
	}
	return p10, p90
}

func floatPtr(v float64) *float64 { return &v }

// ShiftPoints adds delta to every timestamp (for baseline ghost overlays).
func ShiftPoints(pts []Point, delta time.Duration) []Point {
	if len(pts) == 0 {
		return pts
	}
	sec := int64(delta.Seconds())
	out := make([]Point, len(pts))
	for i, p := range pts {
		out[i].T = p.T + sec
		out[i].V = p.V
	}
	return out
}

// BandHasData reports whether any bucket has samples.
func BandHasData(buckets [hoursPerWeek]BandBucket) bool {
	for i := range buckets {
		if buckets[i].N > 0 {
			return true
		}
	}
	return false
}
