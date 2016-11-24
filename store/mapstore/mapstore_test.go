package mapstore

import (
	"fmt"
	"reflect"
	"testing"

	"sync"

	"gitlab.fg/go/stela"
)

func TestDiscover(t *testing.T) {
	m := MapStore{}

	var tests = []struct {
		service    *stela.Service
		shouldFail bool
	}{
		{&stela.Service{
			Name:    "test.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		}, false},
		// Don't allow duplicates
		{&stela.Service{
			Name:    "test.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		}, true},
		{&stela.Service{
			Name:    "test.service.fg",
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

	expectedServices := make(map[string][]stela.Service)
	for _, test := range tests {
		if err := m.Register(test.service); test.shouldFail != (err != nil) {
			t.Fatal(err)
		}

		// Store the successful services into our own expected map
		if !test.shouldFail {
			expectedServices[test.service.Name] = append([]stela.Service{*test.service}, expectedServices[test.service.Name]...)
		}
	}

	// Verify the services map is what we expect
	if !reflect.DeepEqual(expectedServices, m.services) {
		t.Fatalf("Registered services failed. Expected %v, Received %v", expectedServices, m.services)
	}
}

func Test_rotateServices(t *testing.T) {
	m := MapStore{}

	var tests = []struct {
		service          stela.Service
		expectedPriority int
	}{
		{stela.Service{
			Name:    "priority.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		}, 2},
		{stela.Service{
			Name:    "priority.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.2",
			Port:    9000,
		}, 1},
		{stela.Service{
			Name:    "priority.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.3",
			Port:    9000,
		}, 0},
	}
	var expectedServices []stela.Service
	for _, test := range tests {
		// First we need to register them
		if err := m.Register(&test.service); err != nil {
			t.Fatal(err)
		}

		expectedServices = append(expectedServices, test.service)
	}

	// Loop through services registered and make sure their priorities match
	for i, s := range m.services[tests[0].service.Name] {
		ts := tests[i]
		if s.Equal(&ts.service) {
			if s.Priority != ts.expectedPriority {
				t.Fatalf("Test_rotateServices: Priorites didn't match. Expected: %v, Received: %v", ts.expectedPriority, s.Priority)
			}
		}
	}
}

func TestSubscribe(t *testing.T) {
	m := MapStore{}
	c := &stela.Client{}

	var testServices = []*stela.Service{
		&stela.Service{
			Name:    "subscribe.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		},
		&stela.Service{
			Name:    "subscribe.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.2",
			Port:    9001,
		},
		&stela.Service{
			Name:    "subscribe.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.3",
			Port:    9002,
		},
	}

	// Subscribe to service
	if err := m.Subscribe(testServices[0].Name, c); err != nil {
		t.Fatal(err)
	}

	var registered []*stela.Service
	var deregistered []*stela.Service

	// Listen for registered services
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wg.Done()
		for {
			select {
			case s := <-c.SubscribeCh():
				fmt.Println(s, s.Action)
				switch s.Action {
				case stela.RegisterAction:
					registered = append(registered, s)
				case stela.DeregisterAction:
					deregistered = append(deregistered, s)
				}
			}
		}
	}()
	wg.Wait()

	for _, s := range testServices {
		// Register services
		fmt.Println("reg", s)
		m.Register(s)
	}

	for _, s := range testServices {
		// Deregister services
		m.Deregister(s)
	}

	testSubscribe := func(serviceSlice []*stela.Service) {
		if len(serviceSlice) != len(testServices) {
			t.Fatalf("Subscribe didn't match testServices. Received: %d, Expected: %d", len(serviceSlice), len(testServices))
		}

		for i, rs := range serviceSlice {
			s := testServices[i]
			if rs.Equal(s) {
				fmt.Println("service equal", i)
				// Remove from slice
				// serviceSlice = append(serviceSlice[:i], serviceSlice[i+1:]...)
			}
		}

		// if len(serviceSlice) != 0 {
		// 	t.Fatalf("Subscribe slice should be 0. Received: %d", len(serviceSlice))
		// }
	}

	testSubscribe(registered)
	testSubscribe(deregistered)
}
