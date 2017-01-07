package store

import "gitlab.fg/go/stela"

// Store represents a key value service storage backed by raft
type Store interface {
	Register(s *stela.Service) error
	Deregister(s *stela.Service) *stela.Service
	Discover(serviceName string) ([]*stela.Service, error)
	DiscoverOne(serviceName string) (*stela.Service, error)
	DiscoverAll() []*stela.Service
	Subscribe(serviceName string, c *stela.Client) error
	Unsubscribe(serviceName string, c *stela.Client) error
	Client(id string) (*stela.Client, error)
	NotifyClients(*stela.Service)
	ServicesByClient(c *stela.Client) []*stela.Service
	// Clients(address string) ([]*stela.Client, error) // Returns all clients by a given ip address
	AddClient(c *stela.Client) error
	RemoveClient(c *stela.Client)
	// RemoveClients(clients []*stela.Client)
}
