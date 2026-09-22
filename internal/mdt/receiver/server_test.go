package receiver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/mdt/pb/mdtdialout"
	"github.com/netspec/netspec/internal/mdt/pb/telemetrybis"
	"github.com/netspec/netspec/internal/mdt/transformer"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

func TestMdtDialoutEmitsInterfaceEvents(t *testing.T) {
	var mu sync.Mutex
	var got []collector.PushTelemetryEvent
	xf := transformer.New(transformer.Config{ResendInterval: time.Hour})
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Workers:    1,
		QueueSize:  8,
	}, xf, func(ev collector.PushTelemetryEvent) {
		mu.Lock()
		got = append(got, ev)
		mu.Unlock()
	}, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	var addr string
	for time.Now().Before(deadline) {
		addr = srv.Addr()
		if addr != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("server did not bind")
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := mdtdialout.NewGRPCMdtDialoutClient(conn)
	stream, err := client.MdtDialout(ctx)
	if err != nil {
		t.Fatal(err)
	}

	msg := decoderFixture()
	raw, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&mdtdialout.MdtDialoutArgs{ReqId: 1, Data: raw}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	_, _ = stream.Recv()

	waitUntil(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 2
	})
	mu.Lock()
	defer mu.Unlock()
	if got[0].Device != "csw-01" || got[0].Interface != "GigabitEthernet1/0/1" {
		t.Fatalf("event: %+v", got[0])
	}
	if got[0].OperStatus != "down" || got[1].OperStatus != "up" {
		t.Fatalf("oper: %+v %+v", got[0], got[1])
	}
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}

	st := srv.Stats()
	if st.PacketsReceived != 1 || st.RecordsUnpacked != 2 {
		t.Fatalf("receiver stats: %+v", st)
	}
	if st.Transformer.Emitted != 2 || st.QueueCap != 8 {
		t.Fatalf("transformer stats: %+v", st)
	}
	if st.StreamsOpened != 1 || st.LastPacketAt.IsZero() {
		t.Fatalf("stream stats: %+v", st)
	}
}

func TestQueueBlockedIncrements(t *testing.T) {
	block := make(chan struct{})
	xf := transformer.New(transformer.Config{ResendInterval: time.Hour})
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Workers:    1,
		QueueSize:  1,
	}, xf, func(collector.PushTelemetryEvent) {
		<-block
	}, zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()

	addr := waitAddr(t, srv)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	stream, err := mdtdialout.NewGRPCMdtDialoutClient(conn).MdtDialout(ctx)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := proto.Marshal(decoderFixture())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := stream.Send(&mdtdialout.MdtDialoutArgs{ReqId: int64(i + 1), Data: raw}); err != nil {
			t.Fatal(err)
		}
	}

	waitUntil(t, 2*time.Second, func() bool {
		return srv.Stats().QueueBlocked > 0
	})
	close(block)
	_ = stream.CloseSend()
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func waitAddr(t *testing.T, srv *Server) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if addr := srv.Addr(); addr != "" {
			return addr
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not bind")
	return ""
}

func decoderFixture() *telemetrybis.Telemetry {
	return &telemetrybis.Telemetry{
		NodeId:       &telemetrybis.Telemetry_NodeIdStr{NodeIdStr: "csw-01"},
		EncodingPath: "openconfig-interfaces:interfaces/interface",
		DataGpbkv: []*telemetrybis.TelemetryField{
			row("GigabitEthernet1/0/1", "DOWN", "UP"),
			row("GigabitEthernet1/0/2", "UP", "UP"),
		},
	}
}

func row(name, oper, admin string) *telemetrybis.TelemetryField {
	return &telemetrybis.TelemetryField{
		Fields: []*telemetrybis.TelemetryField{
			{Name: "keys", Fields: []*telemetrybis.TelemetryField{
				{Name: "name", ValueByType: &telemetrybis.TelemetryField_StringValue{StringValue: name}},
			}},
			{Name: "content", Fields: []*telemetrybis.TelemetryField{
				{Name: "oper-status", ValueByType: &telemetrybis.TelemetryField_StringValue{StringValue: oper}},
				{Name: "admin-status", ValueByType: &telemetrybis.TelemetryField_StringValue{StringValue: admin}},
			}},
		},
	}
}

func waitUntil(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out")
}
