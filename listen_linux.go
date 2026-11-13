package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"syscall"

	"golang.org/x/net/icmp"
	"golang.org/x/sys/unix"
)

// listenICMP opens an ICMP socket. network is one of udp4, udp6 (unprivileged
// ping sockets), ip4:icmp or ip6:ipv6-icmp (raw sockets). With dontFragment
// set, outgoing packets are never fragmented and oversized sends fail.
//
// Ping sockets get IP_RECVERR so ICMP errors for our echo requests are queued
// on the socket error queue (see readPacket); raw sockets see them anyway.
func listenICMP(network, address string, dontFragment bool) (net.PacketConn, error) {
	switch network {
	case "udp4", "udp6":
		return listenPingSocket(network, address, dontFragment)
	case "ip4:icmp", "ip6:ipv6-icmp":
		if !dontFragment {
			return icmp.ListenPacket(network, address)
		}
		lc := net.ListenConfig{
			Control: func(_, _ string, c syscall.RawConn) error {
				var serr error
				err := c.Control(func(fd uintptr) {
					serr = setDontFragment(int(fd), network == "ip6:ipv6-icmp")
				})
				if err != nil {
					return err
				}
				return serr
			},
		}
		return lc.ListenPacket(context.Background(), network, address)
	default:
		return nil, fmt.Errorf("unsupported network %q", network)
	}
}

// listenPingSocket mirrors icmp.ListenPacket for unprivileged ping sockets,
// which offers no hook to set socket options before use.
func listenPingSocket(network, address string, dontFragment bool) (net.PacketConn, error) {
	ip := net.ParseIP(address)
	if ip == nil {
		return nil, fmt.Errorf("invalid address %q", address)
	}

	var (
		family, proto int
		sa            unix.Sockaddr
	)
	if network == "udp6" {
		family, proto = unix.AF_INET6, unix.IPPROTO_ICMPV6
		sa6 := &unix.SockaddrInet6{}
		copy(sa6.Addr[:], ip.To16())
		sa = sa6
	} else {
		family, proto = unix.AF_INET, unix.IPPROTO_ICMP
		sa4 := &unix.SockaddrInet4{}
		copy(sa4.Addr[:], ip.To4())
		sa = sa4
	}

	s, err := unix.Socket(family, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, proto)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}
	if dontFragment {
		if err := setDontFragment(s, family == unix.AF_INET6); err != nil {
			unix.Close(s)
			return nil, err
		}
	}
	if err := setRecvErr(s, family == unix.AF_INET6); err != nil {
		unix.Close(s)
		return nil, err
	}
	if err := unix.Bind(s, sa); err != nil {
		unix.Close(s)
		return nil, os.NewSyscallError("bind", err)
	}

	f := os.NewFile(uintptr(s), "datagram-oriented icmp")
	defer f.Close()
	return net.FilePacketConn(f)
}

func setDontFragment(fd int, ipv6 bool) error {
	if ipv6 {
		return os.NewSyscallError("setsockopt", unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_DONTFRAG, 1))
	}
	return os.NewSyscallError("setsockopt", unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_MTU_DISCOVER, unix.IP_PMTUDISC_DO))
}

func setRecvErr(fd int, ipv6 bool) error {
	if ipv6 {
		return os.NewSyscallError("setsockopt", unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_RECVERR, 1))
	}
	return os.NewSyscallError("setsockopt", unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_RECVERR, 1))
}
