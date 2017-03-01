package api

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/forestgiant/netutil"
	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/pb"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// Client struct represents a connection client connection to a stela instance
type Client struct {
	stela.Client

	mu sync.RWMutex
	// ctx       context.Context
	rpc       pb.StelaClient
	conn      *grpc.ClientConn
	Hostname  string
	callbacks map[string]func(s *stela.Service)
}

// NewClient returns a *Client struct
func NewClient(ctx context.Context, stelaAddress string, caFile string) (*Client, error) {
	c := &Client{}
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	// c.ctx = ctx
	c.Hostname = host
	c.Address = netutil.LocalIPv4().String()

	var opts []grpc.DialOption
	// creds, err := credentials.NewClientTLSFromFile(caFile, "")
	// if err != nil {
	// 	return nil, err
	// }

	// opts = append(opts, grpc.WithTransportCredentials(creds))
	opts = append(opts, grpc.WithInsecure())
	c.conn, err = grpc.Dial(stelaAddress, opts...)
	if err != nil {
		return nil, err
	}
	c.rpc = pb.NewStelaClient(c.conn)

	// Add the Client
	resp, err := c.rpc.AddClient(ctx, &pb.AddClientRequest{ClientAddress: c.Address})
	if err != nil {
		return nil, err
	}
	c.ID = resp.ClientId

	// Connect the client to the server
	if err := c.connect(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Client) init() {
	if c.callbacks == nil {
		c.callbacks = make(map[string]func(s *stela.Service))
	}
}

func (c *Client) connect() error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	stream, err := c.rpc.Connect(ctx, &pb.ConnectRequest{ClientId: c.ID})
	if err != nil {
		return err
	}

	go func() {
		defer c.conn.Close()
		defer cancelFunc()
		for {
			rs, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return
			}

			// Send service to callback if any subscribed
			if c.callbacks != nil {
				c.mu.RLock()
				c.callbacks[rs.Name](&stela.Service{
					Name:     rs.Name,
					Hostname: rs.Hostname,
					IPv4:     rs.IPv4,
					IPv6:     rs.IPv6,
					Port:     rs.Port,
					Priority: rs.Priority,
					Action:   rs.Action,
					Value:    rs.Value,
				})
				c.mu.RUnlock()
			}
		}
	}()

	return nil
}

// Subscribe stores a callback to the service name and notifies the stela instance to notify your client on changes.
func (c *Client) Subscribe(ctx context.Context, serviceName string, callback func(s *stela.Service)) error {
	_, err := c.rpc.Subscribe(ctx,
		&pb.SubscribeRequest{
			ClientId:    c.ID,
			ServiceName: serviceName,
		})
	if err != nil {
		return err
	}

	c.init()

	// Store callback
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks[serviceName] = callback

	return nil
}

// Unsubscribe removes the callback to the service name and let's the stela instance know the client
// doesn't want updates on that serviceName.
func (c *Client) Unsubscribe(ctx context.Context, serviceName string) error {
	if c.callbacks == nil {
		return errors.New("Client hasn't subscribed")
	}

	_, err := c.rpc.Unsubscribe(ctx,
		&pb.SubscribeRequest{
			ClientId:    c.ID,
			ServiceName: serviceName,
		})
	if err != nil {
		return err
	}

	// Delete callback
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.callbacks, serviceName)

	return nil
}

// Register registers a service to the stela instance the client is connected to.
func (c *Client) Register(ctx context.Context, s *stela.Service) error {
	s.Hostname = c.Hostname
	if s.IPv4 == "" {
		s.IPv4 = c.Address
	}
	_, err := c.rpc.Register(ctx,
		&pb.RegisterRequest{
			ClientId: c.ID,
			Service: &pb.ServiceMessage{
				Name:     s.Name,
				Hostname: s.Hostname,
				IPv4:     s.IPv4,
				IPv6:     s.IPv6,
				Port:     s.Port,
				Priority: s.Priority,
				Value:    s.Value,
			},
		})
	if err != nil {
		return err
	}

	return nil
}

// Deregister deregisters a service to the stela instance the client is connected to.
func (c *Client) Deregister(ctx context.Context, s *stela.Service) error {
	// s.IPv4 = c.Address
	s.Hostname = c.Hostname
	_, err := c.rpc.Deregister(ctx,
		&pb.RegisterRequest{
			ClientId: c.ID,
			Service: &pb.ServiceMessage{
				Name:     s.Name,
				Hostname: s.Hostname,
				IPv4:     s.IPv4,
				IPv6:     s.IPv6,
				Port:     s.Port,
				Priority: s.Priority,
			},
		})
	if err != nil {
		return err
	}

	return nil
}

// Discover services registered with the same service name.
func (c *Client) Discover(ctx context.Context, serviceName string) ([]*stela.Service, error) {
	resp, err := c.rpc.Discover(ctx, &pb.DiscoverRequest{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}

	// Convert response to stela services
	var services []*stela.Service
	for _, ds := range resp.Services {
		s := &stela.Service{
			Name:     ds.Name,
			Hostname: ds.Hostname,
			IPv4:     ds.IPv4,
			IPv6:     ds.IPv6,
			Port:     ds.Port,
			Priority: ds.Priority,
			Value:    ds.Value,
		}
		services = append(services, s)
	}

	return services, nil
}

// DiscoverRegex finds services by name based on a regular expression.
func (c *Client) DiscoverRegex(ctx context.Context, reg string) ([]*stela.Service, error) {
	resp, err := c.rpc.DiscoverRegex(ctx, &pb.DiscoverRequest{ServiceName: reg})
	if err != nil {
		return nil, err
	}

	// Convert response to stela services
	var services []*stela.Service
	for _, ds := range resp.Services {
		s := &stela.Service{
			Name:     ds.Name,
			Hostname: ds.Hostname,
			IPv4:     ds.IPv4,
			IPv6:     ds.IPv6,
			Port:     ds.Port,
			Priority: ds.Priority,
			Value:    ds.Value,
		}
		services = append(services, s)
	}

	return services, nil
}

// DiscoverOne finds a single instance of a service based on name.
func (c *Client) DiscoverOne(ctx context.Context, serviceName string) (*stela.Service, error) {
	resp, err := c.rpc.DiscoverOne(ctx, &pb.DiscoverRequest{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}

	// Convert response to stela services
	return &stela.Service{
		Name:     resp.Name,
		Hostname: resp.Hostname,
		IPv4:     resp.IPv4,
		IPv6:     resp.IPv6,
		Port:     resp.Port,
		Priority: resp.Priority,
		Value:    resp.Value,
	}, nil
}

// DiscoverAll finds all services registered.
func (c *Client) DiscoverAll(ctx context.Context) ([]*stela.Service, error) {
	resp, err := c.rpc.DiscoverAll(ctx, &pb.DiscoverAllRequest{})
	if err != nil {
		return nil, err
	}

	// Convert response to stela services
	var services []*stela.Service
	for _, ds := range resp.Services {
		s := &stela.Service{
			Name:     ds.Name,
			Hostname: ds.Hostname,
			IPv4:     ds.IPv4,
			IPv6:     ds.IPv6,
			Port:     ds.Port,
			Priority: ds.Priority,
			Value:    ds.Value,
		}
		services = append(services, s)
	}

	return services, nil
}

// Close cancels the stream to the gRPC stream established by connect().
func (c *Client) Close() {

	c.conn.Close()
}
