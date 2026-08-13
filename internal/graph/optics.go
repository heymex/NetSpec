package graph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/netspec/netspec/internal/graph/vm"
)

// OpticsSeries is the DOM payload for one device/interface optic.
type OpticsSeries struct {
	Device     string             `json:"device"`
	Interface  string             `json:"interface"`
	Start      int64              `json:"start"`
	End        int64              `json:"end"`
	StepSec    int                `json:"step_sec"`
	HasData    bool               `json:"has_data"`
	Series     map[string][]Point `json:"series"`
	Thresholds OpticsThresholds   `json:"thresholds"`
	Profile    string             `json:"threshold_profile"`
}

// OpticsThresholds are visual reference lines (warn/alarm), not alerts.
type OpticsThresholds struct {
	RxLowDbm  *float64 `json:"rx_low_dbm,omitempty"`
	RxHighDbm *float64 `json:"rx_high_dbm,omitempty"`
	TxLowDbm  *float64 `json:"tx_low_dbm,omitempty"`
	TxHighDbm *float64 `json:"tx_high_dbm,omitempty"`
	TempHighC *float64 `json:"temp_high_c,omitempty"`
}

// DefaultOpticsThresholds returns typical SFP-10G-SR visual ranges.
// Per-PID tables can replace this later; nothing pages off these lines.
func DefaultOpticsThresholds() (OpticsThresholds, string) {
	rxLo, rxHi := -11.1, -1.0
	txLo, txHi := -7.3, -1.0
	tempHi := 70.0
	return OpticsThresholds{
		RxLowDbm:  &rxLo,
		RxHighDbm: &rxHi,
		TxLowDbm:  &txLo,
		TxHighDbm: &txHi,
		TempHighC: &tempHi,
	}, "sfp-10g-sr-typical"
}

// FetchOpticsSeries loads DOM gauges for one interface over [end-window, end].
func FetchOpticsSeries(ctx context.Context, client *vm.Client, device, iface string, end time.Time, window, step time.Duration) (*OpticsSeries, error) {
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
		{"rx_power_dbm", fmt.Sprintf(`transceiver_rx_power_dbm%s`, sel)},
		{"tx_power_dbm", fmt.Sprintf(`transceiver_tx_power_dbm%s`, sel)},
		{"laser_bias_ma", fmt.Sprintf(`transceiver_laser_bias_ma%s`, sel)},
		{"temp_celsius", fmt.Sprintf(`transceiver_temp_celsius%s`, sel)},
		{"voltage_volts", fmt.Sprintf(`transceiver_voltage_volts%s`, sel)},
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

	th, profile := DefaultOpticsThresholds()
	out := &OpticsSeries{
		Device:     device,
		Interface:  iface,
		Start:      start.Unix(),
		End:        end.Unix(),
		StepSec:    int(step.Seconds()),
		Series:     map[string][]Point{},
		Thresholds: th,
		Profile:    profile,
	}
	for _, key := range []string{"rx_power_dbm", "tx_power_dbm", "laser_bias_ma", "temp_celsius", "voltage_volts"} {
		pts, has := firstSeriesPoints(results[key])
		out.Series[key] = pts
		if has {
			out.HasData = true
		}
	}
	return out, nil
}
