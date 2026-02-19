package collector

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultDialTimeout   = 10 * time.Second
	defaultBackoffMin    = 2 * time.Second
	defaultBackoffMax    = 120 * time.Second
	defaultUpdatesBuffer = 256
)

// Collector manages gNMI subscriptions to network devices
type Collector struct {
	address    string
	username   string
	password   string
	port       int
	client     gnmi.GNMI_SubscribeClient
	conn       *grpc.ClientConn
	logger     zerolog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	updateChan chan *gnmi.Notification
	errors     chan error
	backoff    Backoff
	dialTimeout time.Duration
	mu         sync.RWMutex
	health     DeviceHealth
	tlsConfig  *TLSConfig
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	Enabled            bool
	InsecureSkipVerify bool
	ServerName         string
	CAFile             string
	CertFile           string
	KeyFile            string
}

// Backoff holds backoff configuration
type Backoff struct {
	Min time.Duration
	Max time.Duration
}

// DeviceHealth tracks connection state for a device
type DeviceHealth struct {
	Connected      bool
	LastUpdate     time.Time
	LastError      string
	ReconnectCount int
	UpdateCount    int64
	SyncReceived   bool
	LastPath       string
	LastValue      string
	ConnectedSince time.Time
}

// NewCollector creates a new gNMI collector
func NewCollector(address string, username string, password string, port int, logger zerolog.Logger) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	return &Collector{
		address:     address,
		username:    username,
		password:    password,
		port:        port,
		logger:      logger,
		ctx:         ctx,
		cancel:      cancel,
		updateChan:  make(chan *gnmi.Notification, defaultUpdatesBuffer),
		errors:      make(chan error, 1),
		backoff:     Backoff{Min: defaultBackoffMin, Max: defaultBackoffMax},
		dialTimeout: defaultDialTimeout,
		health:      DeviceHealth{Connected: false},
	}
}

// SetTLSConfig sets TLS configuration for the collector
func (c *Collector) SetTLSConfig(cfg *TLSConfig) {
	c.tlsConfig = cfg
}

// Errors returns the error channel
func (c *Collector) Errors() <-chan error {
	return c.errors
}

// Health returns the current health status
func (c *Collector) Health() DeviceHealth {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

// Connect establishes a gNMI connection to the device with retry logic
func (c *Collector) Connect() error {
	// Close any existing connection before reconnecting to prevent
	// stale gRPC sessions from accumulating on the switch
	c.closeExisting()

	attempt := 0
	for {
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}

		err := c.connectOnce()
		if err == nil {
			c.mu.Lock()
			c.health.Connected = true
			c.health.LastError = ""
			c.health.SyncReceived = false
			c.health.ConnectedSince = time.Now()
			c.mu.Unlock()
			return nil
		}

		attempt++
		backoff := c.backoffDuration(attempt)
		c.mu.Lock()
		c.health.Connected = false
		c.health.LastError = err.Error()
		c.health.ReconnectCount++
		c.mu.Unlock()

		c.logger.Warn().
			Err(err).
			Dur("backoff", backoff).
			Int("attempt", attempt).
			Msg("gNMI connection failed, retrying")

		select {
		case <-time.After(backoff):
			continue
		case <-c.ctx.Done():
			return c.ctx.Err()
		}
	}
}

// closeExisting tears down any existing gRPC connection and subscription
// to prevent stale sessions from accumulating on the switch
func (c *Collector) closeExisting() {
	if c.client != nil {
		c.client.CloseSend()
		c.client = nil
	}
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// connectOnce attempts a single connection
func (c *Collector) connectOnce() error {
	addr := fmt.Sprintf("%s:%d", c.address, c.port)

	c.logger.Info().
		Str("address", addr).
		Msg("Connecting to gNMI device")

	dialCtx, dialCancel := context.WithTimeout(c.ctx, c.dialTimeout)
	defer dialCancel()

	opts, err := c.dialOptions()
	if err != nil {
		return fmt.Errorf("dial options: %w", err)
	}

	// WithBlock ensures the connection is fully established before returning.
	// Without it, DialContext returns immediately and the deferred context
	// cancellation tears down the in-progress connection.
	conn, err := grpc.DialContext(dialCtx, addr, append(opts, grpc.WithBlock())...)
	if err != nil {
		return fmt.Errorf("failed to dial gNMI server: %w", err)
	}

	c.conn = conn
	client := gnmi.NewGNMIClient(conn)

	// Create subscribe client
	subClient, err := client.Subscribe(c.ctx)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create subscribe client: %w", err)
	}

	c.client = subClient

	// Start subscription
	if err := c.startSubscription(); err != nil {
		subClient.CloseSend()
		conn.Close()
		return fmt.Errorf("failed to start subscription: %w", err)
	}

	// Start receiver goroutine
	go c.receiveUpdates()

	c.logger.Info().Msg("gNMI connection established")
	return nil
}

// dialOptions builds gRPC dial options
func (c *Collector) dialOptions() ([]grpc.DialOption, error) {
	creds, err := c.transportCredentials()
	if err != nil {
		return nil, err
	}
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}
	
	// Add PerRPCCredentials for basic auth if username/password are provided
	// This matches gnmic's behavior: --insecure --username --password
	if c.username != "" || c.password != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(&basicAuth{username: c.username, password: c.password}))
	}
	
	return opts, nil
}

// transportCredentials returns appropriate transport credentials
func (c *Collector) transportCredentials() (credentials.TransportCredentials, error) {
	if c.tlsConfig == nil || !c.tlsConfig.Enabled {
		return insecure.NewCredentials(), nil
	}

	certPool, err := loadCertPool(c.tlsConfig.CAFile)
	if err != nil {
		return nil, err
	}
	certs, err := loadClientCert(c.tlsConfig.CertFile, c.tlsConfig.KeyFile)
	if err != nil {
		return nil, err
	}
	tlsCfg := &tls.Config{
		RootCAs:            certPool,
		Certificates:       certs,
		ServerName:         c.tlsConfig.ServerName,
		InsecureSkipVerify: c.tlsConfig.InsecureSkipVerify,
	}
	return credentials.NewTLS(tlsCfg), nil
}

// loadCertPool loads CA certificates
func loadCertPool(caFile string) (*x509.CertPool, error) {
	if caFile == "" {
		return x509.NewCertPool(), nil
	}
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read ca file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("invalid ca certs")
	}
	return pool, nil
}

// loadClientCert loads client certificate and key
func loadClientCert(certFile, keyFile string) ([]tls.Certificate, error) {
	if certFile == "" && keyFile == "" {
		return nil, nil
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}
	return []tls.Certificate{cert}, nil
}

// basicAuth implements gRPC PerRPCCredentials for basic auth
type basicAuth struct {
	username string
	password string
}

func (b *basicAuth) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	if b.username == "" && b.password == "" {
		return nil, nil
	}
	// Use HTTP Basic Authentication format: "Basic <base64(username:password)>"
	// This matches how gnmic sends credentials
	auth := b.username + ":" + b.password
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	return map[string]string{
		"authorization": "Basic " + encoded,
	}, nil
}

func (b *basicAuth) RequireTransportSecurity() bool {
	return false
}

// backoffDuration calculates exponential backoff with jitter
func (c *Collector) backoffDuration(attempt int) time.Duration {
	if attempt <= 0 {
		return c.backoff.Min
	}
	backoff := c.backoff.Min << attempt
	if backoff > c.backoff.Max {
		backoff = c.backoff.Max
	}
	jitter := time.Duration(rand.Int63n(int64(c.backoff.Min)))
	return backoff + jitter
}

// startSubscription sets up the gNMI subscription
func (c *Collector) startSubscription() error {
	// Subscribe to interface state container using SAMPLE mode.
	// IOS-XE does not support ON_CHANGE for interface state leaves,
	// and does not support subscribing to individual leaves like oper-status.
	// Subscribe to the /state container and filter updates in the handler.
	subscriptions := []*gnmi.Subscription{
		{
			Path: &gnmi.Path{
				Elem: []*gnmi.PathElem{
					{Name: "interfaces"},
					{Name: "interface", Key: map[string]string{"name": "*"}},
					{Name: "state"},
				},
			},
			Mode:           gnmi.SubscriptionMode_SAMPLE,
			SampleInterval: 10000000000, // 10 seconds in nanoseconds
		},
	}

	req := &gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{
				Subscription: subscriptions,
				Mode:         gnmi.SubscriptionList_STREAM,
				UpdatesOnly:  false, // IOS-XE does not support updates_only
			},
		},
	}

	return c.client.Send(req)
}

// receiveUpdates receives updates from the gNMI stream
func (c *Collector) receiveUpdates() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			resp, err := c.client.Recv()
			if err != nil {
				c.emitError(fmt.Errorf("receive update: %w", err))
				// Connection lost, will be retried by Connect()
				return
			}

			switch v := resp.Response.(type) {
			case *gnmi.SubscribeResponse_Update:
				c.handleNotification(v.Update)
			case *gnmi.SubscribeResponse_Error:
				c.emitError(fmt.Errorf("subscribe error: %s", v.Error.Message))
				return
			case *gnmi.SubscribeResponse_SyncResponse:
				c.logger.Info().Msg("gNMI subscription sync complete — stream is active")
				c.mu.Lock()
				c.health.LastUpdate = time.Now()
				c.health.SyncReceived = true
				c.mu.Unlock()
			}
		}
	}
}

// handleNotification processes a gNMI notification
func (c *Collector) handleNotification(notif *gnmi.Notification) {
	if notif == nil {
		return
	}
	ts := time.Unix(0, notif.Timestamp)
	if notif.Timestamp == 0 {
		ts = time.Now()
	}

	// Build path and value strings for debug logging and health tracking
	var lastPath, lastValue string
	for _, update := range notif.Update {
		fullPath := ""
		if notif.Prefix != nil {
			fullPath = pathToString(notif.Prefix)
		}
		if update.Path != nil {
			fullPath += pathToString(update.Path)
		}
		val := typedValueToString(update.Val)
		lastPath = fullPath
		lastValue = val

		c.logger.Debug().
			Str("path", fullPath).
			Str("value", val).
			Time("timestamp", ts).
			Msg("gNMI update received")
	}

	c.mu.Lock()
	c.health.LastUpdate = ts
	c.health.UpdateCount++
	if lastPath != "" {
		c.health.LastPath = lastPath
		c.health.LastValue = lastValue
	}
	c.mu.Unlock()

	select {
	case c.updateChan <- notif:
	default:
		c.logger.Warn().Msg("Update channel full, dropping notification")
	}
}

// emitError sends an error to the error channel
func (c *Collector) emitError(err error) {
	select {
	case c.errors <- err:
	default:
		// Error channel full, drop it
	}
}

// Updates returns the channel for receiving telemetry updates
func (c *Collector) Updates() <-chan *gnmi.Notification {
	return c.updateChan
}

// Done returns a channel that is closed when the collector is shut down.
// Goroutines should select on this to exit when Close() is called.
func (c *Collector) Done() <-chan struct{} {
	return c.ctx.Done()
}

// TestConnection performs a one-shot gNMI Capabilities request to verify
// the device is reachable and responding. Returns the supported models count
// and any error encountered.
func (c *Collector) TestConnection() (int, string, error) {
	addr := fmt.Sprintf("%s:%d", c.address, c.port)

	dialCtx, dialCancel := context.WithTimeout(context.Background(), c.dialTimeout)
	defer dialCancel()

	opts, err := c.dialOptions()
	if err != nil {
		return 0, "", fmt.Errorf("dial options: %w", err)
	}

	conn, err := grpc.DialContext(dialCtx, addr, opts...)
	if err != nil {
		return 0, "", fmt.Errorf("failed to dial: %w", err)
	}
	defer conn.Close()

	client := gnmi.NewGNMIClient(conn)

	capCtx, capCancel := context.WithTimeout(context.Background(), c.dialTimeout)
	defer capCancel()

	resp, err := client.Capabilities(capCtx, &gnmi.CapabilityRequest{})
	if err != nil {
		return 0, "", fmt.Errorf("capabilities request failed: %w", err)
	}

	version := resp.GetGNMIVersion()
	modelCount := len(resp.GetSupportedModels())

	c.logger.Info().
		Int("models", modelCount).
		Str("gnmi_version", version).
		Msg("Connection test successful")

	return modelCount, version, nil
}

// Close closes the gNMI connection
func (c *Collector) Close() error {
	c.cancel()
	if c.client != nil {
		c.client.CloseSend()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// parsePath parses a string path into a gNMI Path
func parsePath(path string) (*gnmi.Path, error) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil, fmt.Errorf("path is empty")
	}
	parts := strings.Split(trimmed, "/")
	elems := make([]*gnmi.PathElem, 0, len(parts))
	for _, part := range parts {
		name, keys, err := parsePathElem(part)
		if err != nil {
			return nil, err
		}
		elems = append(elems, &gnmi.PathElem{Name: name, Key: keys})
	}
	return &gnmi.Path{Elem: elems}, nil
}

// parsePathElem parses a path element with optional keys
func parsePathElem(segment string) (string, map[string]string, error) {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return "", nil, fmt.Errorf("path segment empty")
	}
	name := segment
	keys := map[string]string{}
	for {
		open := strings.Index(name, "[")
		if open == -1 {
			break
		}
		close := strings.Index(name[open:], "]")
		if close == -1 {
			return "", nil, fmt.Errorf("invalid key selector in %s", segment)
		}
		close += open
		selector := name[open+1 : close]
		name = name[:open] + name[close+1:]
		kv := strings.SplitN(selector, "=", 2)
		if len(kv) != 2 {
			return "", nil, fmt.Errorf("invalid key selector %s", selector)
		}
		keys[kv[0]] = kv[1]
	}
	if len(keys) == 0 {
		keys = nil
	}
	return name, keys, nil
}

// pathToString converts a gNMI Path to string representation
func pathToString(path *gnmi.Path) string {
	if path == nil {
		return ""
	}
	var b strings.Builder
	for _, elem := range path.Elem {
		b.WriteString("/")
		b.WriteString(elem.Name)
		if len(elem.Key) > 0 {
			keys := make([]string, 0, len(elem.Key))
			for k := range elem.Key {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				b.WriteString("[")
				b.WriteString(k)
				b.WriteString("=")
				b.WriteString(elem.Key[k])
				b.WriteString("]")
			}
		}
	}
	return b.String()
}

// typedValueToString extracts string value from gNMI TypedValue
func typedValueToString(value *gnmi.TypedValue) string {
	if value == nil {
		return ""
	}
	switch v := value.Value.(type) {
	case *gnmi.TypedValue_StringVal:
		return v.StringVal
	case *gnmi.TypedValue_IntVal:
		return fmt.Sprintf("%d", v.IntVal)
	case *gnmi.TypedValue_UintVal:
		return fmt.Sprintf("%d", v.UintVal)
	case *gnmi.TypedValue_BoolVal:
		return fmt.Sprintf("%t", v.BoolVal)
	case *gnmi.TypedValue_DoubleVal:
		return fmt.Sprintf("%f", v.DoubleVal)
	case *gnmi.TypedValue_FloatVal:
		return fmt.Sprintf("%f", v.FloatVal)
	case *gnmi.TypedValue_DecimalVal:
		return fmt.Sprintf("%d", v.DecimalVal.Digits)
	case *gnmi.TypedValue_JsonVal:
		return string(v.JsonVal)
	case *gnmi.TypedValue_JsonIetfVal:
		return string(v.JsonIetfVal)
	case *gnmi.TypedValue_AsciiVal:
		return v.AsciiVal
	case *gnmi.TypedValue_BytesVal:
		return string(v.BytesVal)
	default:
		return ""
	}
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
