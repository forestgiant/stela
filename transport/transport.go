package transport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"

	"github.com/forestgiant/netutil"

	"sync"

	"time"

	"github.com/forestgiant/disco/node"
	"github.com/forestgiant/stela"
	"github.com/forestgiant/stela/pb"
	"github.com/forestgiant/stela/store"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Server implements the stela.proto service
type Server struct {
	mu      sync.Mutex
	Store   store.Store
	peers   []*node.Node
	Timeout time.Duration
	Proxy   *Proxy
}

// Proxy information used to connect to stela peers
type Proxy struct {
	ServerName string
	CertPath   string
	KeyPath    string
	CAPath     string
}

// AddClient adds a client to the store and returns it's id
func (s *Server) AddClient(ctx context.Context, req *pb.AddClientRequest) (*pb.AddClientResponse, error) {
	// Add client to the store
	c := &stela.Client{}
	c.Address = req.ClientAddress
	if err := s.Store.AddClient(c); err != nil {
		return nil, err
	}

	return &pb.AddClientResponse{
		ClientId: c.ID,
	}, nil
}

// Connect a stela client to a stream of possible subscriptions. Uses the client id in to keep track
// of services and subscriptions it registers
func (s *Server) Connect(req *pb.ConnectRequest, stream pb.Stela_ConnectServer) error {
	ctx := stream.Context()

	// Look up client
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return err
	}

	// Send services over stream
	for {
		select {
		case rs := <-c.SubscribeCh():
			response := &pb.ServiceMessage{
				Name:     rs.Name,
				Hostname: rs.Hostname,
				IPv4:     rs.IPv4,
				IPv6:     rs.IPv6,
				Port:     rs.Port,
				Priority: rs.Priority,
				Action:   rs.Action,
				Value:    rs.Value,
			}

			if err := stream.Send(response); err != nil {
				return err
			}
		case <-ctx.Done():
			// Remove all services the client registered
			registeredServices := s.Store.ServicesByClient(c)
			for _, rs := range registeredServices {
				s.Store.Deregister(rs)

				// Notify all peers about the deregistered service
				rs.Action = stela.DeregisterAction
				if err := s.peerNotify(rs); err != nil {
					continue
				}
			}

			// Remove client from store to cleanup
			s.Store.RemoveClient(c)

			return nil
		}
	}
}

// Subscribe a client to a service name.
func (s *Server) Subscribe(ctx context.Context, req *pb.SubscribeRequest) (*pb.SubscribeResponse, error) {
	// Look up client that sent the request
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return nil, err
	}

	// Subscribe the client to the serviceName
	err = s.Store.Subscribe(req.ServiceName, c)
	if err != nil {
		return nil, err
	}

	return &pb.SubscribeResponse{}, nil
}

// Unsubscribe a client to a service name.
func (s *Server) Unsubscribe(ctx context.Context, req *pb.SubscribeRequest) (*pb.SubscribeResponse, error) {
	// Look up client that sent the request
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return nil, err
	}

	// Subscribe the client to the serviceName
	err = s.Store.Unsubscribe(req.ServiceName, c)
	if err != nil {
		return nil, err
	}

	return &pb.SubscribeResponse{}, nil
}

// Register a service to a client
func (s *Server) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {

	// Verify the Value isn't more than 256 bytes
	l := len(req.Service.Value)
	if l > stela.DefaultMaxValueBytes {
		return nil, fmt.Errorf("Value is greater than %d bytes. Received: %d", stela.DefaultMaxValueBytes, l)
	}

	// Look up client that sent the request
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return nil, err
	}

	// Convert req to service
	service := &stela.Service{
		Client:   c,
		Name:     req.Service.Name,
		Hostname: req.Service.Hostname,
		IPv4:     req.Service.IPv4,
		IPv6:     req.Service.IPv6,
		Port:     req.Service.Port,
		Priority: req.Service.Priority,
		Action:   stela.RegisterAction,
		Value:    req.Service.Value,
	}

	// Register service to store
	if err := s.Store.Register(service); err != nil {
		return nil, err
	}

	// Notify all peers about the new service
	if err := s.peerNotify(service); err != nil {
		return nil, err
	}

	return &pb.RegisterResponse{}, nil
}

// Deregister a service
func (s *Server) Deregister(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	// Look up client that sent the request
	c, err := s.Store.Client(req.ClientId)
	if err != nil {
		return nil, err
	}

	// Convert req to service
	service := &stela.Service{
		Client:   c,
		Name:     req.Service.Name,
		Hostname: req.Service.Hostname,
		IPv4:     req.Service.IPv4,
		IPv6:     req.Service.IPv6,
		Port:     req.Service.Port,
	}

	// Deregister service
	ds := s.Store.Deregister(service)

	// Notify all peers that a service was deregistered
	if ds != nil {
		if err := s.peerNotify(ds); err != nil {
			return nil, err
		}
	}

	return &pb.RegisterResponse{}, nil
}

// peerNotify calls NotifyClients on all Store peers
func (s *Server) peerNotify(service *stela.Service) error {
	opts, err := s.gRPCOptions()
	if err != nil {
		return err
	}

	for _, p := range s.peers {
		go func(p *node.Node) {
			// Create context with timeout
			ctx, cancelFunc := context.WithTimeout(context.Background(), s.Timeout)
			defer cancelFunc()
			waitCh := make(chan struct{})

			go func() {
				defer close(waitCh)

				address := string(p.Payload)
				address, err := convertToLocalIP(address)
				if err != nil {
					return
				}

				// Dial the server
				conn, err := grpc.Dial(address, opts...)
				if err != nil {
					return
				}
				defer conn.Close()
				c := pb.NewStelaClient(conn)
				serviceMessage := &pb.ServiceMessage{
					Name:     service.Name,
					Hostname: service.Hostname,
					IPv4:     service.IPv4,
					IPv6:     service.IPv6,
					Port:     service.Port,
					Priority: service.Priority,
					Action:   service.Action,
					Value:    service.Value,
				}

				_, err = c.NotifyClients(ctx, serviceMessage)
				if err != nil {
					// fmt.Println("peerNotify err,", err)
					return
				}
			}()

			// Block until the context times out or the client is notified
			select {
			case <-waitCh:
				return
			case <-ctx.Done():
				return
			}
		}(p)
	}

	return nil
}

// NotifyClients tells all locally subscribed clients about a service change
func (s *Server) NotifyClients(ctx context.Context, req *pb.ServiceMessage) (*pb.NotifyResponse, error) {
	// Convert req to service
	service := &stela.Service{
		Name:     req.Name,
		Hostname: req.Hostname,
		IPv4:     req.IPv4,
		IPv6:     req.IPv6,
		Port:     req.Port,
		Priority: req.Priority,
		Action:   req.Action,
		Value:    req.Value,
	}

	s.Store.NotifyClients(service)

	return &pb.NotifyResponse{}, nil
}

// InstanceDiscover all services registered under a service name. Ex. "test.services.fg"
func (s *Server) InstanceDiscover(ctx context.Context, req *pb.DiscoverRequest) (*pb.DiscoverResponse, error) {
	services, err := s.Store.Discover(req.ServiceName)
	if err != nil {
		return nil, err
	}

	// Convert stela.Service struct to pb.ServiceMessage
	var srs []*pb.ServiceMessage
	for _, ds := range services {
		sr := &pb.ServiceMessage{
			Name:     ds.Name,
			Hostname: ds.Hostname,
			IPv4:     ds.IPv4,
			IPv6:     ds.IPv6,
			Port:     ds.Port,
			Priority: ds.Priority,
			Value:    ds.Value,
		}
		srs = append(srs, sr)
	}

	return &pb.DiscoverResponse{Services: srs}, nil
}

// InstanceDiscoverRegex all services registered under a service name. Ex. "test.services.fg"
func (s *Server) InstanceDiscoverRegex(ctx context.Context, req *pb.DiscoverRequest) (*pb.DiscoverResponse, error) {
	services, err := s.Store.DiscoverRegex(req.ServiceName)
	if err != nil {
		return nil, err
	}

	// Convert stela.Service struct to pb.ServiceMessage
	var srs []*pb.ServiceMessage
	for _, ds := range services {
		sr := &pb.ServiceMessage{
			Name:     ds.Name,
			Hostname: ds.Hostname,
			IPv4:     ds.IPv4,
			IPv6:     ds.IPv6,
			Port:     ds.Port,
			Priority: ds.Priority,
			Value:    ds.Value,
		}
		srs = append(srs, sr)
	}

	return &pb.DiscoverResponse{Services: srs}, nil
}

// InstanceDiscoverOne service registered under a service name.
func (s *Server) InstanceDiscoverOne(ctx context.Context, req *pb.DiscoverRequest) (*pb.ServiceMessage, error) {
	service, err := s.Store.DiscoverOne(req.ServiceName)
	if err != nil {
		return nil, err
	}

	// Convert stela.Service struct to pb.ServiceResponse
	return &pb.ServiceMessage{
		Name:     service.Name,
		Hostname: service.Hostname,
		IPv4:     service.IPv4,
		IPv6:     service.IPv6,
		Port:     service.Port,
		Priority: service.Priority,
		Value:    service.Value,
	}, nil
}

// InstanceDiscoverAll returns all services registered with stela even other clients TODO
func (s *Server) InstanceDiscoverAll(ctx context.Context, req *pb.DiscoverAllRequest) (*pb.DiscoverResponse, error) {
	services := s.Store.DiscoverAll()

	// Convert stela.Service struct to pb.ServiceResponse
	var srs []*pb.ServiceMessage
	for _, ds := range services {
		sr := &pb.ServiceMessage{
			Name:     ds.Name,
			Hostname: ds.Hostname,
			IPv4:     ds.IPv4,
			IPv6:     ds.IPv6,
			Port:     ds.Port,
			Priority: ds.Priority,
			Value:    ds.Value,
		}
		srs = append(srs, sr)
	}

	return &pb.DiscoverResponse{Services: srs}, nil
}

// Discover all services registered under a service name. Ex. "test.services.fg"
func (s *Server) Discover(ctx context.Context, req *pb.DiscoverRequest) (*pb.DiscoverResponse, error) {
	opts, err := s.gRPCOptions()
	if err != nil {
		return nil, err
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(s.peers))
	waitCh := make(chan struct{})

	var results []*pb.ServiceMessage
	var mu sync.Mutex

	go func() {
		for _, p := range s.peers {
			go func(p *node.Node) {
				defer wg.Done()

				address := string(p.Payload)
				address, err := convertToLocalIP(address)
				if err != nil {
					return
				}

				// Dial the server
				conn, err := grpc.Dial(address, opts...)
				if err != nil {
					return
				}
				defer conn.Close()
				c := pb.NewStelaClient(conn)
				resp, err := c.InstanceDiscover(ctx, req)
				if err != nil {
					return
				}

				mu.Lock()
				defer mu.Unlock()
				results = append(results, resp.Services...)
			}(p)
		}

		wg.Wait()
		close(waitCh)
	}()

	// Block until the context times out or the client is notified
	select {
	case <-waitCh:
		break
	case <-ctx.Done():
		break
	}

	return &pb.DiscoverResponse{Services: results}, nil
}

// DiscoverRegex all services registered under a service name. Ex. "test.services.fg"
func (s *Server) DiscoverRegex(ctx context.Context, req *pb.DiscoverRequest) (*pb.DiscoverResponse, error) {
	opts, err := s.gRPCOptions()
	if err != nil {
		return nil, err
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(s.peers))
	waitCh := make(chan struct{})

	var results []*pb.ServiceMessage
	var mu sync.Mutex

	go func() {
		for _, p := range s.peers {
			go func(p *node.Node) {
				defer wg.Done()

				address := string(p.Payload)
				address, err := convertToLocalIP(address)
				if err != nil {
					return
				}

				// Dial the server
				conn, err := grpc.Dial(address, opts...)
				if err != nil {
					return
				}
				defer conn.Close()
				c := pb.NewStelaClient(conn)
				resp, err := c.InstanceDiscoverRegex(ctx, req)
				if err != nil {
					return
				}

				mu.Lock()
				defer mu.Unlock()
				results = append(results, resp.Services...)
			}(p)
		}

		wg.Wait()
		close(waitCh)
	}()

	// Block until the context times out or the client is notified
	select {
	case <-waitCh:
		break
	case <-ctx.Done():
		break
	}

	return &pb.DiscoverResponse{Services: results}, nil
}

// DiscoverOne service registered under a service name.
// TODO Round Robin peers
func (s *Server) DiscoverOne(ctx context.Context, req *pb.DiscoverRequest) (*pb.ServiceMessage, error) {
	opts, err := s.gRPCOptions()
	if err != nil {
		return nil, err
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(s.peers))
	waitCh := make(chan struct{})

	var results []*pb.ServiceMessage
	var mu sync.Mutex

	go func() {
		for _, p := range s.peers {
			go func(p *node.Node) {
				defer wg.Done()

				address := string(p.Payload)
				address, err := convertToLocalIP(address)
				if err != nil {
					return
				}

				// Dial the server
				conn, err := grpc.Dial(address, opts...)
				if err != nil {
					return
				}
				defer conn.Close()
				c := pb.NewStelaClient(conn)
				resp, err := c.InstanceDiscoverOne(ctx, req)
				if err != nil {
					return
				}

				mu.Lock()
				defer mu.Unlock()
				results = append(results, resp)
			}(p)
		}

		wg.Wait()
		close(waitCh)
	}()

	// Block until the context times out or the client is notified
	select {
	case <-waitCh:
		break
	case <-ctx.Done():
		break
	}

	// Give back a random result
	if len(results) < 1 {
		return nil, errors.New("DiscoverOne returned no results")
	}

	var index int
	if len(results) > 1 {
		index = rand.Intn(len(results) - 1)
	} else {
		index = 0
	}

	return results[index], nil
}

// DiscoverAll returns all services registered with any stela member peer
func (s *Server) DiscoverAll(ctx context.Context, req *pb.DiscoverAllRequest) (*pb.DiscoverResponse, error) {
	opts, err := s.gRPCOptions()
	if err != nil {
		return nil, err
	}

	wg := &sync.WaitGroup{}
	wg.Add(len(s.peers))
	waitCh := make(chan struct{})

	var results []*pb.ServiceMessage
	var mu sync.Mutex

	go func() {
		for _, p := range s.peers {
			go func(p *node.Node) {
				defer wg.Done()

				address := string(p.Payload)
				address, err := convertToLocalIP(address)
				if err != nil {
					return
				}

				// Dial the server
				conn, err := grpc.Dial(address, opts...)
				if err != nil {
					return
				}
				defer conn.Close()
				c := pb.NewStelaClient(conn)
				resp, err := c.InstanceDiscoverAll(ctx, req)
				if err != nil {
					return
				}

				mu.Lock()
				defer mu.Unlock()
				results = append(results, resp.Services...)
			}(p)
		}

		wg.Wait()
		close(waitCh)
	}()

	// Block until the context times out or the client is notified
	select {
	case <-waitCh:
		break
	case <-ctx.Done():
		break
	}

	return &pb.DiscoverResponse{Services: results}, nil
}

// SetPeers sets the peers slice
func (s *Server) SetPeers(peers []*node.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers = peers
}

func (s *Server) gRPCOptions() ([]grpc.DialOption, error) {
	var opts []grpc.DialOption
	if s.Proxy != nil {
		// Load the client certificates from disk
		certificate, err := tls.LoadX509KeyPair(s.Proxy.CertPath, s.Proxy.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("Could not load client key pair: %s", err)
		}

		// Create a certificate pool from the certificate authority
		certPool := x509.NewCertPool()
		ca, err := ioutil.ReadFile(s.Proxy.CAPath)
		if err != nil {
			return nil, fmt.Errorf("Could not read ca certificate: %s", err)
		}

		// Append the certificates from the CA
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			return nil, errors.New("Failed to append ca certs")
		}

		creds := credentials.NewTLS(&tls.Config{
			ServerName:   s.Proxy.ServerName,
			Certificates: []tls.Certificate{certificate},
			RootCAs:      certPool,
		})

		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	return opts, nil
}

func convertToLocalIP(address string) (string, error) {
	// Test if the address is this instance and convert to localhost
	ip, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	if netutil.IsLocalhost(ip) {
		address = fmt.Sprintf("%s:%s", "127.0.0.1", port)
	}

	return address, nil
}
