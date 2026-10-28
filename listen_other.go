//go:build !linux

package main

import (
	"errors"
	"net"
)

func listenICMPDontFragment(network, address string) (net.PacketConn, error) {
	return nil, errors.New("dont_fragment is only supported on Linux")
}
