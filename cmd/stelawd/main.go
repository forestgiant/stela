package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	fglog "github.com/forestgiant/log"
	"github.com/forestgiant/semver"

	"golang.org/x/net/context"

	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/api"
)

func main() {
	logger := fglog.Logger{}.With("time", fglog.DefaultTimestamp, "caller", fglog.DefaultCaller, "cli", "stelawd")
	err := semver.SetVersion(stela.Version)
	if err != nil {
		logger.Error("error", err.Error())
		os.Exit(1)
	}

	// Check for command line configuration flags
	var (
		watcherListUsage = "Path to the list of services to watch."
		watcherListPtr   = flag.String("watchlist", "watch.list", watcherListUsage)
		caFileUsage      = "Path to the ca file for gRPC client to connect with."
		caFilePtr        = flag.String("cafile", "../certs/ca.pem", caFileUsage)
	)
	flag.Parse()

	ctx, cancelFunc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFunc()
	stelaClient, err := api.NewClient(ctx, stela.DefaultStelaAddress, *caFilePtr)
	if err != nil {
		logger.Error("Failed to create stela client. Make sure there is a stela instance running", "error", err.Error())
		os.Exit(1)
	}
	defer stelaClient.Close()

	// Read service names and ip address/ports from config
	config, err := openConfig(*watcherListPtr)
	if err != nil {
		logger.Error("Failed to open config:", "error", err.Error())
		os.Exit(1)
	}
	watchers, err := createWatchers(stelaClient, config)
	if err != nil {
		logger.Error("Failed to create watchers:", "error", err.Error())
		os.Exit(1)
	}

	// Print status for each service being watched
	for _, w := range watchers {
		if err := w.watch(); err != nil {
			logger.Error("Failed to watch:", "error", err.Error(), "watcher:", w)
			os.Exit(1)
		}
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

func openConfig(filePath string) (io.Reader, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	return bufio.NewReader(f), nil
}

func createWatchers(stelaClient *api.Client, config io.Reader) ([]*watcher, error) {
	r := csv.NewReader(config)
	r.Comment = '#'
	var watchers []*watcher

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}

		if len(record) < 4 {
			return nil, fmt.Errorf("Could not process record: %v. Not enough fields.", record)
		}

		// Verify serviceName
		if strings.TrimSpace(record[0]) == "" {
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

		// Create and verify value
		// Value can only be a string for a stelawd config
		if strings.TrimSpace(record[2]) == "" {
			return nil, fmt.Errorf("Could not process value for: %v", record)
		}
		value := strings.TrimSpace(record[2])

		// Create and verify interval
		interval, err := strconv.Atoi(strings.TrimSpace(record[3]))
		if err != nil {
			return nil, fmt.Errorf("Could not process interval %v for: %v", record[3], record)
		}

		w := &watcher{
			service: &stela.Service{
				Name:    serviceName,
				Address: ip,
				Port:    int32(port),
				Value:   value,
			},
			interval:    time.Millisecond * time.Duration(interval),
			stelaClient: stelaClient,
		}

		if !w.valid() {
			return nil, fmt.Errorf("Invalid inputs. Could not process record: %v", record)
		}
		watchers = append(watchers, w)
	}

	return watchers, nil
}
