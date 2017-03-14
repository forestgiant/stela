package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/forestgiant/disco"
	"github.com/forestgiant/disco/node"
)

// Discover other nodes with the disco package via multicast
// This creates a simple membership list of nodes
func main() {
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
			case n := <-discoveredChan:
				fmt.Println(len(d.Members()), "Members")
				switch n.Action {
				case node.RegisterAction:
					fmt.Println("Adding", n)
				case node.DeregisterAction:
					fmt.Println("Removing", n)
				}
			}
		}
	}()

	// Get a unique address
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	// Register ourselve as a node
	n := &node.Node{Payload: []byte(ln.Addr().String()), SendInterval: 2 * time.Second}
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
