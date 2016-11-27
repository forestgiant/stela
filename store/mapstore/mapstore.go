package mapstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"

	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/raftutil"
)

// MapStore implements the Store interface creating an in memory map of stela.Services
type MapStore struct {
	services      map[string][]stela.Service // Map of service names that holds a slice of registered services
	clients       []*stela.Client
	subscribers   map[string][]*stela.Client // Store clients that subscribe to a service name
	muServices    *sync.RWMutex              // Mutex used to lock services map
	muSubscribers *sync.RWMutex              // Mutex used to lock subscriber map
	muClients     *sync.RWMutex              // Mutex used to lock client slice
	peerStore     raft.PeerStore
	raftDir       string
	raftTransport raft.StreamLayer
	raft          *raft.Raft
}

// byPriority is a sort interface to sort []srvWithKey slices
type byPriority []stela.Service

func (s byPriority) Len() int           { return len(s) }
func (s byPriority) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byPriority) Less(i, j int) bool { return s[i].Priority < s[j].Priority }

// Open initiates raft
func (m *MapStore) Open(enableSingle bool) error {
	// Make a directory for raft files
	if err := os.MkdirAll(m.raftDir, 0755); err != nil {
		return err
	}

	// Setup the raft config
	config := m.raftConfig()

	// Check for existing peers
	peers, err := raftutil.ReadPeersJSON(filepath.Join(m.raftDir, "peers.json"))
	if err != nil {
		return err
	}

	// Allow the node to entry single-mode, potentially electing itself, if
	// explicitly enabled and there is only 1 node in the cluster already.
	if enableSingle && len(peers) <= 1 {
		fmt.Println("Enabling single node mode")
		config.EnableSingleNode = true
		config.DisableBootstrapAfterElect = false
	}

	// Setup Raft communication.
	transport := raft.NewNetworkTransport(m.raftTransport, 3, 10*time.Second, os.Stderr)

	// Create peer storage.
	m.peerStore = raft.NewJSONPeers(m.raftDir, transport)

	// Setup snapshot store
	// TODO pass in forestgiant log instead of os.Stderr
	snapshot, err := raft.NewFileSnapshotStore(m.raftDir, 2, os.Stderr)
	if err != nil {
		return fmt.Errorf("Error creating snapshot store: %s", err)
	}

	// Create a boltdb logStore for raft
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(m.raftDir, "raft.db"))
	if err != nil {
		return fmt.Errorf("Error creating boltdb logStore: %s", err)
	}

	// Start raft
	ra, err := raft.NewRaft(config, m, logStore, logStore, snapshot, m.peerStore, transport)
	if err != nil {
		return fmt.Errorf("Error creating raft: %s", err)
	}
	m.raft = ra

	return nil
}

func (m *MapStore) init() {
	if m.services == nil {
		m.services = make(map[string][]stela.Service)
	}
	if m.muServices == nil {
		m.muServices = new(sync.RWMutex)
	}
	if m.muSubscribers == nil {
		m.muSubscribers = new(sync.RWMutex)
	}
	if m.muClients == nil {
		m.muClients = new(sync.RWMutex)
	}
}

// Register takes a service adding it to the services map and let's all client subscriers know
func (m *MapStore) Register(s *stela.Service) error {
	if !s.Valid() {
		return errors.New("Service registered is invalid")
	}

	m.init()

	// Error if it has been added before
	if m.hasService(s) {
		return errors.New("Service is already registered")
	}

	// Rotate before the other services adding the new service
	m.rotateServices(s.Name)

	// Set all new incoming services Priority to 0
	s.Priority = 0

	// Add service to the beginning of the services map since it's new
	m.muServices.Lock()
	defer m.muServices.Unlock()
	m.services[s.Name] = append([]stela.Service{*s}, m.services[s.Name]...) // Prepend new service

	// Let subscribers know about new service
	for _, c := range m.subscribers[s.Name] {
		s.Action = stela.RegisterAction
		c.Notify(s)
	}

	return nil
}

// Deregister removes a service from the map and notifies all client subscribers
func (m *MapStore) Deregister(s *stela.Service) {
	// Notify clients that a new service is deregistered (for service name)
	for _, c := range m.subscribers[s.Name] {
		s.Action = stela.DeregisterAction
		c.Notify(s)
	}

	// Remove service from services map
	for k, v := range m.services {
		for i, rs := range v {
			if rs.Equal(s) {
				// Remove from slice
				v = append(v[:i], v[i+1:]...)
			}
		}

		// If that was the last service in the slice delete the key
		if len(v) == 0 {
			delete(m.services, k)
		}
	}
}

func (m *MapStore) initSubscribe() {
	if m.subscribers == nil {
		m.subscribers = make(map[string][]*stela.Client)
	}

	if m.muSubscribers == nil {
		m.muSubscribers = &sync.RWMutex{}
	}
}

// Subscribe allows a stela.Client to subscribe to a specific serviceName
func (m *MapStore) Subscribe(serviceName string, c *stela.Client) error {
	m.initSubscribe()

	// Add client to list of subscribers
	m.muSubscribers.Lock()
	defer m.muSubscribers.Unlock()
	m.subscribers[serviceName] = append(m.subscribers[serviceName], c)

	return nil
}

// Unsubscribe quits sending service changes to the stela.Client
func (m *MapStore) Unsubscribe(serviceName string, c *stela.Client) error {
	if m.services[serviceName] == nil {
		return fmt.Errorf("No service registered under: %s", serviceName)
	}

	m.initSubscribe()

	// Remove client to list of subscribers
	m.muSubscribers.Lock()
	defer m.muSubscribers.Unlock()
	subscribers := m.subscribers[serviceName]
	for i, rc := range m.subscribers[serviceName] {
		// Make sure the client is registered
		if c == rc {
			// Remove it from the subscriber slice
			subscribers = append(subscribers[:i], subscribers[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("Client not find in subscriber list: %v", c)
}

// Discover finds all services registered under a serviceName
func (m *MapStore) Discover(serviceName string) ([]stela.Service, error) {
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

	return &s, nil
}

// Join the mapstore's raft
func (m *MapStore) Join(addr string) error {
	return nil
}

// Remove from mapstore's raft
func (m *MapStore) Remove(addr string) error {
	return nil
}

// Subscribers returns a slice of stela.Clients from a service name
func (m *MapStore) Subscribers(serviceName string) []*stela.Client {
	return m.subscribers[serviceName]
}

// AddClient adds to client slice m.clients
func (m *MapStore) AddClient(c *stela.Client) {
	m.init()
	m.muClients.Lock()
	defer m.muClients.Unlock()
	m.clients = append(m.clients, c)
}

// RemoveClient removes client from the slice m.clients, services it registered and any subscriptions
func (m *MapStore) RemoveClient(c *stela.Client) {
	m.init()
	m.muClients.Lock()
	defer m.muClients.Unlock()
	for i, rc := range m.clients {
		if rc == c {
			m.clients = append(m.clients[:i], m.clients[i+1:]...)
		}
	}

	// Remove any services the client registered
	for k, v := range m.services {
		// Remove any services that the client registered
		for i, s := range v {
			if s.Client == c {
				// Remove from slice
				v = append(v[:i], v[i+1:]...)
			}
		}

		// If that was the last service in the slice delete the key
		if len(v) == 0 {
			delete(m.services, k)
		}
	}

	// Remove client from Subscribers
	for k, v := range m.subscribers {
		for i, rc := range v {
			if rc == c {
				// Remove from slice
				v = append(v[:i], v[i+1:]...)
			}
		}

		// If that was the last client subscribed delete the key
		if len(v) == 0 {
			delete(m.subscribers, k)
		}
	}
}

// RemoveClients convenience method to RemoveClient for each client in provided slice
func (m *MapStore) RemoveClients(clients []*stela.Client) {
	for _, c := range clients {
		m.RemoveClient(c)
	}
}

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

// Clients returns a slice of clients from m.clients based on ip address
func (m *MapStore) Clients(address string) ([]*stela.Client, error) {
	m.init()
	m.muClients.Lock()
	defer m.muClients.Unlock()

	// Look for clients matching the address
	var clients []*stela.Client
	for _, c := range m.clients {
		if c.Address == address {
			clients = append(clients, c)
		}
	}

	if len(clients) == 0 {
		return nil, fmt.Errorf("No clients found with the address: %s", address)
	}

	return clients, nil
}

func (m *MapStore) hasService(s *stela.Service) bool {
	m.muServices.Lock()
	defer m.muServices.Unlock()
	for _, rs := range m.services[s.Name] {
		if s.Equal(&rs) {
			return true // service is already a registered
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

// Raft
type raftCommand struct {
	Operation raftOperation
	Service   *stela.Service
}

type raftOperation int

const (
	registerOperation raftOperation = iota
	deregisterOperation
	discoverOperation
	discoverOneOperation
)

type fsmDiscoverResponse struct {
	results []stela.Service
	error   error
}

type fsmDiscoverOneResponse struct {
	result *stela.Service
	error  error
}

type fsmResponse struct {
	error error
}

// Apply is used for raft consensus FSM
func (m *MapStore) Apply(l *raft.Log) interface{} {
	var c raftCommand
	if err := json.Unmarshal(l.Data, &c); err != nil {
		return err
	}

	if c.Service == nil {
		return errors.New("raftCommand.Service is nil")
	}

	switch c.Operation {
	case registerOperation:
		err := m.Register(c.Service)
		return &fsmResponse{error: err}
	case deregisterOperation:
		m.Deregister(c.Service)
		return &fsmResponse{error: nil}
	case discoverOperation:
		svcResponse, err := m.Discover(c.Service.Name)
		return &fsmDiscoverResponse{results: svcResponse, error: err}
	case discoverOneOperation:
		svcResponse, err := m.DiscoverOne(c.Service.Name)
		return &fsmDiscoverOneResponse{result: svcResponse, error: err}
	}

	return nil
}

// Snapshot is used for raft consensus FSM
func (m *MapStore) Snapshot() (raft.FSMSnapshot, error) {
	// // Clone srv and a record maps
	// d.srvMutex.Lock()
	// srvRecords := make(map[string]map[string][]byte)
	// for k, v := range d.srvRecords {
	// 	srvRecords[k] = v
	// }
	// d.srvMutex.Unlock()

	// d.aMutex.Lock()
	// aRecords := make(map[string][]byte)
	// for k, v := range d.aRecords {
	// 	aRecords[k] = v
	// }
	// d.aMutex.Unlock()

	// d.registerMutex.Lock()
	// registeredServices := make(map[stela.Service]*stela.Service)
	// for k, v := range d.registeredServices {
	// 	registeredServices[k] = v
	// }
	// d.registerMutex.Unlock()

	// return &fsmSnapshot{srvRecords: srvRecords, aRecords: aRecords, registeredServices: registeredServices}, nil
	return nil, nil
}

// Restore is used for raft consensus FSM
func (m *MapStore) Restore(rc io.ReadCloser) error {
	// fsm := new(fsmSnapshot)
	// if err := json.NewDecoder(rc).Decode(&fsm); err != nil {
	// 	return err
	// }

	// // Set the state from the snapshot, no lock required according to
	// // Hashicorp docs.
	// d.srvRecords = fsm.srvRecords
	// d.aRecords = fsm.aRecords
	// d.registeredServices = fsm.registeredServices
	return nil
}

// WaitForLeader blocks until a raft leader is detected or the timeout happens
func (m *MapStore) WaitForLeader() error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-tick.C:
			if m.raft.Leader() != "" {
				return nil
			}
		case <-timeout.C:
			return fmt.Errorf("WaitForLeader timed out")
		}
	}
}

func (m *MapStore) raftConfig() *raft.Config {
	return raft.DefaultConfig()
}

type fsmSnapshot struct {
	services map[string][]stela.Service
}

func (f *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		// Encode data.
		b, err := json.Marshal(f)
		if err != nil {
			return err
		}

		// Write data to sink.
		if _, err := sink.Write(b); err != nil {
			return err
		}

		// Close the sink.
		if err := sink.Close(); err != nil {
			return err
		}

		return nil
	}()

	if err != nil {
		sink.Cancel()
		return err
	}

	return nil
}

func (f *fsmSnapshot) Release() {}

// Helper functions for stela's Raft implementation
const portIncrement int = 1000

// CreateRaftPort takes a port and increases it
func CreateRaftPort(port string) (string, error) {
	p, err := strconv.Atoi(port)
	if err != nil {
		return "", err
	}

	port = strconv.Itoa(p + portIncrement)

	return port, nil
}

func stelaPortFromRaft(port string) (string, error) {
	p, err := strconv.Atoi(port)
	if err != nil {
		return "", err
	}

	port = strconv.Itoa(p - portIncrement)

	return port, nil
}
