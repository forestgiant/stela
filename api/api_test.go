package api

import (
	"testing"

	"gitlab.fg/go/stela"
)

func TestRegisterAndDiscover(t *testing.T) {
	serviceName := "apitest.services.fg"
	c, err := NewClient("../testdata/ca.pem")
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
