/*
	// Usage:
	import (
		stela "gitlab.fg/go/stela/api"
	)

	// Create stela client
	client, err := stela.NewClient(stela.DefaultStelaAddress)
	if err != nil {
		log.Fatal(err)
	}

	// Create a service
	service := new(stela.Service)
	service.Name = "test.service.fg"
	service.Port = 8001

	// Now register with stela
	errCallback := func(err error) {
		fmt.Println("Registration error: %s", err)
	}
	client.RegisterService(service, errCallback, true)

	// When finished deregister
	err = client.DeregisterService(service)
	if err != nil {
		log.Fatal(err)
	}
*/

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/forestgiant/netutil"
	"gitlab.fg/go/disco"
	"gitlab.fg/go/stela"
)

// Client struct holds all the information about a client registering a service
type Client struct {
	ServerAddress      string                           // Stela server address
	HTTPClient         *http.Client                     // Used to make request to the stela server
	Hostname           string                           // Hostname set automatically
	Address            string                           // IPv4 address
	registeredServices map[stela.Service]*stela.Service // Map of registered services
	mu                 sync.Mutex                       // protects registeredServices map
}

// DiscoverStelaNode tries to find any stela instances running on a local
// network via IPv6 multicast
func DiscoverStelaNode(multicastPort int) (string, error) {
	d := disco.Disco{}
	ctx, cancelFunc := context.WithCancel(context.Background())
	multicastAddr := fmt.Sprintf("%s:%d", stela.DefaultMulticastAddress, multicastPort)
	discoveredCh, err := d.Discover(ctx, multicastAddr)
	if err != nil {
		log.Fatal(err)
	}

	// If we don't get a response in 3 seconds error
	time.AfterFunc(time.Second*3, func() { cancelFunc() })

	// See if there are any other stela nodes
	select {
	case n := <-discoveredCh:
		return n.Values["StelaAddress"], nil
	case <-ctx.Done():
		return "", errors.New("Couldn't discover stela node")
	}
}

// NewClient creates a stela client to access *Client methods
// Default HTTPClient Timeout to 1 second
func NewClient(stelaAddress string) (*Client, error) {
	c := new(Client)
	c.ServerAddress = stelaAddress
	c.HTTPClient = &http.Client{
		Timeout: 1 * time.Second,
	}

	// Set Hostname and IP
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	c.Hostname = host
	c.Address = netutil.LocalIPv4().String()

	// Let's check if there is a client running locally
	// url := fmt.Sprintf("http://%s/verify", c.ServerAddress)
	// res, err := http.Get(url)
	// if err != nil || res.StatusCode != http.StatusOK {
	// 	return nil, fmt.Errorf("Unabled to connect to stela instance. Status is: %v", res.StatusCode)
	// }

	return c, nil
}

// RegisterService registers a service with stela and continues registering every 2 seconds
// If there is a problem with the registration it timesout and calls timeoutHandler
func (c *Client) RegisterService(s *stela.Service) error {
	s.Target = c.Hostname
	s.Address = c.Address

	// Marshal JSON to send
	jsonStr, err := json.Marshal(s)
	if err != nil {
		return err
	}

	if err := c.registerWithHTTP(context.TODO(), jsonStr, s); err != nil {
		return err
	}

	// Setup registeredServices map
	if c.registeredServices == nil {
		c.registeredServices = make(map[stela.Service]*stela.Service)
	}

	c.mu.Lock()
	c.registeredServices[*s] = s
	c.mu.Unlock()

	go func(jsonStr []byte, s *stela.Service) {
		tick := time.NewTicker(2 * time.Second)
		ctx, cancel := context.WithCancel(context.TODO())
		defer cancel()
		defer tick.Stop()
		for {
			select {
			case <-s.DeregisterCh():
				return
			case <-tick.C:
				// TODO: For now we ignore errors. Revisit if we see a reason to.
				if err := c.registerWithHTTP(ctx, jsonStr, s); err != nil {
					cancel()
					// If we're having trouble registering call timeout handler
					// timeoutHandler(err)
				}
			}
		}
	}(jsonStr, s)

	return nil
}

func (c *Client) registerWithHTTP(ctx context.Context, jsonStr []byte, s *stela.Service) error {
	url := fmt.Sprintf("http://%s/registerservice", c.ServerAddress)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req = req.WithContext(ctx)
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
		return fmt.Errorf("Registration compare failed: \n s1: %v \n not equal to \n s2: %v", s, registerResponse)
	}

	return nil
}

// DeregisterService deregisters a service with stela
func (c *Client) DeregisterService(s *stela.Service) error {
	// Stop registeration ticker for service
	// Stop registration timeout
	c.mu.Lock()
	if c.registeredServices[*s] != nil {
		c.registeredServices[*s].StopRegistering()
		delete(c.registeredServices, *s)
	}
	c.mu.Unlock()

	url := fmt.Sprintf("http://%s/deregisterservice", c.ServerAddress)

	s.Target = c.Hostname
	s.Address = c.Address

	// Marshal JSON to send
	jsonStr, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("json Marshall error: %v", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Only proceed if 200 OK
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Deregistration failed. Response status is: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Unmarshall into response struct
	deregisterResponse := new(stela.Service)
	err = json.Unmarshal(body, deregisterResponse)
	if err != nil {
		return err
	}

	// Make sure the Service we sent is the one echoed back
	if !s.Equal(deregisterResponse) {
		return fmt.Errorf("Deregistration compare failed: \n s1: %v \n not equal to \n s2: %v", s, deregisterResponse)
	}

	return nil
}

// Discover finds a service via http api and returns a slice
func (c *Client) Discover(serviceName string) ([]*stela.Service, error) {
	url := fmt.Sprintf("http://%s/discover", c.ServerAddress)

	s := new(stela.Service)
	s.Name = serviceName

	// Marshal JSON to send
	jsonStr, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("json Marshall error: %v", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Only proceed if 200 OK
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Discover failed. Response status is: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unmarshall into response struct
	var discoverResponse []*stela.Service
	err = json.Unmarshal(body, &discoverResponse)
	if err != nil {
		return nil, err
	}

	// Loop through all responses and update address if it's
	for _, service := range discoverResponse {
		if netutil.IsLocalhost(service.Address) {
			service.Address = "127.0.0.1"
		}
	}

	return discoverResponse, nil
}

// DiscoverOne returns service with priority 0 and rotates priority
func (c *Client) DiscoverOne(serviceName string) (*stela.Service, error) {
	url := fmt.Sprintf("http://%s/discoverone", c.ServerAddress)

	s := new(stela.Service)
	s.Name = serviceName

	// Marshal JSON to send
	jsonStr, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("json Marshall error: %v", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Only proceed if 200 OK
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DiscoverOne failed. Response status is: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unmarshall into response struct
	discoverResponse := new(stela.Service)
	err = json.Unmarshal(body, discoverResponse)
	if err != nil {
		return nil, err
	}

	if netutil.IsLocalhost(discoverResponse.Address) {
		discoverResponse.Address = "127.0.0.1"
	}

	return discoverResponse, nil
}

//TODO Create DiscoverLocal to find services running on local machine

// RoundRobin rotates the SRV records corresponding to the service name
func (c *Client) RoundRobin(serviceName string) error {
	url := fmt.Sprintf("http://%s/roundrobin", c.ServerAddress)

	s := new(stela.Service)
	s.Name = serviceName

	// Marshal JSON to send
	jsonStr, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("json Marshall error: %v", err)
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Only proceed if 200 OK
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("RoundRobin failed. Response status is: %s", resp.Status)
	}

	return nil
}
