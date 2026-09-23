package influx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/netspec/netspec/internal/mdt/graphmap"
	"github.com/rs/zerolog"
)

func TestNewEmptyURLDisabled(t *testing.T) {
	t.Parallel()
	if c := New(Config{}, zerolog.Nop()); c != nil {
		t.Fatal("empty URL should disable client")
	}
	if (Stats{}).Enabled {
		t.Fatal("zero stats should be disabled")
	}
}

func TestWriteLineProtocolAndFlush(t *testing.T) {
	t.Parallel()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/write" || r.URL.Query().Get("db") != "netspecgraph" {
			t.Errorf("url %s", r.URL.String())
		}
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New(Config{URL: srv.URL, FlushEvery: time.Hour, MaxBatch: 100}, zerolog.Nop())
	if c == nil {
		t.Fatal("client")
	}
	c.Write([]graphmap.Sample{
		{Name: "if_in_octets_total", Device: "csw-01", Interface: "GigabitEthernet1/0/1", Value: 100, Timestamp: 1_700_000_000_000},
		{Name: "if_oper_status", Device: "csw 01", Interface: "Gi1/0/1", Value: 1, Timestamp: 1_700_000_000_000},
	})
	c.Close()
	if !strings.Contains(got, "if_in_octets_total,device=csw-01,interface=GigabitEthernet1/0/1 value=100 1700000000000000000") {
		t.Fatalf("lp:\n%s", got)
	}
	if !strings.Contains(got, `if_oper_status,device=csw\ 01,interface=Gi1/0/1 value=1 1700000000000000000`) {
		t.Fatalf("escaped lp:\n%s", got)
	}
	st := c.Stats()
	if !st.Enabled || st.Samples != 2 || st.WriteOK != 1 || st.WriteFailed != 0 {
		t.Fatalf("stats: %+v", st)
	}
}

func TestWriteFailureIncrements(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := New(Config{URL: srv.URL, FlushEvery: time.Hour, MaxBatch: 1}, zerolog.Nop())
	c.Write([]graphmap.Sample{{Name: "if_speed_bps", Device: "d", Interface: "i", Value: 10}})
	c.Close()
	if c.Stats().WriteFailed != 1 {
		t.Fatalf("stats: %+v", c.Stats())
	}
}

func TestObserveMapAndOpticsFloat(t *testing.T) {
	t.Parallel()
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := New(Config{URL: srv.URL, FlushEvery: time.Hour}, zerolog.Nop())
	c.ObserveMap(graphmap.Result{SkippedCopper: 2, SkippedUnmapped: 3, SkippedNoTags: 1})
	c.Write([]graphmap.Sample{{
		Name: "transceiver_rx_power_dbm", Device: "asw-01", Interface: "Te1/1/1", Value: -2.5,
	}})
	c.Close()
	if !strings.Contains(got, "transceiver_rx_power_dbm,device=asw-01,interface=Te1/1/1 value=-2.5\n") {
		t.Fatalf("lp: %q", got)
	}
	st := c.Stats()
	if st.SkippedCopper != 2 || st.SkippedUnmapped != 3 || st.SkippedNoTags != 1 {
		t.Fatalf("skips: %+v", st)
	}
}

func TestLpNanos(t *testing.T) {
	t.Parallel()
	if lpNanos(0) != 0 {
		t.Fatal("zero")
	}
	if lpNanos(1_700_000_000) != 1_700_000_000_000_000_000 {
		t.Fatalf("seconds: %d", lpNanos(1_700_000_000))
	}
	if lpNanos(1_700_000_000_000) != 1_700_000_000_000_000_000 {
		t.Fatalf("ms: %d", lpNanos(1_700_000_000_000))
	}
}
