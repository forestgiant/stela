package mapstore

import (
	"reflect"
	"testing"

	"golang.org/x/net/context"

	"time"

	"gitlab.fg/go/stela"
)

func Test_rotateServices(t *testing.T) {
	m := MapStore{}
	serviceName := "priority.service.fg"

	var tests = []struct {
		service          *stela.Service
		expectedPriority int32
	}{
		{&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9000,
		}, 2},
		{&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.2",
			Port:    9000,
		}, 1},
		{&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.3",
			Port:    9000,
		}, 0},
	}
	var expectedServices []*stela.Service
	for _, test := range tests {
		// First we need to register them
		if err := m.Register(test.service); err != nil {
			t.Fatal(err)
		}

		expectedServices = append(expectedServices, test.service)
	}

	// Loop through services registered and make sure their priorities match
	for i, s := range m.services[serviceName] {
		ts := tests[i]
		if s.Equal(ts.service) {
			if s.Priority != ts.expectedPriority {
				t.Fatalf("Test_rotateServices: Priorites didn't match. Expected: %v, Received: %v", ts.expectedPriority, s.Priority)
			}
		}
	}
}

func TestSubscribe(t *testing.T) {
	m := MapStore{}
	c := &stela.Client{}
	serviceName := "subscribe.service.fg"

	// Subscribe to service
	if err := m.Subscribe(serviceName, c); err != nil {
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

	var registered []*stela.Service
	var deregistered []*stela.Service

	// Listen for registered services
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
	serviceName := "testd&r.service.fg"

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
	for _, test := range tests {
		if err := m.Register(test.service); test.shouldFail != (err != nil) {
			t.Fatal(err)
		}

		// Store the successful services into our own expected map
		if !test.shouldFail {
			expectedServices[test.service.Name] = append([]*stela.Service{test.service}, expectedServices[test.service.Name]...)
		}
	}

	// Verify the services map is what we expect
	if !reflect.DeepEqual(expectedServices, m.services) {
		t.Fatalf("Registered services failed. Expected %v, Received %v", expectedServices, m.services)
	}

	// Now see if we can discover them
	services, err := m.Discover(serviceName)
	if err != nil {
		t.Fatal("Discover failed")
	}

	if len(services) != len(expectedServices[serviceName]) {
		t.Fatal("Discover failed")
	}

	// Compare the discovered services
	equalServices(t, services, expectedServices[serviceName])

	// Let's DiscoverOne
	s, err := m.DiscoverOne(serviceName)
	if err != nil {
		t.Fatal("DiscoverOne failed")
	}

	// Make sure it's one from the registered services
	found := false
	for _, rs := range m.services[serviceName] {
		if s.Equal(rs) {
			found = true
		}
	}
	if !found {
		t.Fatal("DiscoverOne returned invalid result")
	}

	// Try to Discover/DiscoverOne a serviceName that isn't registered
	_, err = m.Discover("madeupservicename")
	if err == nil {
		t.Fatal("Discover should have failed")
	}

	_, err = m.DiscoverOne("madeupservicename")
	if err == nil {
		t.Fatal("DiscoverOne should have failed")
	}
}

func TestDiscoverAll(t *testing.T) {
	serviceName := "service1.services.fg"
	serviceNameTwo := "service2.services.fg"
	m := MapStore{}

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
			Port:    9000,
		},
		&stela.Service{
			Name:    serviceNameTwo,
			Target:  "jlu.macbook",
			Address: "localhost",
			Port:    80,
		},
	}

	for _, s := range testServices {
		if err := m.Register(s); err != nil {
			t.Fatal(err)
		}
	}

	services := m.DiscoverAll()
	equalServices(t, services, testServices)
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

func TestAddClient(t *testing.T) {
	m := MapStore{}

	var tests = []struct {
		client     *stela.Client
		shouldFail bool
	}{
		{&stela.Client{
			Address: "192.168.2.1",
		}, false},
		{&stela.Client{
			Address: "192.168",
		}, true},
		{&stela.Client{
			Address: "192.168.2.2",
		}, false},
	}

	var expectedClients []*stela.Client
	for _, test := range tests {
		if err := m.AddClient(test.client); test.shouldFail != (err != nil) {
			t.Fatal("TestAddClient failed:", err)
		}

		// Store the successful clients into our own service
		if !test.shouldFail {
			expectedClients = append(expectedClients, test.client)
		}
	}

	// Try to add the first client twice
	if err := m.AddClient(tests[0].client); err == nil {
		t.Fatal("TestAddClient should fail. Can't add the same client twice")
	}

	// Verify m.clients is what we expect
	if !reflect.DeepEqual(expectedClients, m.clients) {
		t.Fatalf("TestAddClients failed. Expected %v, Received %v", expectedClients, m.clients)
	}
}

// func TestRemoveClients(t *testing.T) {
// 	m := MapStore{}
// 	serviceName := "subscribe.service.fg"

// 	testClients := []*stela.Client{
// 		&stela.Client{Address: "127.0.0.1"},
// 		&stela.Client{Address: "127.0.0.1"},
// 	}

// 	testServices := []*stela.Service{
// 		&stela.Service{
// 			Name:    serviceName,
// 			Target:  "jlu.macbook",
// 			Address: "127.0.0.1",
// 			Port:    9000,
// 			Client:  testClients[0],
// 		},
// 		&stela.Service{
// 			Name:    serviceName,
// 			Target:  "fg.macbook",
// 			Address: "127.0.0.1",
// 			Port:    9000,
// 			Client:  testClients[0],
// 		},
// 		&stela.Service{
// 			Name:    serviceName,
// 			Target:  "jlu.macbook",
// 			Address: "localhost",
// 			Port:    80,
// 			Client:  testClients[0],
// 		},
// 		&stela.Service{
// 			Name:    serviceName,
// 			Target:  "my.macbook",
// 			Address: "localhost",
// 			Port:    80,
// 			Client:  testClients[1],
// 		},
// 	}

// 	// Add the clients
// 	for _, c := range testClients {
// 		if err := m.AddClient(c); err != nil {
// 			t.Fatal("AddClient failed")
// 		}

// 		// Subscribe to service
// 		if err := m.Subscribe(serviceName, c); err != nil {
// 			t.Fatal(err)
// 		}
// 	}

// 	// Register services with client
// 	for _, s := range testServices {
// 		if err := m.Register(s); err != nil {
// 			t.Fatal("TestRemoveClient registeration failed")
// 		}
// 	}

// 	// Now remove the clients
// 	m.RemoveClients(testClients)

// 	// Verify the services were removed from m.services
// 	for _, v := range m.services {
// 		for _, s := range v {
// 			// See if any belong to our client
// 			for _, c := range testClients {
// 				if s.Client.Equal(c) {
// 					t.Fatal("Service registered belongs to our client")
// 				}
// 			}
// 		}
// 	}

// 	// Verify the client was removed from m.clients
// 	for _, rc := range m.clients {
// 		for _, c := range testClients {
// 			if rc.Equal(c) {
// 				t.Fatal("Client shouldn't be registered")
// 			}
// 		}
// 	}

// 	// Verify client was removed from m.subscriptions
// 	subscribers := m.subscribers[serviceName]
// 	for _, rc := range subscribers {
// 		for _, c := range testClients {
// 			if rc.Equal(c) {
// 				t.Fatal("Client shouldn't be subscribed")
// 			}
// 		}
// 	}
// }

func TestClient(t *testing.T) {
	m := MapStore{}
	c := &stela.Client{Address: "127.0.0.1"}
	m.AddClient(c)

	rc, err := m.Client(c.ID)
	if err != nil {
		t.Fatal("Client getter failed")
	}

	if !rc.Equal(c) {
		t.Fatal("Client getter failed")
	}

	_, err = m.Client("notregistered")
	if err == nil {
		t.Fatal("Client getter failed")
	}
}

// func TestClients(t *testing.T) {
// 	m := MapStore{}

// 	testClients := []*stela.Client{
// 		&stela.Client{Address: "127.0.0.1"},
// 		&stela.Client{Address: "127.0.0.1"},
// 	}

// 	// Add the clients
// 	for _, c := range testClients {
// 		if err := m.AddClient(c); err != nil {
// 			t.Fatal("AddClient failed")
// 		}
// 	}

// 	clients, err := m.Clients("127.0.0.1")
// 	if err != nil {
// 		t.Fatal("Clients getter failed")
// 	}

// 	if !reflect.DeepEqual(testClients, clients) {
// 		t.Fatalf("TestClients failed. Expected %v, Received %v", testClients, clients)
// 	}

// 	// Asking for an address that isn't registered should error
// 	clients, err = m.Clients("192.168.1.1")
// 	if err == nil {
// 		t.Fatal("Clients getter failed")
// 	}
// }
