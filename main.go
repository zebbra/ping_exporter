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
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
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
	maxCount          = kingpin.Flag("ping.max-count", "Maximum allowed packet count.").Default("100").Int()
	maxPacketSize     = kingpin.Flag("ping.max-packet-size", "Maximum allowed packet size.").Default("65507").Int()
	externalURL       = kingpin.Flag("web.external-url", "The URL under which Ping exporter is externally reachable.").String()
	routePrefix       = kingpin.Flag("web.route-prefix", "Prefix for the internal routes of web endpoints.").String()
)

func init() {
	prometheus.MustRegister(version.NewCollector("ping_exporter"))
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
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	level.Info(logger).Log("msg", "Starting ping_exporter", "version", commonversion.Info())
	level.Info(logger).Log("msg", commonversion.BuildContext())

	// Infer external URL if not provided
	if *externalURL == "" {
		hostname, err := os.Hostname()
		if err != nil {
			level.Error(logger).Log("msg", "Failed to get hostname", "err", err)
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

	// Setup HTTP handlers
	setupHandlers(promLogger)

	srv := &http.Server{}
	srvc := make(chan struct{})

	go func() {
		if err := web.ListenAndServe(srv, toolkitFlags, logger); err != nil {
			level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
			close(srvc)
		}
	}()

	level.Info(logger).Log("msg", "Ping exporter started", "address", (*toolkitFlags.WebListenAddresses)[0])

	for {
		select {
		case <-term:
			level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
			return 0
		case <-srvc:
			return 1
		}
	}
}

func setupHandlers(logger *slog.Logger) {
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
		handleProbe(w, r, logger)
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

func handleProbe(w http.ResponseWriter, r *http.Request, logger *slog.Logger) {
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
		ipProtocol = "ip4"
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
	success := probePing(ctx, target, count, interval, packetSize, ipProtocol, sourceIP, dontFragment, registry, probeLogger)
	duration := time.Since(start).Seconds()

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
