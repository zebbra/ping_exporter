package main

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
)

func TestResolveTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		network string
		wantErr bool
	}{
		{
			name:    "valid IPv4 address",
			target:  "127.0.0.1",
			network: "ip4",
			wantErr: false,
		},
		{
			name:    "valid hostname",
			target:  "localhost",
			network: "ip4",
			wantErr: false,
		},
		{
			name:    "invalid hostname",
			target:  "invalid.nonexistent.domain.test",
			network: "ip4",
			wantErr: true,
		},
		{
			name:    "IPv6 loopback",
			target:  "::1",
			network: "ip6",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := resolveTarget(tt.target, tt.network, Capabilities{IPv4: true, IPv6: true})
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveTarget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && addr == nil {
				t.Errorf("resolveTarget() returned nil address for valid target")
			}
		})
	}
}

func TestCalculateStats(t *testing.T) {
	tests := []struct {
		name     string
		stats    *PingStats
		expected *PingStats
	}{
		{
			name: "no packets received",
			stats: &PingStats{
				PacketsSent:     3,
				PacketsReceived: 0,
				RTTs:            []time.Duration{},
				IPVersion:       "4",
			},
			expected: &PingStats{
				PacketsSent:     3,
				PacketsReceived: 0,
				PacketLoss:      1.0,
				RTTs:            []time.Duration{},
				IPVersion:       "4",
			},
		},
		{
			name: "all packets received",
			stats: &PingStats{
				PacketsSent:     3,
				PacketsReceived: 3,
				RTTs: []time.Duration{
					10 * time.Millisecond,
					12 * time.Millisecond,
					8 * time.Millisecond,
				},
				IPVersion: "4",
			},
			expected: &PingStats{
				PacketsSent:     3,
				PacketsReceived: 3,
				PacketLoss:      0.0,
				MinRTT:          8 * time.Millisecond,
				MaxRTT:          12 * time.Millisecond,
				AvgRTT:          10 * time.Millisecond,
				IPVersion:       "4",
			},
		},
		{
			name: "partial packet loss",
			stats: &PingStats{
				PacketsSent:     5,
				PacketsReceived: 3,
				RTTs: []time.Duration{
					10 * time.Millisecond,
					15 * time.Millisecond,
					5 * time.Millisecond,
				},
				IPVersion: "6",
			},
			expected: &PingStats{
				PacketsSent:     5,
				PacketsReceived: 3,
				PacketLoss:      0.4,
				MinRTT:          5 * time.Millisecond,
				MaxRTT:          15 * time.Millisecond,
				AvgRTT:          10 * time.Millisecond,
				IPVersion:       "6",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calculateStats(tt.stats)

			if tt.stats.PacketLoss != tt.expected.PacketLoss {
				t.Errorf("PacketLoss = %v, want %v", tt.stats.PacketLoss, tt.expected.PacketLoss)
			}

			if len(tt.expected.RTTs) > 0 {
				if tt.stats.MinRTT != tt.expected.MinRTT {
					t.Errorf("MinRTT = %v, want %v", tt.stats.MinRTT, tt.expected.MinRTT)
				}
				if tt.stats.MaxRTT != tt.expected.MaxRTT {
					t.Errorf("MaxRTT = %v, want %v", tt.stats.MaxRTT, tt.expected.MaxRTT)
				}
				if tt.stats.AvgRTT != tt.expected.AvgRTT {
					t.Errorf("AvgRTT = %v, want %v", tt.stats.AvgRTT, tt.expected.AvgRTT)
				}
			}
		})
	}
}

func TestRegisterPingMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	stats := &PingStats{
		PacketsSent:     5,
		PacketsReceived: 4,
		PacketLoss:      0.2,
		RTTs: []time.Duration{
			10 * time.Millisecond,
			12 * time.Millisecond,
			8 * time.Millisecond,
			15 * time.Millisecond,
		},
		MinRTT:    8 * time.Millisecond,
		MaxRTT:    15 * time.Millisecond,
		AvgRTT:    11250 * time.Microsecond,
		IPVersion: "4",
	}

	registerPingMetrics(registry, stats)

	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	expectedMetrics := map[string]bool{
		"probe_ping_packets_sent":      false,
		"probe_ping_packets_received":  false,
		"probe_ping_packet_loss_ratio": false,
		"probe_ping_ip_version":        false,
		"probe_ping_rtt_seconds":       false,
	}

	for _, mf := range metricFamilies {
		if _, exists := expectedMetrics[*mf.Name]; exists {
			expectedMetrics[*mf.Name] = true
		}
	}

	for metric, found := range expectedMetrics {
		if !found {
			t.Errorf("Expected metric %s not found", metric)
		}
	}
}

func TestProbePingTimeout(t *testing.T) {
	logger := promslog.New(&promslog.Config{})
	registry := prometheus.NewRegistry()

	// Use a very short timeout to ensure the test completes quickly
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Use an unreachable IP to ensure timeout
	success := probePing(ctx, "192.0.2.1", 1, 100*time.Millisecond, 64, "ip4", "", false, Capabilities{IPv4: true, IPv6: true}, registry, logger)

	if success {
		t.Error("Expected ping to fail due to timeout, but it succeeded")
	}
}

func TestProbePingLocalhost(t *testing.T) {
	logger := promslog.New(&promslog.Config{})
	registry := prometheus.NewRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	success := probePing(ctx, "127.0.0.1", 1, 100*time.Millisecond, 64, "ip4", "", false, Capabilities{IPv4: true, IPv6: true}, registry, logger)

	// Note: This test may fail in some environments where ICMP is blocked
	// In those cases, the test should still complete without error
	if !success {
		t.Log("Ping to localhost failed - this may be expected in some environments")
	}

	// Verify that metrics were registered regardless of success
	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(metricFamilies) == 0 {
		t.Error("No metrics were registered")
	}
}

func TestProbePingInvalidTarget(t *testing.T) {
	logger := promslog.New(&promslog.Config{})
	registry := prometheus.NewRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	success := probePing(ctx, "invalid.nonexistent.domain.test", 1, 100*time.Millisecond, 64, "ip4", "", false, Capabilities{IPv4: true, IPv6: true}, registry, logger)

	if success {
		t.Error("Expected ping to fail for invalid target, but it succeeded")
	}
}

func TestGetICMPSequence(t *testing.T) {
	seq1 := getICMPSequence()
	seq2 := getICMPSequence()
	seq3 := getICMPSequence()

	if seq2 != seq1+1 {
		t.Errorf("Expected sequence %d, got %d", seq1+1, seq2)
	}

	if seq3 != seq2+1 {
		t.Errorf("Expected sequence %d, got %d", seq2+1, seq3)
	}
}

func TestPingStatsInitialization(t *testing.T) {
	stats := &PingStats{
		RTTs: make([]time.Duration, 0, 5),
	}

	if len(stats.RTTs) != 0 {
		t.Errorf("Expected empty RTTs slice, got length %d", len(stats.RTTs))
	}

	if cap(stats.RTTs) != 5 {
		t.Errorf("Expected RTTs capacity 5, got %d", cap(stats.RTTs))
	}
}

func BenchmarkCalculateStats(b *testing.B) {
	stats := &PingStats{
		PacketsSent:     100,
		PacketsReceived: 95,
		RTTs:            make([]time.Duration, 95),
		IPVersion:       "4",
	}

	// Fill RTTs with sample data
	for i := 0; i < 95; i++ {
		stats.RTTs[i] = time.Duration(i+1) * time.Millisecond
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calculateStats(stats)
	}
}

func BenchmarkRegisterPingMetrics(b *testing.B) {
	stats := &PingStats{
		PacketsSent:     10,
		PacketsReceived: 9,
		PacketLoss:      0.1,
		RTTs: []time.Duration{
			10 * time.Millisecond,
			12 * time.Millisecond,
			8 * time.Millisecond,
		},
		MinRTT:    8 * time.Millisecond,
		MaxRTT:    12 * time.Millisecond,
		AvgRTT:    10 * time.Millisecond,
		IPVersion: "4",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry := prometheus.NewRegistry()
		registerPingMetrics(registry, stats)
	}
}

func TestPingStatsEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		stats *PingStats
	}{
		{
			name: "single RTT measurement",
			stats: &PingStats{
				PacketsSent:     1,
				PacketsReceived: 1,
				RTTs:            []time.Duration{10 * time.Millisecond},
			},
		},
		{
			name: "zero RTT measurements",
			stats: &PingStats{
				PacketsSent:     3,
				PacketsReceived: 0,
				RTTs:            []time.Duration{},
			},
		},
		{
			name: "very large RTT values",
			stats: &PingStats{
				PacketsSent:     2,
				PacketsReceived: 2,
				RTTs: []time.Duration{
					time.Second,
					2 * time.Second,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			calculateStats(tt.stats)

			registry := prometheus.NewRegistry()
			registerPingMetrics(registry, tt.stats)

			// Verify metrics can be gathered
			_, err := registry.Gather()
			if err != nil {
				t.Errorf("Failed to gather metrics: %v", err)
			}
		})
	}
}

func TestPerPacketTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 5s - 2*1s for intervals, split across 3 packets
	got := perPacketTimeout(ctx, 3, time.Second)
	if got < 900*time.Millisecond || got > time.Second {
		t.Errorf("Expected ~1s per packet, got %v", got)
	}

	if got := perPacketTimeout(ctx, 10, time.Second); got != 0 {
		t.Errorf("Expected 0 when intervals exceed the budget, got %v", got)
	}

	if got := perPacketTimeout(context.Background(), 3, time.Second); got != defaultPerPacketTimeout {
		t.Errorf("Expected default timeout without deadline, got %v", got)
	}
}

func TestProbePingDeadlineKeepsMetrics(t *testing.T) {
	logger := promslog.New(&promslog.Config{})
	caps := DetectCapabilities(logger)
	if !caps.IPv4 {
		t.Skip("No IPv4 ICMP socket available")
	}

	registry := prometheus.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	// TEST-NET-1 never answers, so every packet runs into its reply timeout
	probePing(ctx, "192.0.2.1", 3, 10*time.Millisecond, 64, "ip4", "", false, caps, registry, logger)

	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}
	found := map[string]float64{}
	for _, mf := range mfs {
		found[mf.GetName()] = mf.GetMetric()[0].GetGauge().GetValue()
	}
	if found["probe_ping_packets_sent"] != 3 {
		t.Errorf("Expected 3 packets sent, got %v", found["probe_ping_packets_sent"])
	}
	if found["probe_ping_packet_loss_ratio"] != 1 {
		t.Errorf("Expected packet loss ratio 1, got %v", found["probe_ping_packet_loss_ratio"])
	}
}
