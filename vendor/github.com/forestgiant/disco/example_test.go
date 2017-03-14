package disco

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/forestgiant/disco/node"
)

// Discover any other members on multicasting on an IPv6 multicast address
func DiscoverMembers() {
	// To discover other members you must know what IPv6 multicast address they are multicasting on.
	multicastAddr := "[ff12::9000]:30000"

	// Start discovering
	d := &Disco{}
	discoveredChan, err := d.Discover(context.TODO(), multicastAddr)
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
}

// If you want to want other members to know about your node then you need to create a new Node and multicast.
func RegisterNode() {
	// Must multicast on the same IPv6 multicast address others are Discovering on
	multicastAddr := "[ff12::9000]:21099"

	// Get a unique address
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	// Register ourselve as a node
	n := &node.Node{Payload: []byte(ln.Addr().String()), SendInterval: 2 * time.Second}
	if err := n.Multicast(context.TODO(), multicastAddr); err != nil {
		log.Fatal(err)
	}
}
