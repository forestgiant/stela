package stela

import (
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	// DefaultStelaHTTPAddress by default stela assumes there is a local instance running
	DefaultStelaHTTPAddress = "127.0.0.1:9000"

	// DefaultMulticastAddress is the multicast IPV6 address stela communicates on
	DefaultMulticastAddress = "[ff12::9000]"

	// DefaultMulticastPort is the default multicast port
	DefaultMulticastPort = 8053
)

// Actions for Service
const (
	RegisterAction   = iota
	DeregisterAction = iota
)

// Service used in request/response and
// storing RR in boltdb
type Service struct {
	Name         string
	Target       string
	Address      string
	Port         int32
	Priority     int32
	Timeout      int32 // The length of time, in milliseconds, before a service is deregistered
	Action       int32
	Client       *Client // Store reference to the client that registered the service
	registerCh   chan struct{}
	deregisterCh chan struct{}
	stopped      bool
	mu           *sync.Mutex // protects registerCh, deregisterCh, stopped
}

// Equal is a duplicate method from api/api.go
func (s Service) Equal(testService *Service) bool {
	if s.Name != testService.Name {
		return false
	}
	if s.Target != testService.Target {
		return false
	}
	if s.Address != testService.Address {
		return false
	}
	if s.Port != testService.Port {
		return false
	}
	return true
}

// Valid test the required fields of a Service
func (s *Service) Valid() bool {
	if s.Name == "" {
		return false
	}
	if s.Target == "" {
		return false
	}
	if s.Address == "" {
		return false
	}
	return true
}

// NewSRV is convenience method added to Service to easily return new SRV record
func (s *Service) NewSRV() *dns.SRV {
	target := dns.Fqdn(s.Target)    // hostname of the machine providing the service, ending in a dot.
	serviceName := dns.Fqdn(s.Name) // domain name for which this record is valid, ending in a dot.

	SRV := new(dns.SRV)

	SRV.Hdr = dns.RR_Header{
		Name:   serviceName,
		Rrtype: dns.TypeSRV,
		Class:  dns.ClassINET,
		Ttl:    86400}
	SRV.Priority = uint16(0)
	SRV.Weight = uint16(0)
	SRV.Port = uint16(s.Port)
	SRV.Target = target

	return SRV
}

// NewA is convenience method added to Service to easily return new A record
func (s *Service) NewA(ip net.IP) dns.RR {
	serviceName := dns.Fqdn(s.Target) // domain name for which this record is valid, ending in a dot.

	A := new(dns.A)
	A.Hdr = dns.RR_Header{
		Name:   serviceName,
		Rrtype: dns.TypeA,
		Class:  dns.ClassINET,
		Ttl:    86400}
	A.A = ip

	return A
}

// init service chans
func (s *Service) init() {
	if s.mu == nil {
		s.mu = &sync.Mutex{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registerCh == nil {
		s.registerCh = make(chan struct{})
	}
	if s.deregisterCh == nil {
		s.deregisterCh = make(chan struct{})
	}
}

// DeregisterCh returns a read only chan
func (s *Service) DeregisterCh() <-chan struct{} {
	s.init()
	return s.deregisterCh
}

// RegisterCh returns a read only chan
func (s *Service) RegisterCh() <-chan struct{} {
	s.init()
	return s.registerCh
}

// KeepRegistered sends struct to the registerCh chan
func (s *Service) KeepRegistered() {
	s.init()
	s.registerCh <- struct{}{}
}

// StopRegistering closes the deregisterCh chan
func (s *Service) StopRegistering() {
	s.init()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.stopped = true
	close(s.deregisterCh)
}

// Client struct holds all the information about a client registering a service
type Client struct {
	Address     string // IPv4 address
	ID          string
	subscribeCh chan *Service // Used to send changes in services
}

func (c *Client) init() {
	if c.subscribeCh == nil {
		c.subscribeCh = make(chan *Service)
	}
}

// SubscribeCh returns a channel of Services that is sent a service when one is removed or added
func (c *Client) SubscribeCh() <-chan *Service {
	c.init()

	return c.subscribeCh
}

// Notify sends a service to the client's subscribeCh
func (c *Client) Notify(s *Service) {
	c.init()

	// Notify the client of the new service and timeout if nothing reads it within 10 millisecond
	t := time.NewTimer(time.Millisecond * 10)
	select {
	case c.subscribeCh <- s:
	case <-t.C:
		t.Stop()
	}
}
