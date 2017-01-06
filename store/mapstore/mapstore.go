package mapstore

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"

	"github.com/hashicorp/raft"
	"gitlab.fg/go/stela"
)

// MapStore implements the Store interface creating an in memory map of stela.Services
type MapStore struct {
	RaftDir       string
	RaftAddr      string
	services      map[string][]*stela.Service // Map of service names that holds a slice of registered services
	clients       []*stela.Client
	subscribers   map[string][]*stela.Client // Store clients that subscribe to a service name
	muServices    sync.RWMutex               // Mutex used to lock services map
	muSubscribers sync.RWMutex               // Mutex used to lock subscriber map
	muClients     sync.RWMutex               // Mutex used to lock client slice
	peerStore     raft.PeerStore
	raft          *raft.Raft
}

// byPriority is a sort interface to sort []srvWithKey slices
type byPriority []*stela.Service

func (s byPriority) Len() int           { return len(s) }
func (s byPriority) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byPriority) Less(i, j int) bool { return s[i].Priority < s[j].Priority }

func (m *MapStore) init() {
	if m.services == nil {
		m.services = make(map[string][]*stela.Service)
	}
	if m.subscribers == nil {
		m.subscribers = make(map[string][]*stela.Client)
	}
}

// Register takes a service adding it to the services map and let's all client subscriers know
func (m *MapStore) Register(s *stela.Service) error {
	if !s.Valid() {
		return fmt.Errorf("%v registered is invalid", s)
	}

	m.init()

	// Error if it has been added before
	if m.hasService(s) {
		return fmt.Errorf("%v is already registered", s)
	}

	// Rotate before the other services adding the new service
	m.rotateServices(s.Name)

	// Set all new incoming services Priority to 0
	s.Priority = 0

	// Add service to the beginning of the services map since it's new
	m.muServices.Lock()
	defer m.muServices.Unlock()
	m.services[s.Name] = append([]*stela.Service{s}, m.services[s.Name]...) // Prepend new service

	// Let subscribers know about new service
	// m.NotifyClients(s, stela.RegisterAction)

	return nil
}

// Deregister removes a service from the map and notifies all client subscribers
func (m *MapStore) Deregister(s *stela.Service) {
	// Notify clients that a new service is deregistered (for service name)
	// m.NotifyClients(s, stela.DeregisterAction)

	m.muServices.Lock()
	defer m.muServices.Unlock()

	// Remove service from services slice
	services := m.services[s.Name]
	for i := len(services) - 1; i >= 0; i-- {
		rs := services[i]
		if rs.Equal(s) {
			// Remove from slice
			services = append(services[:i], services[i+1:]...)
		}
	}

	// If that was the last service in the slice delete the key
	if len(services) == 0 {
		delete(m.services, s.Name)
	} else {
		m.services[s.Name] = services
	}
}

// NotifyClients let's all locally subscribed clients, on this stela instance, know about service subscription changes
func (m *MapStore) NotifyClients(s *stela.Service) {
	// Let subscribers know about service change
	for _, c := range m.subscribers[s.Name] {
		c.Notify(s)
	}
}

// Subscribe allows a stela.Client to subscribe to a specific serviceName
func (m *MapStore) Subscribe(serviceName string, c *stela.Client) error {
	m.init()

	// Add client to list of subscribers
	m.muSubscribers.Lock()
	defer m.muSubscribers.Unlock()
	m.subscribers[serviceName] = append(m.subscribers[serviceName], c)

	return nil
}

// Unsubscribe quits sending service changes to the stela.Client
func (m *MapStore) Unsubscribe(serviceName string, c *stela.Client) error {
	m.init()

	// Remove client to list of subscribers
	m.muSubscribers.Lock()
	defer m.muSubscribers.Unlock()
	subscribers := m.subscribers[serviceName]
	for i := len(subscribers) - 1; i >= 0; i-- {
		rc := subscribers[i]
		// Make sure the client is registered
		if c == rc {
			// Remove it from the subscriber slice
			subscribers = append(subscribers[:i], subscribers[i+1:]...)

			// If that was the last subscriber in the slice delete the key
			if len(subscribers) == 0 {
				delete(m.subscribers, serviceName)
			} else {
				m.subscribers[serviceName] = subscribers
			}

			return nil
		}
	}

	return fmt.Errorf("Client not find in subscriber list: %v", c)
}

// Discover finds all services registered under a serviceName
func (m *MapStore) Discover(serviceName string) ([]*stela.Service, error) {
	services := m.services[serviceName]
	if len(services) == 0 {
		return nil, fmt.Errorf("No services discovered with the service name: %s", serviceName)
	}

	return m.services[serviceName], nil
}

// DiscoverOne returns only one of the services registered under a serviceName
func (m *MapStore) DiscoverOne(serviceName string) (*stela.Service, error) {
	services, err := m.Discover(serviceName)
	if err != nil {
		return nil, err
	}

	// Get Service with lowest priority
	sort.Sort(byPriority(services))

	// Store the service with the lowest priority
	s := services[0]

	// Rotate services
	m.rotateServices(serviceName)

	return s, nil
}

// DiscoverAll returns all the services registered with the store
func (m *MapStore) DiscoverAll() []*stela.Service {
	var all []*stela.Service
	m.muServices.RLock()
	defer m.muServices.RUnlock()
	for _, v := range m.services {
		all = append(all, v...)
	}

	return all
}

// AddClient adds to client slice m.clients and return the client id
func (m *MapStore) AddClient(c *stela.Client) error {
	// Validate client ip address
	if ip := net.ParseIP(c.Address); ip == nil {
		return fmt.Errorf("Client has invalid address: %s", c.Address)
	}

	// Don't add duplicate clients
	if m.hasClient(c) {
		return errors.New("Client is already registered")
	}

	m.init()
	m.muClients.Lock()
	defer m.muClients.Unlock()
	m.clients = append(m.clients, c)

	// client id is the index of the client just added
	clientid, err := generateID(10)
	if err != nil {
		return err
	}
	c.ID = clientid

	return nil
}

// RemoveClient removes client from the slice m.clients, services it registered and any subscriptions
func (m *MapStore) RemoveClient(c *stela.Client) {
	m.init()
	m.muClients.Lock()
	defer m.muClients.Unlock()
	for i := len(m.clients) - 1; i >= 0; i-- {
		rc := m.clients[i]
		if rc == c {
			m.clients = append(m.clients[:i], m.clients[i+1:]...)
		}
	}

	m.muSubscribers.Lock()
	defer m.muSubscribers.Unlock()
	// Remove client from Subscribers
	for k, v := range m.subscribers {
		for i := len(v) - 1; i >= 0; i-- {
			rc := v[i]
			if rc == c {
				// Remove from slice
				v = append(v[:i], v[i+1:]...)
			}
		}

		// If that was the last client subscribed delete the key
		if len(v) == 0 {
			delete(m.subscribers, k)
		} else {
			m.subscribers[k] = v
		}
	}
}

// ServicesByClient returns all services a client has registered
func (m *MapStore) ServicesByClient(c *stela.Client) []*stela.Service {
	m.muServices.Lock()
	defer m.muServices.Unlock()

	var clientServices []*stela.Service

	// Remove any services the client registered
	for _, services := range m.services {
		// Remove any services that the client registered
		for i := len(services) - 1; i >= 0; i-- {
			s := services[i]
			if s == nil || s.Client == nil {
				// return errors.New("service or client are nil")
				continue
			}
			if s.Client.Equal(c) {
				// Remove from slice
				clientServices = append(clientServices, s)
			}
		}
	}

	return clientServices
}

// RemoveClients convenience method to RemoveClient for each client in provided slice
// func (m *MapStore) RemoveClients(clients []*stela.Client) {
// 	for _, c := range clients {
// 		m.RemoveClient(c)
// 	}
// }

// Client returns a client from m.clients based on id
func (m *MapStore) Client(id string) (*stela.Client, error) {
	m.init()
	m.muClients.Lock()
	defer m.muClients.Unlock()
	for _, c := range m.clients {
		if c.ID == id {
			return c, nil
		}
	}

	return nil, fmt.Errorf("Couldn't find a client from id: %s", id)
}

// // Clients returns a slice of clients from m.clients based on ip address
// func (m *MapStore) Clients(address string) ([]*stela.Client, error) {
// 	m.init()
// 	m.muClients.Lock()
// 	defer m.muClients.Unlock()

// 	// Look for clients matching the address
// 	var clients []*stela.Client
// 	for _, c := range m.clients {
// 		if c.Address == address {
// 			clients = append(clients, c)
// 		}
// 	}

// 	if len(clients) == 0 {
// 		return nil, fmt.Errorf("No clients found with the address: %s", address)
// 	}

// 	return clients, nil
// }

func (m *MapStore) hasService(s *stela.Service) bool {
	m.init()
	m.muServices.Lock()
	defer m.muServices.Unlock()
	for _, rs := range m.services[s.Name] {
		if s.Equal(rs) {
			return true // service is already a registered
		}
	}

	return false
}

func (m *MapStore) hasClient(c *stela.Client) bool {
	m.init()
	m.muClients.Lock()
	defer m.muClients.Unlock()
	for _, rc := range m.clients {
		if c.Equal(rc) {
			return true // client is already a registered
		}
	}

	return false
}

// TODO remove addingService and make sure to prepend to service map
func (m *MapStore) rotateServices(serviceName string) error {
	// Use length of all services to modulate priority
	mod := int32(len(m.services[serviceName]))

	// Now update all the Priorities
	m.muServices.Lock()
	defer m.muServices.Unlock()
	for i, s := range m.services[serviceName] {
		// Update SRV priority
		m.services[serviceName][i].Priority = (s.Priority + 1) % mod
	}

	return nil
}

func generateID(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%X", b), nil
}
