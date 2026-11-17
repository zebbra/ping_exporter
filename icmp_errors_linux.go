package main

import (
	"encoding/binary"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// readPacket reads the next ICMP message. On ping sockets (udp4/udp6), ICMP
// errors are not delivered as packets but queued on the socket error queue
// (IP_RECVERR); those are returned as a queuedICMPError with n == 0. Raw
// sockets receive errors as regular ICMP messages.
func readPacket(conn net.PacketConn, b []byte) (int, net.Addr, *queuedICMPError, error) {
	uc, ok := conn.(*net.UDPConn)
	if !ok {
		n, peer, err := conn.ReadFrom(b)
		return n, peer, nil, err
	}
	rc, err := uc.SyscallConn()
	if err != nil {
		return 0, nil, nil, err
	}

	var (
		n    int
		peer net.Addr
		qerr *queuedICMPError
		rerr error
	)
	oob := make([]byte, 256)
	err = rc.Read(func(fd uintptr) bool {
		for {
			en, oobn, _, _, e := unix.Recvmsg(int(fd), b, oob, unix.MSG_ERRQUEUE|unix.MSG_DONTWAIT)
			if e == nil {
				qerr = parseErrQueue(b[:en], oob[:oobn])
				return true
			}

			var from unix.Sockaddr
			n, from, e = unix.Recvfrom(int(fd), b, unix.MSG_DONTWAIT)
			switch e {
			case nil:
				peer = sockaddrToUDPAddr(from)
				return true
			case unix.EAGAIN:
				return false
			case unix.ENETUNREACH, unix.EHOSTUNREACH, unix.ECONNREFUSED, unix.EMSGSIZE, unix.EACCES, unix.EPROTO:
				// Pending socket error reported alongside a queued ICMP
				// error; go back and read it from the error queue.
				continue
			default:
				rerr = os.NewSyscallError("recvfrom", e)
				return true
			}
		}
	})
	if err != nil {
		return 0, nil, nil, err
	}
	return n, peer, qerr, rerr
}

// parseErrQueue decodes a message read with MSG_ERRQUEUE from a ping socket.
// payload is the echo request the error refers to. It returns nil for
// anything that is not an ICMP error.
func parseErrQueue(payload, oob []byte) *queuedICMPError {
	cmsgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil
	}
	for _, c := range cmsgs {
		v4 := c.Header.Level == unix.IPPROTO_IP && c.Header.Type == unix.IP_RECVERR
		v6 := c.Header.Level == unix.IPPROTO_IPV6 && c.Header.Type == unix.IPV6_RECVERR
		if !v4 && !v6 {
			continue
		}
		// struct sock_extended_err, followed by the offender's sockaddr.
		d := c.Data
		if len(d) < 16 {
			continue
		}
		origin := d[4]
		if origin != unix.SO_EE_ORIGIN_ICMP && origin != unix.SO_EE_ORIGIN_ICMP6 {
			continue
		}
		q := &queuedICMPError{typ: int(d[5]), code: int(d[6]), seq: -1}
		if len(payload) >= 8 {
			q.seq = int(binary.BigEndian.Uint16(payload[6:8]))
		}
		sa := d[16:]
		if len(sa) >= 2 {
			switch binary.NativeEndian.Uint16(sa[0:2]) {
			case unix.AF_INET:
				if len(sa) >= 8 {
					q.from = net.IP(append([]byte(nil), sa[4:8]...))
				}
			case unix.AF_INET6:
				if len(sa) >= 24 {
					q.from = net.IP(append([]byte(nil), sa[8:24]...))
				}
			}
		}
		return q
	}
	return nil
}

func sockaddrToUDPAddr(sa unix.Sockaddr) net.Addr {
	switch a := sa.(type) {
	case *unix.SockaddrInet4:
		return &net.UDPAddr{IP: net.IP(append([]byte(nil), a.Addr[:]...))}
	case *unix.SockaddrInet6:
		addr := &net.UDPAddr{IP: net.IP(append([]byte(nil), a.Addr[:]...))}
		if a.ZoneId != 0 {
			if ifi, err := net.InterfaceByIndex(int(a.ZoneId)); err == nil {
				addr.Zone = ifi.Name
			}
		}
		return addr
	}
	return nil
}
