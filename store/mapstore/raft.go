package mapstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/raftutil"
)

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

// Join the mapstore's raft
func (m *MapStore) Join(addr string) error {
	return nil
}

// Remove from mapstore's raft
func (m *MapStore) Remove(addr string) error {
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
