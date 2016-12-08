package api

import (
	"os"

	"github.com/forestgiant/netutil"
	"gitlab.fg/go/stela"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type Client struct {
	stela.Client
	rpc      stela.StelaClient
	conn     *grpc.ClientConn
	Hostname string
}

func NewClient(caFile string) (*Client, error) {
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
	c.conn, err = grpc.Dial(stela.DefaultStelaAddress, opts...)
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

	return c, nil
}

func (c *Client) RegisterService(s *stela.Service) error {
	s.Address = c.Address
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
	resp, err := c.rpc.Discover(context.Background(), &stela.DiscoverRequest{ServiceName: serviceName})
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
