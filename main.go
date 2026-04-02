// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	commonversion "github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	webflag "github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

var (
	toolkitFlags      = webflag.AddFlags(kingpin.CommandLine, ":9115")
	defaultCount      = kingpin.Flag("ping.default-count", "Default packet count when not specified.").Default("3").Int()
	defaultInterval   = kingpin.Flag("ping.default-interval", "Default interval when not specified.").Default("1s").Duration()
	defaultPacketSize = kingpin.Flag("ping.default-packet-size", "Default packet size when not specified.").Default("64").Int()
	defaultTimeout    = kingpin.Flag("ping.default-timeout", "Default timeout when not specified.").Default("5s").Duration()
	defaultIPProtocol = kingpin.Flag("ping.default-ip-protocol", "Default IP protocol when not specified in probe URL (ip4, ip6, auto). auto resolves literal IPs by family and prefers IPv6 for hostnames.").Default("auto").Enum("ip4", "ip6", "auto")
	maxCount          = kingpin.Flag("ping.max-count", "Maximum allowed packet count.").Default("100").Int()
	maxPacketSize     = kingpin.Flag("ping.max-packet-size", "Maximum allowed packet size.").Default("65507").Int()
	externalURL       = kingpin.Flag("web.external-url", "The URL under which Ping exporter is externally reachable.").String()
	routePrefix       = kingpin.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints.").String()
)

var (
	probesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ping_exporter_probes_total",
		Help: "Total number of probes executed.",
	}, []string{"result", "ip_version"})

	packetsSentTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ping_exporter_packets_sent_total",
		Help: "Total ICMP packets sent across all probes.",
	}, []string{"ip_version"})

	packetsReceivedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ping_exporter_packets_received_total",
		Help: "Total ICMP packets received across all probes.",
	}, []string{"ip_version"})

	probeDurationHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "ping_exporter_probe_duration_seconds",
		Help:    "Histogram of probe durations.",
		Buckets: prometheus.DefBuckets,
	}, []string{"ip_version"})

	capabilityInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ping_exporter_capability_info",
		Help: "ICMP socket capabilities detected at startup (1 = active).",
	}, []string{"ipv4", "ipv6"})
)

func init() {
	prometheus.MustRegister(version.NewCollector("ping_exporter"))
	prometheus.MustRegister(probesTotal, packetsSentTotal, packetsReceivedTotal, probeDurationHistogram, capabilityInfo)
}

func main() {
	os.Exit(run())
}

func run() int {
	kingpin.CommandLine.UsageWriter(os.Stdout)
	promslogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.Version(commonversion.Print("ping_exporter"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	promLogger := promslog.New(promslogConfig)

	promLogger.Info("Starting ping_exporter", "version", commonversion.Info())
	promLogger.Info(commonversion.BuildContext())

	// Infer external URL if not provided
	if *externalURL == "" {
		hostname, err := os.Hostname()
		if err != nil {
			promLogger.Error("Failed to get hostname", "err", err)
			return 1
		}
		listenAddr := (*toolkitFlags.WebListenAddresses)[0]
		*externalURL = fmt.Sprintf("http://%s%s/", hostname, listenAddr)
	}

	// Set route prefix
	if *routePrefix == "" {
		*routePrefix = "/"
	}
	*routePrefix = "/" + strings.Trim(*routePrefix, "/")
	if *routePrefix != "/" {
		*routePrefix = *routePrefix + "/"
	}

	// Setup signal handling
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	// Detect ICMP socket capabilities once at startup
	caps := DetectCapabilities(promLogger)

	ipv4Socket := "unavailable"
	if caps.IPv4UDP {
		ipv4Socket = "udp/unprivileged"
	} else if caps.IPv4 {
		ipv4Socket = "raw/CAP_NET_RAW"
	}
	ipv6Socket := "unavailable"
	if caps.IPv6UDP {
		ipv6Socket = "udp/unprivileged"
	} else if caps.IPv6 {
		ipv6Socket = "raw/CAP_NET_RAW"
	}

	if !caps.IPv4 && !caps.IPv6 {
		promLogger.Error("No ICMP socket available — cannot ping anything", "ipv4", ipv4Socket, "ipv6", ipv6Socket, "hint", "add CAP_NET_RAW or set net.ipv4.ping_group_range")
		return 1
	}

	promLogger.Info("ICMP sockets ready", "ipv4", ipv4Socket, "ipv6", ipv6Socket)
	capabilityInfo.WithLabelValues(ipv4Socket, ipv6Socket).Set(1)

	// Setup HTTP handlers
	setupHandlers(promLogger, caps)

	srv := &http.Server{}
	srvc := make(chan struct{})

	go func() {
		if err := web.ListenAndServe(srv, toolkitFlags, promLogger); err != nil {
			promLogger.Error("Error starting HTTP server", "err", err)
			close(srvc)
		}
	}()

	promLogger.Info("Ping exporter started", "address", (*toolkitFlags.WebListenAddresses)[0])

	for {
		select {
		case <-term:
			promLogger.Info("Received SIGTERM, exiting gracefully...")
			return 0
		case <-srvc:
			return 1
		}
	}
}

func setupHandlers(logger *slog.Logger, caps Capabilities) {
	// Root redirect
	if *routePrefix != "/" {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			http.Redirect(w, r, *externalURL, http.StatusFound)
		})
	}

	// Metrics endpoint
	http.Handle(path.Join(*routePrefix, "metrics"), promhttp.Handler())

	// Health endpoint
	http.HandleFunc(path.Join(*routePrefix, "/-/healthy"), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Healthy"))
	})

	// Probe endpoint
	http.HandleFunc(path.Join(*routePrefix, "probe"), func(w http.ResponseWriter, r *http.Request) {
		handleProbe(w, r, logger, caps)
	})

	// Root page
	http.HandleFunc(*routePrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>
    <head><title>Ping Exporter</title></head>
    <body>
    <h1>Ping Exporter</h1>
    <p><a href="probe?target=1.1.1.1&count=5&interval=1s&packet_size=64">Probe 1.1.1.1 with 5 packets</a></p>
    <p><a href="probe?target=google.com&count=3&debug=true">Debug probe google.com</a></p>
    <p><a href="metrics">Metrics</a></p>
    </body>
    </html>`))
	})
}

func handleProbe(w http.ResponseWriter, r *http.Request, logger *slog.Logger, caps Capabilities) {
	params := r.URL.Query()

	// Get target
	target := params.Get("target")
	if target == "" {
		http.Error(w, "Target parameter is missing", http.StatusBadRequest)
		return
	}

	// Parse parameters with defaults
	count := *defaultCount
	if countStr := params.Get("count"); countStr != "" {
		if c, err := strconv.Atoi(countStr); err == nil && c > 0 && c <= *maxCount {
			count = c
		}
	}

	interval := *defaultInterval
	if intervalStr := params.Get("interval"); intervalStr != "" {
		if i, err := time.ParseDuration(intervalStr); err == nil && i > 0 {
			interval = i
		}
	}

	packetSize := *defaultPacketSize
	if sizeStr := params.Get("packet_size"); sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= *maxPacketSize {
			packetSize = s
		}
	}

	timeout := *defaultTimeout
	if timeoutStr := params.Get("timeout"); timeoutStr != "" {
		if t, err := time.ParseDuration(timeoutStr); err == nil && t > 0 {
			timeout = t
		}
	}

	ipProtocol := params.Get("ip_protocol")
	if ipProtocol == "" {
		ipProtocol = *defaultIPProtocol
	}

	sourceIP := params.Get("source_ip")
	dontFragment := params.Get("dont_fragment") == "true"
	debug := params.Get("debug") == "true"

	// Create probe logger
	probeLogger := logger.With("target", target, "count", count, "interval", interval, "packet_size", packetSize)

	// Set log level for this probe if specified
	if logLevelStr := params.Get("log_level"); logLevelStr != "" {
		// This would ideally create a new logger with the specified level
		// For now, we'll just log the requested level
		probeLogger = probeLogger.With("probe_log_level", logLevelStr)
	}

	probeLogger.Info("Beginning probe")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	// Create Prometheus registry for this probe
	registry := prometheus.NewRegistry()

	// Run the ping probe
	start := time.Now()
	success := probePing(ctx, target, count, interval, packetSize, ipProtocol, sourceIP, dontFragment, caps, registry, probeLogger)
	duration := time.Since(start).Seconds()

	// Update exporter-level operational metrics
	ipVersion := "unknown"
	sent, received := 0.0, 0.0
	if mfs, err := registry.Gather(); err == nil {
		for _, mf := range mfs {
			switch mf.GetName() {
			case "probe_ping_ip_version":
				if len(mf.GetMetric()) > 0 {
					switch mf.GetMetric()[0].GetGauge().GetValue() {
					case 4:
						ipVersion = "4"
					case 6:
						ipVersion = "6"
					}
				}
			case "probe_ping_packets_sent":
				if len(mf.GetMetric()) > 0 {
					sent = mf.GetMetric()[0].GetGauge().GetValue()
				}
			case "probe_ping_packets_received":
				if len(mf.GetMetric()) > 0 {
					received = mf.GetMetric()[0].GetGauge().GetValue()
				}
			}
		}
	}
	probeDurationHistogram.WithLabelValues(ipVersion).Observe(duration)
	packetsSentTotal.WithLabelValues(ipVersion).Add(sent)
	packetsReceivedTotal.WithLabelValues(ipVersion).Add(received)
	if success {
		probesTotal.WithLabelValues("success", ipVersion).Inc()
	} else {
		probesTotal.WithLabelValues("failure", ipVersion).Inc()
	}

	// Create duration metric
	probeDurationGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_duration_seconds",
		Help: "Returns how long the probe took to complete in seconds",
	})
	probeDurationGauge.Set(duration)
	registry.MustRegister(probeDurationGauge)

	// Create success metric
	probeSuccessGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "probe_success",
		Help: "Displays whether or not the probe was a success",
	})
	if success {
		probeSuccessGauge.Set(1)
		probeLogger.Info("Probe succeeded", "duration_seconds", duration)
	} else {
		probeSuccessGauge.Set(0)
		probeLogger.Error("Probe failed", "duration_seconds", duration)
	}
	registry.MustRegister(probeSuccessGauge)

	// Return debug output or metrics
	if debug {
		w.Header().Set("Content-Type", "text/plain")
		debugOutput := fmt.Sprintf("Logs for the probe:\n")
		debugOutput += fmt.Sprintf("Target: %s\n", target)
		debugOutput += fmt.Sprintf("Count: %d\n", count)
		debugOutput += fmt.Sprintf("Interval: %s\n", interval)
		debugOutput += fmt.Sprintf("Packet Size: %d\n", packetSize)
		debugOutput += fmt.Sprintf("IP Protocol: %s\n", ipProtocol)
		debugOutput += fmt.Sprintf("Success: %t\n", success)
		debugOutput += fmt.Sprintf("Duration: %.3fs\n", duration)
		debugOutput += "\n\nMetrics that would have been returned:\n"
		w.Write([]byte(debugOutput))

		// Also write the metrics
		h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
		h.ServeHTTP(w, r)
		return
	}

	// Return Prometheus metrics
	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}
