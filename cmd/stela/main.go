package main

import (
	"net"

	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/store/mapstore"
	"gitlab.fg/go/stela/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

func main() {
	listener, err := net.Listen("tcp", stela.DefaultStelaAddress)
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
}
