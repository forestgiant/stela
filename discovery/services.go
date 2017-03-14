package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.fg/go/stela"

	"github.com/forestgiant/portutil"
	"github.com/hashicorp/raft"
	"github.com/miekg/dns"
)

type srvWithKey struct {
	srv *dns.SRV
	key []byte
}

// byPriority is a sort interface to sort []srvWithKey slices
type byPrioritySRVWithKey []*srvWithKey

func (s byPrioritySRVWithKey) Len() int           { return len(s) }
func (s byPrioritySRVWithKey) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byPrioritySRVWithKey) Less(i, j int) bool { return s[i].srv.Priority < s[j].srv.Priority }

// byPriority is a sort interface to sort []srvWithKey slices
type byPrioritySRV []*dns.SRV

func (s byPrioritySRV) Len() int           { return len(s) }
func (s byPrioritySRV) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s byPrioritySRV) Less(i, j int) bool { return s[i].Priority < s[j].Priority }

// DiscoveryService public interface for discoveryService struct
type DiscoveryService interface {
	ParseDNSQuery(m *dns.Msg) error
	Register(serviceName string, target string, address string, port int) (*stela.Service, error)
	Deregister(serivceName string, target string, address string, port int) (*stela.Service, error)
	RotateSRVPriorities(serviceName string, addingService bool) error
	Discover(serviceName string) ([]*stela.Service, error)
	DiscoverOne(serviceName string) (*stela.Service, error)

	// Raft FSM Interface
	WaitForLeader() error
	Join(addr string) error
	Remove(addr string) error
	Apply(*raft.Log) interface{}
	Snapshot() (raft.FSMSnapshot, error)
	Restore(io.ReadCloser) error
}

type discoveryService struct {
	registeredServices map[stela.Service]*stela.Service // Map of registered services
	srvRecords         map[string]map[string][]byte     // Map of SRV records
	aRecords           map[string][]byte                // Map of A records
	srvMutex           *sync.RWMutex                    // Mutex used to lock srvRecords map
	aMutex             *sync.RWMutex                    // Mutex used to lock aRecords map
	registerMutex      *sync.RWMutex                    // Mutex used to lock registeredServices map
	raftBind           string                           // Raft binding
	raft               *raft.Raft                       // Raft
	peerJSONFileName   string
	raftDBName         string
}

// SetupSvc and raft
func SetupSvc(enableSingleNode bool, stelaPort, stelaDNSPort int, peerJSONFileName, raftAddr, raftDBName, raftDir string) (DiscoveryService, error) {
	svc, err := NewDiscoveryService(peerJSONFileName, raftDBName, raftDir, raftAddr, enableSingleNode, raft.DefaultConfig())
	if err != nil {
		return nil, err
	}

	// Setup DNS Server handlefunc to parse queries and create records based on DB stores
	dns.HandleFunc(".", handleDNSRequest(svc))

	// Setup udp and tcp DNS servers
	netTypes := [2]string{"udp", "tcp"}
	for _, netProto := range netTypes {
		port, err := portutil.Verify(netProto, stelaDNSPort)
		if err != nil {
			return nil, err
		}

		go func(netProto string, port int) {
			addr := portutil.JoinHostPort("127.0.0.1", port)
			fmt.Println("DNS starting at 127.0.0.1 -p", port, "Potocol:", netProto)
			server := &dns.Server{Addr: addr, Net: netProto, TsigSecret: nil}
			server.ListenAndServe()
		}(netProto, port)
	}

	// Create basic endpoints for notifying of multicast and verfication of stela
	// Address must be localhost address
	mux := newCustomMux()

	// Add additional handlers
	go func(stelaAddress string) {
		mux.Handle("/registerservice", handleRegisterService(svc))
		mux.Handle("/deregisterservice", handleDeregisterService(svc))
		mux.Handle("/roundrobin", handleRoundRobin(svc))
		mux.Handle("/discover", handleDiscover(svc))
		mux.Handle("/discoverone", handleDiscoverOne(svc))
		mux.Handle("/join", handleJoin(svc))
		mux.Handle("/remove", handleRemove(svc))
		mux.Handle("/verify", handleVerify(svc))

		log.Fatal(http.ListenAndServe(stelaAddress, mux))
	}(portutil.JoinHostPort("", stelaPort))

	return svc, nil
}

// NewDiscoveryService setups the bolt db database
func NewDiscoveryService(peerJSONFileName, raftDBName, raftDir, raftAddr string, enableSingle bool, raftConfig *raft.Config) (DiscoveryService, error) {
	// Setup Service Discovery
	var svc DiscoveryService
	svc = new(discoveryService)

	// Setup maps
	d := svc.(*discoveryService)
	if d.srvRecords == nil {
		d.srvRecords = make(map[string]map[string][]byte)
	}

	d.peerJSONFileName = peerJSONFileName
	d.raftDBName = raftDBName

	if d.aRecords == nil {
		d.aRecords = make(map[string][]byte)
	}

	if d.srvMutex == nil {
		d.srvMutex = new(sync.RWMutex)
	}

	if d.aMutex == nil {
		d.aMutex = new(sync.RWMutex)
	}

	d.raftBind = raftAddr

	// Setup raft consensus
	err := d.setupRaft(raftDir, enableSingle, raftConfig)
	if err != nil {
		return nil, err
	}

	return svc, nil
}

// Register adds a service (A records or SRV) to the boltdb and
// sets a timer for health checking
func (d *discoveryService) Register(serviceName string, target string, address string, port int) (*stela.Service, error) {
	if d.registeredServices == nil {
		d.registeredServices = make(map[stela.Service]*stela.Service)
	}

	if d.registerMutex == nil {
		d.registerMutex = new(sync.RWMutex)
	}

	//fmt.Println("Registering Test", serviceName)
	s := new(stela.Service)
	s.Name = serviceName
	s.Target = target
	s.Address = address
	s.Port = port
	sKey := stela.Service{Name: s.Name, Target: s.Target, Address: s.Address, Port: s.Port}

	// Create srv
	srv := s.NewSRV()

	// Create A
	a := s.NewA(net.ParseIP(s.Address))

	// Load Balance each registered SRV by incrementing their priority
	// Do this before new SRV is added
	d.RotateSRVPriorities(serviceName, true)

	// store srv to boltdb
	err := d.storeRR(srv)
	if err != nil {
		return nil, err
	}

	// store a record to boltdb
	err = d.storeRR(a)
	if err != nil {
		return nil, err
	}

	d.registerMutex.Lock()
	defer d.registerMutex.Unlock()
	if d.registeredServices[sKey] == nil {
		fmt.Println("Creating register channel for service", serviceName)
		d.registeredServices[sKey] = s

		go func() {
			// fmt.Println("Starting goroutine waiting for", sKey, " to re-register or time out")
			for {
				timeout := time.NewTimer(5 * time.Second)

				select {
				case <-s.RegisterCh():
					timeout.Stop()
					continue
				case <-s.DeregisterCh():
					timeout.Stop()
					return
				case <-timeout.C:
					//TODO make sure this is applied to raft consensus
					timeout.Stop()
					s.StopRegistering()

					// Create command to pass to raft to deregister
					c := new(raftCommand)
					c.Operation = deregisterOperation
					c.Service = s
					cmd, _ := json.Marshal(c)

					// Set the raft apply and it will call svc.Register when consensus is met
					raftFuture := d.raft.Apply(cmd, raftTimeout)
					raftFuture.Error()

					return
				}
			}
		}()
	} else {
		s := d.registeredServices[sKey]
		s.KeepRegistered() // Sends struct{} to registerCh to keep it registered
	}

	return s, nil
}

// Deregister will remove an SRV record from the boltdb and echoes the removed SRV as Service struct
// TODO clean up registeredServices map
func (d *discoveryService) Deregister(serviceName string, target string, address string, port int) (*stela.Service, error) {
	fmt.Println("Deregistering", serviceName)
	s := new(stela.Service)
	s.Name = serviceName
	s.Address = address
	s.Target = target
	s.Port = port

	d.registerMutex.Lock()
	// Stop registration timeout
	if d.registeredServices[*s] == nil {
		d.registerMutex.Unlock()
		return nil, errors.New("Service is not registered.")
	}
	d.registeredServices[*s].StopRegistering()
	delete(d.registeredServices, *s)
	d.registerMutex.Unlock()

	// create srv
	srv := s.NewSRV()

	//create A
	a := s.NewA(net.ParseIP(s.Address))

	// delete srv from map
	err := d.deleteRR(srv)
	if err != nil {
		return nil, err
	}

	// delete a from map
	err = d.deleteRR(a)
	if err != nil {
		return nil, err
	}

	// Reorganize SRV priorites
	mapKey, err := generateServiceKey(serviceName, dns.TypeSRV)
	if err != nil {
		return nil, err
	}

	// Look up all SRVs in the bucket and reset priority
	var srvs []*srvWithKey

	for k, v := range d.srvRecords[mapKey] {
		// create rr from value
		rr, err := dns.NewRR(string(v))
		if err != nil {
			return nil, err
		}

		// Update SRV priority
		srvPlus := new(srvWithKey)
		srvPlus.srv = rr.(*dns.SRV)
		srvPlus.key = []byte(k)
		srvs = append(srvs, srvPlus)
	}

	// Sort by priority
	sort.Sort(byPrioritySRVWithKey(srvs))

	for i, srvPlus := range srvs {
		srvPlus.srv.Priority = uint16(i)
		d.srvMutex.Lock()
		d.srvRecords[mapKey][string(srvPlus.key)] = []byte(srvPlus.srv.String())
		d.srvMutex.Unlock()
	}

	return s, nil
}

// RotateSRVPriorities searches all SRV keys in the bucketSRVRecords for the rrkey passed in
// then reorganizes all registered services priority. It will modulate based on if we
// need to prepare for a new service being added
func (d *discoveryService) RotateSRVPriorities(serviceName string, addingService bool) error {
	key, err := generateServiceKey(serviceName, dns.TypeSRV)
	if err != nil {
		return err
	}

	// Look up all SRVs to get length
	srvs, err := d.getAllSRVForBucket(key)
	var srvLength int
	if err != nil {
		return nil
	}
	srvLength = len(srvs)

	// Use length to modulate priority
	var mod int
	if addingService {
		mod = srvLength + 1
	} else {
		mod = srvLength
	}

	mapKey, err := generateServiceKey(serviceName, dns.TypeSRV)
	if err != nil {
		return err
	}
	// m := d.srvRecords[mapKey]

	for k, v := range d.srvRecords[mapKey] {
		// create rr from value
		rr, err := dns.NewRR(string(v))
		if err != nil {
			return err
		}

		// Update SRV priority
		srv := rr.(*dns.SRV)
		srv.Priority = uint16((int(srv.Priority) + 1) % mod)

		// Write the new SRV value to the key
		d.srvMutex.Lock()
		d.srvRecords[mapKey][k] = []byte(srv.String())
		d.srvMutex.Unlock()
	}

	return nil
}

// Miekg DNS service
// Parses the messages and adds answers
func (d *discoveryService) ParseDNSQuery(m *dns.Msg) error {
	for _, q := range m.Question {
		// generate the key that was used to store the record
		key, err := generateServiceKey(q.Name, q.Qtype)
		if err != nil {
			return err
		}

		fmt.Println("DNS Query", key, q.Qtype)

		// Only handle TypeSRV
		switch q.Qtype {
		case dns.TypeSRV:
			// get the resource record
			records, err := d.getAllSRVForBucket(key)
			if err != nil {
				return err
			}

			// iterate of all records and return them
			for _, rr := range records {
				if rr.Header().Name == q.Name {
					m.Answer = append(m.Answer, rr)
				}
			}
		case dns.TypeA:
			rr, err := d.getARecord(key)
			if err != nil {
				return err
			}

			if rr.Header().Name == q.Name {
				m.Answer = append(m.Answer, rr)
			}
		}
	}

	return nil
}

func (d *discoveryService) Discover(serviceName string) ([]*stela.Service, error) {
	key, err := generateServiceKey(serviceName, dns.TypeSRV)
	if err != nil {
		return nil, err
	}

	srvs, err := d.getAllSRVForBucket(key)
	if err != nil {
		return nil, err
	}

	var services []*stela.Service

	for _, srv := range srvs {
		service, err := d.serviceFromSRV(srv)
		if err != nil {
			return nil, err
		}

		services = append(services, service)
	}

	return services, nil
}

func (d *discoveryService) DiscoverOne(serviceName string) (*stela.Service, error) {
	key, err := generateServiceKey(serviceName, dns.TypeSRV)
	if err != nil {
		return nil, err
	}

	// Get all SRVs
	srvs, err := d.getAllSRVForBucket(key)
	if err != nil {
		return nil, err
	}

	if len(srvs) == 0 {
		return nil, fmt.Errorf("No services registered for: %v", serviceName)
	}

	// Get Service with lowest priority
	sort.Sort(byPrioritySRV(srvs))

	service, err := d.serviceFromSRV(srvs[0])
	if err != nil {
		return nil, err
	}

	// RotateSRVPriorities
	d.RotateSRVPriorities(serviceName, false)

	return service, nil
}

func (d *discoveryService) serviceFromSRV(srv *dns.SRV) (*stela.Service, error) {
	s := new(stela.Service)
	s.Name = strings.TrimSuffix(srv.Header().Name, ".")
	s.Target = strings.TrimSuffix(srv.Target, ".")
	s.Priority = int(srv.Priority)
	s.Port = int(srv.Port)

	key, err := generateServiceKey(s.Target, dns.TypeA)
	if err != nil {
		return nil, err
	}

	rr, err := d.getARecord(key)
	if err != nil {
		return nil, err
	}

	address := rr.(*dns.A).A.String()
	s.Address = address

	return s, nil
}

// storeRR stores the SRV or A record in a bolt bucket
func (d *discoveryService) storeRR(rr dns.RR) error {
	var rrkey string
	var err error

	// If it's an SRV we need to create a bucket inside the bucketSRVRecords bucket
	switch rr.Header().Rrtype {
	case dns.TypeSRV:
		mapKey, err := generateServiceKey(rr.Header().Name, rr.Header().Rrtype)
		if err != nil {
			return err
		}

		srv := rr.(*dns.SRV)
		rrkey, err = generateServiceKey(srv.Target, srv.Port)
		if err != nil {
			return err
		}

		// Store rr in srv map
		if d.srvRecords[mapKey] == nil {
			d.srvRecords[mapKey] = make(map[string][]byte)
		}

		d.srvMutex.Lock()
		d.srvRecords[mapKey][rrkey] = []byte(rr.String())
		d.srvMutex.Unlock()
	case dns.TypeA:
		// generate key
		rrkey, err = generateServiceKey(rr.Header().Name, rr.Header().Rrtype)
		if err != nil {
			return err
		}

		// Store rr in srv map
		d.aMutex.Lock()
		d.aRecords[rrkey] = []byte(rr.String())
		d.aMutex.Unlock()
	}

	return nil
}

// deleteRR removes resource records of type SRV or A
func (d *discoveryService) deleteRR(rr dns.RR) error {
	var mapKey, rrkey string

	// err := d.boltdb.Update(func(tx *bolt.Tx) error {
	// 	var b *bolt.Bucket
	var err error

	switch rr.Header().Rrtype {
	case dns.TypeSRV:
		mapKey, err = generateServiceKey(rr.Header().Name, rr.Header().Rrtype)
		if err != nil {
			return err
		}
		srv := rr.(*dns.SRV)
		rrkey, err = generateServiceKey(srv.Target, srv.Port)
		if err != nil {
			return err
		}

		d.srvMutex.Lock()
		delete(d.srvRecords[mapKey], rrkey)
		d.srvMutex.Unlock()
	case dns.TypeA:
		// generate key
		rrkey, err = generateServiceKey(rr.Header().Name, rr.Header().Rrtype)
		if err != nil {
			return err
		}

		d.aMutex.Lock()
		delete(d.aRecords, rrkey)
		d.aMutex.Unlock()
	}

	return nil
}

// getARecord: Returns a single Resource Record
// from the bucketARecords boltdb bucket
// the key is generated from generateServiceKey()
func (d *discoveryService) getARecord(key string) (dns.RR, error) {
	// var rr dns.RR
	var v []byte

	d.aMutex.RLock()
	v = d.aRecords[key]
	d.aMutex.RUnlock()

	rr, err := dns.NewRR(string(v))
	if err != nil {
		return nil, err
	}
	if rr == nil {
		return nil, fmt.Errorf("getRR: Error creating rr from string value: %s, key: %v", string(v), key)
	}

	return rr, nil
}

func (d *discoveryService) getAllSRVForBucket(key string) ([]*dns.SRV, error) {
	var rrSlice []*dns.SRV

	d.srvMutex.RLock()
	for _, v := range d.srvRecords[key] {
		// create rr from value
		rr, err := dns.NewRR(string(v))
		if err != nil {
			return nil, err
		}
		if rr == nil {
			return nil, fmt.Errorf("getAllSRVForBucket: Error creating rr from string value: %s", string(v))
		}
		srv := rr.(*dns.SRV)

		rrSlice = append(rrSlice, srv)
	}
	d.srvMutex.RUnlock()

	return rrSlice, nil
}

// generateServiceKey: Returns a reverse domain string used
// as a key from the serviceName and resource record type
func generateServiceKey(serviceName string, rrType uint16) (string, error) {
	labelCount, ok := dns.IsDomainName(serviceName)
	if ok {
		labels := dns.SplitDomainName(serviceName)

		// reverse strings for reverse domain lookup
		var tempLabel string
		for i := 0; i < len(labels); i++ {
			tempLabel = labels[i]
			labels[i] = labels[labelCount-1]
			labels[labelCount-1] = tempLabel
		}

		key := strings.Join(labels, ".")
		key = strings.Join([]string{key, strconv.Itoa(int(rrType))}, "_")

		return key, nil
	}

	return "", errors.New("generateServiceKey: Couldn't generate a key")
}
