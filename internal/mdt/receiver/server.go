package receiver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netspec/netspec/internal/collector"
	"github.com/netspec/netspec/internal/mdt/bandwidth"
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
	byDeviceStatsLimit    = 50
	byteRateWindowSec     = 10
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
	onRecords  func([]*decoder.Record)
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
	bytesReceived       atomic.Uint64

	lastMu       sync.Mutex
	lastPacketAt time.Time

	pathMu sync.Mutex
	byPath map[string]uint64
	byKind map[string]uint64

	bwMu       sync.Mutex
	groups     *bandwidth.Registry
	byDevice   map[string]bwCounters
	byGroup    map[string]bwCounters
	bytesBySec map[int64]uint64

	mu      sync.Mutex
	started bool
}

type bwCounters struct {
	Bytes   uint64
	Packets uint64
}

// PathStat is an encoding_path or kind rollup from unpacked kvGPB rows.
type PathStat struct {
	Kind         string `json:"kind"`
	EncodingPath string `json:"encoding_path,omitempty"`
	Count        uint64 `json:"count"`
}

// DeviceBandwidthStat is dial-out transport bytes attributed to a node_id.
type DeviceBandwidthStat struct {
	Device  string `json:"device"`
	Bytes   uint64 `json:"bytes_received"`
	Packets uint64 `json:"packets"`
}

// GroupBandwidthStat is dial-out transport bytes rolled up by operator group.
type GroupBandwidthStat struct {
	Group   string `json:"group"`
	Bytes   uint64 `json:"bytes_received"`
	Packets uint64 `json:"packets"`
}

// Stats is a point-in-time copy of receiver + transformer counters.
type Stats struct {
	ListenAddr             string                `json:"listen_addr"`
	QueueLen               int                   `json:"queue_len"`
	QueueCap               int                   `json:"queue_cap"`
	QueueHighWater         uint64                `json:"queue_high_water"`
	QueueBlocked           uint64                `json:"queue_blocked"`
	PacketsReceived        uint64                `json:"packets_received"`
	PacketsEmpty           uint64                `json:"packets_empty"`
	PacketsErrorPayload    uint64                `json:"packets_error_payload"`
	DecodeFailed           uint64                `json:"decode_failed"`
	RecordsUnpacked        uint64                `json:"records_unpacked"`
	StreamsActive          int64                 `json:"streams_active"`
	StreamsOpened          uint64                `json:"streams_opened"`
	StreamsClosed          uint64                `json:"streams_closed"`
	BytesReceived          uint64                `json:"bytes_received_total"`
	BytesReceivedPerSecond float64               `json:"bytes_received_per_second"`
	ByDevice               []DeviceBandwidthStat `json:"by_device,omitempty"`
	ByGroup                []GroupBandwidthStat  `json:"by_group,omitempty"`
	LastPacketAt           time.Time             `json:"last_packet_at,omitempty"`
	ByKind                 []PathStat            `json:"by_kind,omitempty"`
	ByEncodingPath         []PathStat            `json:"by_encoding_path,omitempty"`
	Transformer            transformer.Snapshot  `json:"transformer"`
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
		cfg:        cfg,
		log:        log,
		xf:         xf,
		onEvent:    onEvent,
		queue:      make(chan job, cfg.QueueSize),
		byPath:     map[string]uint64{},
		byKind:     map[string]uint64{},
		byDevice:   map[string]bwCounters{},
		byGroup:    map[string]bwCounters{},
		bytesBySec: map[int64]uint64{},
	}
}

// SetOnRecords installs an optional hook that receives every unpacked kvGPB
// row set (Graph VM mapper). Nil is a no-op. Safe to call before Start.
func (s *Server) SetOnRecords(fn func([]*decoder.Record)) {
	if s == nil {
		return
	}
	s.onRecords = fn
}

// SetBandwidthGroups installs the device→group registry used for by_group
// rollups. Nil treats every device as bandwidth.Ungrouped. Safe before Start.
func (s *Server) SetBandwidthGroups(r *bandwidth.Registry) {
	if s == nil {
		return
	}
	s.bwMu.Lock()
	defer s.bwMu.Unlock()
	s.groups = r
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
	s.pathMu.Lock()
	byKind := snapshotPathStats(s.byKind, false)
	byPath := snapshotPathStats(s.byPath, true)
	s.pathMu.Unlock()
	byDevice, byGroup, bytesPerSec := s.snapshotBandwidth()
	return Stats{
		ListenAddr:             addr,
		QueueLen:               qlen,
		QueueCap:               qcap,
		QueueHighWater:         s.queueHighWater.Load(),
		QueueBlocked:           s.queueBlocked.Load(),
		PacketsReceived:        s.packetsReceived.Load(),
		PacketsEmpty:           s.packetsEmpty.Load(),
		PacketsErrorPayload:    s.packetsErrorPayload.Load(),
		DecodeFailed:           s.decodeFailed.Load(),
		RecordsUnpacked:        s.recordsUnpacked.Load(),
		StreamsActive:          s.streamsActive.Load(),
		StreamsOpened:          s.streamsOpened.Load(),
		StreamsClosed:          s.streamsClosed.Load(),
		BytesReceived:          s.bytesReceived.Load(),
		BytesReceivedPerSecond: bytesPerSec,
		ByDevice:               byDevice,
		ByGroup:                byGroup,
		LastPacketAt:           last,
		ByKind:                 byKind,
		ByEncodingPath:         byPath,
		Transformer:            s.xf.Snapshot(),
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

func (s *Server) notePaths(recs []*decoder.Record) {
	if len(recs) == 0 {
		return
	}
	s.pathMu.Lock()
	defer s.pathMu.Unlock()
	for _, rec := range recs {
		path := ""
		if rec != nil {
			path = rec.EncodingPath
		}
		s.byPath[path]++
		s.byKind[decoder.ClassifyPath(path)]++
	}
}

func snapshotPathStats(m map[string]uint64, asPath bool) []PathStat {
	if len(m) == 0 {
		return nil
	}
	out := make([]PathStat, 0, len(m))
	for key, n := range m {
		ps := PathStat{Count: n}
		if asPath {
			ps.EncodingPath = key
			ps.Kind = decoder.ClassifyPath(key)
		} else {
			ps.Kind = key
		}
		out = append(out, ps)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].EncodingPath < out[j].EncodingPath
	})
	return out
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
	n := uint64(len(j.data))
	recs, err := decoder.Unpack(j.peer, j.data)
	if err != nil {
		s.decodeFailed.Add(1)
		s.noteDeviceBandwidth("unknown", n, 1)
		s.log.Warn().Err(err).Str("peer", j.peer).Msg("Failed to unmarshal MDT telemetry")
		return
	}
	s.recordsUnpacked.Add(uint64(len(recs)))
	s.notePaths(recs)
	s.noteDeviceBandwidth(nodeIDFrom(recs), n, 1)
	if s.onRecords != nil {
		s.onRecords(recs)
	}
	for _, ev := range s.xf.Events(recs) {
		s.onEvent(ev)
	}
}

func nodeIDFrom(recs []*decoder.Record) string {
	for _, rec := range recs {
		if rec == nil {
			continue
		}
		if id := strings.TrimSpace(rec.NodeID); id != "" {
			return id
		}
	}
	return "unknown"
}

func (s *Server) noteDeviceBandwidth(device string, bytes, packets uint64) {
	if s == nil || (bytes == 0 && packets == 0) {
		return
	}
	device = strings.TrimSpace(device)
	if device == "" {
		device = "unknown"
	}
	s.bwMu.Lock()
	defer s.bwMu.Unlock()
	d := s.byDevice[device]
	d.Bytes += bytes
	d.Packets += packets
	s.byDevice[device] = d
	group := bandwidth.Ungrouped
	if s.groups != nil {
		group = s.groups.GroupOf(device)
	}
	g := s.byGroup[group]
	g.Bytes += bytes
	g.Packets += packets
	s.byGroup[group] = g
}

func (s *Server) noteByteRate(n uint64) {
	if n == 0 {
		return
	}
	now := time.Now().Unix()
	s.bwMu.Lock()
	defer s.bwMu.Unlock()
	s.bytesBySec[now] += n
}

func (s *Server) snapshotBandwidth() ([]DeviceBandwidthStat, []GroupBandwidthStat, float64) {
	s.bwMu.Lock()
	defer s.bwMu.Unlock()

	nowSec := time.Now().Unix()
	var recent uint64
	for sec, n := range s.bytesBySec {
		age := nowSec - sec
		if age >= 0 && age < byteRateWindowSec {
			recent += n
		}
		if age > 1200 {
			delete(s.bytesBySec, sec)
		}
	}
	rate := float64(recent) / float64(byteRateWindowSec)

	byDevice := make([]DeviceBandwidthStat, 0, len(s.byDevice))
	for device, c := range s.byDevice {
		byDevice = append(byDevice, DeviceBandwidthStat{
			Device:  device,
			Bytes:   c.Bytes,
			Packets: c.Packets,
		})
	}
	sort.Slice(byDevice, func(i, j int) bool {
		if byDevice[i].Bytes != byDevice[j].Bytes {
			return byDevice[i].Bytes > byDevice[j].Bytes
		}
		return byDevice[i].Device < byDevice[j].Device
	})
	if len(byDevice) > byDeviceStatsLimit {
		byDevice = byDevice[:byDeviceStatsLimit]
	}

	// Ensure configured groups appear even at zero so operators see empty sites.
	if s.groups != nil {
		for _, name := range s.groups.Names() {
			if _, ok := s.byGroup[name]; !ok {
				s.byGroup[name] = bwCounters{}
			}
		}
	}
	byGroup := make([]GroupBandwidthStat, 0, len(s.byGroup))
	for group, c := range s.byGroup {
		byGroup = append(byGroup, GroupBandwidthStat{
			Group:   group,
			Bytes:   c.Bytes,
			Packets: c.Packets,
		})
	}
	sort.Slice(byGroup, func(i, j int) bool {
		if byGroup[i].Bytes != byGroup[j].Bytes {
			return byGroup[i].Bytes > byGroup[j].Bytes
		}
		return byGroup[i].Group < byGroup[j].Group
	})
	return byDevice, byGroup, rate
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
		nbytes := uint64(len(packet.GetData()))
		s.bytesReceived.Add(nbytes)
		s.noteByteRate(nbytes)
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
