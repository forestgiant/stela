package multicast

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv6"
)

// Multicast struct
type Multicast struct {
	Address string
	done    chan struct{}
	sendErr error
	mu      sync.Mutex // protect done and closed
	closed  bool
}

// Response struct is sent over a channel when a pong is successful
type Response struct {
	Payload []byte
	SrcIP   net.IP
}

// ErrNoIPv6 error if no IPv6 interfaces were found
var ErrNoIPv6 = errors.New("Couldn't find any IPv6 network intefaces")

// init creates done chan
func (m *Multicast) init() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.done == nil {
		m.done = make(chan struct{})
	}

	if m.Address == "" {
		return errors.New("Address is blank")
	}

	ip, _, err := net.SplitHostPort(m.Address)
	if err != nil {
		return err
	}

	if !net.ParseIP(ip).IsMulticast() {
		return errors.New("multicastAddress is not valid")
	}

	return nil
}

// Done returns a channel that can be used to wait till send is stopped
func (m *Multicast) Done() <-chan struct{} {
	m.init()
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.done
}

// SendErr returns any send errors
func (m *Multicast) SendErr() error {
	return m.sendErr
}

// Stop quits sending over multicast
func (m *Multicast) Stop() {
	m.init()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}

	m.sendErr = nil // reset any send errors after we stop
	close(m.done)
	m.closed = true
}

// Listen when a multicast is received we serve it
func (m *Multicast) Listen(ctx context.Context) (<-chan Response, error) {
	if err := m.init(); err != nil {
		return nil, err
	}

	respCh := make(chan Response)
	gaddr, err := net.ResolveUDPAddr("udp6", m.Address)
	conn, err := net.ListenPacket("udp6", m.Address)
	if err != nil {
		return nil, err
	}

	intfs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	pconn := ipv6.NewPacketConn(conn)
	joined := 0
	for _, intf := range intfs {
		pconn.JoinGroup(&intf, &net.UDPAddr{IP: gaddr.IP})
		joined++
	}

	if joined == 0 {
		return nil, errors.New("no multicast interfaces available")
	}

	// if the context is done close the connection to stop the for loop from blocking
	go func() {
		select {
		case <-ctx.Done():
			pconn.Close()
			return
		case <-m.Done():
			pconn.Close()
			return
		}
	}()

	buf := make([]byte, 65536)
	go func() {
		for {
			select {
			default:
				n, _, src, err := pconn.ReadFrom(buf)
				if err != nil {
					continue
				}

				// make a copy because we will overwrite buf
				b := make([]byte, n)
				copy(b, buf)

				// check if b is a valid address format
				payload := b
				resp := Response{
					Payload: payload,
					SrcIP:   src.(*net.UDPAddr).IP,
				}

				respCh <- resp
			case <-ctx.Done():
				return
			case <-m.Done():
				return
			}
		}
	}()

	return respCh, nil
}

// Send out to try to find others listening
func (m *Multicast) Send(ctx context.Context, interval time.Duration, payload []byte) error {
	if err := m.init(); err != nil {
		return err
	}

	gaddr, err := net.ResolveUDPAddr("udp6", m.Address)
	if err != nil {
		return err
	}

	conn, err := net.ListenPacket("udp6", "[::]:0")
	if err != nil {
		return err
	}

	bs := payload
	intfs, err := net.Interfaces()
	if err != nil {
		return err
	}

	send := func() {
		// Track if any of the interfaces succesfully sent a message
		success := 0

		// Set write control message to 1 so it can be forward by the router
		// TODO: may need to up this if there are multiple routers on the network
		// https://tools.ietf.org/html/rfc2460
		wcm := &ipv6.ControlMessage{
			HopLimit: 1,
		}

		// Create ipv6 packet conn
		pconn := ipv6.NewPacketConn(conn)

		// Loop through all the interfaces
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

			if !containsIPv6(addrs) {
				continue
			}

			wcm.IfIndex = intf.Index
			pconn.SetWriteDeadline(time.Now().Add(time.Second))
			_, err = pconn.WriteTo(bs, wcm, gaddr)
			pconn.SetWriteDeadline(time.Time{})

			if err != nil {
				continue
			}

			// fmt.Println("Sending Ping on interface:", intf.Name, intf.Flags)
			success++
		}

		if success <= 0 {
			// stop the multicast if there was an error and set the error
			m.sendErr = ErrNoIPv6
			m.Stop()
			return
		}
	}

	go func() {
		for {
			select {
			case <-time.After(interval):
				send()
			case <-ctx.Done():
				return
			case <-m.done:
				return
			}
		}
	}()

	// call send right away
	send()

	return nil
}

// containsIPv6 checks to see if any net.Addr has an IPv6 address
func containsIPv6(addrs []net.Addr) bool {
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok {
			if !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() == nil {
					return true
				}
			}
		}
	}

	return false
}
