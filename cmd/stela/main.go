/*
Creates UDP and TCP dns server that responds to SRV record request only
to register a service post JSON to http://127.0.0.1:9000/registerservice
```
{
		"name":     "test.service.fg",
 		"target":   "127.0.0.1",
    	"port":     8010
}
```
You can test if it was registerd with dig:
```
dig @127.0.0.1 -p 8053 test.service.fg SRV
```
*/
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.fg/go/disco"
	"gitlab.fg/go/disco/node"
	"gitlab.fg/go/stela"
	"gitlab.fg/go/stela/discovery"

	"github.com/forestgiant/netutil"
	"github.com/forestgiant/portutil"
	"github.com/forestgiant/raftutil"
	"github.com/forestgiant/semver"
)

const (
	dbName              = "stela.db"        // Name of database file
	raftDBName          = "raft_stela.db"   // Name of raft database file
	peerJSONFileName    = "peer_stela.json" // Name of raft database file
	defaultStelaPort    = 9000              // Default port for stela's http api
	defaultStelaDNSPort = 8053              // Default port to start stela's dns server

	// Version sets semantic version string
	Version = "0.9.5"
)

func main() {
	// Setup Semantic Version flags
	err := semver.SetVersion(Version)
	if err != nil {
		log.Fatal(err)
	}

	// Check for command line configuration flags
	var (
		raftDirUsage       = "Path to raft directory."
		raftDirPtr         = flag.String("raftdir", "", raftDirUsage)
		stelaPortUsage     = "Address for stela's HTTP API."
		stelaPortPtr       = flag.Int("port", defaultStelaPort, stelaPortUsage)
		stelaDNSPortUsage  = "Address for stela dns server port."
		stelaDNSPortPtr    = flag.Int("dnsport", defaultStelaDNSPort, stelaDNSPortUsage)
		multicastAddrUsage = "Port used to multicast to other stela members."
		multicastPortPtr   = flag.Int("multicast", stela.DefaultMulticastPort, multicastAddrUsage)
		enableSingleUsage  = "Start in single node mode. One node needs to be true so others can join."
		enableSinglePtr    = flag.Bool("bootstrap", false, enableSingleUsage)
	)
	flag.Parse()

	// Make error channel
	errCh := make(chan error)

	// Check if ports are avaiable
	_, err = portutil.VerifyTCP(*stelaPortPtr)
	if err != nil {
		log.Fatal(err)
	}

	stelaAddr := portutil.JoinHostPort("", *stelaPortPtr)

	fmt.Println("Stela starting at", stelaAddr)

	// Create the raft addr from the stela port
	_, stelaPort, err := net.SplitHostPort(stelaAddr)
	raftPort, err := discovery.CreateRaftPort(stelaPort)
	raftAddr, err := portutil.ReplacePortInAddr(stelaAddr, raftPort)
	if err != nil {
		log.Fatal(err)
	}

	portutil.VerifyHostPort("tcp", raftAddr)
	if err != nil {
		log.Fatal(err)
	}

	// If local host convert to external ip
	stelaAddr, err = netutil.ConvertToLocalIPv4(stelaAddr)
	if err != nil {
		log.Fatal(err)
	}
	raftAddr, err = netutil.ConvertToLocalIPv4(raftAddr)
	if err != nil {
		log.Fatal(err)
	}

	// Discover stela member nodes
	discoverCtx, cancelDiscover := context.WithCancel(context.Background())
	d := &disco.Disco{}

	// Start discovering
	firstNode := make(chan *node.Node)

	multicastAddr := fmt.Sprintf("%s:%d", stela.DefaultMulticastAddress, *multicastPortPtr)
	discoveredCh, err := d.Discover(discoverCtx, multicastAddr)
	if err != nil {
		log.Fatal(err)
	}

	// If we aren't starting in singleNodeMode then we need to look for other stela instances to join
	var n *node.Node
	if *enableSinglePtr == false {
		// If we don't get a response in 3 seconds then we os.Exit(1)
		t := time.AfterFunc(time.Second*3, func() {
			log.Fatal("Couldn't find another stela node. Check if you are running an instance in bootstrap mode.")
		})

		// See if there are any other stela nodes
		go func() {
			defer t.Stop() // stop the timeout when we find another node
			for {
				select {
				case n := <-discoveredCh:
					fmt.Println(len(d.Members()), "Members")
					firstNode <- n
					return
				case <-discoverCtx.Done():
					return
				}
			}
		}()
		n = <-firstNode // Block until we find another stela node
	}

	// Remove raftDir before setting up service
	if err := raftutil.RemoveRaftFiles(*raftDirPtr); err != nil {
		fmt.Println("RemoveRaftFiles failed", err)
	}

	discovery.SetupSvc(*enableSinglePtr, *stelaPortPtr, *stelaDNSPortPtr, peerJSONFileName, raftAddr, raftDBName, *raftDirPtr)

	// If join was specified, make the join request.
	if *enableSinglePtr == false {
		if err := join(n.Values["StelaAddress"], raftAddr); err != nil {
			log.Fatalf("failed to join node at %s: %s", n.Values, err.Error())
		}
	}

	// When a `Node` is discovered you check it's Action to see if it's being registered or deregistered.
	// go func() {
	// 	for {
	// 		select {
	// 		case n := <-discoveredCh:
	// 			fmt.Println(len(d.Members()), "Members")
	// 			switch n.Action {
	// 			case node.DeregisterAction:
	// 				fmt.Println("Removing", n)
	// 				if err := remove(stelaAddr, raftAddr); err != nil {
	// 					log.Fatalf("failed to remove node at %s: %s", n.Values, err.Error())
	// 				}
	// 			}
	// 		case <-discoverCtx.Done():
	// 			return
	// 		}

	// 	}
	// }()

	// Now let others discover us
	multicastCtx, cancelMulticast := context.WithCancel(context.Background())
	stelaNode := node.Node{Values: map[string]string{"StelaAddress": stelaAddr, "RaftAddress": raftAddr}, SendInterval: 2 * time.Second}
	if err := stelaNode.Multicast(multicastCtx, multicastAddr); err != nil {
		fmt.Println("Multicast error!!")
		log.Fatal(err)
	}

	// Listen for shutdown signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigs:
			cancelDiscover()
			cancelMulticast()
		}
	}()

	// Select will block until context is done or an error
	select {
	case <-multicastCtx.Done():
		fmt.Println("Closing Stela")
		return
	case err := <-errCh:
		fmt.Println("Fatal Error", "Main:", err)
		return
		// case cleanupError := <-cleanupChan:
		// 	if cleanupError != nil {
		// 		fmt.Println("There was an error cleaning up: ", cleanupError)
		// 	}
	}
}

func join(stelaAddr, raftAddr string) error {
	fmt.Println("join?", stelaAddr)

	joinMap := map[string]string{"addr": raftAddr}

	b, err := json.Marshal(joinMap)
	if err != nil {
		return err
	}

	resp, err := http.Post(fmt.Sprintf("http://%s/join", stelaAddr), "application-type/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func remove(stelaAddr, raftAddr string) error {
	joinMap := map[string]string{"addr": raftAddr}

	b, err := json.Marshal(joinMap)
	if err != nil {
		return err
	}

	resp, err := http.Post(fmt.Sprintf("http://%s/remove", stelaAddr), "application-type/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
