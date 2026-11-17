package main

import (
	"encoding/binary"
	"net"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func errQueueCmsg(level, typ int, origin, icmpType, icmpCode byte, sa []byte) []byte {
	data := make([]byte, 16, 16+len(sa))
	data[4], data[5], data[6] = origin, icmpType, icmpCode
	data = append(data, sa...)

	b := make([]byte, unix.CmsgSpace(len(data)))
	h := (*unix.Cmsghdr)(unsafe.Pointer(&b[0]))
	h.Level, h.Type = int32(level), int32(typ)
	h.SetLen(unix.CmsgLen(len(data)))
	copy(b[unix.CmsgLen(0):], data)
	return b
}

func TestParseErrQueue(t *testing.T) {
	echo := []byte{8, 0, 0, 0, 0, 1, 0, 7} // seq 7

	sa4 := make([]byte, 16)
	binary.NativeEndian.PutUint16(sa4[0:2], unix.AF_INET)
	copy(sa4[4:8], net.ParseIP("198.51.100.1").To4())

	sa6 := make([]byte, 28)
	binary.NativeEndian.PutUint16(sa6[0:2], unix.AF_INET6)
	copy(sa6[8:24], net.ParseIP("2001:db8::fe"))

	t.Run("v4 net unreachable", func(t *testing.T) {
		q := parseErrQueue(echo, errQueueCmsg(unix.IPPROTO_IP, unix.IP_RECVERR, unix.SO_EE_ORIGIN_ICMP, 3, 0, sa4))
		if q == nil {
			t.Fatal("expected queued error")
		}
		if q.typ != 3 || q.code != 0 || q.seq != 7 || !q.from.Equal(net.ParseIP("198.51.100.1")) {
			t.Errorf("unexpected result %+v", q)
		}
	})

	t.Run("v6 no route", func(t *testing.T) {
		q := parseErrQueue(echo, errQueueCmsg(unix.IPPROTO_IPV6, unix.IPV6_RECVERR, unix.SO_EE_ORIGIN_ICMP6, 1, 0, sa6))
		if q == nil {
			t.Fatal("expected queued error")
		}
		if q.typ != 1 || q.code != 0 || q.seq != 7 || !q.from.Equal(net.ParseIP("2001:db8::fe")) {
			t.Errorf("unexpected result %+v", q)
		}
	})

	t.Run("local error ignored", func(t *testing.T) {
		if q := parseErrQueue(echo, errQueueCmsg(unix.IPPROTO_IP, unix.IP_RECVERR, unix.SO_EE_ORIGIN_LOCAL, 0, 0, nil)); q != nil {
			t.Errorf("expected nil, got %+v", q)
		}
	})
}
