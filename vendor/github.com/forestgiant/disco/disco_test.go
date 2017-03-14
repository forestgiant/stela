package disco

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/forestgiant/disco/node"
)

var testMulticastAddress = "[ff12::9000]:30000"

func Test_register(t *testing.T) {
	d := &Disco{}
	n := &node.Node{}
	r := make(chan *node.Node)
	closeCh := make(chan struct{})
	errCh := make(chan error)
	go func() {
		defer close(closeCh)
		select {
		case <-r:
			// Make sure we have 1 member
			if len(d.Members()) != 1 {
				t.Errorf("TestDeregister: One node should be registered. Received: %b, Should be: %b \n",
					len(d.Members()), 0)
			}

			// Now deregister and make sure we have 0 members
			d.deregister(n)
			if len(d.Members()) != 0 {
				t.Errorf("TestDeregister: All nodes should be deregistered. Received: %b, Should be: %b \n",
					len(d.Members()), 0)
			}
		case <-time.After(100 * time.Millisecond):
			errCh <- errors.New("Test_register timed out")
		}
	}()

	d.register(r, n)

	// Block until closeCh is closed on a timed out happens
	for {
		select {
		case <-closeCh:
			return
		case err := <-errCh:
			t.Fatal(err)
		}
	}
}

func TestDiscover(t *testing.T) {
	var tests = []struct {
		n         *node.Node
		shouldErr bool
	}{
		{&node.Node{Payload: []byte("Discover")}, false},
		{&node.Node{SendInterval: 2 * time.Second}, false},
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc() // stop disco
	wg := &sync.WaitGroup{}
	d := &Disco{}

	discoveredChan, err := d.Discover(ctx, testMulticastAddress)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		// Select will block until a result comes in
		for {
			select {
			case rn := <-discoveredChan:
				for _, test := range tests {
					if rn.Equal(test.n) {
						test.n.Stop() // stop the node from multicasting
						wg.Done()
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Multicast nodes so they can be discovered
	for _, test := range tests {
		// Add to the WaitGroup for each test that should pass and add it to the nodes to verify
		if !test.shouldErr {
			wg.Add(1)

			if err := test.n.Multicast(ctx, testMulticastAddress); err != nil {
				t.Fatal("Multicast error", err)
			}
		} else {
			if err := test.n.Multicast(ctx, testMulticastAddress); err == nil {
				t.Fatal("Multicast of node should fail", err)
			}
		}

	}

	wg.Wait()
}

func TestDiscoverSameNode(t *testing.T) {
	var tests = []struct {
		n         *node.Node
		shouldErr bool
	}{
		{&node.Node{Payload: []byte("DiscoverSameNode")}, false},
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc() // stop disco
	wg := &sync.WaitGroup{}
	d := &Disco{}
	discoveredChan, err := d.Discover(ctx, testMulticastAddress)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		// Select will block until a result comes in
		for {
			select {
			case rn := <-discoveredChan:
				for _, test := range tests {
					if rn.Equal(test.n) {
						test.n.Stop() // stop the node from multicasting
						wg.Done()
					} else {
						fmt.Println("not equal")
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Multicast nodes so they can be discovered
	for _, test := range tests {
		// Add to the WaitGroup for each test that should pass and add it to the nodes to verify
		if !test.shouldErr {
			wg.Add(1)

			if err := test.n.Multicast(ctx, testMulticastAddress); err != nil {
				t.Fatal("Multicast error", err)
			}
		} else {
			if err := test.n.Multicast(ctx, testMulticastAddress); err == nil {
				t.Fatal("Multicast of node should fail", err)
			}
		}

	}

	wg.Wait()
}
