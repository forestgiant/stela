package stela

import (
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	// Version of stela
	Version = "1.0.0"

	// DefaultStelaAddress by default stela assumes there is a local instance running
	DefaultStelaAddress = "127.0.0.1:31000"

	// DefaultStelaPort by default stela assumes there is a local instance running
	DefaultStelaPort = 31000

	// DefaultMulticastAddress is the multicast IPV6 address stela communicates on
	DefaultMulticastAddress = "[ff12::9000]"

	// DefaultMulticastPort is the default multicast port
	DefaultMulticastPort = 31053

	// ServiceName is how stela instances are registered as services
	ServiceName = "stela.services.fg"

	// DefaultMaxValueBytes only allows the Value byte slice to be 256 bytes
	DefaultMaxValueBytes = 256
)

// Actions for Service
const (
	RegisterAction   = iota
	DeregisterAction = iota
)

// Service used in request/response
type Service struct {
	Name     string
	Hostname string
	// Address      string // Automatically set from host:port when service is registered
	IPv4         string
	IPv6         string
	Port         int32
	Priority     int32
	Timeout      int32 // The length of time, in milliseconds, before a service is deregistered
	Action       int32
	Client       *Client // Store reference to the client that registered the service
	Value        interface{}
	id           string // Automatically set when the service is registered
	registerCh   chan struct{}
	deregisterCh chan struct{}
	stopped      bool
	mu           *sync.Mutex // protects registerCh, deregisterCh, stopped
}

// Stringer
func (s Service) String() string {
	return fmt.Sprintf("Name: %s, Hostname: %s, IPv4: %s, IPv6: %s, Port: %d, Action: %d", s.Name, s.Hostname, s.IPv4, s.IPv6, s.Port, s.Action)
}

// Equal tests if a Service is the same
func (s Service) Equal(testService *Service) bool {
	// if s.id != testService.id {
	// 	return false
	// }
	if s.Name != testService.Name {
		return false
	}
	if s.Hostname != testService.Hostname {
		return false
	}
	if s.IPv4 != testService.IPv4 {
		return false
	}
	if s.IPv6 != testService.IPv6 {
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
	if s.Hostname == "" {
		return false
	}
	if s.IPv4 == "" {
		if s.IPv6 == "" {
			return false
		}
	}
	if s.IPv6 == "" {
		if s.IPv4 == "" {
			return false
		}
	}
	return true
}

// GenerateID will overwrite the id with a new rand 10 byte string
func (s *Service) GenerateID() error {
	id, err := generateID(10)
	if err != nil {
		return err
	}
	s.id = id
	return nil
}

// ID getter return s.id
func (s *Service) ID() string {
	return s.id
}

// IPv4Address helper to combine IPv4 and Port
func (s *Service) IPv4Address() string {
	return fmt.Sprintf("%s:%d", s.IPv4, s.Port)
}

// IPv6Address helper to combine IPv4 and Port
func (s *Service) IPv6Address() string {
	return fmt.Sprintf("%s:%d", s.IPv6, s.Port)
}

// NewSRV is convenience method added to Service to easily return new SRV record
func (s *Service) NewSRV() *dns.SRV {
	target := dns.Fqdn(s.Hostname)  // hostname of the machine providing the service, ending in a dot.
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
	serviceName := dns.Fqdn(s.Hostname) // domain name for which this record is valid, ending in a dot.

	A := new(dns.A)
	A.Hdr = dns.RR_Header{
		Name:   serviceName,
		Rrtype: dns.TypeA,
		Class:  dns.ClassINET,
		Ttl:    86400}
	A.A = ip

	return A
}

// Client struct holds all the information about a client registering a service
type Client struct {
	Address     string // IPv4 address
	ID          string
	mu          sync.Mutex    // protect subscribeCh
	subscribeCh chan *Service // used to send changes in services
}

// Equal tests if a Service is the same
func (c *Client) Equal(testClient *Client) bool {
	if c.Address != testClient.Address {
		return false
	}
	if c.ID != testClient.ID {
		return false
	}
	return true
}

func (c *Client) init() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subscribeCh == nil {
		c.subscribeCh = make(chan *Service)
	}
}

// GenerateID will overwrite the id with a new rand 10 byte string
func (c *Client) GenerateID() error {
	id, err := generateID(10)
	if err != nil {
		return err
	}
	c.ID = id
	return nil
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

type value struct {
	Val interface{}
}

// EncodeValue converts interface{} to a byte slice
func EncodeValue(v interface{}) []byte {
	val := value{
		Val: v,
	}
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	enc.Encode(val)

	return buf.Bytes()
}

// DecodeValue converts byte slice to interface{}
func DecodeValue(b []byte) interface{} {
	// Decode
	var data value
	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	dec.Decode(&data)

	return data.Val
}

func generateID(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%X", b), nil
}
