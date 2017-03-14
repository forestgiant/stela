package node

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/forestgiant/disco/multicast"
)

const testMulticastAddress = "[ff12::9000]:30002"

func Test_init(t *testing.T) {
	n := &Node{}
	n.init()

	if n.registerCh == nil {
		t.Fatal("registerCh should not be nil after init() method is called")
	}
}

// func TestString(t *testing.T) {
// 	localIPv4 := localIPv4()
// 	localIPv6 := localIPv6()

// 	var tests = []struct {
// 		n         *Node
// 		s         []string
// 		shouldErr bool
// 	}{
// 		{&Node{}, []string{""}, true},
// 		{&Node{}, []string{"IPv4: <nil>", "IPv6: <nil>", "Values: map[]"}, false},
// 		{&Node{ipv4: localIPv4}, []string{fmt.Sprintf("IPv4: %s", localIPv4), "IPv6: <nil>", "Values: map[]"}, false},
// 		{&Node{ipv6: localIPv6}, []string{"IPv4: <nil>", fmt.Sprintf("IPv6: %s", localIPv6), "Values: map[]"}, false},
// 		{&Node{ipv4: localIPv4, ipv6: localIPv6}, []string{fmt.Sprintf("IPv4: %s", localIPv4), fmt.Sprintf("IPv6: %s", localIPv6), "Values: map[]"}, false},
// 		{&Node{Values: map[string]string{"foo": "v1", "bar": "v2"}, ipv4: localIPv4, ipv6: localIPv6},
// 			[]string{fmt.Sprintf("IPv4: %s", localIPv4), fmt.Sprintf("IPv6: %s", localIPv6), "Values:", "foo:v1", "bar:v2"}, false},
// 	}

// 	for _, test := range tests {
// 		actual := fmt.Sprint(test.n)
// 		if !test.shouldErr {
// 			for _, s := range test.s {
// 				if !strings.Contains(actual, s) {
// 					t.Errorf("Stringer failed. Received %s, should be: %s", actual, test.s)
// 				}
// 			}
// 		} else {
// 			for _, s := range test.s {
// 				if !strings.Contains(actual, s) {
// 					t.Errorf("Stringer should fail. Received %s, should be: %s", actual, test.s)
// 				}
// 			}
// 		}
// 	}
// }

func TestEncodeDecode(t *testing.T) {
	var tests = []struct {
		n *Node
	}{
		{&Node{}},
		{&Node{
			ipv4:    net.ParseIP("127.0.0.1"),
			ipv6:    net.IPv6loopback,
			Payload: []byte("payload"),
		}},
	}

	for i, test := range tests {
		bytes := test.n.Encode()

		decodedN, err := DecodeNode(bytes)
		if err != nil {
			t.Fatal(err)
		}

		if !test.n.Equal(decodedN) {
			t.Fatalf("Test %d failed. Nodes should be equal after decode. \n Received: %s, \n Expected: %s \n", i, decodedN, test.n)
		}
	}
}

func TestRegisterCh(t *testing.T) {
	n := &Node{}
	if n.RegisterCh() != n.registerCh {
		t.Fatal("RegisterCh() method should return n.registerCh")
	}
}

func TestKeepRegistered(t *testing.T) {
	n := &Node{}
	closeCh := make(chan struct{})
	errCh := make(chan error)
	go func() {
		defer close(closeCh)
		select {
		case <-n.RegisterCh():
		case <-time.After(100 * time.Millisecond):
			errCh <- errors.New("Test_register timed out")
		}
	}()

	n.KeepRegistered()

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

func TestIPGetters(t *testing.T) {
	localIPv4 := localIPv4()
	localIPv6 := localIPv6()

	var tests = []struct {
		n    *Node
		ipv4 net.IP
		ipv6 net.IP
	}{
		{&Node{}, nil, nil},
		{&Node{}, []byte{}, []byte{}},
		{&Node{ipv4: localIPv4}, localIPv4, []byte{}},
		{&Node{ipv6: localIPv6}, []byte{}, localIPv6},
		{&Node{ipv4: localIPv4, ipv6: localIPv6}, localIPv4, localIPv6},
	}

	for _, test := range tests {
		ipv4 := test.n.IPv4()
		ipv6 := test.n.IPv6()
		if !test.n.ipv4.Equal(ipv4) || !test.n.ipv6.Equal(ipv6) {
			t.Error("IP Getter failed", test.n, ipv4, ipv6)
		}
	}
}

func TestEqual(t *testing.T) {
	var tests = []struct {
		a        *Node
		b        *Node
		expected bool
	}{
		{&Node{}, &Node{}, true},
		{&Node{Payload: []byte("foo, bar")}, &Node{Payload: []byte("foo, bar")}, true},
		{&Node{Payload: []byte("foo, bar")}, &Node{}, false},
	}

	for _, test := range tests {
		actual := test.a.Equal(test.b)
		if actual != test.expected {
			t.Errorf("Compare failed %v should equal %v.", test.a, test.b)
		}
	}
}

func TestMulticast(t *testing.T) {
	var tests = []struct {
		n                *Node
		multicastAddress string
		shouldErr        bool
	}{
		{&Node{SendInterval: 1 * time.Second}, "[ff12::9000]:21090", false},
		{&Node{Payload: []byte("foo, bar"), SendInterval: 1 * time.Second}, "[ff12::9000]:21090", false},
		{&Node{Payload: []byte("somekey, somevalue"), SendInterval: 1 * time.Second}, "[ff12::9000]:21090", false},
		{&Node{Payload: []byte("another payload"), SendInterval: 1 * time.Second}, ":21090", true},
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	wg := &sync.WaitGroup{}

	// Perform our test in a new goroutine so we don't block
	go func() {
		// Listen for nodes
		listener := &multicast.Multicast{Address: testMulticastAddress}
		results, err := listener.Listen(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Check if nodes received are the nodes we are testing
		go func() {
			for {
				select {
				case resp := <-results:
					buffer := bytes.NewBuffer(resp.Payload)

					rn := &Node{}
					dec := gob.NewDecoder(buffer)
					if err := dec.Decode(rn); err != nil {
						errCh <- err
					}

					// Check if any nodes coming in are the ones we are waiting for
					for _, test := range tests {
						if rn.Equal(test.n) {
							test.n.Stop() // stop the node from multicasting
							wg.Done()
						}
					}
				// case <-time.After(100 * time.Millisecond):
				// 	errCh <- errors.New("TestMulticast timed out")
				case <-ctx.Done():
					return
				case <-listener.Done():
					return
				}
			}
		}()

		go func() {
			for _, test := range tests {
				// Add to the WaitGroup for each test that should pass and add it to the nodes to verify
				if !test.shouldErr {
					wg.Add(1)

					if err := test.n.Multicast(ctx, test.multicastAddress); err != nil {
						t.Fatal("Multicast error", err)
					}
				} else {
					if err := test.n.Multicast(ctx, test.multicastAddress); err == nil {
						t.Fatal("Multicast of node should fail", err)
					}
				}
			}
		}()

		wg.Wait()
		listener.Stop()
		cancelFunc()
	}()

	// Block until the ctx is canceled or we receive an error, such as a timeout
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			t.Fatal(err)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("TestMulticast timed out")
			return
		}
	}
}

func TestStop(t *testing.T) {
	n := &Node{Payload: []byte("foo, bar")}

	if err := n.Multicast(context.TODO(), testMulticastAddress); err != nil {
		t.Fatal("Multicast error", err)
	}
	time.AfterFunc(100*time.Millisecond, func() { n.Stop() })
	timeout := time.AfterFunc(200*time.Millisecond, func() { t.Fatal("TestStopChan timedout") })

	// Block until the stopCh is closed
	for {
		select {
		case <-n.Done():
			timeout.Stop() // cancel the timeout
			return
		}
	}
}

func TestLocalIPv4(t *testing.T) {
	l := localIPv4()
	_, err := net.InterfaceAddrs()
	if err != nil {
		// if there was an error with the interface
		// then it should be loopback
		if !l.IsLoopback() {
			t.Error("LocalIP should be loopback")
		}
	}
}

func TestLocalIPv6(t *testing.T) {
	l := localIPv6()
	_, err := net.InterfaceAddrs()
	if err != nil {
		// if there was an error with the interface
		// then it should be loopback
		if !l.IsLoopback() {
			t.Error("LocalIP should be loopback")
		}
	}
}
