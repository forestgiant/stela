package store

import "gitlab.fg/go/stela"

// Store represents a key value service storage backed by raft
type Store interface {
	Open(enableSingle bool) error
	Register(s *stela.Service) error
	Deregister(s *stela.Service) error
	Discover(serviceName string) ([]*stela.Service, error)
	DiscoverOne(serviceName string) (*stela.Service, error)
	Subscribe(serviceName string, c *stela.Client) error
	Unsubscribe(serviceName string, c *stela.Client) error
	Subscribers(serviceName string) []*stela.Client
	Client(id string) *stela.Client
	Clients(address string) []*stela.Client // Returns all clients by a given ip address
	AddClient(c *stela.Client)
	RemoveClient(c *stela.Client) error
	RemoveClients(clients []*stela.Client) error
}

// RaftPeer represents a store that uses rafter
type RaftPeer interface {
	Join(addr string) error   // Add peer raft
	Remove(addr string) error // Remove peer from raft
}
