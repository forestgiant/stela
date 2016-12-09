package api

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/net/context"

	"time"

	"gitlab.fg/go/stela"
)

func TestMain(m *testing.M) {
	kill, err := startStelaInstance(stela.DefaultStelaAddress, stela.DefaultMulticastPort)
	if err != nil {
		log.Fatal(err)
	}

	// Give the instance time to start
	time.Sleep(time.Millisecond * 50)

	t := m.Run()

	kill()
	os.Exit(t)
}

func TestRegisterAndDiscover(t *testing.T) {
	serviceName := "apitest.services.fg"
	c, err := NewClient(stela.DefaultStelaAddress, "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}

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

	expectedServices := make(map[string][]*stela.Service)
	for i, test := range tests {
		if err := c.RegisterService(test.service); test.shouldFail != (err != nil) {
			t.Fatal(i, test, err)
		}

		// Store the successful services into our own expected map
		if !test.shouldFail {
			expectedServices[test.service.Name] = append([]*stela.Service{test.service}, expectedServices[test.service.Name]...)
		}
	}

	// Now see if we can discover them
	services, err := c.Discover(serviceName)
	if err != nil {
		t.Fatal(err)
	}

	if len(services) != len(expectedServices[serviceName]) {
		t.Fatal("Discover failed")
	}

	// Compare the discovered services
	equalServices(t, services, expectedServices[serviceName])

	// Deregister all services registered
	for _, s := range expectedServices[serviceName] {
		c.DeregisterService(s)
	}
}

func TestConnectSubscribe(t *testing.T) {
	serviceName := "testSubscribe.services.fg"

	c, err := NewClient(stela.DefaultStelaAddress, "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}

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

func startStelaInstance(stelaAddress string, multicastPort int) (kill func(), err error) {
	// Run a stela instance
	cmd := exec.Command("stela", "-address", stelaAddress, "-multicast", fmt.Sprint(multicastPort))
	if err := cmd.Start(); err != nil {
		fmt.Println("does it start?")
		return nil, err
	}

	return func() {
		if err := cmd.Process.Kill(); err != nil {
			fmt.Println("failed to kill: ", err)
		}
	}, nil
}
