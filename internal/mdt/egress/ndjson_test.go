package egress

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/mdt/bandwidth"
	"github.com/rs/zerolog"
)

func TestParseTargets(t *testing.T) {
	t.Parallel()
	got, err := ParseTargets("10.0.0.1:57500,10.0.0.1:57500, 10.0.0.2:57501", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Port != 57500 || got[1].Host != "10.0.0.2" {
		t.Fatalf("%+v", got)
	}
	fb, err := ParseTargets("", "netspec-netspec", 57500)
	if err != nil || len(fb) != 1 || fb[0].Host != "netspec-netspec" {
		t.Fatalf("%+v %v", fb, err)
	}
}

func TestClientSendNDJSON(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	gotCh := make(chan collector.PushTelemetryEvent, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		sc := bufio.NewScanner(conn)
		if !sc.Scan() {
			return
		}
		var ev collector.PushTelemetryEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			return
		}
		gotCh <- ev
	}()

	_, portS, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portS)
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient([]Target{{Host: "127.0.0.1", Port: port}}, "", zerolog.Nop())
	c.SetBandwidthGroups(bandwidth.NewRegistry([]bandwidth.Group{
		{Name: "campus_core", Devices: []string{"csw-01"}},
	}))
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	evIn := collector.PushTelemetryEvent{
		Device:      "csw-01",
		Interface:   "Gi1/0/1",
		OperStatus:  "down",
		AdminStatus: "up",
		Source:      "netspec-mdt",
	}
	if err := c.Send(ctx, evIn); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-gotCh:
		if ev.Device != "csw-01" || ev.OperStatus != "down" || ev.Source != "netspec-mdt" {
			t.Fatalf("%+v", ev)
		}
	case <-ctx.Done():
		t.Fatal("did not receive event")
	}
	st := c.Stats()
	if st.ForwardOK != 1 || st.ForwardFailed != 0 || st.ForwardConns != 1 {
		t.Fatalf("egress stats: %+v", st)
	}
	if st.BytesForwarded == 0 {
		t.Fatalf("expected forwarded bytes, got %+v", st)
	}
	found := false
	for _, g := range st.ByGroup {
		if g.Group == "campus_core" {
			found = true
			if g.Bytes != st.BytesForwarded || g.Events != 1 {
				t.Fatalf("campus_core: %+v total=%d", g, st.BytesForwarded)
			}
		}
	}
	if !found {
		t.Fatalf("missing campus_core in %+v", st.ByGroup)
	}
}
