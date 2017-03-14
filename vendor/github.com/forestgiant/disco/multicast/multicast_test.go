package multicast

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

var testMulticastAddress = "[ff12::9000]:30001"

func TestListen(t *testing.T) {
	listener := &Multicast{Address: testMulticastAddress}
	respCh, err := listener.Listen(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	sender := &Multicast{Address: testMulticastAddress}
	if err := sender.Send(context.TODO(), 0, []byte{}); err != nil {
		t.Fatal("Multicast Send should fail", err)
	}

	for {
		select {
		case <-respCh:
			listener.Done()
			sender.Done()
			return
		}
	}

}

func TestSend(t *testing.T) {
	var tests = []struct {
		m         *Multicast
		delay     time.Duration
		payload   []byte
		shouldErr bool
	}{
		{&Multicast{}, 0, nil, true},
		{&Multicast{Address: testMulticastAddress}, 3, []byte("Hello TestSendAndListen"), false},
		{&Multicast{Address: testMulticastAddress}, 0, []byte("Say hello again"), false},
		{&Multicast{Address: testMulticastAddress}, 1, []byte("123412341234"), false},
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	errCh := make(chan error, 1)

	// Don't block for test
	go func() {
		listener := &Multicast{Address: testMulticastAddress}

		// Create a listener
		respCh, err := listener.Listen(ctx)
		if err != nil {
			errCh <- err
		}

		// Check if the resp sent over the response channel matches the test's payload
		go func() {
			for {
				select {
				case resp := <-respCh:
					// Check the slice of test to see if the response equals what is expected
					for _, test := range tests {
						if bytes.Equal(resp.Payload, test.payload) {
							test.m.Stop()
							wg.Done()
						}
					}
				case <-ctx.Done():
					return
				case <-listener.Done():
					return
				}
			}
		}()

		go func() {
			for _, test := range tests {
				if !test.shouldErr {
					wg.Add(1)

					if err := test.m.Send(ctx, test.delay, test.payload); err != nil {
						t.Fatal("Multicast Send should fail", err)
					}

				} else {
					if err := test.m.Send(ctx, test.delay, test.payload); err == nil {
						t.Fatal("Multicast Send should fail", err)
					}
				}

				// Check for any send errors
				go func(test struct {
					m         *Multicast
					delay     time.Duration
					payload   []byte
					shouldErr bool
				}) {
					select { // Check to see if it errored
					case <-test.m.Done():
						if test.m.SendErr() != nil {
							errCh <- test.m.SendErr()
						}
					}
				}(test)
			}
		}()

		wg.Wait() // Block until all test multicasts are stopped
		listener.Stop()
		cancel()
	}()

	// Block until the ctx is canceled or we receive an error, such as a timeout
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				t.Fatal("err during Send()", err)
			}
			return
		case <-time.After(5000 * time.Millisecond):
			t.Fatal("TestSendAndListen timed out")
			return
		}
	}
}

func TestListenCtxDone(t *testing.T) {
	// Create a listener
	ctx, cancel := context.WithCancel(context.Background())
	listener := &Multicast{Address: testMulticastAddress}
	_, err := listener.Listen(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Test context Done()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			}
		}
	}()
	cancel()
}

func TestListenStop(t *testing.T) {
	// Create a listener
	listener := &Multicast{Address: testMulticastAddress}
	_, err := listener.Listen(context.TODO())
	if err != nil {
		t.Fatal(err)
	}

	// Test listener Done()
	go func() {
		for {
			select {
			case <-listener.Done():
				return
			}
		}
	}()
	listener.Stop()
}

func TestSendStop(t *testing.T) {
	m := &Multicast{Address: testMulticastAddress}

	payload := []byte("Hello TestStop")
	if err := m.Send(context.TODO(), 3, payload); err != nil {
		t.Fatal("Send error", err)
	}
	time.AfterFunc(100*time.Millisecond, func() { m.Stop() })
	timeout := time.AfterFunc(200*time.Millisecond, func() { t.Fatal("TestStopChan timedout") })

	// Block until the stopCh is closed
	for {
		select {
		case <-m.Done():
			timeout.Stop() // cancel the timeout
			if m.SendErr() != nil {
				t.Fatal("m.Err():", m.SendErr())
			}
			return
		}
	}
}

func TestCtxCancelFunc(t *testing.T) {
	m := &Multicast{Address: testMulticastAddress}

	payload := []byte("Hello TestCtxCancelFunc")
	ctx, cancel := context.WithCancel(context.Background())
	if err := m.Send(ctx, 3, payload); err != nil {
		t.Fatal("Send error", err)
	}
	time.AfterFunc(100*time.Millisecond, func() { cancel() })
	timeout := time.AfterFunc(200*time.Millisecond, func() { t.Fatal("TestStopChan timedout") })

	// Block until the stopCh is closed
	for {
		select {
		case <-ctx.Done():
			timeout.Stop() // cancel the timeout
			return
		}
	}
}
