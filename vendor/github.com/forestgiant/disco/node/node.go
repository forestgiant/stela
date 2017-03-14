package node

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/adler32"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/forestgiant/disco/multicast"
)

// Modes of a node being registered
const (
	RegisterAction   = iota
	DeregisterAction = iota
)

// Node represents a machine registered with Disco
type Node struct {
	// Values       Values
	Payload      []byte // max 256 bytes
	SrcIP        net.IP
	SendInterval time.Duration
	Action       int
	ipv6         net.IP // set by localIPv4 function
	ipv4         net.IP // set by localIPv6 function
	mc           *multicast.Multicast
	mu           sync.Mutex // protect ipv4, ipv6, mc, SendInterval, registerCh
	registerCh   chan struct{}
}

// Values stores any values passed to the node
type Values map[string]string

func (n *Node) init() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.registerCh == nil {
		n.registerCh = make(chan struct{})
	}
}

func (n *Node) String() string {
	return fmt.Sprintf("IPv4: %s, IPv6: %s, Payload: %s", n.ipv4, n.ipv6, n.Payload)
}

// Equal compares nodes
func (n *Node) Equal(b *Node) bool {
	n.mu.Lock()
	b.mu.Lock()
	defer n.mu.Unlock()
	defer b.mu.Unlock()

	if !n.ipv4.Equal(b.ipv4) {
		return false
	}
	if !n.ipv6.Equal(b.ipv6) {
		return false
	}

	// Check if the payloads are the same
	if !bytes.Equal(n.Payload, b.Payload) {
		return false
	}

	return true
}

// Encode will convert a Node to a byte slice
// Checksum - 4 bytes
// IPv4 value - 4 bytes
// IPv6 value - 16 bytes
// Payload (unknown length)
func (n *Node) Encode() []byte {
	payloadSize := len(n.Payload)
	buf := make([]byte, 32+payloadSize)

	// Add IPv4 to buffer
	index := 4 // after checksum
	for i, b := range n.IPv4().To4() {
		buf[index+i] = b
	}

	// Add IPv6 to buffer
	index = 8 // after ipv4
	for i, b := range n.IPv6() {
		buf[index+i] = b
	}

	// Add SendInterval int64
	index = 24
	for i := uint(0); i < 8; i++ {
		buf[index+int(i)] = byte(n.SendInterval >> (i * 8))
	}

	// Add Payload to buffer
	index = 32 // after SendInterval
	for i, b := range n.Payload {
		buf[index+i] = b
	}

	// Run a checksum on the existing buffer
	checksum := adler32.Checksum(buf[4:])

	// Add checksum to buffer
	for i := uint(0); i < 4; i++ {
		buf[int(i)] = byte(checksum >> (i * 8))
	}

	return buf
}

// DecodeNode decodes the bytes and returns a *Node struct
func DecodeNode(b []byte) (*Node, error) {
	// Verify checksum
	checksum := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	if checksum != adler32.Checksum(b[4:]) {
		return nil, errors.New("checksum didn't match")
	}

	// Get ipv4
	index := 4 // after checksum
	ipv4 := make(net.IP, net.IPv4len)
	for i := range ipv4 {
		ipv4[i] = b[index+i]
	}

	// Get ipv6
	index = 8 // after ipv4
	ipv6 := make(net.IP, net.IPv6len)
	for i := range ipv6 {
		ipv6[i] = b[index+i]
	}

	// If the ips returned are unspecified then return nil
	if ipv4.IsUnspecified() {
		ipv4 = nil
	}
	if ipv6.IsUnspecified() {
		ipv6 = nil
	}

	// Decode SendInterval
	index = 24
	sendInterval := uint64(b[index+0]) | uint64(b[index+1])<<8 | uint64(b[index+2])<<16 | uint64(b[index+3])<<24 |
		uint64(b[index+4])<<32 | uint64(b[index+5])<<40 | uint64(b[index+6])<<48 | uint64(b[index+7])<<56

	// Get payload
	payload := b[32:]

	return &Node{ipv4: ipv4, ipv6: ipv6, SendInterval: time.Duration(sendInterval), Payload: payload}, nil
}

// // GobEncode gob interface
// func (n *Node) GobEncode() ([]byte, error) {
// 	n.mu.Lock()
// 	defer n.mu.Unlock()

// 	w := new(bytes.Buffer)
// 	encoder := gob.NewEncoder(w)

// 	if err := encoder.Encode(n.SendInterval); err != nil {
// 		return nil, err
// 	}

// 	if err := encoder.Encode(n.ipv4); err != nil {
// 		return nil, err
// 	}

// 	if err := encoder.Encode(n.ipv6); err != nil {
// 		return nil, err
// 	}

// 	if err := encoder.Encode(n.Payload); err != nil {
// 		return nil, err
// 	}

// 	return w.Bytes(), nil
// }

// // GobDecode gob interface
// func (n *Node) GobDecode(buf []byte) error {
// 	n.mu.Lock()
// 	defer n.mu.Unlock()

// 	r := bytes.NewBuffer(buf)
// 	decoder := gob.NewDecoder(r)

// 	if err := decoder.Decode(&n.SendInterval); err != nil {
// 		return err
// 	}

// 	if err := decoder.Decode(&n.ipv4); err != nil {
// 		return err
// 	}

// 	if err := decoder.Decode(&n.ipv6); err != nil {
// 		return err
// 	}

// 	return decoder.Decode(&n.Payload)
// }

// Done returns a channel that can be used to wait till Multicast is stopped
func (n *Node) Done() <-chan struct{} {
	return n.mc.Done()
}

// RegisterCh returns a channel to know if the node should stay registered
func (n *Node) RegisterCh() <-chan struct{} {
	n.init()
	return n.registerCh
}

// KeepRegistered sends an anonymous struct{} to registeredChan to indicate the node should stay registered
func (n *Node) KeepRegistered() {
	n.init()
	n.registerCh <- struct{}{}
}

// Multicast start the mulicast ping
func (n *Node) Multicast(ctx context.Context, multicastAddress string) error {
	n.mu.Lock()
	n.ipv4 = localIPv4()
	n.ipv6 = localIPv6()

	if n.SendInterval.Seconds() == float64(0) {
		n.SendInterval = 1 * time.Second // default to 1 second
	}

	n.mu.Unlock()

	// Encode node to be sent via multicast
	n.mu.Lock()
	n.mc = &multicast.Multicast{Address: multicastAddress}
	n.mu.Unlock()
	if err := n.mc.Send(ctx, n.SendInterval, n.Encode()); err != nil {
		return err
	}

	return nil
}

// Stop closes the StopCh to stop multicast sending
func (n *Node) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.mc.Stop()
}

// IPv4 getter for ipv4Address
func (n *Node) IPv4() net.IP {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ipv4
}

// IPv6 getter for ipv6Address
func (n *Node) IPv6() net.IP {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.ipv6
}

// localIPv4 return the ipv4 address of the computer
// If it can't get the local ip it returns 127.0.0.1
// https://github.com/forestgiant/netutil
func localIPv4() net.IP {
	loopback := net.ParseIP("127.0.0.1")

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return loopback
	}

	for _, addr := range addrs {
		// check the address type and make sure it's not loopback
		if ipnet, ok := addr.(*net.IPNet); ok {
			if !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.To4()
				}
			}
		}
	}

	return loopback
}

// localIPv6 return the ipv6 address of the computer
// If it can't get the local ip it returns net.IPv6loopback
// https://github.com/forestgiant/netutil
func localIPv6() net.IP {
	loopback := net.IPv6loopback

	intfs, err := net.Interfaces()
	if err != nil {
		return loopback
	}

	for _, intf := range intfs {
		// If the interface is a loopback or doesn't have multicasting let's skip it
		if strings.Contains(intf.Flags.String(), net.FlagLoopback.String()) || !strings.Contains(intf.Flags.String(), net.FlagMulticast.String()) {
			continue
		}

		// Now let's check if the interface has an ipv6 address
		addrs, err := intf.Addrs()
		if err != nil {
			continue
		}

		for _, address := range addrs {
			if ipnet, ok := address.(*net.IPNet); ok {
				if !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() == nil {
						return ipnet.IP
					}
				}
			}
		}
	}

	return loopback
}
