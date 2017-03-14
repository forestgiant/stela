package portutil

import (
	"net"
	"sync"
	"testing"
)

func TestVerifyTCP(t *testing.T) {
	testPort := 8000

	port, err := VerifyTCP(testPort)
	if err != nil {
		t.Errorf("Couldn't Verify port: %d. Error: %s", port, err)
		return
	}

	if port == 0 {
		t.Errorf("Couldn't Verify port: %d. Error: %s", port, err)
		return
	}

	if port != testPort {
		t.Errorf("testPort: %d should equal port: %d", testPort, port)
		return
	}
}

func TestVerifyUDP(t *testing.T) {
	testPort := 8000

	port, err := VerifyUDP(testPort)
	if err != nil {
		t.Errorf("Couldn't Verify port: %d. Error: %s", port, err)
		return
	}

	if port == 0 {
		t.Errorf("Couldn't Verify port: %d. Error: %s", port, err)
		return
	}

	if port != testPort {
		t.Errorf("testPort: %d should equal port: %d", testPort, port)
		return
	}
}

func TestVerify(t *testing.T) {
	testPort := 9000

	tests := []struct {
		netProto string
	}{
		{udp},
		{tcp},
	}
	for _, test := range tests {
		port, err := Verify(test.netProto, testPort)
		if err != nil {
			t.Errorf("Couldn't Verify port on UDP: %d. Error: %s", port, err)
			return
		}

		if port == 0 {
			t.Errorf("Couldn't Verify port on UDP: %d. Error: %s", port, err)
			return
		}

		if port != testPort {
			t.Errorf("testPort: %d should equal UDP port: %d", testPort, port)
			return
		}
	}
}

func TestVerifyHostPort(t *testing.T) {
	tests := []struct {
		netProto string
		address  string
		pass     bool
	}{
		{udp, "127.0.0.1:9080", true},
		{tcp, "127.0.0.1:9080", true},
		{tcp, ":9080", true},
		{udp, "", false},
		{tcp, "", false},
	}
	for _, test := range tests {
		addr, err := VerifyHostPort(test.netProto, test.address)
		if test.pass {
			if err != nil {
				t.Errorf("Couldn't VerifyHostPort: %s. Error: %s", addr, err)
				return
			}

			if addr == "" {
				t.Errorf("Couldn't VerifyHostPort: %s. Error: %s", addr, err)
				return
			}

			if addr != test.address {
				t.Errorf("testPort: %s should equal port: %s", test.address, addr)
				return
			}
		} else {
			if err == nil && addr != "" {
				t.Errorf("VerifyHostPort should error: %s. Error: %s", addr, err)
				return
			}
		}
	}
}

func TestGetUnique(t *testing.T) {
	// test GetUnique
	tests := []struct {
		netProto string
	}{
		{udp},
		{tcp},
	}

	port, err := GetUniqueTCP()
	if err != nil || port == 0 {
		t.Fatalf("Err getting unique port TCP: %s", err)
	}

	port, err = GetUniqueUDP()
	if err != nil || port == 0 {
		t.Fatalf("Err getting unique port UDP: %s", err)
	}

	// test GetUnique
	for _, test := range tests {
		port, err = GetUnique(test.netProto)
		if err != nil || port == 0 {
			t.Fatalf("%s: Err getting unique port: %s", test.netProto, err)
		}
	}

	// open up all ports
	listeners := openAllTCPPorts()

	_, err = GetUniqueTCP()
	if err == nil {
		t.Fatalf("All ports should be closed: %s", err)
	}

	_, err = GetUniqueUDP()
	if err == nil {
		t.Fatalf("All ports should be closed: %s", err)
	}

	// test GetUnique
	for _, test := range tests {
		_, err := GetUnique(test.netProto)
		if err == nil {
			t.Fatalf("%s: All ports should be closed: %s", test.netProto, err)
		}
	}

	// cleanup listeners
	for _, listener := range listeners {
		// close listener
		listener.Close()
	}
}

// This test creates a listener at a port and then calls Verify()
func TestPortTaken(t *testing.T) {
	tests := []struct {
		netProto string
	}{
		{udp},
		{tcp},
	}

	for _, test := range tests {
		port, _ := GetUnique(test.netProto)

		// Create a listener
		var err error
		var ln interface{}

		switch test.netProto {
		case udp:
			ln, err = newListenerUDP(port)
			if err != nil {
				t.Error("Err creating UDP listener", err)
				return
			}
			defer ln.(*net.UDPConn).Close()

			// test newListenerUDP error
			_, err = newListenerUDP(-1)
			if err == nil {
				t.Fatal("Err creating UDP listener", err)
				return
			}
		case tcp:
			ln, err = newListenerTCP(port)
			if err != nil {
				t.Error("Err creating TCP listener", err)
				return
			}
			defer ln.(net.Listener).Close()
		}

		// Now try to use the same port
		_, err = Verify(test.netProto, port)
		if err == nil {
			t.Errorf("Failed to detect port was taken.")
		}

		_, err = VerifyHostPort(test.netProto, JoinHostPort("127.0.0.1", port))
		if err == nil {
			t.Errorf("Failed to detect port was taken.")
		}

	}
}

func TestGetPortFromAddr(t *testing.T) {
	tests := []struct {
		address string
		pass    bool
	}{
		{"127.0.0.1:9080", true},
		{"", false},
		{"):*", false},
	}
	for _, test := range tests {
		port, err := GetPortFromAddr(test.address)
		if err != nil || port == 0 {
			if test.pass {
				t.Fatalf("%s: Err getting unique port: %s", test.address, err)
			}
		}
	}
}

func TestReplacePortInAddr(t *testing.T) {
	tests := []struct {
		address string
		newPort string
		pass    bool
	}{
		{"127.0.0.1:9080", "8080", true},
		{"127.0.0.1:9080", ":8080", false},
		{":9080", "8080", false},
		{"", "", false},
		{"):*", "8000", false},
	}
	for _, test := range tests {
		newAddress, err := ReplacePortInAddr(test.address, test.newPort)
		if err != nil || newAddress == "" {
			if test.pass {
				t.Fatalf("%s: Err getting unique port: %s", test.address, err)
			}
		}
	}
}

// openAllTCPPorts tries to open all tcp ports on your computer
// and returns a slice of net.Listeners so you can close
func openAllTCPPorts() []net.Listener {
	var listeners []net.Listener
	listMutex := new(sync.Mutex)
	waitChan := make(chan struct{})
	shutdownChan := make(chan struct{})

	openPorts := func() {
		ln, err := newListenerTCP(0)
		if err != nil {
			close(waitChan)
			close(shutdownChan)
			return
		}

		// append listener to slice
		listMutex.Lock()
		listeners = append(listeners, ln)
		listMutex.Unlock()
	}

	go func() {
		for {
			select {
			default:
				openPorts()
			case <-shutdownChan:
				return
			}
		}
	}()
	<-waitChan

	return listeners
}
