package metrics

import (
	"fmt"
	"io"
)

// WritePrometheus renders Snapshot as Prometheus text exposition.
func WritePrometheus(w io.Writer, snap Snapshot) error {
	p := promWriter{w: w}
	rx := snap.Receiver
	xf := rx.Transformer
	fw := snap.Forward

	p.gauge("netspec_mdt_queue_len", "Decode queue occupancy.", float64(rx.QueueLen))
	p.gauge("netspec_mdt_queue_cap", "Decode queue capacity.", float64(rx.QueueCap))
	p.gauge("netspec_mdt_queue_high_water", "Maximum observed decode queue occupancy since start.", float64(rx.QueueHighWater))
	p.counter("netspec_mdt_queue_blocked_total", "Packets that waited because the decode queue was full.", rx.QueueBlocked)
	p.counter("netspec_mdt_packets_received_total", "MDT dial-out packets that carried telemetry bytes.", rx.PacketsReceived)
	p.counter("netspec_mdt_packets_empty_total", "MDT dial-out packets with empty data (idle/keepalive).", rx.PacketsEmpty)
	p.counter("netspec_mdt_packets_error_payload_total", "MDT dial-out packets that carried an errors string.", rx.PacketsErrorPayload)
	p.counter("netspec_mdt_decode_failed_total", "Telemetry protobuf unmarshal failures.", rx.DecodeFailed)
	p.counter("netspec_mdt_records_unpacked_total", "kvGPB rows unpacked from MDT packets.", rx.RecordsUnpacked)
	p.gauge("netspec_mdt_streams_active", "Open MdtDialout gRPC streams.", float64(rx.StreamsActive))
	p.counter("netspec_mdt_streams_opened_total", "MdtDialout streams accepted.", rx.StreamsOpened)
	p.counter("netspec_mdt_streams_closed_total", "MdtDialout streams closed.", rx.StreamsClosed)
	p.counter("netspec_mdt_events_emitted_total", "PushTelemetryEvent rows forwarded toward NetSpec ingest.", xf.Emitted)
	p.skip("unknown_path", xf.SkippedUnknownPath)
	p.skip("no_status", xf.SkippedNoStatus)
	p.skip("allowlist", xf.SkippedAllowlist)
	p.skip("dedup", xf.SkippedDedup)
	p.gauge("netspec_mdt_tracked_interfaces", "Device/interface pairs retained for 60s resend.", float64(xf.TrackedInterfaces))
	p.counter("netspec_mdt_forward_ok_total", "NDJSON events successfully written to ingest.", fw.ForwardOK)
	p.counter("netspec_mdt_forward_failed_total", "NDJSON events that failed to write to ingest.", fw.ForwardFailed)
	p.gauge("netspec_mdt_forward_conns", "Cached NDJSON TCP connections.", float64(fw.ForwardConns))
	return p.err
}

type promWriter struct {
	w   io.Writer
	err error
	// skipHeaderOnce ensures HELP/TYPE for skipped_total is written once.
	skipMeta bool
}

func (p *promWriter) counter(name, help string, v uint64) {
	p.meta(name, help, "counter")
	p.printf("%s %d\n", name, v)
}

func (p *promWriter) gauge(name, help string, v float64) {
	p.meta(name, help, "gauge")
	p.printf("%s %g\n", name, v)
}

func (p *promWriter) skip(reason string, v uint64) {
	const name = "netspec_mdt_events_skipped_total"
	if !p.skipMeta {
		p.meta(name, "Transformer rows not emitted, by reason.", "counter")
		p.skipMeta = true
	}
	p.printf("%s{reason=%q} %d\n", name, reason, v)
}

func (p *promWriter) meta(name, help, typ string) {
	p.printf("# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

func (p *promWriter) printf(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format, args...)
}
