package graph

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/graph/vm"
)

// InterfaceSeries is the vertical-slice payload for one device/interface.
type InterfaceSeries struct {
	Device    string             `json:"device"`
	Interface string             `json:"interface"`
	Start     int64              `json:"start"`
	End       int64              `json:"end"`
	StepSec   int                `json:"step_sec"`
	SpeedBPS  *float64           `json:"speed_bps"` // nil if absent/zero — util not computed
	HasData   bool               `json:"has_data"`
	Series    map[string][]Point `json:"series"`
	Band      *BandPayload       `json:"band,omitempty"`
	Baseline  *BaselinePayload   `json:"baseline,omitempty"`
}

// Point is a chart sample. V is omitted/null when there is no data at T.
type Point struct {
	T int64    `json:"t"`
	V *float64 `json:"v"`
}

// BandPayload is the hour-of-week p10/p90 envelope projected onto the chart window.
type BandPayload struct {
	WindowDays int  `json:"window_days"`
	Timezone   string `json:"timezone"`
	HasData    bool `json:"has_data"`
	// Series keys: in_p10, in_p90, out_p10, out_p90 (absolute bps; UI negates out).
	Series map[string][]Point `json:"series"`
}

// BaselinePayload is a time-shifted historical ghost of the same chart duration.
type BaselinePayload struct {
	Shift     string `json:"shift"` // e.g. "168h", "8736h" (52w)
	Label     string `json:"label"`
	HasData   bool   `json:"has_data"`
	Series    map[string][]Point `json:"series"` // in_bps, out_bps (shifted onto current axis)
}

// SeriesOptions controls band/baseline enrichment on FetchInterfaceSeries.
type SeriesOptions struct {
	Location   *time.Location // site-local for hour-of-week buckets
	BandWindow time.Duration  // trailing window; 0 → 21d; <0 disables
	Baseline   time.Duration  // lookback shift; 0 disables
	BaselineLabel string
}

// FetchInterfaceSeries loads utilization/errors/oper for one interface over [end-window, end].
func FetchInterfaceSeries(ctx context.Context, client *vm.Client, device, iface string, end time.Time, window, step time.Duration) (*InterfaceSeries, error) {
	return FetchInterfaceSeriesOpts(ctx, client, device, iface, end, window, step, SeriesOptions{})
}

// FetchInterfaceSeriesOpts is FetchInterfaceSeries plus optional seasonality band / baseline.
func FetchInterfaceSeriesOpts(ctx context.Context, client *vm.Client, device, iface string, end time.Time, window, step time.Duration, opts SeriesOptions) (*InterfaceSeries, error) {
	if client == nil {
		return nil, fmt.Errorf("vm client is nil")
	}
	if window <= 0 {
		window = 6 * time.Hour
	}
	if step <= 0 {
		step = 30 * time.Second
	}
	if end.IsZero() {
		end = time.Now().UTC()
	}
	start := end.Add(-window)
	sel, telemetryIface, err := selectorForDeviceIface(ctx, client, device, iface)
	if err != nil {
		return nil, fmt.Errorf("resolve interface label: %w", err)
	}

	type namedQuery struct {
		key string
		q   string
	}
	queries := []namedQuery{
		{"in_bps", fmt.Sprintf(`rate(if_in_octets_total%s[2m]) * 8`, sel)},
		{"out_bps", fmt.Sprintf(`rate(if_out_octets_total%s[2m]) * 8`, sel)},
		{"in_errors_ps", fmt.Sprintf(`rate(if_in_errors_total%s[2m])`, sel)},
		{"out_errors_ps", fmt.Sprintf(`rate(if_out_errors_total%s[2m])`, sel)},
		{"in_discards_ps", fmt.Sprintf(`rate(if_in_discards_total%s[2m])`, sel)},
		{"out_discards_ps", fmt.Sprintf(`rate(if_out_discards_total%s[2m])`, sel)},
		{"oper_status", fmt.Sprintf(`if_oper_status%s`, sel)},
		{"speed_bps", fmt.Sprintf(`if_speed_bps%s`, sel)},
	}

	results := make(map[string][]vm.Series, len(queries))
	errs := make([]error, len(queries))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for i, nq := range queries {
		wg.Add(1)
		go func(i int, nq namedQuery) {
			defer wg.Done()
			series, err := client.QueryRange(ctx, nq.q, start, end, step)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", nq.key, err)
				return
			}
			results[nq.key] = series
		}(i, nq)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	out := &InterfaceSeries{
		Device:    device,
		Interface: telemetryIface,
		Start:     start.Unix(),
		End:       end.Unix(),
		StepSec:   int(step.Seconds()),
		Series:    map[string][]Point{},
	}

	for _, key := range []string{"in_bps", "out_bps", "in_errors_ps", "out_errors_ps", "in_discards_ps", "out_discards_ps", "oper_status"} {
		pts, has := firstSeriesPoints(results[key])
		out.Series[key] = pts
		if has {
			out.HasData = true
		}
	}

	speedPts, _ := firstSeriesPoints(results["speed_bps"])
	speed := lastNonNil(speedPts)
	if speed != nil && *speed > 0 && !math.IsInf(*speed, 0) {
		out.SpeedBPS = speed
		out.Series["in_util_pct"] = scalePoints(out.Series["in_bps"], 100.0 / *speed)
		out.Series["out_util_pct"] = scalePoints(out.Series["out_bps"], 100.0 / *speed)
	} else {
		// Guard divide-by-zero / absent speed: expose bps only, util series absent.
		out.Series["in_util_pct"] = nilPoints(out.Series["in_bps"])
		out.Series["out_util_pct"] = nilPoints(out.Series["out_bps"])
	}

	loc := opts.Location
	if loc == nil {
		loc = time.UTC
	}
	tzName := loc.String()

	bandWindow := opts.BandWindow
	if bandWindow == 0 {
		bandWindow = 21 * 24 * time.Hour
	}
	if bandWindow > 0 {
		liveTimes := pointTimes(out.Series["in_bps"])
		if len(liveTimes) == 0 {
			liveTimes = pointTimes(out.Series["out_bps"])
		}
		band, err := fetchBand(ctx, client, sel, end, bandWindow, window, step, loc, tzName, speed, liveTimes)
		if err != nil {
			return nil, err
		}
		out.Band = band
	}

	if opts.Baseline > 0 {
		base, err := fetchBaseline(ctx, client, sel, start, end, step, opts.Baseline, opts.BaselineLabel, speed)
		if err != nil {
			return nil, err
		}
		out.Baseline = base
	}

	return out, nil
}

func fetchBand(ctx context.Context, client *vm.Client, sel string, end time.Time, bandWindow, chartWindow, chartStep time.Duration, loc *time.Location, tzName string, speed *float64, liveTimes []int64) (*BandPayload, error) {
	bandStart := end.Add(-bandWindow)
	// Coarser step for the trailing window keeps the query cheap.
	bandStep := 5 * time.Minute
	if bandWindow >= 28*24*time.Hour {
		bandStep = 15 * time.Minute
	}

	inQ := fmt.Sprintf(`rate(if_in_octets_total%s[2m]) * 8`, sel)
	outQ := fmt.Sprintf(`rate(if_out_octets_total%s[2m]) * 8`, sel)

	var inSeries, outSeries []vm.Series
	var inErr, outErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		inSeries, inErr = client.QueryRange(ctx, inQ, bandStart, end, bandStep)
	}()
	go func() {
		defer wg.Done()
		outSeries, outErr = client.QueryRange(ctx, outQ, bandStart, end, bandStep)
	}()
	wg.Wait()
	if inErr != nil {
		return nil, fmt.Errorf("band in_bps: %w", inErr)
	}
	if outErr != nil {
		return nil, fmt.Errorf("band out_bps: %w", outErr)
	}

	inPts, _ := firstSeriesPoints(inSeries)
	outPts, _ := firstSeriesPoints(outSeries)
	inBuckets := BuildHourOfWeekBand(inPts, loc)
	outBuckets := BuildHourOfWeekBand(outPts, loc)

	refTimes := liveTimes
	if len(refTimes) == 0 {
		chartStart := end.Add(-chartWindow)
		for t := chartStart; !t.After(end); t = t.Add(chartStep) {
			refTimes = append(refTimes, t.Unix())
			if len(refTimes) > 20000 {
				break
			}
		}
	}
	inP10, inP90 := ProjectBandOnto(inBuckets, refTimes, loc)
	outP10, outP90 := ProjectBandOnto(outBuckets, refTimes, loc)

	has := BandHasData(inBuckets) || BandHasData(outBuckets)
	payload := &BandPayload{
		WindowDays: int(bandWindow / (24 * time.Hour)),
		Timezone:   tzName,
		HasData:    has,
		Series: map[string][]Point{
			"in_p10":  inP10,
			"in_p90":  inP90,
			"out_p10": outP10,
			"out_p90": outP90,
		},
	}
	if speed != nil && *speed > 0 {
		factor := 100.0 / *speed
		payload.Series["in_p10_pct"] = scalePoints(inP10, factor)
		payload.Series["in_p90_pct"] = scalePoints(inP90, factor)
		payload.Series["out_p10_pct"] = scalePoints(outP10, factor)
		payload.Series["out_p90_pct"] = scalePoints(outP90, factor)
	}
	return payload, nil
}

func pointTimes(pts []Point) []int64 {
	if len(pts) == 0 {
		return nil
	}
	out := make([]int64, len(pts))
	for i, p := range pts {
		out[i] = p.T
	}
	return out
}

func fetchBaseline(ctx context.Context, client *vm.Client, sel string, start, end time.Time, step, shift time.Duration, label string, speed *float64) (*BaselinePayload, error) {
	histStart := start.Add(-shift)
	histEnd := end.Add(-shift)
	inQ := fmt.Sprintf(`rate(if_in_octets_total%s[2m]) * 8`, sel)
	outQ := fmt.Sprintf(`rate(if_out_octets_total%s[2m]) * 8`, sel)

	var inSeries, outSeries []vm.Series
	var inErr, outErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		inSeries, inErr = client.QueryRange(ctx, inQ, histStart, histEnd, step)
	}()
	go func() {
		defer wg.Done()
		outSeries, outErr = client.QueryRange(ctx, outQ, histStart, histEnd, step)
	}()
	wg.Wait()
	if inErr != nil {
		return nil, fmt.Errorf("baseline in_bps: %w", inErr)
	}
	if outErr != nil {
		return nil, fmt.Errorf("baseline out_bps: %w", outErr)
	}

	inPts, inHas := firstSeriesPoints(inSeries)
	outPts, outHas := firstSeriesPoints(outSeries)
	inShifted := ShiftPoints(inPts, shift)
	outShifted := ShiftPoints(outPts, shift)

	if label == "" {
		label = shift.String() + " ago"
	}
	payload := &BaselinePayload{
		Shift:   shift.String(),
		Label:   label,
		HasData: inHas || outHas,
		Series: map[string][]Point{
			"in_bps":  inShifted,
			"out_bps": outShifted,
		},
	}
	if speed != nil && *speed > 0 {
		factor := 100.0 / *speed
		payload.Series["in_util_pct"] = scalePoints(inShifted, factor)
		payload.Series["out_util_pct"] = scalePoints(outShifted, factor)
	}
	return payload, nil
}

func firstSeriesPoints(series []vm.Series) ([]Point, bool) {
	if len(series) == 0 || len(series[0].Points) == 0 {
		return nil, false
	}
	pts := make([]Point, len(series[0].Points))
	has := false
	for i, p := range series[0].Points {
		pts[i] = Point{T: p.T, V: p.V}
		if p.V != nil {
			has = true
		}
	}
	return pts, has
}

func lastNonNil(pts []Point) *float64 {
	for i := len(pts) - 1; i >= 0; i-- {
		if pts[i].V != nil {
			v := *pts[i].V
			return &v
		}
	}
	return nil
}

func scalePoints(pts []Point, factor float64) []Point {
	if pts == nil {
		return nil
	}
	out := make([]Point, len(pts))
	for i, p := range pts {
		out[i].T = p.T
		if p.V == nil {
			continue
		}
		v := *p.V * factor
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out[i].V = &v
	}
	return out
}

func nilPoints(pts []Point) []Point {
	if pts == nil {
		return nil
	}
	out := make([]Point, len(pts))
	for i, p := range pts {
		out[i].T = p.T
	}
	return out
}
