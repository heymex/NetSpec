package receiver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/mdt/decoder"
	"github.com/netspec/netspec/internal/mdt/pb/mdtdialout"
	"github.com/netspec/netspec/internal/mdt/transformer"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
)

const (
	defaultMaxRecvMsgSize = 4 * 1024 * 1024
	defaultQueueSize      = 1024
	defaultKeepaliveMin   = 5 * time.Minute
)

// Config is the gRPC MDT dial-out listener.
type Config struct {
	ListenAddr                  string
	MaxRecvMsgSize              int
	QueueSize                   int
	Workers                     int
	KeepaliveMinTime            time.Duration
	PermitKeepaliveWithoutCalls bool
}

// Server implements mdt_dialout.gRPCMdtDialout.
type Server struct {
	mdtdialout.UnimplementedGRPCMdtDialoutServer
	cfg        Config
	log        zerolog.Logger
	xf         *transformer.Transformer
	onEvent    func(collector.PushTelemetryEvent)
	queue      chan job
	grpcServer *grpc.Server
	listener   net.Listener

	packetsReceived     atomic.Uint64
	packetsEmpty        atomic.Uint64
	packetsErrorPayload atomic.Uint64
	decodeFailed        atomic.Uint64
	recordsUnpacked     atomic.Uint64
	queueBlocked        atomic.Uint64
	queueHighWater      atomic.Uint64
	streamsActive       atomic.Int64
	streamsOpened       atomic.Uint64
	streamsClosed       atomic.Uint64

	lastMu       sync.Mutex
	lastPacketAt time.Time

	mu      sync.Mutex
	started bool
}

// Stats is a point-in-time copy of receiver + transformer counters.
type Stats struct {
	ListenAddr          string               `json:"listen_addr"`
	QueueLen            int                  `json:"queue_len"`
	QueueCap            int                  `json:"queue_cap"`
	QueueHighWater      uint64               `json:"queue_high_water"`
	QueueBlocked        uint64               `json:"queue_blocked"`
	PacketsReceived     uint64               `json:"packets_received"`
	PacketsEmpty        uint64               `json:"packets_empty"`
	PacketsErrorPayload uint64               `json:"packets_error_payload"`
	DecodeFailed        uint64               `json:"decode_failed"`
	RecordsUnpacked     uint64               `json:"records_unpacked"`
	StreamsActive       int64                `json:"streams_active"`
	StreamsOpened       uint64               `json:"streams_opened"`
	StreamsClosed       uint64               `json:"streams_closed"`
	LastPacketAt        time.Time            `json:"last_packet_at,omitempty"`
	Transformer         transformer.Snapshot `json:"transformer"`
}

type job struct {
	peer string
	data []byte
}

func New(cfg Config, xf *transformer.Transformer, onEvent func(collector.PushTelemetryEvent), log zerolog.Logger) *Server {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "0.0.0.0:57500"
	}
	if cfg.MaxRecvMsgSize <= 0 {
		cfg.MaxRecvMsgSize = defaultMaxRecvMsgSize
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = defaultQueueSize
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.KeepaliveMinTime <= 0 {
		cfg.KeepaliveMinTime = defaultKeepaliveMin
	}
	if xf == nil {
		xf = transformer.New(transformer.Config{})
	}
	if onEvent == nil {
		onEvent = func(collector.PushTelemetryEvent) {}
	}
	return &Server{
		cfg:     cfg,
		log:     log,
		xf:      xf,
		onEvent: onEvent,
		queue:   make(chan job, cfg.QueueSize),
	}
}

func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *Server) Stats() Stats {
	if s == nil {
		return Stats{}
	}
	s.mu.Lock()
	addr := ""
	if s.listener != nil {
		addr = s.listener.Addr().String()
	}
	s.mu.Unlock()
	s.lastMu.Lock()
	last := s.lastPacketAt
	s.lastMu.Unlock()
	qlen, qcap := 0, 0
	if s.queue != nil {
		qlen = len(s.queue)
		qcap = cap(s.queue)
	}
	return Stats{
		ListenAddr:          addr,
		QueueLen:            qlen,
		QueueCap:            qcap,
		QueueHighWater:      s.queueHighWater.Load(),
		QueueBlocked:        s.queueBlocked.Load(),
		PacketsReceived:     s.packetsReceived.Load(),
		PacketsEmpty:        s.packetsEmpty.Load(),
		PacketsErrorPayload: s.packetsErrorPayload.Load(),
		DecodeFailed:        s.decodeFailed.Load(),
		RecordsUnpacked:     s.recordsUnpacked.Load(),
		StreamsActive:       s.streamsActive.Load(),
		StreamsOpened:       s.streamsOpened.Load(),
		StreamsClosed:       s.streamsClosed.Load(),
		LastPacketAt:        last,
		Transformer:         s.xf.Snapshot(),
	}
}

func (s *Server) noteQueued() {
	n := uint64(len(s.queue))
	for {
		old := s.queueHighWater.Load()
		if n <= old || s.queueHighWater.CompareAndSwap(old, n) {
			return
		}
	}
}

// Start listens and serves until ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("mdt receiver already started")
	}
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("mdt listen %s: %w", s.cfg.ListenAddr, err)
	}
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(s.cfg.MaxRecvMsgSize),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             s.cfg.KeepaliveMinTime,
			PermitWithoutStream: s.cfg.PermitKeepaliveWithoutCalls,
		}),
	}
	gs := grpc.NewServer(opts...)
	mdtdialout.RegisterGRPCMdtDialoutServer(gs, s)
	s.listener = ln
	s.grpcServer = gs
	s.started = true
	s.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker()
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info().Str("address", ln.Addr().String()).Msg("MDT dial-out gRPC listening")
		errCh <- gs.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		gs.GracefulStop()
		close(s.queue)
		wg.Wait()
		return nil
	case err := <-errCh:
		close(s.queue)
		wg.Wait()
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
		return nil
	}
}

func (s *Server) worker() {
	for j := range s.queue {
		s.handle(j)
	}
}

func (s *Server) handle(j job) {
	recs, err := decoder.Unpack(j.peer, j.data)
	if err != nil {
		s.decodeFailed.Add(1)
		s.log.Warn().Err(err).Str("peer", j.peer).Msg("Failed to unmarshal MDT telemetry")
		return
	}
	s.recordsUnpacked.Add(uint64(len(recs)))
	for _, ev := range s.xf.Events(recs) {
		s.onEvent(ev)
	}
}

func (s *Server) MdtDialout(stream mdtdialout.GRPCMdtDialout_MdtDialoutServer) error {
	peerAddr := "unknown"
	if p, ok := peer.FromContext(stream.Context()); ok && p.Addr != nil {
		peerAddr = p.Addr.String()
	}
	s.streamsOpened.Add(1)
	s.streamsActive.Add(1)
	defer func() {
		s.streamsActive.Add(-1)
		s.streamsClosed.Add(1)
	}()
	s.log.Debug().Str("peer", peerAddr).Msg("MDT dial-out connected")

	for {
		packet, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				s.log.Debug().Err(err).Str("peer", peerAddr).Msg("MDT dial-out stream closed")
			}
			return nil
		}
		if len(packet.GetData()) == 0 {
			if packet.GetErrors() != "" {
				s.packetsErrorPayload.Add(1)
				s.log.Warn().Str("peer", peerAddr).Str("errors", packet.GetErrors()).Msg("MDT dial-out error payload")
			} else {
				s.packetsEmpty.Add(1)
			}
			continue
		}
		s.packetsReceived.Add(1)
		s.lastMu.Lock()
		s.lastPacketAt = time.Now()
		s.lastMu.Unlock()
		data := append([]byte(nil), packet.GetData()...)
		item := job{peer: peerAddr, data: data}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case s.queue <- item:
			s.noteQueued()
		default:
			s.queueBlocked.Add(1)
			select {
			case <-stream.Context().Done():
				return stream.Context().Err()
			case s.queue <- item:
				s.noteQueued()
			}
		}
	}
}
