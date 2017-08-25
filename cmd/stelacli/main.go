package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	fglog "github.com/forestgiant/log"
	"github.com/forestgiant/semver"
	"github.com/forestgiant/stela"
	"github.com/forestgiant/stela/api"
	"golang.org/x/net/context"
)

func main() {
	logger := fglog.Logger{}.With("time", fglog.DefaultTimestamp, "caller", fglog.DefaultCaller, "cli", "stelacli")
	err := semver.SetVersion(stela.Version)
	if err != nil {
		logger.Error("Unable to set semantic version.", "error", err.Error())
		os.Exit(1)
	}

	// Check for command line configuration flags
	var (
		certPathPtr     = flag.String("cert", "client.crt", "Path to the certificate file for the server.")
		keyPathPtr      = flag.String("key", "client.key", "Path to the private key file for the server.")
		caPathPtr       = flag.String("ca", "ca.crt", "Path to the private key file for the server.")
		stelaAddressPtr = flag.String("stelaAddr", stela.DefaultStelaAddress, "Address of stela instance to connect to.")
		insecurePtr     = flag.Bool("insecure", false, "Disable SSL, allowing unenecrypted communication with this service.")

		// CLI API Flags
		// Discover (Regex, One, Regex)
		discoverAllPtr = flag.Bool("discoverAll", false, "Prints all registered services from a stela instance.")
	)
	flag.Parse()

	ctx, cancelFunc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFunc()

	var stelaClient *api.Client
	if *insecurePtr {
		stelaClient, err = api.NewClient(ctx, *stelaAddressPtr, nil)
		if err != nil {
			logger.Error("Failed to create stela client. Make sure there is a stela instance running", "error", err.Error())
			os.Exit(1)
		}
	} else {
		stelaClient, err = api.NewTLSClient(ctx, *stelaAddressPtr, stela.DefaultServerName, *certPathPtr, *keyPathPtr, *caPathPtr)
		if err != nil {
			logger.Error("Failed to create stela client. Make sure there is a stela instance running", "error", err.Error())
			os.Exit(1)
		}
	}
	defer stelaClient.Close()

	if *discoverAllPtr {
		services, err := stelaClient.DiscoverAll(ctx)
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(services)
		}
	}
}
