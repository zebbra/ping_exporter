package main

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestICMPErrorReason(t *testing.T) {
	tests := []struct {
		ipv6       bool
		typ, code  int
		wantReason string
	}{
		{false, 3, 0, "net_unreachable"},
		{false, 3, 1, "host_unreachable"},
		{false, 3, 3, "port_unreachable"},
		{false, 3, 4, "frag_needed"},
		{false, 3, 13, "admin_prohibited"},
		{false, 3, 5, "other"},
		{false, 11, 0, "ttl_exceeded"},
		{false, 11, 1, "other"},
		{false, 12, 0, "parameter_problem"},
		{true, 1, 0, "net_unreachable"},
		{true, 1, 1, "admin_prohibited"},
		{true, 1, 3, "host_unreachable"},
		{true, 2, 0, "frag_needed"},
		{true, 3, 0, "ttl_exceeded"},
		{true, 4, 0, "parameter_problem"},
		{true, 1, 2, "other"},
	}
	for _, tt := range tests {
		if got := icmpErrorReason(tt.ipv6, tt.typ, tt.code); got != tt.wantReason {
			t.Errorf("icmpErrorReason(ipv6=%v, %d, %d) = %q, want %q", tt.ipv6, tt.typ, tt.code, got, tt.wantReason)
		}
	}
}

func quotedEcho4(dst net.IP, id, seq int) []byte {
	b := make([]byte, 28)
	b[0] = 0x45
	b[9] = 1
	copy(b[16:20], dst.To4())
	b[20] = 8
	binary.BigEndian.PutUint16(b[24:26], uint16(id))
	binary.BigEndian.PutUint16(b[26:28], uint16(seq))
	return b
}

func quotedEcho6(dst net.IP, id, seq int) []byte {
	b := make([]byte, 48)
	b[0] = 0x60
	b[6] = 58
	copy(b[24:40], dst.To16())
	b[40] = 128
	binary.BigEndian.PutUint16(b[44:46], uint16(id))
	binary.BigEndian.PutUint16(b[46:48], uint16(seq))
	return b
}

func TestMatchQuotedEcho(t *testing.T) {
	dst4 := net.ParseIP("192.0.2.1")
	dst6 := net.ParseIP("2001:db8::1")
	tests := []struct {
		name    string
		data    []byte
		dst     net.IP
		checkID bool
		want    bool
	}{
		{"v4 match", quotedEcho4(dst4, 42, 7), dst4, true, true},
		{"v4 wrong seq", quotedEcho4(dst4, 42, 8), dst4, true, false},
		{"v4 wrong id", quotedEcho4(dst4, 43, 7), dst4, true, false},
		{"v4 id ignored", quotedEcho4(dst4, 43, 7), dst4, false, true},
		{"v4 wrong dst", quotedEcho4(net.ParseIP("192.0.2.2"), 42, 7), dst4, true, false},
		{"v4 truncated", quotedEcho4(dst4, 42, 7)[:24], dst4, true, false},
		{"v6 match", quotedEcho6(dst6, 42, 7), dst6, true, true},
		{"v6 wrong seq", quotedEcho6(dst6, 42, 8), dst6, true, false},
		{"v6 truncated", quotedEcho6(dst6, 42, 7)[:44], dst6, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchQuotedEcho(tt.data, tt.dst, 42, 7, tt.checkID); got != tt.want {
				t.Errorf("matchQuotedEcho() = %v, want %v", got, tt.want)
			}
		})
	}
}
