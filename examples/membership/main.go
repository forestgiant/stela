package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"time"

	fglog "github.com/forestgiant/log"
	"github.com/forestgiant/portutil"
	"github.com/forestgiant/stela"
	"github.com/forestgiant/stela/api"
	"golang.org/x/net/context"
)

func main() {
	// Setup FG Logging
	logger := fglog.Logger{}.With("time", fglog.DefaultTimestamp, "caller", fglog.DefaultCaller, "service", "stela: membership example")

	// Create unique port for member
	port, err := portutil.GetUnique("tcp")
	if err != nil {
		log.Fatal(err)
	}

	// Create an insecure stela client
	ctx, cancelFunc := context.WithCancel(context.Background())
	stelaClient, err := api.NewClient(ctx, stela.DefaultStelaAddress, nil) // The default is localhost
	if err != nil {
		logger.Error("failed to create stela client:", "error", err.Error())
		os.Exit(1)
	}
	defer stelaClient.Close()
	serviceName := "members.test.fg"

	// Subscribe to that serviceName to be notified if anyone else registers a service
	subscribeCtx, cancelSubscribe := context.WithCancel(context.Background())
	defer cancelSubscribe()
	stelaClient.Subscribe(subscribeCtx, serviceName, func(s *stela.Service) {
		switch s.Action {
		case stela.RegisterAction:
			fmt.Printf("New member registered: %s:%d. With message: %v \n", s.IPv4, s.Port, stela.DecodeValue(s.Value))
		case stela.DeregisterAction:
			fmt.Printf("New member deregistered: %s:%d. With message: %v \n", s.IPv4, s.Port, stela.DecodeValue(s.Value))
		}

		// Print total members
		discoverCtx, cancelDiscover := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancelDiscover()
		services, err := stelaClient.Discover(discoverCtx, serviceName)
		if err != nil {
			logger.Error("failed to discover with stela:", "error", err.Error())
		}
		fmt.Println("Total members:", len(services))
	})

	// Now register with stela
	// Create a service
	memberService := &stela.Service{
		Name:  serviceName,
		Port:  int32(port),
		Value: stela.EncodeValue("Hello World."),
	}

	registerCtx, cancelRegister := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelRegister()
	if err := stelaClient.Register(registerCtx, memberService); err != nil {
		logger.Error("failed to register with stela:", "error", err.Error())
		os.Exit(1)
	}

	// Print start message
	startMessage := fmt.Sprintf("Starting member on port: %d", port)
	fmt.Println(startMessage)

	// Listen for shutdown signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigs:
			cancelFunc()
		}
	}()

	// Select will block until a signal comes in
	select {
	case <-ctx.Done():
		stelaClient.Close()
		fmt.Println("Closing member")
		return
	}
}
