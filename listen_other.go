//go:build !linux

package main

import (
	"errors"
	"net"

	"golang.org/x/net/icmp"
)

// listenICMP opens an ICMP socket. network is one of udp4, udp6 (unprivileged
// ping sockets), ip4:icmp or ip6:ipv6-icmp (raw sockets).
func listenICMP(network, address string, dontFragment bool) (net.PacketConn, error) {
	if dontFragment {
		return nil, errors.New("dont_fragment is only supported on Linux")
	}
	return icmp.ListenPacket(network, address)
}

// readPacket reads the next ICMP message. Only Linux ping sockets report
// errors out of band, so there is never a queued error here.
func readPacket(conn net.PacketConn, b []byte) (int, net.Addr, *queuedICMPError, error) {
	n, peer, err := conn.ReadFrom(b)
	return n, peer, nil, err
}
