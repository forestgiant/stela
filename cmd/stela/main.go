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

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"golang.org/x/net/context"

	"github.com/forestgiant/netutil"
	"github.com/forestgiant/semver"

	"gitlab.fg/go/disco"
	"gitlab.fg/go/disco/node"
	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/store/mapstore"
	"gitlab.fg/go/stela/transport"
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
		stelaPortUsage     = "Port for stela's gRPC API."
		stelaPortPtr       = flag.Int("port", stela.DefaultStelaPort, stelaPortUsage)
		multicastPortUsage = "Port used to multicast to other stela members."
		multicastPortPtr   = flag.Int("multicast", stela.DefaultMulticastPort, multicastPortUsage)
	)
	flag.Parse()

	stelaAddr := fmt.Sprintf("127.0.0.1:%d", *stelaPortPtr)
	startMessage := fmt.Sprintf("Starting stela gRPC server on: %s and multicasting on port: %d", stelaAddr, *multicastPortPtr)
	fmt.Println(startMessage)

	// Create store and transport
	m := &mapstore.MapStore{}
	t := &transport.Server{Store: m}

	// Setup disco and listen for other stela instances
	multicastAddr := fmt.Sprintf("%s:%d", stela.DefaultMulticastAddress, *multicastPortPtr)
	d := &disco.Disco{}
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Start discovering
	discoveredChan, err := d.Discover(ctx, multicastAddr)
	if err != nil {
		log.Fatal(err)
	}

	// When a `Node` is discovered update the transport peers
	go func() {
		for {
			select {
			case <-discoveredChan:
				fmt.Println(len(d.Members()), "Members")
				t.SetPeers(d.Members())
			case <-ctx.Done():
			}
		}
	}()

	// Register ourselves as a node
	networkAddr, err := netutil.ConvertToLocalIPv4(stelaAddr)
	if err != nil {
		log.Fatal(err)
	}

	n := &node.Node{Values: map[string]string{"Address": networkAddr}, SendInterval: 2 * time.Second}
	if err := n.Multicast(ctx, multicastAddr); err != nil {
		log.Fatal(err)
	}

	//Setup gRPC server
	listener, err := net.Listen("tcp", stelaAddr)
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

	stela.RegisterStelaServer(grpcServer, t)
	grpcServer.Serve(listener)

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
		fmt.Println("Closing stela")
		return
	}
}
