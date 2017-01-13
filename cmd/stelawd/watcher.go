package main

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/api"
	"golang.org/x/net/context"
)

type watcher struct {
	service     *stela.Service
	interval    time.Duration
	ticker      *time.Ticker
	stelaClient *api.Client
}

func (w *watcher) watch() {
	// Based on the interval in the config check net.Listen to see if the address/port is taken.
	w.ticker = time.NewTicker(w.interval)

	updateRegistry := func() {
		registerCtx, cancelRegister := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancelRegister()

		// Verify service is running
		conn, err := net.Dial("tcp", net.JoinHostPort(w.service.Address, strconv.Itoa(int(w.service.Port))))
		// If there was an error that means the connection didn't happen and we need to deregister the service
		if err != nil {
			w.stelaClient.DeregisterService(registerCtx, w.service)
			return
		}
		conn.Close()
		w.stelaClient.RegisterService(registerCtx, w.service)
	}

	go func() {
		for range w.ticker.C {
			updateRegistry()
		}
	}()

}

func (w *watcher) String() string {
	return fmt.Sprintf("Name: %s, Address: %s, Port: %d, Interval: %v", w.service.Name, w.service.Address, w.service.Port, w.interval)
}

func (w *watcher) stop() {
	w.ticker.Stop()
}
