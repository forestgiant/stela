package store

import "gitlab.fg/go/stela"

// Store represents a key value service storage backed by raft
type Store interface {
	Open(enableSingle bool) error
	Register(s *stela.Service) error
	Deregister(s *stela.Service) error
	Discover(serviceName string) ([]*stela.Service, error)
	DiscoverOne(serviceName string) (*stela.Service, error)
	Join(addr string) error   // Add peer raft
	Remove(addr string) error // Remove peer from raft
}
