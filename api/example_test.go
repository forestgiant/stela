package api

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/net/context"

	"gitlab.fg/go/stela"
)

func ExampleStelaApi() {
	// Create stela client
	ctx, cancelFunc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFunc()
	client, err := NewClient(ctx, stela.DefaultStelaAddress, "../testdata/ca.pem") // The default is localhost
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
	subscribeCtx, cancelSubscribe := context.WithCancel(context.Background())
	defer cancelSubscribe()
	client.Subscribe(subscribeCtx, serviceName, func(s *stela.Service) {
		switch s.Action {
		case stela.RegisterAction:
			fmt.Println("a new test.service.fg was found!", s)
		case stela.DeregisterAction:
			fmt.Println("a service was removed", s)
		}
	})

	// Now register with stela
	registerCtx, cancelRegister := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelRegister()
	if err := client.RegisterService(registerCtx, service); err != nil {
		log.Fatal(err)
	}

	// Discover all endpoints registered with that name
	// Typically done from another service
	discoverCtx, cancelDiscover := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelDiscover()
	services, err := client.Discover(discoverCtx, serviceName)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found services:", services)

	// When finished deregister
	deregisterCtx, cancelDeregister := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelDeregister()
	err = client.DeregisterService(deregisterCtx, service)
	if err != nil {
		log.Fatal(err)
	}
}
