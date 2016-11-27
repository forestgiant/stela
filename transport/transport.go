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

// Connect a stela client to a stream of possible subscriptions. Uses the client id in to keep track
// of services and subscriptions it registers
func (s *Server) Connect(cr *stela.ConnectRequest, stream stela.Stela_ConnectServer) error {
	ctx := stream.Context()

	// Add client to the store
	c := &stela.Client{}
	c.ID = cr.ClientId
	c.Address = cr.ClientAddress
	s.Store.AddClient(c)

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
	c := s.Store.Client(req.ClientId)
	if c == nil {
		return nil, grpc.Errorf(codes.Aborted, "Can't subscribe with a client that hasn't connected")
	}

	// Subscribe the client to the serviceName
	s.Store.Subscribe(req.ServiceName, c)

	return &stela.SubscribeResponse{}, nil
}

// Register a service to a client
func (s *Server) Register(ctx context.Context, req *stela.RegisterRequest) (*stela.RegisterResponse, error) {
	// Look up client that sent the request
	c := s.Store.Client(req.ClientId)
	if c == nil {
		return nil, grpc.Errorf(codes.Aborted, "Can't register with a client that hasn't connected")
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
