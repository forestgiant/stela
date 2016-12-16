package api

import (
	"fmt"
	"log"

	"golang.org/x/net/context"

	"gitlab.fg/go/stela"
)

func ExampleStelaApi() {
	// Create stela client
	client, err := NewClient(context.Background(), stela.DefaultStelaAddress, "../testdata/ca.pem") // The default is localhost
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	serviceName := "test.service.fg"

	// Create a service
	service := &stela.Service{
		Name: serviceName,
		Port: 8001,
	}

	// Subscribe to that serviceName to be notified if anyone else registers a service
	client.Subscribe(serviceName, func(s *stela.Service) {
		switch s.Action {
		case stela.RegisterAction:
			fmt.Println("a new test.service.fg was found!", s)
		case stela.DeregisterAction:
			fmt.Println("a service was removed", s)
		}
	})

	// Now register with stela
	if err := client.RegisterService(service); err != nil {
		log.Fatal(err)
	}

	// Discover all endpoints registered with that name
	// Typically done from another service
	services, err := client.Discover(serviceName)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found services:", services)

	// When finished deregister
	err = client.DeregisterService(service)
	if err != nil {
		log.Fatal(err)
	}
}
