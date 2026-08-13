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
	Device    string            `json:"device"`
	Interface string            `json:"interface"`
	Start     int64             `json:"start"`
	End       int64             `json:"end"`
	StepSec   int               `json:"step_sec"`
	SpeedBPS  *float64          `json:"speed_bps"` // nil if absent/zero — util not computed
	HasData   bool              `json:"has_data"`
	Series    map[string][]Point `json:"series"`
}

// Point is a chart sample. V is omitted/null when there is no data at T.
type Point struct {
	T int64    `json:"t"`
	V *float64 `json:"v"`
}

// FetchInterfaceSeries loads utilization/errors/oper for one interface over [end-window, end].
func FetchInterfaceSeries(ctx context.Context, client *vm.Client, device, iface string, end time.Time, window, step time.Duration) (*InterfaceSeries, error) {
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
	sel := vm.Selector(device, iface)

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
		Interface: iface,
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

	return out, nil
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
