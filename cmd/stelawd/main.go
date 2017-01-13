package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/api"
)

func main() {
	ctx, cancelFunc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFunc()
	stelaClient, err := api.NewClient(ctx, stela.DefaultStelaAddress, "../testdata/ca.pem")
	if err != nil {
		log.Fatal(err)
	}
	defer stelaClient.Close()

	// Read service names and ip address/ports from config
	watchers, err := createWatchers(stelaClient, "watch.list")
	if err != nil {
		log.Fatal(err)
	}

	// Print status for each service being watched
	for _, w := range watchers {
		w.watch()
		fmt.Printf("Registering service: %s \n", w)
	}

	// Listen for shutdown signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigs:
		// stop all watchers
		for _, w := range watchers {
			w.stop()
		}
		fmt.Println("Stopping watch dog")
		return
	}

}

func createWatchers(stelaClient *api.Client, filePath string) ([]*watcher, error) {
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
			interval:    time.Millisecond * time.Duration(interval),
			stelaClient: stelaClient,
		}

		watchers = append(watchers, w)
	}

	return watchers, nil
}
