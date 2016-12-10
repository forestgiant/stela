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

	mu        sync.RWMutex
	rpc       stela.StelaClient
	conn      *grpc.ClientConn
	Hostname  string
	callbacks map[string]func(s *stela.Service)
}

func NewClient(stelaAddress string, caFile string) (*Client, error) {
	c := &Client{}
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
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
	resp, err := c.rpc.AddClient(context.Background(), &stela.AddClientRequest{ClientAddress: c.Address})
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
	stream, err := c.rpc.Connect(context.Background(), &stela.ConnectRequest{ClientId: c.ID})
	if err != nil {
		return err
	}

	go func() {
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
				})
				c.mu.RUnlock()
			}
		}
	}()

	return nil
}

func (c *Client) Subscribe(serviceName string, callback func(s *stela.Service)) error {
	_, err := c.rpc.Subscribe(context.Background(),
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

func (c *Client) Unsubscribe(serviceName string) error {
	if c.callbacks == nil {
		return errors.New("Client hasn't subscribed")
	}

	_, err := c.rpc.Unsubscribe(context.Background(),
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

func (c *Client) RegisterService(s *stela.Service) error {
	s.Target = c.Hostname
	_, err := c.rpc.Register(context.Background(),
		&stela.RegisterRequest{
			ClientId: c.ID,
			Name:     s.Name,
			Hostname: s.Target,
			Address:  s.Address,
			Port:     s.Port,
			Priority: s.Priority,
		})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) DeregisterService(s *stela.Service) error {
	s.Address = c.Address
	s.Target = c.Hostname
	_, err := c.rpc.Deregister(context.Background(),
		&stela.RegisterRequest{
			ClientId: c.ID,
			Name:     s.Name,
			Hostname: s.Target,
			Address:  s.Address,
			Port:     s.Port,
			Priority: s.Priority,
		})
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) Discover(serviceName string) ([]*stela.Service, error) {
	resp, err := c.rpc.PeerDiscover(context.Background(), &stela.DiscoverRequest{ServiceName: serviceName})
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
		}
		services = append(services, s)
	}

	return services, nil
}

func (c *Client) DiscoverOne(serviceName string) (*stela.Service, error) {
	resp, err := c.rpc.PeerDiscoverOne(context.Background(), &stela.DiscoverRequest{ServiceName: serviceName})
	if err != nil {
		return nil, err
	}

	// Convert response to stela services
	return &stela.Service{
		Name:     resp.Name,
		Target:   resp.Hostname,
		Address:  resp.Address,
		Port:     resp.Port,
		Priority: resp.Priority}, nil
}

func (c *Client) DiscoverAll() ([]*stela.Service, error) {
	resp, err := c.rpc.PeerDiscoverAll(context.Background(), &stela.DiscoverAllRequest{})
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
		}
		services = append(services, s)
	}

	return services, nil
}

func (c *Client) Close() {
	c.conn.Close()
}
