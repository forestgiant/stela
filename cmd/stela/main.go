package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"

	"golang.org/x/net/context"

	"github.com/forestgiant/netutil"
	"github.com/forestgiant/semver"

	"runtime"

	"github.com/forestgiant/disco"
	"github.com/forestgiant/disco/node"
	"github.com/forestgiant/stela"
	"github.com/forestgiant/stela/api"
	"github.com/forestgiant/stela/pb"
	"github.com/forestgiant/stela/store"
	"github.com/forestgiant/stela/store/mapstore"
	"github.com/forestgiant/stela/transport"

	fggrpclog "github.com/forestgiant/grpclog"
	fglog "github.com/forestgiant/log"
)

func init() {
	l := fglog.Logger{}.With("logger", "grpc")
	grpclog.SetLogger(&fggrpclog.Structured{Logger: &l})
}

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
		certPathUsage      = "Path to the certificate file for the server."
		certPathPtr        = flag.String("cert", "server.crt", certPathUsage)
		keyPathUsage       = "Path to the private key file for the server."
		keyPathPtr         = flag.String("key", "server.key", keyPathUsage)
		caPathUsage        = "Path to the private key file for the server."
		caPathPtr          = flag.String("ca", "ca.crt", caPathUsage)
		serverNameUsage    = "The common name of the server you are connecting to."
		serverNamePtr      = flag.String("serverName", stela.DefaultServerName, serverNameUsage)
		insecureUsage      = "Disable SSL, allowing unenecrypted communication with this service."
		insecurePtr        = flag.Bool("insecure", false, insecureUsage)
	)
	flag.Parse()

	if *statusPtr {
		stelas, err := discoverStelas(*insecurePtr, *certPathPtr, *keyPathPtr, *caPathPtr, *serverNamePtr)
		if err != nil {
			fmt.Println("There are 0 stela instances currently running. Make sure you are running a local stela instance.")
		} else {
			fmt.Printf("There are %d stela instances running.\n", len(stelas))
		}

		os.Exit(0)
	}

	stelaAddr := fmt.Sprintf(":%d", *stelaPortPtr)
	startMessage := fmt.Sprintf("Starting stela gRPC server on: %s and multicasting on port: %d", stelaAddr, *multicastPortPtr)
	logger.Info(startMessage)

	// Create store and transport
	m := &mapstore.MapStore{}
	t := &transport.Server{
		Store:   m,
		Timeout: 1 * time.Second,
	}

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
		logger.Error("netutil failed to convert to local ipv4:", "error", err.Error())
		os.Exit(1)
	}

	n := &node.Node{Payload: []byte(networkAddr), SendInterval: 2 * time.Second}
	multicastErrCh := n.Multicast(ctx, multicastAddr)

	// Log any multicast errors
	go func() {
		for {
			select {
			case err := <-multicastErrCh:
				fmt.Println(err)
			case <-ctx.Done():
				return
			}
		}
	}()

	//Setup gRPC server
	listener, err := net.Listen("tcp", stelaAddr)
	if err != nil {
		logger.Error("failed to listen:", "error", err.Error())
		os.Exit(1)
	}

	// Setup credentials
	var opts []grpc.ServerOption
	if !*insecurePtr {
		// Add proxy info to transport server
		t.Proxy = &transport.Proxy{
			ServerName: *serverNamePtr,
			CAPath:     *caPathPtr,
			CertPath:   *certPathPtr,
			KeyPath:    *keyPathPtr,
		}

		// Load the certificates from disk
		certificate, err := tls.LoadX509KeyPair(*certPathPtr, *keyPathPtr)
		if err != nil {
			logger.Error("Failed to load certificate:", "error", err.Error())
			os.Exit(1)
		}

		// Create a certificate pool from the certificate authority
		certPool := x509.NewCertPool()
		ca, err := ioutil.ReadFile(*caPathPtr)
		if err != nil {
			logger.Error("Failed to read CA certificate:", "error", err.Error())
			os.Exit(1)
		}

		// Append the client certificates from the CA
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			logger.Error("Failed to append client cert:.", "error", err.Error())
			os.Exit(1)
		}

		// Create the TLS credentials
		creds := credentials.NewTLS(&tls.Config{
			ClientAuth:   tls.RequireAndVerifyClientCert,
			Certificates: []tls.Certificate{certificate},
			ClientCAs:    certPool,
		})

		opts = append(opts, grpc.Creds(creds))
	}

	grpcServer := grpc.NewServer(opts...)
	pb.RegisterStelaServer(grpcServer, t)

	go func() {
		grpcServer.Serve(listener)
	}()
	runtime.Gosched()

	// Now register ourselves as a service
	if err := registerStela(m, networkAddr); err != nil {
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
		grpcServer.Stop()
		logger.Info("Closing stela")
		return
	}
}

func registerStela(s store.Store, networkAddr string) error {
	name, err := os.Hostname()
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
	if err := s.Register(&stela.Service{Name: stela.ServiceName, Hostname: name, IPv4: ip, Port: int32(port)}); err != nil {
		return err
	}

	return nil
}

func discoverStelas(insecure bool, certPath, keyPath, caFile, serverNameOverride string) ([]*stela.Service, error) {
	var c *api.Client
	var err error
	if insecure {
		c, err = api.NewClient(context.Background(), stela.DefaultStelaAddress, nil)
		if err != nil {
			return nil, err
		}
	} else {
		c, err = api.NewTLSClient(context.Background(), stela.DefaultStelaAddress, serverNameOverride, certPath, keyPath, caFile)
		if err != nil {
			return nil, err
		}
	}

	defer c.Close()

	ctx, cancelFunc := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFunc()
	stelas, err := c.Discover(ctx, stela.ServiceName)
	if err != nil {
		return nil, err
	}

	return stelas, nil
}
