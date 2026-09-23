package decoder

import (
	"testing"

	"github.com/netspec/netspec/internal/mdt/pb/telemetrybis"
	"google.golang.org/protobuf/proto"
)

func TestTelemetryBisFieldNumbers(t *testing.T) {
	t.Parallel()
	md := (&telemetrybis.Telemetry{}).ProtoReflect().Descriptor()
	if n := md.Fields().ByName("encoding_path").Number(); n != 6 {
		t.Fatalf("encoding_path field number=%d want 6", n)
	}
	if n := md.Fields().ByName("data_gpbkv").Number(); n != 11 {
		t.Fatalf("data_gpbkv field number=%d want 11", n)
	}
	if n := md.Fields().ByName("msg_timestamp").Number(); n != 10 {
		t.Fatalf("msg_timestamp field number=%d want 10", n)
	}
	fd := (&telemetrybis.TelemetryField{}).ProtoReflect().Descriptor()
	if n := fd.Fields().ByName("string_value").Number(); n != 5 {
		t.Fatalf("string_value field number=%d want 5", n)
	}
	if n := fd.Fields().ByName("fields").Number(); n != 15 {
		t.Fatalf("fields field number=%d want 15", n)
	}

	msg := &telemetrybis.Telemetry{
		NodeId:       &telemetrybis.Telemetry_NodeIdStr{NodeIdStr: "csw-01"},
		EncodingPath: "openconfig-interfaces:interfaces/interface",
		DataGpbkv: []*telemetrybis.TelemetryField{{
			Fields: []*telemetrybis.TelemetryField{
				kvString("keys", "", []*telemetrybis.TelemetryField{
					kvLeaf("name", "GigabitEthernet1/0/1"),
				}),
				kvString("content", "", []*telemetrybis.TelemetryField{
					kvLeaf("oper-status", "UP"),
				}),
			},
		}},
	}
	b, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	round := &telemetrybis.Telemetry{}
	if err := proto.Unmarshal(b, round); err != nil {
		t.Fatal(err)
	}
	if round.GetNodeIdStr() != "csw-01" || round.GetEncodingPath() != msg.EncodingPath {
		t.Fatalf("round trip: node=%q path=%q", round.GetNodeIdStr(), round.GetEncodingPath())
	}
	if len(round.GetDataGpbkv()) != 1 {
		t.Fatalf("data_gpbkv len=%d", len(round.GetDataGpbkv()))
	}
}

func TestUnpackMultipleInterfaces(t *testing.T) {
	t.Parallel()
	msg := interfaceStateMsg("csw-01", "openconfig-interfaces:interfaces/interface", []ifaceRow{
		{name: "GigabitEthernet1/0/1", oper: "DOWN", admin: "UP"},
		{name: "GigabitEthernet1/0/2", oper: "UP", admin: "UP"},
	})
	raw, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	recs, err := Unpack("10.0.0.1:41234", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].Interface != "GigabitEthernet1/0/1" || recs[1].Interface != "GigabitEthernet1/0/2" {
		t.Fatalf("interfaces: %+v %+v", recs[0], recs[1])
	}
	if recs[0].Fields["oper_status"] != "DOWN" {
		t.Fatalf("oper_status: %q fields=%v", recs[0].Fields["oper_status"], recs[0].Fields)
	}
	if recs[0].Fields["admin_status"] != "UP" {
		t.Fatalf("admin_status: %q", recs[0].Fields["admin_status"])
	}
	if recs[0].NodeID != "csw-01" {
		t.Fatalf("node: %q", recs[0].NodeID)
	}
	if recs[0].Peer != "10.0.0.1:41234" {
		t.Fatalf("peer: %q", recs[0].Peer)
	}
}

func TestUnpackNumericCounterLeaves(t *testing.T) {
	t.Parallel()
	msg := &telemetrybis.Telemetry{
		NodeId:       &telemetrybis.Telemetry_NodeIdStr{NodeIdStr: "csw-01"},
		EncodingPath: "ietf-interfaces:interfaces-state/interface",
		MsgTimestamp: 1_700_000_000_000,
		DataGpbkv: []*telemetrybis.TelemetryField{{
			Fields: []*telemetrybis.TelemetryField{
				kvString("keys", "", []*telemetrybis.TelemetryField{
					kvLeaf("name", "GigabitEthernet1/0/1"),
				}),
				kvString("content", "", []*telemetrybis.TelemetryField{
					kvLeaf("oper-status", "up"),
					kvUint("speed", 1_000_000_000),
					kvString("statistics", "", []*telemetrybis.TelemetryField{
						kvUint("in-octets", 100),
						kvUint("out-octets", 200),
					}),
				}),
			},
		}},
	}
	recs := UnpackMessage("", msg)
	if len(recs) != 1 {
		t.Fatalf("len=%d", len(recs))
	}
	if recs[0].Fields["statistics/in_octets"] != "100" {
		t.Fatalf("in_octets path: %v", recs[0].Fields)
	}
	if recs[0].Fields["speed"] != "1000000000" {
		t.Fatalf("speed: %v", recs[0].Fields)
	}
	if recs[0].Timestamp != 1_700_000_000_000 {
		t.Fatalf("ts=%d", recs[0].Timestamp)
	}
}

func TestUnpackNativeIOSXEPath(t *testing.T) {
	t.Parallel()
	msg := interfaceStateMsg("dist-sw-01", "Cisco-IOS-XE-interfaces-oper:interfaces/interface", []ifaceRow{
		{name: "TenGigabitEthernet1/1/1", oper: "if-oper-state-ready", admin: "if-state-up"},
	})
	recs := UnpackMessage("", msg)
	if len(recs) != 1 {
		t.Fatalf("len=%d", len(recs))
	}
	if recs[0].EncodingPath != "Cisco-IOS-XE-interfaces-oper:interfaces/interface" {
		t.Fatalf("path: %q", recs[0].EncodingPath)
	}
	if recs[0].Fields["oper_status"] != "if-oper-state-ready" {
		t.Fatalf("fields: %v", recs[0].Fields)
	}
}

func TestUnpackSkipsKeysWithoutContent(t *testing.T) {
	t.Parallel()
	msg := &telemetrybis.Telemetry{
		NodeId:       &telemetrybis.Telemetry_NodeIdStr{NodeIdStr: "csw-01"},
		EncodingPath: "openconfig-interfaces:interfaces/interface",
		DataGpbkv: []*telemetrybis.TelemetryField{{
			Fields: []*telemetrybis.TelemetryField{
				kvString("keys", "", []*telemetrybis.TelemetryField{
					kvLeaf("name", "GigabitEthernet1/0/1"),
				}),
			},
		}},
	}
	if recs := UnpackMessage("", msg); len(recs) != 0 {
		t.Fatalf("want skip, got %+v", recs)
	}
}

func TestUnpackNestedStateFields(t *testing.T) {
	t.Parallel()
	msg := &telemetrybis.Telemetry{
		NodeId:       &telemetrybis.Telemetry_NodeIdStr{NodeIdStr: "csw-01"},
		EncodingPath: "openconfig-interfaces:interfaces/interface",
		DataGpbkv: []*telemetrybis.TelemetryField{{
			Fields: []*telemetrybis.TelemetryField{
				kvString("keys", "", []*telemetrybis.TelemetryField{
					kvLeaf("name", "Gi1/0/1"),
				}),
				kvString("content", "", []*telemetrybis.TelemetryField{
					kvString("state", "", []*telemetrybis.TelemetryField{
						kvLeaf("oper-status", "UP"),
						kvLeaf("admin-status", "UP"),
					}),
				}),
			},
		}},
	}
	recs := UnpackMessage("", msg)
	if len(recs) != 1 {
		t.Fatalf("len=%d", len(recs))
	}
	if recs[0].Fields["oper_status"] != "UP" {
		t.Fatalf("short key missing: %v", recs[0].Fields)
	}
	if recs[0].Fields["state/oper_status"] != "UP" {
		t.Fatalf("nested key missing: %v", recs[0].Fields)
	}
}

func TestUnpackDepthCap(t *testing.T) {
	t.Parallel()
	inner := kvLeaf("oper-status", "UP")
	for i := 0; i < 40; i++ {
		inner = &telemetrybis.TelemetryField{Name: "wrap", Fields: []*telemetrybis.TelemetryField{inner}}
	}
	msg := &telemetrybis.Telemetry{
		DataGpbkv: []*telemetrybis.TelemetryField{{
			Fields: []*telemetrybis.TelemetryField{
				kvString("keys", "", []*telemetrybis.TelemetryField{kvLeaf("name", "Gi1/0/1")}),
				{Name: "content", Fields: []*telemetrybis.TelemetryField{inner}},
			},
		}},
	}
	recs := UnpackMessage("", msg)
	if len(recs) != 1 {
		t.Fatalf("len=%d", len(recs))
	}
	if recs[0].Fields["oper_status"] != "" {
		t.Fatalf("depth cap should drop the leaf, got %v", recs[0].Fields)
	}
}

type ifaceRow struct {
	name, oper, admin string
}

func interfaceStateMsg(node, path string, rows []ifaceRow) *telemetrybis.Telemetry {
	gpbkv := make([]*telemetrybis.TelemetryField, 0, len(rows))
	for _, row := range rows {
		gpbkv = append(gpbkv, &telemetrybis.TelemetryField{
			Fields: []*telemetrybis.TelemetryField{
				kvString("keys", "", []*telemetrybis.TelemetryField{
					kvLeaf("name", row.name),
				}),
				kvString("content", "", []*telemetrybis.TelemetryField{
					kvLeaf("oper-status", row.oper),
					kvLeaf("admin-status", row.admin),
				}),
			},
		})
	}
	return &telemetrybis.Telemetry{
		NodeId:       &telemetrybis.Telemetry_NodeIdStr{NodeIdStr: node},
		EncodingPath: path,
		DataGpbkv:    gpbkv,
	}
}

func kvString(name, value string, fields []*telemetrybis.TelemetryField) *telemetrybis.TelemetryField {
	f := &telemetrybis.TelemetryField{Name: name, Fields: fields}
	if value != "" {
		f.ValueByType = &telemetrybis.TelemetryField_StringValue{StringValue: value}
	}
	return f
}

func kvLeaf(name, value string) *telemetrybis.TelemetryField {
	return &telemetrybis.TelemetryField{
		Name:        name,
		ValueByType: &telemetrybis.TelemetryField_StringValue{StringValue: value},
	}
}

func kvUint(name string, v uint64) *telemetrybis.TelemetryField {
	return &telemetrybis.TelemetryField{
		Name:        name,
		ValueByType: &telemetrybis.TelemetryField_Uint64Value{Uint64Value: v},
	}
}
