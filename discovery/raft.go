package discovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gitlab.fg/go/stela"

	"github.com/forestgiant/raftutil"
	"github.com/hashicorp/raft"
	"github.com/hashicorp/raft-boltdb"
)

const (
	retainSnapshotCount = 2
)

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

func (d *discoveryService) setupRaft(raftDir string, enableSingle bool, raftConfig *raft.Config) error {
	// Make sure raftDir is a valid directory
	if raftDir == "" {
		return errors.New("raftDir can't be blank")
	} else if _, err := os.Stat(raftDir); os.IsNotExist(err) {
		return errors.New("raftDir does not exist")
	}

	// Remove all Raft Files at startup
	err := raftutil.RemoveRaftFiles(raftDir)
	if err != nil {
		return err
	}

	// Setup Raft configuration.
	config := raftConfig

	// Check for any existing peers.
	peers, err := raftutil.ReadPeersJSON(filepath.Join(raftDir, d.peerJSONFileName))
	if err != nil {
		return err
	}

	// Allow the node to entry single-mode, potentially electing itself, if
	// explicitly enabled and there is only 1 node in the cluster already.
	if enableSingle && len(peers) <= 1 {
		fmt.Println("enable single node mode")
		raftutil.Bootstrap(config)
	}

	// Setup Raft communication.
	transport, err := raftutil.TCPTransport(d.raftBind)

	// Create peer storage.
	peerStore := raft.NewJSONPeers(raftDir, transport)

	// Create the snapshot store. This allows the Raft to truncate the log.
	snapshots, err := raft.NewFileSnapshotStore(raftDir, retainSnapshotCount, os.Stderr)
	if err != nil {
		return fmt.Errorf("file snapshot store: %s", err)
	}

	// Create the log store and stable store.
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, d.raftDBName))
	if err != nil {
		return fmt.Errorf("new bolt store: %s", err)
	}

	// Instantiate the Raft systems.
	ra, err := raft.NewRaft(config, d, logStore, logStore, snapshots, peerStore, transport)
	if err != nil {
		return fmt.Errorf("new raft: %s", err)
	}
	d.raft = ra

	return nil
}

// Join joins a node, located at addr, to this store. The node must be ready to
// respond to Raft communications at that address.
func (d *discoveryService) Join(addr string) error {
	fmt.Printf("received join request for remote node as %s", addr)

	raftFuture := d.raft.AddPeer(addr)
	if raftFuture.Error() != nil {
		return raftFuture.Error()
	}
	fmt.Printf("node at %s joined successfully", addr)
	return nil
}

func (d *discoveryService) Remove(addr string) error {
	fmt.Printf("received remove request for remote node as %s", addr)

	raftFuture := d.raft.RemovePeer(addr)
	if raftFuture.Error() != nil {
		return raftFuture.Error()
	}
	fmt.Printf("node at %s removed successfully", addr)
	return nil
}

type fsmDiscoverResponse struct {
	results []*stela.Service
	error   error
}

type fsmResponse struct {
	result *stela.Service
	error  error
}

// Apply is used for raft consensus FSM
func (d *discoveryService) Apply(l *raft.Log) interface{} {
	var c raftCommand
	if err := json.Unmarshal(l.Data, &c); err != nil {
		return err
	}

	if c.Service == nil {
		return errors.New("raftCommand.Service is nil")
	}

	switch c.Operation {
	case registerOperation:
		svcResponse, err := d.Register(c.Service.Name, c.Service.Target, c.Service.Address, c.Service.Port)
		return &fsmResponse{result: svcResponse, error: err}
	case deregisterOperation:
		svcResponse, err := d.Deregister(c.Service.Name, c.Service.Target, c.Service.Address, c.Service.Port)
		return &fsmResponse{result: svcResponse, error: err}
	case discoverOperation:
		svcResponse, err := d.Discover(c.Service.Name)
		return &fsmDiscoverResponse{results: svcResponse, error: err}
	case discoverOneOperation:
		svcResponse, err := d.DiscoverOne(c.Service.Name)
		return &fsmResponse{result: svcResponse, error: err}
	}

	return nil
}

// Snapshot is used for raft consensus FSM
func (d *discoveryService) Snapshot() (raft.FSMSnapshot, error) {
	// Clone srv and a record maps
	d.srvMutex.Lock()
	srvRecords := make(map[string]map[string][]byte)
	for k, v := range d.srvRecords {
		srvRecords[k] = v
	}
	d.srvMutex.Unlock()

	d.aMutex.Lock()
	aRecords := make(map[string][]byte)
	for k, v := range d.aRecords {
		aRecords[k] = v
	}
	d.aMutex.Unlock()

	d.registerMutex.Lock()
	registeredServices := make(map[stela.Service]*stela.Service)
	for k, v := range d.registeredServices {
		registeredServices[k] = v
	}
	d.registerMutex.Unlock()

	return &fsmSnapshot{srvRecords: srvRecords, aRecords: aRecords, registeredServices: registeredServices}, nil
}

// Restore is used for raft consensus FSM
func (d *discoveryService) Restore(rc io.ReadCloser) error {
	fsm := new(fsmSnapshot)
	if err := json.NewDecoder(rc).Decode(&fsm); err != nil {
		return err
	}

	// Set the state from the snapshot, no lock required according to
	// Hashicorp docs.
	d.srvRecords = fsm.srvRecords
	d.aRecords = fsm.aRecords
	d.registeredServices = fsm.registeredServices
	return nil
}

// WaitForLeader blocks until a raft leader is detected or the timeout happens
func (d *discoveryService) WaitForLeader() error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case <-tick.C:
			if d.raft.Leader() != "" {
				return nil
			}
		case <-timeout.C:
			return fmt.Errorf("WaitForLeader timed out")
		}
	}
}

type fsmSnapshot struct {
	srvRecords         map[string]map[string][]byte
	aRecords           map[string][]byte
	registeredServices map[stela.Service]*stela.Service
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
