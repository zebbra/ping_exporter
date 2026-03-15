package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

var (
	icmpID            int
	icmpSequence      uint16
	icmpSequenceMutex sync.Mutex
)

// Capabilities holds which ICMP socket families and types are available on this system.
type Capabilities struct {
	IPv4    bool // any IPv4 ICMP socket works
	IPv6    bool // any IPv6 ICMP socket works
	IPv4UDP bool // unprivileged udp4 works (no CAP_NET_RAW needed)
	IPv6UDP bool // unprivileged udp6 works (no CAP_NET_RAW needed)
}

// DetectCapabilities probes which ICMP socket types can be opened.
// It tries unprivileged (udp) first, then raw (icmp) for each family.
func DetectCapabilities(logger *slog.Logger) Capabilities {
	caps := Capabilities{}

	if conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
		conn.Close()
		caps.IPv4 = true
	} else if conn, err := icmp.ListenPacket("udp4", "0.0.0.0"); err == nil {
		conn.Close()
		caps.IPv4 = true
		caps.IPv4UDP = true
	}

	if conn, err := icmp.ListenPacket("ip6:ipv6-icmp", "::"); err == nil {
		conn.Close()
		caps.IPv6 = true
	} else if conn, err := icmp.ListenPacket("udp6", "::"); err == nil {
		conn.Close()
		caps.IPv6 = true
		caps.IPv6UDP = true
	}

	return caps
}

func init() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	// PID is typically 1 when running in a container; in that case, set
	// the ICMP echo ID to a random value to avoid potential clashes with
	// other ping_exporter instances.
	if pid := os.Getpid(); pid == 1 {
		icmpID = r.Intn(1 << 16)
	} else {
		icmpID = pid & 0xffff
	}

	// Start the ICMP echo sequence at a random offset to prevent them from
	// being in sync when several ping_exporter instances are restarted
	// at the same time.
	icmpSequence = uint16(r.Intn(1 << 16))
}

func getICMPSequence() uint16 {
	icmpSequenceMutex.Lock()
	defer icmpSequenceMutex.Unlock()
	icmpSequence++
	return icmpSequence
}

type PingStats struct {
	PacketsSent     int
	PacketsReceived int
	PacketLoss      float64
	RTTs            []time.Duration
	MinRTT          time.Duration
	MaxRTT          time.Duration
	AvgRTT          time.Duration
	StdDevRTT       time.Duration
	IPVersion       string // "4" for IPv4, "6" for IPv6
}

func probePing(ctx context.Context, target string, count int, interval time.Duration, packetSize int, ipProtocol, sourceIP string, dontFragment bool, caps Capabilities, registry *prometheus.Registry, logger *slog.Logger) bool {
	// Resolve target address
	var network string
	switch ipProtocol {
	case "ip4":
		network = "ip4"
	case "ip6":
		network = "ip6"
	default:
		network = "auto"
	}

	dstAddr, err := resolveTarget(target, network, caps)
	if err != nil {
		logger.Error("Failed to resolve target", "err", err)
		return false
	}

	logger.Debug("Target resolved", "target", target, "ip", dstAddr.String())

	// Perform ping
	stats, err := performPing(ctx, dstAddr, sourceIP, count, interval, packetSize, dontFragment, caps, logger)
	if err != nil {
		logger.Info("Ping failed", "err", err)
		return false
	}

	logger.Info("Ping complete", "target", target, "sent", stats.PacketsSent, "received", stats.PacketsReceived, "loss", stats.PacketLoss, "avg_rtt", stats.AvgRTT)

	// Register metrics
	registerPingMetrics(registry, stats)

	return stats.PacketsReceived > 0
}

func resolveTarget(target, network string, caps Capabilities) (*net.IPAddr, error) {
	if network != "auto" {
		return net.ResolveIPAddr(network, target)
	}

	// Auto mode: detect family for literal IPs directly.
	if ip := net.ParseIP(target); ip != nil {
		if ip.To4() != nil {
			return net.ResolveIPAddr("ip4", target)
		}
		return net.ResolveIPAddr("ip6", target)
	}

	// Hostname: prefer IPv6 if the system can actually use it, else IPv4.
	if caps.IPv6 {
		if addr, err := net.ResolveIPAddr("ip6", target); err == nil {
			return addr, nil
		}
	}
	return net.ResolveIPAddr("ip4", target)
}

func performPing(ctx context.Context, dstAddr *net.IPAddr, sourceIP string, count int, interval time.Duration, packetSize int, dontFragment bool, caps Capabilities, logger *slog.Logger) (*PingStats, error) {
	stats := &PingStats{
		RTTs: make([]time.Duration, 0, count),
	}

	var (
		requestType icmp.Type
		replyType   icmp.Type
		conn        *icmp.PacketConn
		v4RawConn   *ipv4.RawConn
		useUDP      bool
	)

	// Determine ICMP types and create connection
	var srcIP net.IP
	if sourceIP != "" {
		srcIP = net.ParseIP(sourceIP)
		if srcIP == nil {
			return nil, fmt.Errorf("invalid source IP: %s", sourceIP)
		}
	}

	if dstAddr.IP.To4() == nil {
		// IPv6
		stats.IPVersion = "6"
		requestType = ipv6.ICMPTypeEchoRequest
		replyType = ipv6.ICMPTypeEchoReply
		if srcIP == nil {
			srcIP = net.ParseIP("::")
		}

		var err error
		if caps.IPv6UDP {
			conn, err = icmp.ListenPacket("udp6", srcIP.String())
			useUDP = true
		} else {
			conn, err = icmp.ListenPacket("ip6:ipv6-icmp", srcIP.String())
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create IPv6 ICMP socket: %w", err)
		}
	} else {
		// IPv4
		stats.IPVersion = "4"
		requestType = ipv4.ICMPTypeEcho
		replyType = ipv4.ICMPTypeEchoReply
		if srcIP == nil {
			srcIP = net.IPv4zero
		}

		var err error
		if dontFragment {
			// Need raw socket for don't fragment
			netConn, err := net.ListenPacket("ip4:icmp", srcIP.String())
			if err != nil {
				return nil, fmt.Errorf("failed to create raw IPv4 ICMP socket: %w", err)
			}
			defer netConn.Close()

			v4RawConn, err = ipv4.NewRawConn(netConn)
			if err != nil {
				return nil, fmt.Errorf("failed to create raw connection: %w", err)
			}
		} else {
			if caps.IPv4UDP {
				conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
				useUDP = true
			} else {
				conn, err = icmp.ListenPacket("ip4:icmp", "0.0.0.0")
			}
			if err != nil {
				return nil, fmt.Errorf("failed to create IPv4 ICMP socket: %w", err)
			}
		}
	}

	if conn != nil {
		defer conn.Close()
	}

	// Create payload
	payload := make([]byte, packetSize)
	copy(payload, "Prometheus Ping Exporter")

	// Send pings
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		default:
		}

		stats.PacketsSent++
		seq := getICMPSequence()

		logger.Debug("Sending ping packet", "seq", seq, "packet", i+1, "of", count)

		// Create a timeout context for this individual ping
		pingCtx, cancel := context.WithTimeout(ctx, time.Second*2)
		rtt, err := sendPing(pingCtx, conn, v4RawConn, dstAddr, srcIP, requestType, replyType, seq, payload, useUDP, logger)
		cancel()

		if err != nil {
			logger.Debug("Ping failed", "seq", seq, "err", err)
		} else {
			stats.PacketsReceived++
			stats.RTTs = append(stats.RTTs, rtt)
			logger.Debug("Ping successful", "seq", seq, "rtt", rtt)
		}

		// Wait for interval (except for last packet)
		if i < count-1 {
			select {
			case <-ctx.Done():
				return stats, ctx.Err()
			case <-time.After(interval):
			}
		}
	}

	// Calculate statistics
	calculateStats(stats)

	return stats, nil
}

func sendPing(ctx context.Context, conn *icmp.PacketConn, v4RawConn *ipv4.RawConn, dstAddr *net.IPAddr, srcIP net.IP, requestType, replyType icmp.Type, seq uint16, payload []byte, useUDP bool, logger *slog.Logger) (time.Duration, error) {
	// Create ICMP message
	body := &icmp.Echo{
		ID:   icmpID,
		Seq:  int(seq),
		Data: payload,
	}

	logger.Debug("Creating ICMP packet", "id", icmpID, "seq", seq, "payload_size", len(payload))

	wm := icmp.Message{
		Type: requestType,
		Code: 0,
		Body: body,
	}

	wb, err := wm.Marshal(nil)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal ICMP packet: %w", err)
	}

	logger.Debug("ICMP packet marshaled", "size", len(wb))

	var dst net.Addr = dstAddr

	if useUDP {
		dst = &net.UDPAddr{IP: dstAddr.IP, Zone: dstAddr.Zone}
	}

	// Send packet and record time
	start := time.Now()

	if v4RawConn != nil {
		// Raw IPv4 with don't fragment
		header := &ipv4.Header{
			Version:  ipv4.Version,
			Len:      ipv4.HeaderLen,
			Protocol: 1,
			TotalLen: ipv4.HeaderLen + len(wb),
			TTL:      64,
			Dst:      dstAddr.IP,
			Src:      srcIP,
			Flags:    ipv4.DontFragment,
		}
		err = v4RawConn.WriteTo(header, wb, nil)
	} else {
		_, err = conn.WriteTo(wb, dst)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to send ICMP packet: %w", err)
	}

	logger.Debug("ICMP packet sent", "dst", dst.String())

	// Wait for reply
	return waitForReply(ctx, conn, v4RawConn, dst, replyType, body.ID, body.Seq, wb, useUDP, start, logger)
}

func waitForReply(ctx context.Context, conn *icmp.PacketConn, v4RawConn *ipv4.RawConn, dst net.Addr, replyType icmp.Type, expectedID, expectedSeq int, expectedData []byte, useUDP bool, sendTime time.Time, logger *slog.Logger) (time.Duration, error) {
	var dstAddr *net.IPAddr
	if ipAddr, ok := dst.(*net.IPAddr); ok {
		dstAddr = ipAddr
	} else if udpAddr, ok := dst.(*net.UDPAddr); ok {
		dstAddr = &net.IPAddr{IP: udpAddr.IP, Zone: udpAddr.Zone}
	}
	rb := make([]byte, 1500)
	deadline, _ := ctx.Deadline()

	if conn != nil {
		err := conn.SetReadDeadline(deadline)
		if err != nil {
			return 0, fmt.Errorf("failed to set read deadline: %w", err)
		}
	} else if v4RawConn != nil {
		err := v4RawConn.SetReadDeadline(deadline)
		if err != nil {
			return 0, fmt.Errorf("failed to set read deadline: %w", err)
		}
	}

	for {
		var n int
		var peer net.Addr
		var err error

		if v4RawConn != nil {
			var h *ipv4.Header
			var p []byte
			h, p, _, err = v4RawConn.ReadFrom(rb)
			if err == nil {
				copy(rb, p)
				n = len(p)
				peer = &net.IPAddr{IP: h.Src}
			}
		} else {
			n, peer, err = conn.ReadFrom(rb)
		}

		receiveTime := time.Now()

		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				logger.Debug("Timeout waiting for ICMP reply")
				return 0, fmt.Errorf("timeout waiting for ICMP reply")
			}
			logger.Debug("Failed to read ICMP reply", "err", err)
			return 0, fmt.Errorf("failed to read ICMP reply: %w", err)
		}

		logger.Debug("Received packet", "from", peer.String(), "size", n, "expected_from", dst.String())

		// Check if this is from our target
		if peer.String() != dst.String() {
			logger.Debug("Packet from unexpected source", "from", peer.String(), "expected", dst.String())
			continue
		}

		// Parse ICMP message
		var rm *icmp.Message
		var parseErr error
		if dstAddr != nil && dstAddr.IP.To4() == nil {
			// IPv6 - protocol 58
			rm, parseErr = icmp.ParseMessage(58, rb[:n])
		} else {
			// IPv4 - protocol 1
			rm, parseErr = icmp.ParseMessage(1, rb[:n])
		}
		if parseErr != nil {
			logger.Debug("Failed to parse ICMP message", "err", parseErr)
			continue
		}

		logger.Debug("Parsed ICMP message", "type", rm.Type, "expected_type", replyType)

		if rm.Type != replyType {
			logger.Debug("Wrong ICMP message type", "got", rm.Type, "expected", replyType)
			continue
		}

		body, ok := rm.Body.(*icmp.Echo)
		if !ok {
			logger.Debug("ICMP message body is not Echo")
			continue
		}

		logger.Debug("Received ICMP Echo", "id", body.ID, "seq", body.Seq, "expected_id", expectedID, "expected_seq", expectedSeq)

		// Check if this is our packet
		if (useUDP && runtime.GOOS == "linux") || body.ID == expectedID {
			if body.Seq == expectedSeq {
				// Calculate RTT properly
				rtt := receiveTime.Sub(sendTime)
				logger.Debug("Found matching ICMP reply", "rtt", rtt)
				return rtt, nil
			} else {
				logger.Debug("Wrong sequence number", "got", body.Seq, "expected", expectedSeq)
			}
		} else {
			logger.Debug("Wrong ICMP ID", "got", body.ID, "expected", expectedID)
		}
	}
}

func calculateStats(stats *PingStats) {
	if len(stats.RTTs) == 0 {
		stats.PacketLoss = 1.0
		return
	}

	stats.PacketLoss = float64(stats.PacketsSent-stats.PacketsReceived) / float64(stats.PacketsSent)

	// Find min, max, and calculate average
	stats.MinRTT = stats.RTTs[0]
	stats.MaxRTT = stats.RTTs[0]
	var sum time.Duration

	for _, rtt := range stats.RTTs {
		if rtt < stats.MinRTT {
			stats.MinRTT = rtt
		}
		if rtt > stats.MaxRTT {
			stats.MaxRTT = rtt
		}
		sum += rtt
	}

	stats.AvgRTT = sum / time.Duration(len(stats.RTTs))

	// Calculate standard deviation
	if len(stats.RTTs) > 1 {
		var variance float64
		avgFloat := float64(stats.AvgRTT)

		for _, rtt := range stats.RTTs {
			diff := float64(rtt) - avgFloat
			variance += diff * diff
		}

		variance /= float64(len(stats.RTTs) - 1)
		stats.StdDevRTT = time.Duration(math.Sqrt(variance))
	}
}

func registerPingMetrics(registry *prometheus.Registry, stats *PingStats) {
	// Packets sent
	packetsSent := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_ping_packets_sent",
		Help: "Number of ICMP packets sent",
	})
	packetsSent.Set(float64(stats.PacketsSent))
	registry.MustRegister(packetsSent)

	// Packets received
	packetsReceived := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_ping_packets_received",
		Help: "Number of ICMP packets received",
	})
	packetsReceived.Set(float64(stats.PacketsReceived))
	registry.MustRegister(packetsReceived)

	// Packet loss ratio
	packetLoss := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_ping_packet_loss_ratio",
		Help: "Packet loss ratio",
	})
	packetLoss.Set(stats.PacketLoss)
	registry.MustRegister(packetLoss)

	// IP version used
	ipVersion := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_ping_ip_version",
		Help: "IP version used for the ping (4 for IPv4, 6 for IPv6)",
	})
	if stats.IPVersion == "4" {
		ipVersion.Set(4)
	} else if stats.IPVersion == "6" {
		ipVersion.Set(6)
	}
	registry.MustRegister(ipVersion)

	// RTT statistics
	if len(stats.RTTs) > 0 {
		rttGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "probe_ping_rtt_seconds",
			Help: "Round-trip time statistics in seconds",
		}, []string{"type"})

		rttGauge.WithLabelValues("best").Set(stats.MinRTT.Seconds())
		rttGauge.WithLabelValues("worst").Set(stats.MaxRTT.Seconds())
		rttGauge.WithLabelValues("mean").Set(stats.AvgRTT.Seconds())

		// Calculate sum and other statistics
		var sum time.Duration
		var sumSquares float64
		for _, rtt := range stats.RTTs {
			sum += rtt
			sumSquares += float64(rtt) * float64(rtt)
		}

		rttGauge.WithLabelValues("sum").Set(sum.Seconds())
		rttGauge.WithLabelValues("range").Set((stats.MaxRTT - stats.MinRTT).Seconds())

		// Standard deviation calculations
		n := float64(len(stats.RTTs))
		mean := float64(stats.AvgRTT)

		// Squared deviation (variance * n)
		variance := (sumSquares - n*mean*mean)
		rttGauge.WithLabelValues("sd").Set(variance / 1e18) // Convert nanoseconds² to seconds²

		// Uncorrected standard deviation
		if n > 0 {
			usd := math.Sqrt(variance / n)
			rttGauge.WithLabelValues("usd").Set(usd / 1e9) // Convert nanoseconds to seconds
		}

		// Corrected standard deviation (Bessel's correction)
		if n > 1 {
			csd := math.Sqrt(variance / (n - 1))
			rttGauge.WithLabelValues("csd").Set(csd / 1e9) // Convert nanoseconds to seconds
		}

		registry.MustRegister(rttGauge)
	}
}
