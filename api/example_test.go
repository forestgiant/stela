package api

import (
	"fmt"
	"log"

	"gitlab.fg/go/stela"
)

func ExampleStelaApi() {
	// Create stela client
	stelaIPAddress := stela.DefaultStelaHTTPAddress // The default is localhost
	client, err := NewClient(stelaIPAddress)
	if err != nil {
		log.Fatal(err)
	}

	// Create a service
	service := new(stela.Service)
	service.Name = "test.service.fg"
	service.Port = 8001

	// Now register with stela
	client.RegisterService(service)

	// Discover all endpoints registered with that name
	// Typically done from another service
	services, err := client.Discover("test.service.fg")
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
