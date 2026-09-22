package decoder

import (
	"strconv"
	"strings"

	"github.com/netspec/netspec/internal/mdt/pb/telemetrybis"
	"google.golang.org/protobuf/proto"
)

const maxDepth = 32

// Record is one keys/content kvGPB row (typically one interface).
type Record struct {
	NodeID         string
	SubscriptionID string
	EncodingPath   string
	Timestamp      uint64
	Peer           string
	Interface      string
	Keys           map[string]string
	Fields         map[string]string
}

// Unpack unmarshals a telemetry_bis.Telemetry blob from MdtDialoutArgs.data.
func Unpack(peer string, data []byte) ([]*Record, error) {
	msg := &telemetrybis.Telemetry{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return UnpackMessage(peer, msg), nil
}

// UnpackMessage walks data_gpbkv as keys/content rows (list-safe).
func UnpackMessage(peer string, msg *telemetrybis.Telemetry) []*Record {
	if msg == nil {
		return nil
	}
	out := make([]*Record, 0, len(msg.GetDataGpbkv()))
	for _, field := range msg.GetDataGpbkv() {
		walkRows(peer, msg, field, 0, &out)
	}
	return out
}

func walkRows(peer string, msg *telemetrybis.Telemetry, field *telemetrybis.TelemetryField, depth int, out *[]*Record) {
	if field == nil || depth > maxDepth {
		return
	}
	keys, content := splitKeysContent(field)
	if keys != nil || content != nil {
		if rec := buildRecord(peer, msg, keys, content); rec != nil {
			*out = append(*out, rec)
		}
		return
	}
	for _, sub := range field.GetFields() {
		walkRows(peer, msg, sub, depth+1, out)
	}
}

func splitKeysContent(field *telemetrybis.TelemetryField) (keys, content *telemetrybis.TelemetryField) {
	for _, sub := range field.GetFields() {
		switch sub.GetName() {
		case "keys":
			keys = sub
		case "content":
			content = sub
		}
	}
	return keys, content
}

func buildRecord(peer string, msg *telemetrybis.Telemetry, keys, content *telemetrybis.TelemetryField) *Record {
	if content == nil {
		return nil
	}
	rec := &Record{
		NodeID:         msg.GetNodeIdStr(),
		SubscriptionID: msg.GetSubscriptionIdStr(),
		EncodingPath:   msg.GetEncodingPath(),
		Timestamp:      msg.GetMsgTimestamp(),
		Peer:           peer,
		Keys:           map[string]string{},
		Fields:         map[string]string{},
	}
	if keys != nil {
		for _, sub := range keys.GetFields() {
			flatten(sub, "", 0, rec.Keys)
		}
	}
	for _, sub := range content.GetFields() {
		flatten(sub, "", 0, rec.Fields)
	}
	rec.Interface = interfaceName(rec.Keys)
	if rec.Interface == "" {
		rec.Interface = interfaceName(rec.Fields)
	}
	return rec
}

func flatten(field *telemetrybis.TelemetryField, prefix string, depth int, out map[string]string) {
	if field == nil || depth > maxDepth || out == nil {
		return
	}
	local := strings.ReplaceAll(field.GetName(), "-", "_")
	name := joinPath(prefix, local)
	if v := leafString(field); v != "" && name != "" {
		if local != "" {
			if _, exists := out[local]; !exists {
				out[local] = v
			}
		}
		out[name] = v
	}
	for _, sub := range field.GetFields() {
		flatten(sub, name, depth+1, out)
	}
}

func joinPath(prefix, name string) string {
	if name == "" {
		return prefix
	}
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func leafString(field *telemetrybis.TelemetryField) string {
	switch v := field.GetValueByType().(type) {
	case *telemetrybis.TelemetryField_StringValue:
		return v.StringValue
	case *telemetrybis.TelemetryField_BytesValue:
		return string(v.BytesValue)
	case *telemetrybis.TelemetryField_BoolValue:
		if v.BoolValue {
			return "true"
		}
		return "false"
	case *telemetrybis.TelemetryField_Uint32Value:
		return strconv.FormatUint(uint64(v.Uint32Value), 10)
	case *telemetrybis.TelemetryField_Uint64Value:
		return strconv.FormatUint(v.Uint64Value, 10)
	case *telemetrybis.TelemetryField_Sint32Value:
		return strconv.FormatInt(int64(v.Sint32Value), 10)
	case *telemetrybis.TelemetryField_Sint64Value:
		return strconv.FormatInt(v.Sint64Value, 10)
	case *telemetrybis.TelemetryField_DoubleValue:
		return strconv.FormatFloat(v.DoubleValue, 'f', -1, 64)
	case *telemetrybis.TelemetryField_FloatValue:
		return strconv.FormatFloat(float64(v.FloatValue), 'f', -1, 32)
	default:
		return ""
	}
}

func interfaceName(keys map[string]string) string {
	for _, k := range []string{"name", "interface", "interface_name", "if_name", "ifname"} {
		if v := strings.TrimSpace(keys[k]); v != "" {
			return v
		}
	}
	return ""
}
