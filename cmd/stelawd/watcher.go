package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/forestgiant/stela"
	"github.com/forestgiant/stela/api"
	"golang.org/x/net/context"
)

type watcher struct {
	service     *stela.Service
	interval    time.Duration
	ticker      *time.Ticker
	stelaClient *api.Client
}

func (w *watcher) valid() bool {
	// Make sure interval is positive
	if w.interval < 0 || w.interval == 0 {
		return false
	}
	if w.service == nil {
		return false
	}
	if w.stelaClient == nil {
		return false
	}
	if w.service.Name == "" {
		return false
	}
	if w.service.IPv4 == "" {
		return false
	}
	return true
}

func (w *watcher) watch() error {
	if !w.valid() {
		return errors.New("watcher is invalid")
	}

	// Based on the interval in the config check net.Listen to see if the address/port is taken.
	w.ticker = time.NewTicker(w.interval)

	go func() {
		for range w.ticker.C {
			w.updateRegistry()
		}
	}()

	return nil
}

// updateRegistry registers or deregisters a service with the stela client based on net.Dial
// if it registers it returns true, if it deregisters it returns false
func (w *watcher) updateRegistry() bool {
	registerCtx, cancelRegister := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelRegister()

	// Verify service is running
	conn, err := net.Dial("tcp", net.JoinHostPort(w.service.IPv4, strconv.Itoa(int(w.service.Port))))
	// If there was an error that means the connection didn't happen and we need to deregister the service
	if err != nil {
		w.stelaClient.Deregister(registerCtx, w.service)
		return false
	}
	conn.Close()
	if err := w.stelaClient.Register(registerCtx, w.service); err != nil {
		return false
	}
	return true
}

func (w watcher) String() string {
	return fmt.Sprintf("Name: %s, Address: %s, Port: %d, Interval: %v", w.service.Name, w.service.IPv4, w.service.Port, w.interval)
}

func (w *watcher) stop() {
	if w.ticker == nil {
		return
	}
	w.ticker.Stop()
}
