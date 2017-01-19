// The test currently require two stela instances to be running
// One at the default setting > stela
// Another on port 9001 > stela -port 9001

package api

import (
	"testing"
	"time"

	"golang.org/x/net/context"

	"sync"

	"gitlab.fg/go/stela"
)

const timeout = 500 * time.Millisecond

// func TestMain(m *testing.M) {
// 	kill, err := startStelaInstance(stela.DefaultStelaPort, stela.DefaultMulticastPort)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	t := m.Run()

// 	kill()
// 	os.Exit(t)
// }

func TestRegisterAndDiscover(t *testing.T) {
	// Create a second stela instance
	// kill, err := startStelaInstance(9001, stela.DefaultMulticastPort)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// defer kill()
	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()

	serviceName := "apitest.services.fg"
	c, err := NewClient(ctx, stela.DefaultStelaAddress, "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c2, err := NewClient(ctx, "127.0.0.1:9001", "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	// Register services with c2
	c2Services := []*stela.Service{
		&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9001,
		},
		&stela.Service{
			Name:    "discoverall.services.fg",
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    9001,
		},
		&stela.Service{
			Name:    stela.ServiceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    10001,
		},
		&stela.Service{
			Name:    stela.ServiceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    10000,
		},
	}
	for _, s := range c2Services {
		registerCtx, cancelRegister := context.WithCancel(context.Background())
		defer cancelRegister()
		if err := c2.RegisterService(registerCtx, s); err != nil {
			t.Fatal(err)
		}
	}

	var expectedServices []*stela.Service

	// Add c2Service to expected
	expectedServices = append(expectedServices, c2Services[0])

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
		registerCtx, cancelRegister := context.WithCancel(context.Background())
		defer cancelRegister()
		if err := c.RegisterService(registerCtx, test.service); test.shouldFail != (err != nil) {
			t.Fatal(i, test, err)
		}

		// Store the successful services into our own expected map
		if !test.shouldFail {
			expectedServices = append([]*stela.Service{test.service}, expectedServices...)
		}
	}

	// Now see if we can discover them
	discoverCtx, cancelDiscover := context.WithTimeout(context.Background(), timeout)
	defer cancelDiscover()
	services, err := c.Discover(discoverCtx, serviceName)
	if err != nil {
		t.Fatal(err)
	}

	if len(services) != 3 {
		t.Fatal("Discover failed", services, expectedServices)
	}

	// Compare the discovered services
	equalServices(t, services, expectedServices)

	// DiscoverAll should return expected plus 1
	discoverAllCtx, cancelDiscoverAll := context.WithTimeout(context.Background(), timeout)
	defer cancelDiscoverAll()
	da, err := c.DiscoverAll(discoverAllCtx)
	if err != nil {
		t.Fatal(err)
	}

	// Remove 1 from c2Services add 2 for each stela instance running
	if len(da) != len(expectedServices)+len(c2Services)+1 {
		t.Fatal("DiscoverAll failed", da)
	}

	// DiscoverOne with c2
	discoverOneCtx, cancelDiscoverOne := context.WithTimeout(context.Background(), timeout)
	defer cancelDiscoverOne()
	s, err := c2.DiscoverOne(discoverOneCtx, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid() {
		t.Fatal("c2 DiscoveOne was invalid")
	}

	// Register another stela with c
	registerCtx, cancelRegister := context.WithCancel(context.Background())
	defer cancelRegister()
	if err := c.RegisterService(registerCtx,
		&stela.Service{
			Name:    stela.ServiceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.1",
			Port:    10002,
		}); err != nil {
		t.Fatal(err)
	}

	// Discover all stela instances
	discoverCtx, cancelDiscover = context.WithTimeout(context.Background(), timeout)
	defer cancelDiscover()
	stelas, err := c2.Discover(discoverCtx, stela.ServiceName)
	if err != nil {
		t.Fatal(err)
	}
	if len(stelas) != 5 {
		t.Fatal("stela discovery failed", stelas)
	}
}

func TestConnectSubscribe(t *testing.T) {
	serviceName := "testSubscribe.services.fg"
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Connect to both instances
	c, err := NewClient(ctx, stela.DefaultStelaAddress, "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c2, err := NewClient(ctx, "127.0.0.1:9001", "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	c3, err := NewClient(ctx, "127.0.0.1:9001", "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer c3.Close()

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
			Port:    65349,
		},
		&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.3",
			Port:    653490,
		},
	}

	waitCh := make(chan struct{})
	wg := sync.WaitGroup{}
	wg.Add(len(testServices) * 2) // Add dobule for both callbacks
	c2Found := []*stela.Service{}
	c2Callback := func(s *stela.Service) {
		c2Found = append(c2Found, s)

		if s.Action != stela.RegisterAction {
			t.Fatal("Service should be register action")
		}
		wg.Done()
	}

	c3Found := []*stela.Service{}
	c3Callback := func(s *stela.Service) {
		c3Found = append(c3Found, s)

		if s.Action != stela.RegisterAction {
			t.Fatal("Service should be register action")
		}
		wg.Done()
	}

	subscribeCtx, cancelSubscribe := context.WithTimeout(context.Background(), timeout)
	defer cancelSubscribe()
	if err := c2.Subscribe(subscribeCtx, serviceName, c2Callback); err != nil {
		t.Fatal(err)
	}

	if err := c3.Subscribe(subscribeCtx, serviceName, c3Callback); err != nil {
		t.Fatal(err)
	}

	// Now register all testServices from the first client
	for _, s := range testServices {
		registerCtx, cancelRegister := context.WithCancel(context.Background())
		defer cancelRegister()
		if err := c.RegisterService(registerCtx, s); err != nil {
			t.Fatal(err)
		}
	}

	go func() {
		wg.Wait()
		close(waitCh)
	}()

	// Wait for either a timeout or all the subscribed services to be read
	select {
	case <-waitCh:
		break
	case <-ctx.Done():
		if ctx.Err() != nil {
			t.Fatal("TestConnectSubscribe timed out: ", ctx.Err())
		}
	}

	// Make sure the services c2, c3 received were correct
	equalServices(t, testServices, c2Found)
	equalServices(t, testServices, c3Found)

	unsubscribeCtx, cancelUnsubscribe := context.WithTimeout(context.Background(), timeout)
	defer cancelUnsubscribe()
	if err := c2.Unsubscribe(unsubscribeCtx, serviceName); err != nil {
		t.Fatal(err)
	}

	// Verify the map is empty
	if c.callbacks[serviceName] != nil {
		t.Fatal("callbacks map should be empty after Unsubscribe", c.callbacks)
	}
	// time.Sleep(time.Millisecond * 500)
	cancel()
}

// TestValue
func TestValue(t *testing.T) {
	// value
	value := "Test Value"

	// Create a client and subscribe
	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()

	serviceName := "valuetest.services.fg"
	c, err := NewClient(ctx, stela.DefaultStelaAddress, "../testdata/ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	callback := func(s *stela.Service) {
		if s.Value != value {
			t.Fatalf("Value is incorrected. Got %v, Wanted: %v", s.Value, value)
		}
	}

	// Subscribe to a service name and verify a value is returned
	subscribeCtx, cancelSubscribe := context.WithTimeout(context.Background(), timeout)
	defer cancelSubscribe()
	if err := c.Subscribe(subscribeCtx, serviceName, callback); err != nil {
		t.Fatal(err)
	}

	// Register a service with a value
	service := &stela.Service{
		Name:    serviceName,
		Target:  "jlu.macbook",
		Address: "127.0.0.1",
		Port:    9000,
		Value:   value,
	}
	if err := c.RegisterService(ctx, service); err != nil {
		t.Fatal(err)
	}

	// DiscoverOne
	s, err := c.DiscoverOne(ctx, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if s.Value != value {
		t.Fatalf("Value is incorrected. Got %v, Wanted: %v", s.Value, value)
	}

	// Discover the service and make sure the value is returned
	ds, err := c.Discover(ctx, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range ds {
		if s.Name == serviceName {
			if s.Value != value {
				t.Fatalf("Value is incorrected. Got %v, Wanted: %v", ds[0].Value, value)
			}
		}
	}

	// DiscoverAll
	das, err := c.DiscoverAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range das {
		if s.Name == serviceName {
			if s.Value != value {
				t.Fatalf("Value is incorrected. Got %v, Wanted: %v", ds[0].Value, value)
			}
		}
	}

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

// func startStelaInstance(stelaPort, multicastPort int) (kill func(), err error) {
// 	// Run a stela instance
// 	cmd := exec.Command("stela", "-port", fmt.Sprint(stelaPort), "-multicast", fmt.Sprint(multicastPort))
// 	if err := cmd.Start(); err != nil {
// 		return nil, err
// 	}

// 	// Give the instance time to start
// 	time.Sleep(time.Millisecond * 100)

// 	return func() {
// 		if err := cmd.Process.Kill(); err != nil {
// 			fmt.Println("failed to kill: ", err)
// 		}
// 	}, nil
// }
