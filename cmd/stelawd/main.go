package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.fg/go/stela"
)

type watcher struct {
	service  *stela.Service
	interval time.Duration
}

func main() {
	// Read service names and ip address/ports from config
	watchers, err := createWatchers("watch.list")
	if err != nil {
		log.Fatal(err)
	}

	// Print status for each service being watched
	for _, w := range watchers {
		// Base on the interval in the config check net.Listen to see if the address/port is taken.

		// If it is then create a client connection to stela.

		// If it's not then close the connection

		fmt.Printf(
			"Registering service: %s and monitoring every %d milliseconds \n",
			fmt.Sprintf("Name: %s, Address: %s, Port: %d", w.service.Name, w.service.Address, w.service.Port),
			w.interval)
	}
}

func createWatchers(filePath string) ([]*watcher, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bufio.NewReader(f))
	r.Comment = '#'
	var watchers []*watcher

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}

		// Verify serviceName
		if record[0] == "" {
			return nil, fmt.Errorf("Could not process service name for: %v", record)
		}
		serviceName := strings.TrimSpace(record[0])

		// Create and verify address/port
		ip, p, err := net.SplitHostPort(strings.TrimSpace(record[1]))
		if err != nil {
			return nil, fmt.Errorf("Could not process address %v for: %v", record[1], record)
		}
		port, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("Could not process port: %v for: %v", p, record)
		}

		// Create and verify interval
		interval, err := strconv.Atoi(strings.TrimSpace(record[2]))
		if err != nil {
			return nil, fmt.Errorf("Could not process interval %v for: %v", record[2], record)
		}

		w := &watcher{
			service: &stela.Service{
				Name:    serviceName,
				Address: ip,
				Port:    int32(port),
			},
			interval: time.Millisecond * time.Duration(interval),
		}

		watchers = append(watchers, w)
	}

	return watchers, nil
}
