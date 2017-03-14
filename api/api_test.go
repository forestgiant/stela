package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"gitlab.fg/go/stela"

	"github.com/forestgiant/netutil"
	"github.com/forestgiant/portutil"
)

func TestMain(m *testing.M) {
	// Create a temp directory for raft files
	tempDir, err := ioutil.TempDir("", "")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	_, err = DiscoverStelaNode(stela.DefaultMulticastPort)
	if err != nil {
		fmt.Println("Couldn't find a stela node")
		log.Fatal(err)
		// if err := startStelaInstance(tempDir); err != nil {
		// 	log.Fatal(err)
		// }
	}

	// Run all test
	t := m.Run()

	os.Exit(t)
}

func TestCompare(t *testing.T) {
	fmt.Println("Testing Service Compare")
	tests := []struct {
		s1     *stela.Service
		s2     *stela.Service
		result bool
	}{
		{
			&stela.Service{
				Name:    "test.service.fg",
				Target:  "server.local",
				Address: "10.0.0.99",
				Port:    8010},
			&stela.Service{
				Name:    "test.service.fg",
				Target:  "server.local",
				Address: "10.0.0.99",
				Port:    8010},
			true,
		},
		{&stela.Service{Name: "test.service.fg"}, &stela.Service{Name: "simple.service.fg"}, false},
		{&stela.Service{Target: "server.local"}, &stela.Service{Target: "linux.local"}, false},
		{&stela.Service{Address: "10.0.0.99"}, &stela.Service{Address: "10.0.0.1"}, false},
		{&stela.Service{Port: 8010}, &stela.Service{Port: 8000}, false},
		// {&Service{Priority: 0}, &Service{Priority: 10}, false},
		// {&Service{Weight: 1}, &Service{Weight: 100}, false},
		// {&Service{TTL: uint32(86400)}, &Service{TTL: uint32(0)}, false},
	}
	for _, test := range tests {
		if test.s1.Equal(test.s2) != test.result {
			t.Fatalf("Compare error. expected: %t", test.result)
		}
	}
}

func TestSetupClient(t *testing.T) {
	fmt.Println("Testing NewClient")

	stelaAddress := stelaAddress(t)
	client, err := NewClient(stelaAddress)
	if err != nil {
		t.Fatalf("Couldn't create client: %s", err)
	}

	// Make sure external ip was set correctly
	address := netutil.LocalIPv4()

	if client.Address != address.String() {
		t.Fatalf("Address is: %s. Should be: %s", client.Address, address)
	}
}

func TestRegisterService(t *testing.T) {
	fmt.Println("Testing Registration")

	// Create stela client
	stelaAddress := stelaAddress(t)
	client, err := NewClient(stelaAddress)
	if err != nil {
		t.Fatal(err)
	}

	// Create random number
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	testService := testService()
	testService.Name = strconv.Itoa(r.Intn(10000))

	// Register directly with HTTP to avoid health checking
	err = registerWithHTTP(testService, client)
	if err != nil {
		t.Fatal(err)
	}

	//Ensure service is deregistered after 5 seconds
	fmt.Println("Waiting to ensure the registered service is deregistered")
	time.Sleep(6 * time.Second)
	discoverTest := func(results int) {
		services, err := client.Discover(testService.Name)
		if err != nil {
			if results != 0 {
				t.Fatalf("Couldn't run discover: %s", err)
			}
		}

		if len(services) != results {
			for _, s := range services {
				if s.Name == testService.Name {
					t.Fatal("Service was not de-registered after a period of not re-registering itself with the server:", s)
				}
			}
		}
	}
	discoverTest(0)

	client.RegisterService(testService)
	fmt.Println("Waiting to ensure the service is not deregistered")
	time.Sleep(3 * time.Second)
	discoverTest(1)
	client.DeregisterService(testService) // Clean up
}

func TestDiscover(t *testing.T) {
	// Find all endpoints registered
	fmt.Println("Testing Discover")
	stelaAddress := stelaAddress(t)
	client, err := NewClient(stelaAddress)

	testService := testService()
	client.RegisterService(testService) // Register before discovery
	services, err := client.Discover(testService.Name)
	if err != nil {
		t.Fatalf("Couldn't discover: %s", err)
	}

	if len(services) == 0 {
		t.Fatalf("Couldn't discover services. 0 found")
	}

	for _, service := range services {
		if service.Name != testService.Name {
			t.Fatalf("Service names not equal. Received: %s, Should be: %s", service.Name, testService.Name)
		}

		fmt.Println("Service Found:", service)
	}

	client.DeregisterService(testService) // Cleanup
}

func TestDiscoverOne(t *testing.T) {
	fmt.Println("Testing DiscoverOne")
	stelaAddress := stelaAddress(t)
	client, err := NewClient(stelaAddress)
	testService := testService()
	client.RegisterService(testService)
	service, err := client.DiscoverOne(testService.Name)
	if err != nil {
		t.Errorf("Couldn't discover one: %s", err)
	}

	if service.Priority != 0 {
		t.Errorf("TestDiscoverOne: service priority should be 0. Instead it's: %d", service.Priority)
	}

	client.DeregisterService(testService) // Cleanup
}

func TestRoundRobin(t *testing.T) {
	fmt.Println("Testing RoundRobin")
	stelaAddress := stelaAddress(t)
	client, err := NewClient(stelaAddress)
	if err != nil {
		t.Fatalf("RoundRobin failed: %s", err)
	}

	testService := testService()
	client.RegisterService(testService)
	service, err := client.DiscoverOne(testService.Name)
	if err != nil {
		t.Fatalf("Couldn't discover one: %s", err)
	}

	previousPriority := service.Priority
	fmt.Println("TestRoundRobin: Previous Priority:", previousPriority)

	err = client.RoundRobin(testService.Name)
	if err != nil {
		t.Fatalf("TestRoundRobin: RoundRobin failed: %s", err)
	}

	service, err = client.DiscoverOne(testService.Name)
	if err != nil {
		t.Fatalf("TestRoundRobin: Couldn't discover one: %s", err)
	}

	newPriority := service.Priority
	fmt.Println("TestRoundRobin: NEW Priority:", newPriority)

	// Check if SRV priorites have been rotated
	// We only have one registered service so it should stay 0
	if previousPriority != newPriority {
		t.Fatal("TestRoundRobin: Priorities should have matched")
	}

	client.DeregisterService(testService) // Cleanup
}

func TestDeregisterService(t *testing.T) {
	fmt.Println("Testing Deregister")
	// Create stela client
	stelaAddress := stelaAddress(t)
	client, err := NewClient(stelaAddress)
	if err != nil {
		t.Fatal(err)
	}

	// Now deregister with stela
	testService := testService()
	client.RegisterService(testService)
	err = client.DeregisterService(testService)
	if err != nil {
		t.Fatalf("Couldn't deregister: %s", err)
	}

	fmt.Println("Done with deregister test")
}

func TestStress(t *testing.T) {
	stelaAddress := stelaAddress(t)
	// stelaAddress = "127.0.0.1:9001"
	client, err := NewClient(stelaAddress)
	if err != nil {
		t.Fatal(err)
	}
	// Register 7 services under the same name
	srvName := "stress.services.fg"
	for i := 0; i < 7; i++ {
		srv := &stela.Service{Name: srvName, Port: i}
		if err := client.RegisterService(srv); err != nil {
			t.Fatal("TestStress: Registration failed", err)
		}
	}

	// Discover that service 4000 times
	total := 4000
	for i := 0; i < total; i++ {
		services, err := client.Discover(srvName)
		if err != nil {
			t.Errorf("Couldn't discover: %s", err)
		}
		if len(services) != 7 {
			t.Fatal("Length of services should be 1. Received", len(services))
		}
	}

	// Deregister all 7
	for i := 0; i < 7; i++ {
		srv := &stela.Service{Name: srvName, Port: i}
		if err := client.DeregisterService(srv); err != nil {
			t.Fatal("TestStress: DeregisterService failed", err)
		}
	}
}

func registerWithHTTP(s *stela.Service, c *Client) error {
	if s == nil {
		return fmt.Errorf("Service is nil")
	}

	if c == nil {
		return fmt.Errorf("Client is nil")
	}

	// Marshal JSON to send
	jsonStr, err := json.Marshal(s)
	if err != nil {
		return err
	}

	// Register the service with the http api so it doesn't healthcheck
	testService := testService()
	url := fmt.Sprintf("http://%s/registerservice", c.ServerAddress)
	s.Target = testService.Target   //c.Hostname
	s.Address = testService.Address //c.Address

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Only proceed if 200 OK
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Registration failed. Response status is: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Unmarshall into response struct
	registerResponse := new(stela.Service)
	err = json.Unmarshal(body, registerResponse)
	if err != nil {
		return err
	}

	// Make sure the Service we sent is the one echoed back
	if !s.Equal(registerResponse) {
		return fmt.Errorf("Registration Test compare failed: \n s1: %v \n not equal to \n s2: %v", s, registerResponse)
	}

	return err
}

func startStelaInstance(tempDir string) error {
	multicastPort := 10000

	// Get unique port for stela
	port, err := portutil.GetUniqueTCP()
	if err != nil {
		return err
	}

	// Get a unique port for stela dns port
	dnsport, err := portutil.GetUniqueTCP()
	if err != nil {
		return err
	}

	// Run a stela instance
	fmt.Println("Starting a new stela instance")
	// stela -port 9002 -dnsport 8055 -raftdir ~/stela/node2 -multicast 10000
	cmd := exec.Command("stela", fmt.Sprintf("-port %d -dnsport %d -raftdir %s -multicast %d", port, dnsport, tempDir, multicastPort))
	if err := cmd.Start(); err != nil {
		return err
	}

	fmt.Println("Waiting for stela instance to run")
	// Let's check if there is a client running locally
	url := fmt.Sprintf("http://%s/verify", fmt.Sprint("http://localhost:", port))
	var res *http.Response

	for res != nil && res.StatusCode == http.StatusOK {
		res, _ = http.Get(url)
	}

	// stelaAddress, err := DiscoverStelaNode(multicastPort)
	// if err != nil {
	// 	return err
	// }

	fmt.Println("Stela instance is now running")

	return nil
}

func testService() *stela.Service {
	return &stela.Service{
		Name:    "test.service.fg",
		Target:  "server.local",
		Address: "10.0.0.99",
		Port:    8010,
	}
}

func stelaAddress(t *testing.T) string {
	stelaAddress, err := DiscoverStelaNode(stela.DefaultMulticastPort)
	if err != nil {
		t.Fatal(err)
	}

	return stelaAddress
}
