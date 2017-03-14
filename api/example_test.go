package api

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/net/context"

	"github.com/forestgiant/stela"
)

func ExampleStelaApi() {
	// Create an insecure stela client
	ctx, cancelFunc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFunc()
	client, err := NewClient(ctx, stela.DefaultStelaAddress, nil) // The default is localhost
	if err != nil {
		log.Fatal(err)
	}

	// Close the client at the end of your application
	// Be careful, if you close it sooner, you will close all connections
	defer client.Close()

	serviceName := "test.service.fg"

	// Create a service
	service := &stela.Service{
		Name: serviceName,
		Port: 8001,
	}

	// Subscribe to that serviceName to be notified if anyone else registers a service
	subscribeCtx, cancelSubscribe := context.WithTimeout(context.Background(), 50*time.Millisecond)
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
	if err := client.Register(registerCtx, service); err != nil {
		log.Fatal(err)
	}

	// Discover all endpoints registered with that name. Typically done from another service
	//
	// We use a WithCancel instead of a WithTimeout context because Discover
	// asks all known stela instances to Discover and could take a while
	discoverCtx, cancelDiscover := context.WithCancel(context.Background())
	defer cancelDiscover()
	services, err := client.Discover(discoverCtx, serviceName)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Found services:", services)

	// When finished deregister
	deregisterCtx, cancelDeregister := context.WithCancel(context.Background())
	defer cancelDeregister()
	err = client.Deregister(deregisterCtx, service)
	if err != nil {
		log.Fatal(err)
	}
}
