package api

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/forestgiant/netutil"
	"gitlab.fg/go/stela"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type Client struct {
	stela.Client

	mu sync.RWMutex
	// ctx       context.Context
	rpc       stela.StelaClient
	conn      *grpc.ClientConn
	Hostname  string
	callbacks map[string]func(s *stela.Service)
}

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
	c.rpc = stela.NewStelaClient(c.conn)

	// Add the Client
	resp, err := c.rpc.AddClient(ctx, &stela.AddClientRequest{ClientAddress: c.Address})
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
	stream, err := c.rpc.Connect(ctx, &stela.ConnectRequest{ClientId: c.ID})
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
					Target:   rs.Hostname,
					Address:  rs.Address,
					Port:     rs.Port,
					Priority: rs.Priority,
					Action:   rs.Action,
					Value:    stela.DecodeValue(rs.Value),
				})
				c.mu.RUnlock()
			}
		}
	}()

	return nil
}

func (c *Client) Subscribe(ctx context.Context, serviceName string, callback func(s *stela.Service)) error {
	_, err := c.rpc.Subscribe(ctx,
		&stela.SubscribeRequest{
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

func (c *Client) Unsubscribe(ctx context.Context, serviceName string) error {
	if c.callbacks == nil {
		return errors.New("Client hasn't subscribed")
	}

	_, err := c.rpc.Unsubscribe(ctx,
		&stela.SubscribeRequest{
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

func (c *Client) RegisterService(ctx context.Context, s *stela.Service) error {
	s.Target = c.Hostname
	s.Address = c.Address
	_, err := c.rpc.Register(ctx,
		&stela.RegisterRequest{
			ClientId: c.ID,
			Service: &stela.ServiceMessage{
				Name:     s.Name,
				Hostname: s.Target,
				Address:  s.Address,
				Port:     s.Port,
				Priority: s.Priority,
				Value:    stela.EncodeValue(s.Value),
			},
		})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) DeregisterService(ctx context.Context, s *stela.Service) error {
	s.Address = c.Address
	s.Target = c.Hostname
	_, err := c.rpc.Deregister(ctx,
		&stela.RegisterRequest{
			ClientId: c.ID,
			Service: &stela.ServiceMessage{
				Name:     s.Name,
				Hostname: s.Target,
				Address:  s.Address,
				Port:     s.Port,
				Priority: s.Priority,
			},
		})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) Discover(ctx context.Context, serviceName string) ([]*stela.Service, error) {
	resp, err := c.rpc.PeerDiscover(ctx, &stela.DiscoverRequest{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}

	// Convert response to stela services
	var services []*stela.Service
	for _, ds := range resp.Services {
		s := &stela.Service{
			Name:     ds.Name,
			Target:   ds.Hostname,
			Address:  ds.Address,
			Port:     ds.Port,
			Priority: ds.Priority,
			Value:    stela.DecodeValue(ds.Value),
		}
		services = append(services, s)
	}

	return services, nil
}

func (c *Client) DiscoverOne(ctx context.Context, serviceName string) (*stela.Service, error) {
	resp, err := c.rpc.PeerDiscoverOne(ctx, &stela.DiscoverRequest{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}

	// Convert response to stela services
	return &stela.Service{
		Name:     resp.Name,
		Target:   resp.Hostname,
		Address:  resp.Address,
		Port:     resp.Port,
		Priority: resp.Priority,
		Value:    stela.DecodeValue(resp.Value),
	}, nil
}

func (c *Client) DiscoverAll(ctx context.Context) ([]*stela.Service, error) {
	resp, err := c.rpc.PeerDiscoverAll(ctx, &stela.DiscoverAllRequest{})
	if err != nil {
		return nil, err
	}

	// Convert response to stela services
	var services []*stela.Service
	for _, ds := range resp.Services {
		s := &stela.Service{
			Name:     ds.Name,
			Target:   ds.Hostname,
			Address:  ds.Address,
			Port:     ds.Port,
			Priority: ds.Priority,
			Value:    stela.DecodeValue(ds.Value),
		}
		services = append(services, s)
	}

	return services, nil
}

func (c *Client) Close() {

	c.conn.Close()
}
