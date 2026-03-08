package netutil

import "net"

// DetectLocalIP returns the primary non-loopback IPv4 address.
func DetectLocalIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
