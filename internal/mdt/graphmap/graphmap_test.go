package graphmap

import (
	"testing"

	"github.com/netspec/netspec/internal/mdt/decoder"
)

func TestMapIETFInterfaceContract(t *testing.T) {
	t.Parallel()
	rec := &decoder.Record{
		NodeID:       "csw-01",
		Interface:    "GigabitEthernet1/0/1",
		EncodingPath: "ietf-interfaces:interfaces-state/interface",
		Timestamp:    1_700_000_000_000,
		Fields: map[string]string{
			"statistics/in_octets":        "100",
			"statistics/out_octets":       "200",
			"statistics/in_errors":        "1",
			"statistics/out_errors":       "2",
			"statistics/in_discards":      "3",
			"statistics/out_discards":     "4",
			"statistics/in_unicast_pkts":  "10",
			"statistics/out_unicast_pkts": "20",
			"speed":                       "1000000000",
			"oper_status":                 "up",
		},
	}
	got := Map([]*decoder.Record{rec})
	if got.SkippedUnmapped != 0 || got.SkippedCopper != 0 {
		t.Fatalf("skips: %+v", got)
	}
	byName := map[string]Sample{}
	for _, s := range got.Samples {
		byName[s.Name] = s
	}
	want := map[string]float64{
		"if_in_octets_total":        100,
		"if_out_octets_total":       200,
		"if_in_errors_total":        1,
		"if_out_errors_total":       2,
		"if_in_discards_total":      3,
		"if_out_discards_total":     4,
		"if_in_unicast_pkts_total":  10,
		"if_out_unicast_pkts_total": 20,
		"if_speed_bps":              1_000_000_000,
		"if_oper_status":            1,
	}
	if len(byName) != len(want) {
		t.Fatalf("samples=%d want %d names=%v", len(byName), len(want), keys(byName))
	}
	for name, val := range want {
		s, ok := byName[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if s.Device != "csw-01" || s.Interface != "GigabitEthernet1/0/1" {
			t.Fatalf("%s tags: %+v", name, s)
		}
		if s.Value != val {
			t.Fatalf("%s value=%v want %v", name, s.Value, val)
		}
		if s.Timestamp != rec.Timestamp {
			t.Fatalf("%s ts=%d", name, s.Timestamp)
		}
	}
}

func TestOperStatusMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want float64
	}{
		{"up", 1},
		{"UP", 1},
		{"1", 1},
		{"1.0", 1},
		{"down", 0},
		{"testing", 0},
		{"0", 0},
	}
	for _, tc := range cases {
		rec := ietfRec(map[string]string{"oper_status": tc.raw})
		got := Map([]*decoder.Record{rec})
		if len(got.Samples) != 1 || got.Samples[0].Name != "if_oper_status" || got.Samples[0].Value != tc.want {
			t.Errorf("oper %q: %+v", tc.raw, got.Samples)
		}
	}
}

func TestSkipNonNumericCounter(t *testing.T) {
	t.Parallel()
	rec := ietfRec(map[string]string{
		"statistics/in_octets": "not-a-number",
		"speed":                "1000",
	})
	got := Map([]*decoder.Record{rec})
	if len(got.Samples) != 1 || got.Samples[0].Name != "if_speed_bps" {
		t.Fatalf("samples: %+v", got.Samples)
	}
}

func TestOpticsContractAndCopperSkip(t *testing.T) {
	t.Parallel()
	fiber := &decoder.Record{
		NodeID:       "asw-01",
		Interface:    "TenGigabitEthernet1/1/1",
		EncodingPath: "Cisco-IOS-XE-transceiver-oper:transceiver-oper-data/transceiver",
		Fields: map[string]string{
			"input_power/instant":        "-2.5",
			"output_power/instant":       "-1.25",
			"laser_bias_current/instant": "7.5",
			"internal_temp":              "31.2",
			"ethernet_pmd":               "10GBASE-LR",
		},
	}
	copper := &decoder.Record{
		NodeID:       "asw-01",
		Interface:    "GigabitEthernet1/0/1",
		EncodingPath: "Cisco-IOS-XE-transceiver-oper:transceiver-oper-data/transceiver",
		Fields: map[string]string{
			"input_power/instant": "0",
			"ethernet_pmd":        "1000BaseT",
		},
	}
	got := Map([]*decoder.Record{fiber, copper})
	if got.SkippedCopper != 1 {
		t.Fatalf("copper skip: %+v", got)
	}
	byName := map[string]float64{}
	for _, s := range got.Samples {
		if s.Interface != "TenGigabitEthernet1/1/1" {
			t.Fatalf("unexpected iface %s", s.Interface)
		}
		byName[s.Name] = s.Value
	}
	if byName["transceiver_rx_power_dbm"] != -2.5 || byName["transceiver_tx_power_dbm"] != -1.25 {
		t.Fatalf("optics: %v", byName)
	}
	if byName["transceiver_laser_bias_ma"] != 7.5 || byName["transceiver_temp_celsius"] != 31.2 {
		t.Fatalf("optics: %v", byName)
	}
	if _, ok := byName["transceiver_voltage_volts"]; ok {
		t.Fatal("voltage should not be emitted")
	}
}

func TestUnmappedPathAndMissingTags(t *testing.T) {
	t.Parallel()
	other := &decoder.Record{
		NodeID:       "csw-01",
		Interface:    "Gi1/0/1",
		EncodingPath: "Cisco-IOS-XE-bgp-oper:bgp-state-data",
		Fields:       map[string]string{"oper_status": "up"},
	}
	noTags := &decoder.Record{
		EncodingPath: "ietf-interfaces:interfaces-state/interface",
		Fields:       map[string]string{"speed": "1"},
	}
	got := Map([]*decoder.Record{other, noTags})
	if len(got.Samples) != 0 {
		t.Fatalf("samples: %+v", got.Samples)
	}
	if got.SkippedUnmapped != 1 || got.SkippedNoTags != 1 {
		t.Fatalf("skips: %+v", got)
	}
}

func ietfRec(fields map[string]string) *decoder.Record {
	return &decoder.Record{
		NodeID:       "csw-01",
		Interface:    "GigabitEthernet1/0/1",
		EncodingPath: "ietf-interfaces:interfaces-state/interface",
		Fields:       fields,
	}
}

func keys(m map[string]Sample) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
