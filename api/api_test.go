// The test currently require two stela instances to be running
// One at the default setting > stela
// Another on port 31001 > stela -port 31001

package api

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/net/context"

	"sync"

	"gitlab.fg/go/stela"
)

const (
	timeout           = 1000 * time.Millisecond
	stelaTestPort     = 31100
	stelaTestPort2    = 31200
	testMulticastPort = 31153
	caPem             = "../testdata/ca.pem"
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) (exitCode int) {
	kill, err := startStelaInstance(stelaTestPort, testMulticastPort)
	if err != nil {
		log.Fatal(err)
	}
	defer kill()

	// Create a second stela instance
	kill2, err := startStelaInstance(stelaTestPort2, testMulticastPort)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer kill2()

	// Give the instances time to start
	time.Sleep(time.Second * 2)

	// Run test
	t := m.Run()

	return t
}

func TestRegisterAndDiscover(t *testing.T) {
	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()

	serviceName := "apitest.services.fg"

	c, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c2, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort2), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	// Register services with c2
	c2Services := []*stela.Service{
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9001,
		},
		&stela.Service{
			Name:     "discoverall.services.fg",
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9001,
		},
		&stela.Service{
			Name:     stela.ServiceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     10001,
		},
		&stela.Service{
			Name:     stela.ServiceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     10000,
		},
	}
	for _, s := range c2Services {
		registerCtx, cancelRegister := context.WithCancel(context.Background())
		defer cancelRegister()
		if err := c2.Register(registerCtx, s); err != nil {
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
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9000,
		}, false},
		// Don't allow duplicates
		{&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9000,
		}, true},
		{&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "localhost",
			Port:     80,
		}, false},
		{&stela.Service{
			Name:     "",
			Hostname: "",
			IPv4:     "",
			Port:     0,
		}, true},
	}

	for i, test := range tests {
		registerCtx, cancelRegister := context.WithCancel(context.Background())
		defer cancelRegister()
		if err := c.Register(registerCtx, test.service); test.shouldFail != (err != nil) {
			t.Fatal(i, test, err)
		}

		// Store the successful services into our own expected map
		if !test.shouldFail {
			expectedServices = append([]*stela.Service{test.service}, expectedServices...)
		}
	}

	// Now see if we can discover apitest.services.fg service
	services, err := c.Discover(context.Background(), serviceName)
	if err != nil {
		t.Fatal(err)
	}

	// There should only be three registered
	if len(services) != 3 {
		t.Fatalf("Discover failed. \n Received: %s \n Expected: %s \n", services, expectedServices)
	}

	// Compare the discovered services
	equalServices(t, services, expectedServices)

	// DiscoverAll should return expected plus 1
	da, err := c.DiscoverAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Remove 1 from c2Services add 2 for each stela instance running
	if len(da) != len(expectedServices)+len(c2Services)+1 {
		t.Fatal("DiscoverAll failed", da)
	}

	// DiscoverOne with c2
	s, err := c2.DiscoverOne(context.Background(), serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Valid() {
		t.Fatal("c2 DiscoveOne was invalid")
	}

	// Register another stela with c
	registerCtx, cancelRegister := context.WithCancel(context.Background())
	defer cancelRegister()
	if err := c.Register(registerCtx,
		&stela.Service{
			Name:     stela.ServiceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     10002,
		}); err != nil {
		t.Fatal(err)
	}

	// Discover all stela instances. There should be 2 registered with c2, 1 registered with c and 2 running instances
	stelas, err := c.Discover(context.Background(), stela.ServiceName)
	if err != nil {
		t.Fatal(err)
	}
	if len(stelas) != 5 {
		t.Fatalf("stela discovery failed. Got: %d, Wanted %d", len(stelas), 5)
	}

	// Discover all stela instances on second client. There should be 2 registered with c2, 1 registered with c and 2 running instances
	stelas, err = c2.Discover(context.Background(), stela.ServiceName)
	if err != nil {
		t.Fatal(err)
	}
	if len(stelas) != 5 {
		t.Fatalf("stela discovery failed. Got: %d, Wanted %d", len(stelas), 5)
	}
}

func TestDeregister(t *testing.T) {
	serviceName := "deregister.services.fg"

	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()
	c, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	services := []*stela.Service{
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9001,
		},
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9002,
		},
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     10001,
		},
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     10000,
		},
	}

	// Register all the services
	for _, s := range services {
		if err := c.Register(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}

	// Discover to verify the were registered
	found, err := c.Discover(context.Background(), serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != len(services) {
		t.Fatalf("discovery failed. Got: %d, Wanted %d", len(found), len(services))
	}

	// Deregister all services
	for _, s := range services {
		if err := c.Deregister(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}

	// Verify the services were removed
	found2, err := c.Discover(context.Background(), serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if len(found2) != 0 {
		t.Fatalf("discovery failed. Got: %d, Wanted %d", len(found2), 0)
	}
}

func TestDiscoverRegex(t *testing.T) {
	serviceName := "regex.services.fg"
	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()
	c, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	services := []*stela.Service{
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9001,
		},
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9002,
		},
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     10001,
		},
		&stela.Service{
			Name:     "regTest2.services.fg",
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     10000,
		},
	}

	// Register the services
	for i, s := range services {
		if err := c.Register(context.Background(), s); err != nil {
			t.Fatal(i, s, err)
		}
	}

	// Now try to discover with a regex
	found, err := c.DiscoverRegex(context.Background(), "reg.*")
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != len(services) {
		t.Fatal("Did not discover services with regex", found)
	}
}

func TestDiscoverOneWithNothingRegistered(t *testing.T) {
	serviceName := "discoverone.services.fg"

	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()
	c, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Make sure no services for serviceName are registered
	services, err := c.Discover(context.Background(), serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) > 0 {
		t.Fatalf("no services with the name: %s, should be registered", serviceName)
	}

	_, err = c.DiscoverOne(context.Background(), serviceName)
	if err == nil {
		t.Fatal("DiscoverOne should have errored.")
	}

}

func TestMaxValue(t *testing.T) {
	serviceName := "maxvalue.services.fg"

	ctx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()
	c, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	tests :=
		[]struct {
			service    *stela.Service
			shouldFail bool
		}{
			{&stela.Service{
				Name:     serviceName,
				Hostname: "jlu.macbook",
				IPv4:     "127.0.0.1",
				Port:     9001,
				Value:    stela.EncodeValue("Value string"),
			}, false},
			{&stela.Service{
				Name:     serviceName,
				Hostname: "jlu.macbook",
				IPv4:     "127.0.0.1",
				Port:     3002,
				Value:    stela.EncodeValue(&stela.Service{}),
			}, false},
			{&stela.Service{
				Name:     serviceName,
				Hostname: "jlu.macbook",
				IPv4:     "127.0.0.1",
				Port:     10001,
				Value:    make([]byte, 256),
			}, false},
			{&stela.Service{
				Name:     serviceName,
				Hostname: "jlu.macbook",
				IPv4:     "127.0.0.1",
				Port:     9002,
				Value:    make([]byte, 257),
			}, true},
		}

	// Register services to test value
	for i, test := range tests {
		if err := c.Register(context.Background(), test.service); test.shouldFail != (err != nil) {
			t.Fatal(i, test, err)
		}
	}

}

func TestConnectSubscribe(t *testing.T) {
	serviceName := "testSubscribe.services.fg"
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Connect to both instances
	c, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	c2, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort2), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	c3, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort2), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c3.Close()

	// Test services for c
	var testServices = []*stela.Service{
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.1",
			Port:     9000,
		},
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.2",
			Port:     65349,
		},
		&stela.Service{
			Name:     serviceName,
			Hostname: "jlu.macbook",
			IPv4:     "127.0.0.3",
			Port:     653490,
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
		if err := c.Register(registerCtx, s); err != nil {
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
	c, err := NewClient(ctx, fmt.Sprintf("127.0.0.1:%d", stelaTestPort), caPem)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	callback := func(s *stela.Service) {
		if stela.DecodeValue(s.Value) != value {
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
		Name:     serviceName,
		Hostname: "jlu.macbook",
		IPv4:     "127.0.0.1",
		Port:     9000,
		Value:    stela.EncodeValue(value),
	}

	if err := c.Register(ctx, service); err != nil {
		t.Fatal(err)
	}

	// DiscoverOne
	s, err := c.DiscoverOne(ctx, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if stela.DecodeValue(s.Value) != value {
		t.Fatalf("Value is incorrected. Got %v, Wanted: %v", stela.DecodeValue(s.Value), value)
	}

	// Discover the service and make sure the value is returned
	ds, err := c.Discover(ctx, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range ds {
		if s.Name == serviceName {
			if stela.DecodeValue(s.Value) != value {
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
			if stela.DecodeValue(s.Value) != value {
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

func startStelaInstance(stelaPort, multicastPort int) (kill func(), err error) {
	// Run a stela instance
	cmd := exec.Command("stela", "-port", fmt.Sprint(stelaPort), "-multicast", fmt.Sprint(multicastPort))

	// Print std out,err
	createPipeScanners(cmd, fmt.Sprintf("stela port: %d", stelaPort))

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return func() {
		if err := cmd.Process.Kill(); err != nil {
			fmt.Println("failed to kill: ", err)
		}
	}, nil
}

func createPipeScanners(cmd *exec.Cmd, prefix string) {
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	// Created scanners for in, out, and err pipes
	outScanner := bufio.NewScanner(stdout)
	errScanner := bufio.NewScanner(stderr)

	// Scan for text
	go func() {
		for errScanner.Scan() {
			fmt.Printf("[%s] %s\n", prefix, errScanner.Text())
		}
	}()

	go func() {
		for outScanner.Scan() {
			fmt.Printf("[%s] %s\n", prefix, outScanner.Text())
		}
	}()
}
