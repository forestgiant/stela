package transport

import (
	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/store"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// Server implements the stela.proto service
type Server struct {
	Store store.Store
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

	// Send service to clients on interval for simiulation
	for {
		select {
		case s := <-c.SubscribeCh():
			response := &stela.ServiceResponse{
				Name:     s.Name,
				Hostname: s.Target,
				Address:  s.Address,
				Port:     s.Port,
				Priority: s.Priority,
			}

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
	s.Store.Register(service)

	return &stela.RegisterResponse{}, nil
}

// Discover all services registered under a service name. Ex. "test.services.fg"
func (s *Server) Discover(ctx context.Context, req *stela.DiscoverRequest) (*stela.DiscoverResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "Currently Unimplemented")
}

// DiscoverOne service registered under a service name.
func (s *Server) DiscoverOne(ctx context.Context, req *stela.DiscoverRequest) (*stela.ServiceResponse, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "Currently Unimplemented")
}

// Services returns all services registered with stela even other clients TODO
func (s *Server) Services(ctx context.Context) {

}
