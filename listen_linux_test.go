package main

import (
	"context"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"golang.org/x/sys/unix"
)

func TestListenICMPDontFragment(t *testing.T) {
	tests := []struct {
		network, address string
		level, opt, want int
	}{
		{"udp4", "0.0.0.0", unix.IPPROTO_IP, unix.IP_MTU_DISCOVER, unix.IP_PMTUDISC_DO},
		{"udp6", "::", unix.IPPROTO_IPV6, unix.IPV6_DONTFRAG, 1},
		{"ip4:icmp", "0.0.0.0", unix.IPPROTO_IP, unix.IP_MTU_DISCOVER, unix.IP_PMTUDISC_DO},
		{"ip6:ipv6-icmp", "::", unix.IPPROTO_IPV6, unix.IPV6_DONTFRAG, 1},
	}

	for _, tt := range tests {
		t.Run(tt.network, func(t *testing.T) {
			conn, err := listenICMP(tt.network, tt.address, true)
			if err != nil {
				t.Skipf("Cannot open %s socket: %v", tt.network, err)
			}
			defer conn.Close()

			rc, err := conn.(syscall.Conn).SyscallConn()
			if err != nil {
				t.Fatalf("SyscallConn: %v", err)
			}
			var got int
			var gerr error
			rc.Control(func(fd uintptr) {
				got, gerr = unix.GetsockoptInt(int(fd), tt.level, tt.opt)
			})
			if gerr != nil {
				t.Fatalf("getsockopt: %v", gerr)
			}
			if got != tt.want {
				t.Errorf("Expected socket option %d, got %d", tt.want, got)
			}
		})
	}
}

func TestPerformPingDontFragment(t *testing.T) {
	logger := promslog.New(&promslog.Config{})
	caps := DetectCapabilities(logger)
	if !caps.IPv4 {
		t.Skip("No IPv4 ICMP socket available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stats, err := performPing(ctx, &net.IPAddr{IP: net.ParseIP("127.0.0.1")}, "", 1, time.Millisecond, 64, true, caps, logger)
	if err != nil {
		t.Fatalf("Expected DF ping to succeed: %v", err)
	}
	if stats.PacketsReceived != 1 {
		t.Errorf("Expected 1 packet received, got %d", stats.PacketsReceived)
	}
}
