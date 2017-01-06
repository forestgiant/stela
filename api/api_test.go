// The test currently require two stela instances to be running
// One at the default setting > stela
// Another on port 9001 > stela -port 9001

package api

import (
	"testing"
	"time"

	"golang.org/x/net/context"

	"gitlab.fg/go/stela"
)

const timeOut = 500 * time.Millisecond

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
	ctx, cancelFunc := context.WithTimeout(context.Background(), timeOut)
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
		registerCtx, cancelRegister := context.WithTimeout(context.Background(), timeOut)
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
		registerCtx, cancelRegister := context.WithTimeout(context.Background(), timeOut)
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
	discoverCtx, cancelDiscover := context.WithTimeout(context.Background(), timeOut)
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
	discoverAllCtx, cancelDiscoverAll := context.WithTimeout(context.Background(), timeOut)
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
	discoverOneCtx, cancelDiscoverOne := context.WithTimeout(context.Background(), timeOut)
	defer cancelDiscoverOne()
	s, err := c2.DiscoverOne(discoverOneCtx, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid() {
		t.Fatal("c2 DiscoveOne was invalid")
	}

	// Register another stela with c
	registerCtx, cancelRegister := context.WithTimeout(context.Background(), timeOut)
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
	discoverCtx, cancelDiscover = context.WithTimeout(context.Background(), timeOut)
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
	ctx, cancel := context.WithTimeout(context.Background(), timeOut)

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
			Port:    9001,
		},
		&stela.Service{
			Name:    serviceName,
			Target:  "jlu.macbook",
			Address: "127.0.0.3",
			Port:    9002,
		},
	}

	waitCh := make(chan struct{})
	var count int
	callback := func(s *stela.Service) {
		count++
		// Test to make sure c2 receives all services registered with c
		if count == len(testServices) {
			close(waitCh)
		}

		if s.Action != stela.RegisterAction {
			t.Fatal("Service should be register action")
		}
	}

	callback2 := func(s *stela.Service) {
		count++
		// Test to make sure c3 receives all services registered with c
		if count == len(testServices) {
			close(waitCh)
		}

		if s.Action != stela.RegisterAction {
			t.Fatal("Service should be register action")
		}
	}

	// if err := c.Subscribe(serviceName, callback); err != nil {
	// 	t.Fatal(err)
	// }

	subscribeCtx, cancelSubscribe := context.WithCancel(context.Background())
	defer cancelSubscribe()
	if err := c2.Subscribe(subscribeCtx, serviceName, callback); err != nil {
		t.Fatal(err)
	}

	if err := c3.Subscribe(subscribeCtx, serviceName, callback2); err != nil {
		t.Fatal(err)
	}

	for _, s := range testServices {
		registerCtx, cancelRegister := context.WithTimeout(context.Background(), timeOut)
		defer cancelRegister()
		if err := c.RegisterService(registerCtx, s); err != nil {
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

	unsubscribeCtx, cancelUnsubscribe := context.WithTimeout(context.Background(), timeOut)
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
