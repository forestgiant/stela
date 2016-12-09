package transport

import (
	"fmt"

	"sync"

	"gitlab.fg/go/disco/node"
	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/store"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// Server implements the stela.proto service
type Server struct {
	mu    sync.Mutex
	Store store.Store
	peers []*node.Node
}

// AddClient adds a client to the store and returns it's id
func (s *Server) AddClient(ctx context.Context, req *stela.AddClientRequest) (*stela.AddClientResponse, error) {
	// Add client to the store
	c := &stela.Client{}
	c.Address = req.ClientAddress
	if err := s.Store.AddClient(c); err != nil {
		return nil, err
	}

	return &stela.AddClientResponse{
		ClientId: c.ID,
	}, nil
}

// Connect a stela client to a stream of possible subscriptions. Uses the client id in to keep track
// of services and subscriptions it registers
func (s *Server) Connect(req *stela.ConnectRequest, stream stela.Stela_ConnectServer) error {
	ctx := stream.Context()

	// Look up client
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return err
	}

	fmt.Println("connect!", c)

	// Send service to clients on interval for simiulation
	for {
		select {
		case rs := <-c.SubscribeCh():
			response := &stela.ServiceResponse{
				Name:     rs.Name,
				Hostname: rs.Target,
				Address:  rs.Address,
				Port:     rs.Port,
				Priority: rs.Priority,
			}

			fmt.Println("send", rs)

			if err := stream.Send(response); err != nil {
				return err
			}
		case <-ctx.Done():
			// Remove client from store to cleanup
			s.Store.RemoveClient(c)
			return nil
		}
	}
}

// Subscribe a client to a service name.
func (s *Server) Subscribe(ctx context.Context, req *stela.SubscribeRequest) (*stela.SubscribeResponse, error) {
	// Look up client that sent the request
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return nil, err
	}

	// Subscribe the client to the serviceName
	s.Store.Subscribe(req.ServiceName, c)

	return &stela.SubscribeResponse{}, nil
}

// Register a service to a client
func (s *Server) Register(ctx context.Context, req *stela.RegisterRequest) (*stela.RegisterResponse, error) {
	// Look up client that sent the request
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return nil, err
	}

	// Convert req to service
	service := &stela.Service{
		Name:     req.Name,
		Target:   req.Hostname,
		Address:  req.Address,
		Port:     req.Port,
		Priority: req.Priority,
		Action:   stela.RegisterAction,
		Client:   c,
	}

	// Register service to store
	if err := s.Store.Register(service); err != nil {
		return nil, err
	}

	return &stela.RegisterResponse{}, nil
}

// Deregister a service
func (s *Server) Deregister(ctx context.Context, req *stela.RegisterRequest) (*stela.RegisterResponse, error) {
	// Look up client that sent the request
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return nil, err
	}

	// Convert req to service
	service := &stela.Service{
		Name:     req.Name,
		Target:   req.Hostname,
		Address:  req.Address,
		Port:     req.Port,
		Priority: req.Priority,
		Action:   stela.RegisterAction,
		Client:   c,
	}

	// Register service to store
	s.Store.Deregister(service)

	return &stela.RegisterResponse{}, nil
}

// Discover all services registered under a service name. Ex. "test.services.fg"
func (s *Server) Discover(ctx context.Context, req *stela.DiscoverRequest) (*stela.DiscoverResponse, error) {
	services, err := s.Store.Discover(req.ServiceName)
	if err != nil {
		return nil, err
	}

	// Convert stela.Service struct to stela.ServiceResponse
	var srs []*stela.ServiceResponse
	for _, ds := range services {
		sr := &stela.ServiceResponse{
			Name:     ds.Name,
			Hostname: ds.Target,
			Address:  ds.Address,
			Port:     ds.Port,
			Priority: ds.Priority,
		}
		srs = append(srs, sr)
	}

	return &stela.DiscoverResponse{Services: srs}, nil
}

// DiscoverOne service registered under a service name.
func (s *Server) DiscoverOne(ctx context.Context, req *stela.DiscoverRequest) (*stela.ServiceResponse, error) {
	service, err := s.Store.DiscoverOne(req.ServiceName)
	if err != nil {
		return nil, err
	}

	// Convert stela.Service struct to stela.ServiceResponse
	return &stela.ServiceResponse{
		Name:     service.Name,
		Hostname: service.Target,
		Address:  service.Address,
		Port:     service.Port,
		Priority: service.Priority,
	}, nil
}

// DiscoverAll returns all services registered with stela even other clients TODO
func (s *Server) DiscoverAll(ctx context.Context, req *stela.DiscoverAllRequest) (*stela.DiscoverResponse, error) {
	services := s.Store.DiscoverAll()

	// Convert stela.Service struct to stela.ServiceResponse
	var srs []*stela.ServiceResponse
	for _, ds := range services {
		sr := &stela.ServiceResponse{
			Name:     ds.Name,
			Hostname: ds.Target,
			Address:  ds.Address,
			Port:     ds.Port,
			Priority: ds.Priority,
		}
		srs = append(srs, sr)
	}

	return &stela.DiscoverResponse{Services: srs}, nil
}

// PeerDiscover all services registered under a service name. Ex. "test.services.fg"
func (s *Server) PeerDiscover(ctx context.Context, req *stela.DiscoverRequest) (*stela.DiscoverResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "Currently Unimplemented")
}

// PeerDiscoverOne service registered under a service name.
func (s *Server) PeerDiscoverOne(ctx context.Context, req *stela.DiscoverRequest) (*stela.ServiceResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "Currently Unimplemented")
}

// PeerDiscoverAll returns all services registered with any stela member peer
func (s *Server) PeerDiscoverAll(ctx context.Context, req *stela.DiscoverAllRequest) (*stela.DiscoverResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "Currently Unimplemented")
}

// SetPeers
func (s *Server) SetPeers(peers []*node.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers = peers
}
