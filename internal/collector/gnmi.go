package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
}

// NewCollector creates a new gNMI collector
func NewCollector(address string, username string, password string, port int, logger zerolog.Logger) *Collector {
	ctx, cancel := context.WithCancel(context.Background())
	return &Collector{
		address:    address,
		username:   username,
		password:   password,
		port:       port,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		updateChan: make(chan *gnmi.Notification, 100),
	}
}

// Connect establishes a gNMI connection to the device
func (c *Collector) Connect() error {
	addr := fmt.Sprintf("%s:%d", c.address, c.port)
	
	c.logger.Info().
		Str("address", addr).
		Msg("Connecting to gNMI device")

	// For MVP, use insecure connection (can be enhanced with TLS later)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

// startSubscription sets up the gNMI subscription
func (c *Collector) startSubscription() error {
	// Subscribe to interface state paths
	// Path: /interfaces/interface[name=*]/state/oper-status
	// Path: /interfaces/interface[name=*]/state/admin-status
	
	subscriptions := []*gnmi.Subscription{
		{
			Path: &gnmi.Path{
				Elem: []*gnmi.PathElem{
					{Name: "interfaces"},
					{Name: "interface", Key: map[string]string{"name": "*"}},
					{Name: "state"},
					{Name: "oper-status"},
				},
			},
			Mode: gnmi.SubscriptionMode_ON_CHANGE,
		},
		{
			Path: &gnmi.Path{
				Elem: []*gnmi.PathElem{
					{Name: "interfaces"},
					{Name: "interface", Key: map[string]string{"name": "*"}},
					{Name: "state"},
					{Name: "admin-status"},
				},
			},
			Mode: gnmi.SubscriptionMode_ON_CHANGE,
		},
	}

	req := &gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{
				Subscription: subscriptions,
				Mode:         gnmi.SubscriptionList_ON_CHANGE,
				UpdatesOnly:  true,
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
				c.logger.Error().
					Err(err).
					Msg("Error receiving gNMI update")
				// Attempt reconnection
				time.Sleep(5 * time.Second)
				if err := c.Connect(); err != nil {
					c.logger.Error().
						Err(err).
						Msg("Failed to reconnect")
				}
				return
			}

			switch v := resp.Response.(type) {
			case *gnmi.SubscribeResponse_Update:
				select {
				case c.updateChan <- v.Update:
				default:
					c.logger.Warn().Msg("Update channel full, dropping notification")
				}
			case *gnmi.SubscribeResponse_SyncResponse:
				c.logger.Info().Msg("gNMI subscription sync complete")
			}
		}
	}
}

// Updates returns the channel for receiving telemetry updates
func (c *Collector) Updates() <-chan *gnmi.Notification {
	return c.updateChan
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

// Health returns the health status of the collector
func (c *Collector) Health() bool {
	return c.conn != nil && c.client != nil
}
