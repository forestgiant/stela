package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"github.com/forestgiant/netutil"
	"github.com/forestgiant/semver"

	"gitlab.fg/go/disco"
	"gitlab.fg/go/disco/node"
	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/store/mapstore"
	"gitlab.fg/go/stela/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

const (
	Version = "1.0.0"
)

func main() {
	err := semver.SetVersion(Version)
	if err != nil {
		log.Fatal(err)
	}

	// Check for command line configuration flags
	var (
		stelaAddressUsage  = "Address for stela's gRPC API."
		stelaAddressPtr    = flag.String("address", stela.DefaultStelaAddress, stelaAddressUsage)
		multicastAddrUsage = "Port used to multicast to other stela members."
		multicastPortPtr   = flag.Int("multicast", stela.DefaultMulticastPort, multicastAddrUsage)
	)
	flag.Parse()

	startMessage := fmt.Sprintf("Starting stela gRPC server on: %s and multicasting on port: %d", *stelaAddressPtr, *multicastPortPtr)
	fmt.Println(startMessage)

	// Setup disco

	listener, err := net.Listen("tcp", *stelaAddressPtr)
	if err != nil {
		grpclog.Fatalf("failed to listen: %v", err)
	}

	// Setup credentials
	var opts []grpc.ServerOption
	// creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
	// if err != nil {
	// 	grpclog.Fatalf("Failed to generate credentials %v", err)
	// }

	// opts = []grpc.ServerOption{grpc.Creds(creds)}
	grpcServer := grpc.NewServer(opts...)

	// Create MapStore
	m := &mapstore.MapStore{}

	// Setup transport
	t := &transport.Server{Store: m}

	stela.RegisterStelaServer(grpcServer, t)
	grpcServer.Serve(listener)

	// Setup disco and listen for other stela instances
	multicastAddr := "[ff12::9000]:30000"
	d := &disco.Disco{}
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Start discovering
	discoveredChan, err := d.Discover(ctx, multicastAddr)
	if err != nil {
		log.Fatal(err)
	}

	// When a `Node` is discovered you check it's Action to see if it's being registered or deregistered.
	go func() {
		for {
			select {
			case <-discoveredChan:
				fmt.Println(len(d.Members()), "Members")
				t.SetPeers(d.Members())
			}
		}
	}()

	// Find address

	// Register ourselve as a node
	stelaAddr, err := netutil.ConvertToLocalIPv4(*stelaAddressPtr)
	if err != nil {
		log.Fatal(err)
	}
	n := &node.Node{Values: map[string]string{"Address": stelaAddr}, SendInterval: 2 * time.Second}
	if err := n.Multicast(ctx, multicastAddr); err != nil {
		log.Fatal(err)
	}

	// Listen for shutdown signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigs:
			cancelFunc()
		}
	}()

	// Select will block until a result comes in
	select {
	case <-ctx.Done():
		fmt.Println("Closing membership")
		return
		// os.Exit(0)
	}
}
