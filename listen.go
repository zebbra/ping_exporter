package main

import (
	"net"

	"golang.org/x/net/icmp"
)

// listenICMP opens an ICMP socket. network is one of udp4, udp6 (unprivileged
// ping sockets), ip4:icmp or ip6:ipv6-icmp (raw sockets). With dontFragment
// set, outgoing packets are never fragmented and oversized sends fail.
func listenICMP(network, address string, dontFragment bool) (net.PacketConn, error) {
	if dontFragment {
		return listenICMPDontFragment(network, address)
	}
	return icmp.ListenPacket(network, address)
}
