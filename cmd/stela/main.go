package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"golang.org/x/net/context"

	"github.com/forestgiant/netutil"
	"github.com/forestgiant/semver"

	"runtime"

	fglog "github.com/forestgiant/log"
	"gitlab.fg/go/disco"
	"gitlab.fg/go/disco/node"
	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/api"
	"gitlab.fg/go/stela/store"
	"gitlab.fg/go/stela/store/mapstore"
	"gitlab.fg/go/stela/transport"
)

func main() {
	logger := fglog.Logger{}.With("time", fglog.DefaultTimestamp, "caller", fglog.DefaultCaller, "service", "stela")
	err := semver.SetVersion(stela.Version)
	if err != nil {
		logger.Error("error", err.Error())
		os.Exit(1)
	}

	// Check for command line configuration flags
	var (
		statusUsage        = "Shows how many stela instances are currently running. *Only works if you are running a local stela instance."
		statusPtr          = flag.Bool("status", false, statusUsage)
		stelaPortUsage     = "Port for stela's gRPC API."
		stelaPortPtr       = flag.Int("port", stela.DefaultStelaPort, stelaPortUsage)
		multicastPortUsage = "Port used to multicast to other stela members."
		multicastPortPtr   = flag.Int("multicast", stela.DefaultMulticastPort, multicastPortUsage)
	)
	flag.Parse()

	if *statusPtr {
		stelas, err := discoverStelas()
		if err != nil {
			fmt.Println("There are 0 stela instances currently running. Make sure you are running a local stela instance.")
		} else {
			fmt.Printf("There are %d stela instances running.\n", len(stelas))
		}

		os.Exit(0)
	}

	stelaAddr := fmt.Sprintf("127.0.0.1:%d", *stelaPortPtr)
	startMessage := fmt.Sprintf("Starting stela gRPC server on: %s and multicasting on port: %d", stelaAddr, *multicastPortPtr)
	fmt.Println(startMessage)

	// Create store and transport
	m := &mapstore.MapStore{}
	t := &transport.Server{Store: m, Timeout: 1 * time.Second}

	// Setup disco and listen for other stela instances
	multicastAddr := fmt.Sprintf("%s:%d", stela.DefaultMulticastAddress, *multicastPortPtr)
	d := &disco.Disco{}
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Start discovering
	discoveredChan, err := d.Discover(ctx, multicastAddr)
	if err != nil {
		logger.Error("error", err.Error())
		os.Exit(1)
	}

	// When a `Node` is discovered update the transport peers
	go func() {
		for {
			select {
			case <-discoveredChan:
				t.SetPeers(d.Members())
			case <-ctx.Done():
			}
		}
	}()

	// Register ourselves as a node
	networkAddr, err := netutil.ConvertToLocalIPv4(stelaAddr)
	if err != nil {
		logger.Error("error", err.Error())
		os.Exit(1)
	}

	n := &node.Node{Values: map[string]string{"Address": networkAddr}, SendInterval: 2 * time.Second}
	if err := n.Multicast(ctx, multicastAddr); err != nil {
		logger.Error("error", err.Error())
		os.Exit(1)
	}

	//Setup gRPC server
	listener, err := net.Listen("tcp", stelaAddr)
	if err != nil {
		logger.Error("failed to listen:", "error", err.Error())
		os.Exit(1)
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

	go func() {
		grpcServer.Serve(listener)
	}()
	runtime.Gosched()

	// Now register ourselves as a service
	if err := registerStela(m, stelaAddr); err != nil {
		logger.Error("error", err.Error(), stelaAddr)
		os.Exit(1)
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

	// Select will block until a signal comes in
	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
		fmt.Println("Closing stela")
		return
	}
}

func registerStela(s store.Store, networkAddr string) error {
	host, err := os.Hostname()
	if err != nil {
		return err
	}

	ip, p, err := net.SplitHostPort(networkAddr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return err
	}
	if err := s.Register(&stela.Service{Name: stela.ServiceName, Target: host, Address: ip, Port: int32(port)}); err != nil {
		return err
	}

	return nil
}

func discoverStelas() ([]*stela.Service, error) {
	c, err := api.NewClient(context.Background(), stela.DefaultStelaAddress, "../certs/ca.pem")
	if err != nil {
		return nil, err
	}
	defer c.Close()

	stelas, err := c.Discover(stela.ServiceName)
	if err != nil {
		return nil, err
	}

	return stelas, nil
}
