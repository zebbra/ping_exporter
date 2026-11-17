package main

import (
	"encoding/binary"
	"fmt"
	"net"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// icmpError is returned for an echo request that was answered with an ICMP
// error (destination unreachable, time exceeded, ...) instead of a reply.
type icmpError struct {
	reason string
	from   net.IP
	typ    int
	code   int
}

func (e *icmpError) Error() string {
	return fmt.Sprintf("ICMP error from %s: %s (type %d, code %d)", e.from, e.reason, e.typ, e.code)
}

// queuedICMPError is an ICMP error read from a ping socket's error queue.
type queuedICMPError struct {
	from net.IP
	typ  int
	code int
	seq  int
}

// icmpErrorReason maps an ICMP error type/code to the reason label of
// ping_exporter_icmp_errors_total. The label set is fixed.
func icmpErrorReason(ipv6 bool, typ, code int) string {
	if ipv6 {
		switch typ {
		case 1: // Destination Unreachable
			switch code {
			case 0, 6: // no route, reject route
				return "net_unreachable"
			case 1, 5: // administratively prohibited, source address failed policy
				return "admin_prohibited"
			case 3: // address unreachable
				return "host_unreachable"
			case 4:
				return "port_unreachable"
			}
		case 2:
			return "frag_needed" // Packet Too Big
		case 3:
			if code == 0 {
				return "ttl_exceeded" // hop limit exceeded
			}
		case 4:
			return "parameter_problem"
		}
		return "other"
	}

	switch typ {
	case 3: // Destination Unreachable
		switch code {
		case 0, 6, 11: // net unreachable, net unknown, net unreachable for TOS
			return "net_unreachable"
		case 1, 7, 12: // host unreachable, host unknown, host unreachable for TOS
			return "host_unreachable"
		case 2:
			return "protocol_unreachable"
		case 3:
			return "port_unreachable"
		case 4:
			return "frag_needed"
		case 9, 10, 13: // net/host/communication administratively prohibited
			return "admin_prohibited"
		}
	case 11:
		if code == 0 {
			return "ttl_exceeded"
		}
	case 12:
		return "parameter_problem"
	}
	return "other"
}

// originalDatagram returns the invoking packet quoted in an ICMP error
// message, or nil if msg is not an error message.
func originalDatagram(msg *icmp.Message) []byte {
	switch b := msg.Body.(type) {
	case *icmp.DstUnreach:
		return b.Data
	case *icmp.TimeExceeded:
		return b.Data
	case *icmp.PacketTooBig:
		return b.Data
	case *icmp.ParamProb:
		return b.Data
	}
	return nil
}

// matchQuotedEcho reports whether data, the IP packet quoted in an ICMP error,
// is the echo request we sent to dst with the given seq (and id, if checkID).
func matchQuotedEcho(data []byte, dst net.IP, id, seq int, checkID bool) bool {
	var (
		echo    []byte
		quoted  net.IP
		reqType byte
	)
	if dst.To4() != nil {
		if len(data) < 20 || data[0]>>4 != 4 {
			return false
		}
		ihl := int(data[0]&0x0f) * 4
		if ihl < 20 || len(data) < ihl+8 || data[9] != 1 { // protocol ICMP
			return false
		}
		quoted, echo, reqType = net.IP(data[16:20]), data[ihl:], 8
	} else {
		// Our echo requests carry no extension headers.
		if len(data) < 48 || data[0]>>4 != 6 || data[6] != 58 { // next header ICMPv6
			return false
		}
		quoted, echo, reqType = net.IP(data[24:40]), data[40:], 128
	}

	if !quoted.Equal(dst) || echo[0] != reqType {
		return false
	}
	if checkID && int(binary.BigEndian.Uint16(echo[4:6])) != id {
		return false
	}
	return int(binary.BigEndian.Uint16(echo[6:8])) == seq
}

// icmpTypeCode returns the numeric ICMP type of msg.
func icmpTypeCode(msg *icmp.Message) int {
	switch t := msg.Type.(type) {
	case ipv4.ICMPType:
		return int(t)
	case ipv6.ICMPType:
		return int(t)
	}
	return -1
}
