package netutil

import (
	"errors"
	"net"
	"strings"
)

// IsLocalhost takes an address and checks to see if
// it matches the local computers
func IsLocalhost(target string) bool {
	ip := net.ParseIP(target)
	if ip == nil {
		return false
	}

	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ip.Equal(ipnet.IP) {
				return true
			}
		}
	}

	return false
}

// LocalIPv4 return the ipv4 address of the computer
// If it can't get the local ip it returns 127.0.0.1
func LocalIPv4() net.IP {
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

// LocalIPv6 return the ipv6 address of the computer
// If it can't get the local ip it returns net.IPv6loopback
func LocalIPv6() net.IP {
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

// ConvertToLocalIPv4 takes a loopback address and converts it to LocalIPV4
func ConvertToLocalIPv4(addr string) (string, error) {
	// Break a part addresses
	ip, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}

	// If local host convert to external ip
	if IsLocalhost(ip) || ip == "" {
		ip = LocalIPv4().String()
	} else {
		return "", errors.New("Address host must be localhost")
	}

	// Combine back
	return net.JoinHostPort(ip, port), nil
}
