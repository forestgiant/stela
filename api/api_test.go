package api

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/net/context"

	"gitlab.fg/go/stela"
)

func TestMain(m *testing.M) {
	kill, err := startStelaInstance(stela.DefaultStelaPort, stela.DefaultMulticastPort)
	if err != nil {
		log.Fatal(err)
	}

	t := m.Run()

	kill()
	os.Exit(t)
}

func TestRegisterAndDiscover(t *testing.T) {
	// Create a second stela instance
	kill, err := startStelaInstance(9001, stela.DefaultMulticastPort)
	if err != nil {
		t.Fatal(err)
	}
	defer kill()

	serviceName := "apitest.services.fg"
	c, err := NewClient(stela.DefaultStelaAddress, "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}

	c2, err := NewClient("127.0.0.1:9001", "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}

	c2Service := &stela.Service{
		Name:    serviceName,
		Target:  "jlu.macbook",
		Address: "127.0.0.1",
		Port:    9001,
	}

	// Register a single service with c2
	if err := c2.RegisterService(c2Service); err != nil {
		t.Fatal(err)
	}

	var expectedServices []*stela.Service

	// Add c2Service to expected
	expectedServices = append(expectedServices, c2Service)

	var tests = []struct {
		service    *stela.Service
		shouldFail bool
	}{
		{&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		}, false},
		// Don't allow duplicates
		{&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		}, true},
		{&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "localhost",
			Port:    80,
		}, false},
		{&stela.Service{
			Name:    "",
			Target:  "",
			Address: "",
			Port:    0,
		}, true},
	}

	for i, test := range tests {
		if err := c.RegisterService(test.service); test.shouldFail != (err != nil) {
			t.Fatal(i, test, err)
		}

		// Store the successful services into our own expected map
		if !test.shouldFail {
			expectedServices = append([]*stela.Service{test.service}, expectedServices...)
		}
	}

	// Now see if we can discover them
	services, err := c.Discover(serviceName)
	if err != nil {
		t.Fatal(err)
	}

	if len(services) != len(expectedServices) {
		t.Fatal("Discover failed")
	}

	// Compare the discovered services
	equalServices(t, services, expectedServices)

	// Deregister all services registered for client.
	// c2 instance will be killed at end of function
	for _, s := range expectedServices {
		c.DeregisterService(s)
	}
}

func TestConnectSubscribe(t *testing.T) {
	serviceName := "testSubscribe.services.fg"

	// Connect to both instances
	c, err := NewClient(stela.DefaultStelaAddress, "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}

	// Test services for c
	var testServices = []*stela.Service{
		&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		},
		&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.2",
			Port:    9001,
		},
		&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.3",
			Port:    9002,
		},
	}
	ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*100)
	waitCh := make(chan struct{})
	var count int
	callback := func(s *stela.Service) {
		count++
		// Total test services
		if count == len(testServices) {
			close(waitCh)
		}
	}

	if err := c.Subscribe(serviceName, callback); err != nil {
		t.Fatal(err)
	}

	for _, s := range testServices {
		if err := c.RegisterService(s); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for either a timeout or all the subscribed services to be read
	select {
	case <-waitCh:
		break
	case <-ctx.Done():
		if ctx.Err() != nil {
			t.Fatal("TestConnectSubscribe timed out: ", ctx.Err())
		}
	}
	if err := c.Unsubscribe(serviceName); err != nil {
		t.Fatal(err)
	}

	// Verify the map is empty
	if c.callbacks[serviceName] != nil {
		t.Fatal("callbacks map should be empty after Unsubscribe", c.callbacks)
	}

	cancel()
}

// equalServices takes two slices of stela.Service and make sure they are correct
func equalServices(t *testing.T, s1, s2 []*stela.Service) {
	// Make sure the services returned were the ones sent
	total := len(s1)
	for _, rs := range s2 {
		for _, ts := range s1 {
			if rs.Equal(ts) {
				total--
			}
		}
	}

	if total != 0 {
		t.Fatalf("Services returned did not match services in slice")
	}
}

func startStelaInstance(stelaPort, multicastPort int) (kill func(), err error) {
	// Run a stela instance
	cmd := exec.Command("stela", "-port", fmt.Sprint(stelaPort), "-multicast", fmt.Sprint(multicastPort))
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Give the instance time to start
	time.Sleep(time.Millisecond * 100)

	return func() {
		if err := cmd.Process.Kill(); err != nil {
			fmt.Println("failed to kill: ", err)
		}
	}, nil
}
