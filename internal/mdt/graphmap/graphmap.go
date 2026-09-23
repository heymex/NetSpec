// Package graphmap contracts unpacked kvGPB rows into NetSpecGraph series.
// The mapping matches origin/feat/netspecgraph tools/sidecar/telegraf-mdt.conf
// Starlark processors (tags device/interface, single numeric value).
package graphmap

import (
	"strconv"
	"strings"

	"github.com/netspec/netspec/internal/mdt/decoder"
)

const (
	ietfInterfacePath = "ietf-interfaces:interfaces-state/interface"
	ciscoOpticsPath   = "cisco-ios-xe-transceiver-oper:transceiver-oper-data"
)

// Sample is one contracted VictoriaMetrics point.
type Sample struct {
	Name      string
	Device    string
	Interface string
	Value     float64
	Timestamp uint64
}

// Result is Map output, including records that produced no samples.
type Result struct {
	Samples         []Sample
	SkippedCopper   uint64
	SkippedUnmapped uint64
	SkippedNoTags   uint64
}

var interfaceFields = []struct {
	src, name string
	oper      bool
}{
	{"statistics/in_octets", "if_in_octets_total", false},
	{"statistics/out_octets", "if_out_octets_total", false},
	{"statistics/in_errors", "if_in_errors_total", false},
	{"statistics/out_errors", "if_out_errors_total", false},
	{"statistics/in_discards", "if_in_discards_total", false},
	{"statistics/out_discards", "if_out_discards_total", false},
	{"statistics/in_unicast_pkts", "if_in_unicast_pkts_total", false},
	{"statistics/out_unicast_pkts", "if_out_unicast_pkts_total", false},
	{"speed", "if_speed_bps", false},
	{"oper_status", "if_oper_status", true},
}

var opticsFields = []struct {
	src, name string
}{
	{"input_power/instant", "transceiver_rx_power_dbm"},
	{"output_power/instant", "transceiver_tx_power_dbm"},
	{"laser_bias_current/instant", "transceiver_laser_bias_ma"},
	{"internal_temp", "transceiver_temp_celsius"},
}

// Map converts unpacked MDT rows into Graph series. Non-matching paths are
// counted, not forwarded. Counter samples are not deduped (rate() needs them).
func Map(recs []*decoder.Record) Result {
	var out Result
	for _, rec := range recs {
		if rec == nil {
			continue
		}
		mapRecord(rec, &out)
	}
	return out
}

func mapRecord(rec *decoder.Record, out *Result) {
	device := strings.TrimSpace(rec.NodeID)
	iface := strings.TrimSpace(rec.Interface)
	path := rec.EncodingPath
	switch {
	case isIETFInterface(path):
		if device == "" || iface == "" {
			out.SkippedNoTags++
			return
		}
		n := appendInterface(rec, device, iface, &out.Samples)
		if n == 0 {
			out.SkippedUnmapped++
		}
	case isCiscoOptics(path):
		if device == "" || iface == "" {
			out.SkippedNoTags++
			return
		}
		if isCopperPMD(lookup(rec.Fields, "ethernet_pmd")) {
			out.SkippedCopper++
			return
		}
		n := appendOptics(rec, device, iface, &out.Samples)
		if n == 0 {
			out.SkippedUnmapped++
		}
	default:
		out.SkippedUnmapped++
	}
}

func isIETFInterface(path string) bool {
	return strings.Contains(strings.ToLower(path), ietfInterfacePath)
}

func isCiscoOptics(path string) bool {
	return strings.Contains(strings.ToLower(path), ciscoOpticsPath)
}

func appendInterface(rec *decoder.Record, device, iface string, dst *[]Sample) int {
	n := 0
	for _, f := range interfaceFields {
		raw, ok := rec.Fields[f.src]
		if !ok || raw == "" {
			continue
		}
		var val float64
		if f.oper {
			val = operGauge(raw)
		} else {
			v, ok := parseNumeric(raw)
			if !ok {
				continue
			}
			val = v
		}
		*dst = append(*dst, Sample{
			Name:      f.name,
			Device:    device,
			Interface: iface,
			Value:     val,
			Timestamp: rec.Timestamp,
		})
		n++
	}
	return n
}

func appendOptics(rec *decoder.Record, device, iface string, dst *[]Sample) int {
	n := 0
	for _, f := range opticsFields {
		raw, ok := rec.Fields[f.src]
		if !ok || raw == "" {
			continue
		}
		val, ok := parseNumeric(raw)
		if !ok {
			continue
		}
		*dst = append(*dst, Sample{
			Name:      f.name,
			Device:    device,
			Interface: iface,
			Value:     val,
			Timestamp: rec.Timestamp,
		})
		n++
	}
	return n
}

func lookup(fields map[string]string, key string) string {
	if fields == nil {
		return ""
	}
	return fields[key]
}

func isCopperPMD(pmd string) bool {
	pl := strings.ToLower(strings.TrimSpace(pmd))
	if pl == "" {
		return false
	}
	return strings.Contains(pl, "basetx") || strings.Contains(pl, "baset") || strings.Contains(pl, "1000base-t")
}

func operGauge(raw string) float64 {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "up" || s == "1" || s == "1.0" {
		return 1
	}
	return 0
}

func parseNumeric(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	switch strings.ToLower(s) {
	case "true", "false":
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
