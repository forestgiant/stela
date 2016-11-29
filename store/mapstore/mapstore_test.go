package mapstore

import (
	"reflect"
	"testing"

	"golang.org/x/net/context"

	"sync"

	"time"

	"gitlab.fg/go/stela"
)

func Test_rotateServices(t *testing.T) {
	m := MapStore{}

	var tests = []struct {
		service          stela.Service
		expectedPriority int32
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

// func TestRegistration(t *testing.T) {
// 	m := MapStore{}

// 	var testServices = []*stela.Service{
// 		&stela.Service{
// 			Name:    "testregister.service.fg",
// 			Target:  "jlu.macbook",
// 			Address: "127.0.0.1",
// 			Port:    9000,
// 		},
// 		&stela.Service{
// 			Name:    "testregister.service.fg",
// 			Target:  "jlu.macbook",
// 			Address: "127.0.0.2",
// 			Port:    9001,
// 		},
// 		&stela.Service{
// 			Name:    "testregister.service.fg",
// 			Target:  "jlu.macbook",
// 			Address: "127.0.0.3",
// 			Port:    9002,
// 		},
// 	}

// 	for _, s := range testServices {
// 		// Register services
// 		m.Register(s)
// 	}

// 	// Make sure the map contains the correct services
// 	var services []*stela.Service
// 	for _, s := range m.services[testServices[0].Name] {
// 		services = append(services, &s)
// 	}
// 	if len(services) != len(testServices) {
// 		t.Fatalf("TestRegistration failed to register all services. Expected %d services, received: %d", len(testServices), len(services))
// 	}
// 	equalServices(t, testServices, services)

// 	for _, s := range testServices {
// 		// Deregister services
// 		m.Deregister(s)
// 	}

// 	if len(m.services[testServices[0].Name]) != 0 {
// 		t.Fatalf("TestRegistration failed all services should be deregistered")
// 	}
// }

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
	wg.Add(len(testServices))
	writeWaitChan := make(chan struct{})
	readWaitChan := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*100)
	readCount := 0
	go func() {
		close(readWaitChan)
		for {
			select {
			case s := <-c.SubscribeCh():
				switch s.Action {
				case stela.RegisterAction:
					registered = append(registered, s)
				case stela.DeregisterAction:
					deregistered = append(deregistered, s)
				}

				// Count how many times we've received a subscribed service
				// Multiple by two since we are registering an deregistering all testServices
				readCount++
				if readCount == len(testServices)*2 {
					close(writeWaitChan)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Block until we are ready to receive subscription information
	<-readWaitChan

	for _, s := range testServices {
		// Register services
		m.Register(s)
	}

	for _, s := range testServices {
		// Deregister services
		m.Deregister(s)
	}

	// Wait for either a timeout or all the subscribed services to be read
	select {
	case <-writeWaitChan:
		break
	case <-ctx.Done():
		if ctx.Err() != nil {
			t.Fatal("TestSubscribe timed out: ", ctx.Err())
		}
	}

	// Function to compare testServices with serviceSlice arg
	testSubscribe := func(serviceSlice []*stela.Service) {
		if len(serviceSlice) != len(testServices) {
			t.Fatalf("Subscribe didn't match testServices. Received: %d, Expected: %d", len(serviceSlice), len(testServices))
		}

		// Make sure the serviceSlice has all the testServices
		equalServices(t, testServices, serviceSlice)
	}

	// Now make sure both registered and deregistered actions were received
	testSubscribe(registered)
	testSubscribe(deregistered)

	// Stop listening for subscriptions
	cancel()
}

func TestUnsubscribe(t *testing.T) {
	serviceName := "unsubscribe.services.fg"
	m := MapStore{}
	c1 := &stela.Client{}
	c2 := &stela.Client{}

	// Subscribe
	m.Subscribe(serviceName, c1)
	m.Subscribe(serviceName, c2)

	// Unsubscribe c1
	if err := m.Unsubscribe(serviceName, c1); err != nil {
		t.Fatal("Should have Unsubscribed successfully")
	}

	// Unsubscribe c1 again with the same client should fail
	if err := m.Unsubscribe(serviceName, c1); err == nil {
		t.Fatal("Unsubscribe should have failed")
	}

	// Unsubscribe c2
	if err := m.Unsubscribe(serviceName, c2); err != nil {
		t.Fatal("Should have Unsubscribed successfully")
	}

}

func TestRegistrationAndDiscovery(t *testing.T) {
	m := MapStore{}

	var tests = []struct {
		service    *stela.Service
		shouldFail bool
	}{
		{&stela.Service{
			Name:    "testd&r.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		}, false},
		// Don't allow duplicates
		{&stela.Service{
			Name:    "testd&r.service.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		}, true},
		{&stela.Service{
			Name:    "testd&r.service.fg",
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

	// Now see if we can discover them
	// d := m.Discover("testd&r.service.fg")
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
