package transformer

import (
	"testing"
	"time"

	"github.com/netspec/netspec/internal/mdt/decoder"
	"github.com/netspec/netspec/internal/mdt/pb/telemetrybis"
	"google.golang.org/protobuf/proto"
)

func TestNormOperAdminParity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
		fn       func(string) string
	}{
		{"up", "up", NormOper},
		{"DOWN", "down", NormOper},
		{"lower-layer-down", "down", NormOper},
		{"if-oper-state-ready", "up", NormOper},
		{"if-oper-state-lower-layer-down", "down", NormOper},
		{"dormant", "down", NormOper},
		{"weird", "weird", NormOper},
		{"enabled", "up", NormAdmin},
		{"UP", "up", NormAdmin},
		{"disabled", "down", NormAdmin},
		{"administratively-down", "down", NormAdmin},
		{"if-state-up", "up", NormAdmin},
		{"", "", NormOper},
		{"", "", NormAdmin},
	}
	for _, tc := range cases {
		if got := tc.fn(tc.in); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestEventsFromOpenConfigAndNative(t *testing.T) {
	t.Parallel()
	xf := New(Config{ResendInterval: time.Hour})
	recs := []*decoder.Record{
		{
			NodeID:       "csw-01",
			EncodingPath: "openconfig-interfaces:interfaces/interface",
			Interface:    "GigabitEthernet1/0/1",
			Fields:       map[string]string{"oper_status": "DOWN", "admin_status": "UP"},
			Peer:         "10.1.1.1:1",
		},
		{
			NodeID:       "dist-sw-01",
			EncodingPath: "Cisco-IOS-XE-interfaces-oper:interfaces/interface",
			Interface:    "Te1/1/1",
			Fields: map[string]string{
				"state/oper_status":  "if-oper-state-ready",
				"state/admin_status": "if-state-up",
			},
		},
	}
	evs := xf.Events(recs)
	if len(evs) != 2 {
		t.Fatalf("len=%d", len(evs))
	}
	if evs[0].Device != "csw-01" || evs[0].Interface != "GigabitEthernet1/0/1" {
		t.Fatalf("ev0: %+v", evs[0])
	}
	if evs[0].OperStatus != "down" || evs[0].AdminStatus != "up" {
		t.Fatalf("status0: %+v", evs[0])
	}
	if evs[0].Source != DefaultSource {
		t.Fatalf("source: %q", evs[0].Source)
	}
	if evs[1].OperStatus != "up" || evs[1].AdminStatus != "up" {
		t.Fatalf("status1: %+v", evs[1])
	}
}

func TestSkipUnknownPathAndMissingStatus(t *testing.T) {
	t.Parallel()
	xf := New(Config{})
	evs := xf.Events([]*decoder.Record{
		{NodeID: "csw-01", EncodingPath: "Cisco-IOS-XE-device-hardware-oper:device-hardware-data", Interface: "cpu", Fields: map[string]string{"one_minute": "1"}},
		{NodeID: "csw-01", EncodingPath: "openconfig-interfaces:interfaces/interface", Interface: "Gi1/0/1", Fields: map[string]string{"in_octets": "9"}},
	})
	if len(evs) != 0 {
		t.Fatalf("got %+v", evs)
	}
	if xf.SkippedUnknownPath.Load() != 1 {
		t.Fatalf("unknown path count=%d", xf.SkippedUnknownPath.Load())
	}
	if xf.SkippedNoStatus.Load() != 1 {
		t.Fatalf("no status count=%d", xf.SkippedNoStatus.Load())
	}
}

func TestUnpackThenEvents(t *testing.T) {
	t.Parallel()
	msg := &telemetrybis.Telemetry{
		NodeId:       &telemetrybis.Telemetry_NodeIdStr{NodeIdStr: "csw-01"},
		EncodingPath: "openconfig-interfaces:interfaces/interface",
		DataGpbkv: []*telemetrybis.TelemetryField{{
			Fields: []*telemetrybis.TelemetryField{
				{
					Name: "keys",
					Fields: []*telemetrybis.TelemetryField{
						{Name: "name", ValueByType: &telemetrybis.TelemetryField_StringValue{StringValue: "GigabitEthernet1/0/1"}},
					},
				},
				{
					Name: "content",
					Fields: []*telemetrybis.TelemetryField{
						{Name: "oper-status", ValueByType: &telemetrybis.TelemetryField_StringValue{StringValue: "DOWN"}},
						{Name: "admin-status", ValueByType: &telemetrybis.TelemetryField_StringValue{StringValue: "UP"}},
					},
				},
			},
		}},
	}
	raw, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := decoder.Unpack("", raw)
	if err != nil {
		t.Fatal(err)
	}
	evs := New(Config{ResendInterval: time.Hour}).Events(recs)
	if len(evs) != 1 {
		t.Fatalf("len=%d recs=%d", len(evs), len(recs))
	}
	if evs[0].Device != "csw-01" || evs[0].Interface != "GigabitEthernet1/0/1" || evs[0].OperStatus != "down" || evs[0].AdminStatus != "up" {
		t.Fatalf("%+v", evs[0])
	}
}

func TestAllowlistAndResend(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	xf := New(Config{
		ResendInterval: 60 * time.Second,
		AllowedDevices: map[string]struct{}{"csw-01": {}},
		Now:            func() time.Time { return now },
	})
	rec := &decoder.Record{
		NodeID:       "csw-01",
		EncodingPath: "openconfig-interfaces:interfaces/interface",
		Interface:    "Gi1/0/1",
		Fields:       map[string]string{"oper_status": "up", "admin_status": "up"},
	}
	other := *rec
	other.NodeID = "stranger"
	if evs := xf.Events([]*decoder.Record{&other}); len(evs) != 0 {
		t.Fatalf("allowlist leaked: %+v", evs)
	}
	if n := len(xf.Events([]*decoder.Record{rec})); n != 1 {
		t.Fatalf("first emit=%d", n)
	}
	if n := len(xf.Events([]*decoder.Record{rec})); n != 0 {
		t.Fatalf("dedup should drop, got %d", n)
	}
	now = now.Add(61 * time.Second)
	if n := len(xf.Events([]*decoder.Record{rec})); n != 1 {
		t.Fatalf("resend after interval=%d", n)
	}
	rec.Fields["oper_status"] = "down"
	if n := len(xf.Events([]*decoder.Record{rec})); n != 1 {
		t.Fatalf("state change should emit=%d", n)
	}
	snap := xf.Snapshot()
	if snap.Emitted != 3 || snap.SkippedDedup != 1 || snap.SkippedAllowlist != 1 || snap.TrackedInterfaces != 1 {
		t.Fatalf("snapshot: %+v", snap)
	}
}
