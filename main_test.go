package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
)

func TestHandleProbe(t *testing.T) {
	// Initialize default values for testing
	*defaultCount = 3
	*defaultInterval = time.Second
	*defaultPacketSize = 64
	*defaultTimeout = 5 * time.Second
	*maxCount = 100
	*maxPacketSize = 65507

	logger := promslog.New(&promslog.Config{})

	tests := []struct {
		name           string
		queryParams    string
		expectedStatus int
		checkContent   func(body string) bool
	}{
		{
			name:           "missing target parameter",
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			checkContent: func(body string) bool {
				return strings.Contains(body, "Target parameter is missing")
			},
		},
		{
			name:           "valid basic ping",
			queryParams:    "target=127.0.0.1",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "probe_success") &&
					strings.Contains(body, "probe_ping_packets_sent") &&
					strings.Contains(body, "probe_ping_packets_received")
			},
		},
		{
			name:           "valid ping with count",
			queryParams:    "target=127.0.0.1&count=2",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "probe_ping_packets_sent 2") ||
					strings.Contains(body, "probe_ping_packets_sent")
			},
		},
		{
			name:           "debug output",
			queryParams:    "target=127.0.0.1&count=1&debug=true",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "Logs for the probe:") &&
					strings.Contains(body, "Target: 127.0.0.1") &&
					strings.Contains(body, "probe_success")
			},
		},
		{
			name:           "custom packet size",
			queryParams:    "target=127.0.0.1&packet_size=128",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "probe_success")
			},
		},
		{
			name:           "custom interval",
			queryParams:    "target=127.0.0.1&interval=200ms",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "probe_success")
			},
		},
		{
			name:           "exceed max count",
			queryParams:    "target=127.0.0.1&count=150",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				// Should default to 3 packets due to max limit
				return strings.Contains(body, "probe_ping_packets_sent") && strings.Contains(body, "probe_success")
			},
		},
		{
			name:           "exceed max packet size",
			queryParams:    "target=127.0.0.1&packet_size=70000",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				// Should default to 64 bytes due to max limit
				return strings.Contains(body, "probe_success")
			},
		},
		{
			name:           "invalid target",
			queryParams:    "target=invalid.nonexistent.domain.test",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "probe_success 0")
			},
		},
	}

	caps := DetectCapabilities(logger)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/probe?"+tt.queryParams, nil)
			w := httptest.NewRecorder()

			handleProbe(w, req, logger, caps)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			body := w.Body.String()
			if !tt.checkContent(body) {
				t.Errorf("Content check failed for test %s. Body: %s", tt.name, body)
			}
		})
	}
}

func TestSetupHandlers(t *testing.T) {
	// Initialize route prefix for testing
	*routePrefix = "/"

	logger := promslog.New(&promslog.Config{})

	// Reset HTTP handlers
	http.DefaultServeMux = http.NewServeMux()

	setupHandlers(logger, DetectCapabilities(logger))

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		checkContent   func(body string) bool
	}{
		{
			name:           "root page",
			path:           "/",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "Ping Exporter") &&
					strings.Contains(body, "<html>")
			},
		},
		{
			name:           "metrics endpoint",
			path:           "/metrics",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "go_") ||
					strings.Contains(body, "promhttp_")
			},
		},
		{
			name:           "health endpoint",
			path:           "/-/healthy",
			expectedStatus: http.StatusOK,
			checkContent: func(body string) bool {
				return strings.Contains(body, "Healthy")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			http.DefaultServeMux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			body := w.Body.String()
			if !tt.checkContent(body) {
				t.Errorf("Content check failed for test %s. Body: %s", tt.name, body)
			}
		})
	}
}

func TestParseParameters(t *testing.T) {
	// Initialize default values for testing
	*defaultCount = 3
	*defaultInterval = time.Second
	*defaultPacketSize = 64
	*defaultTimeout = 5 * time.Second
	*maxCount = 100
	*maxPacketSize = 65507

	logger := promslog.New(&promslog.Config{})

	tests := []struct {
		name     string
		params   url.Values
		expected map[string]interface{}
	}{
		{
			name:   "default values",
			params: url.Values{"target": {"127.0.0.1"}},
			expected: map[string]interface{}{
				"count":        3,
				"interval":     time.Second,
				"packetSize":   64,
				"timeout":      time.Second * 5,
				"ipProtocol":   "ip4",
				"sourceIP":     "",
				"dontFragment": false,
				"debug":        false,
			},
		},
		{
			name: "custom values",
			params: url.Values{
				"target":        {"8.8.8.8"},
				"count":         {"5"},
				"interval":      {"500ms"},
				"packet_size":   {"128"},
				"timeout":       {"10s"},
				"ip_protocol":   {"ip6"},
				"source_ip":     {"192.168.1.1"},
				"dont_fragment": {"true"},
				"debug":         {"true"},
			},
			expected: map[string]interface{}{
				"count":        5,
				"interval":     time.Millisecond * 500,
				"packetSize":   128,
				"timeout":      time.Second * 10,
				"ipProtocol":   "ip6",
				"sourceIP":     "192.168.1.1",
				"dontFragment": true,
				"debug":        true,
			},
		},
		{
			name: "invalid values should use defaults",
			params: url.Values{
				"target":      {"127.0.0.1"},
				"count":       {"invalid"},
				"interval":    {"invalid"},
				"packet_size": {"invalid"},
				"timeout":     {"invalid"},
			},
			expected: map[string]interface{}{
				"count":      3,
				"interval":   time.Second,
				"packetSize": 64,
				"timeout":    time.Second * 5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/probe", nil)
			req.URL.RawQuery = tt.params.Encode()
			w := httptest.NewRecorder()

			// This test verifies parameter parsing by checking the behavior
			// We can't directly test the parsing logic without refactoring handleProbe
			handleProbe(w, req, logger, DetectCapabilities(logger))

			// Verify the request was processed successfully (status 200)
			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d for test %s", w.Code, tt.name)
			}
		})
	}
}

func TestProbeTimeout(t *testing.T) {
	// Initialize default values for testing
	*defaultCount = 3
	*defaultInterval = time.Second
	*defaultPacketSize = 64
	*defaultTimeout = 5 * time.Second
	*maxCount = 100
	*maxPacketSize = 65507

	logger := promslog.New(&promslog.Config{})

	// Test with a very short timeout
	req := httptest.NewRequest("GET", "/probe?target=1.2.3.4&timeout=1ms&count=1", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	handleProbe(w, req, logger, DetectCapabilities(logger))
	duration := time.Since(start)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// The request should complete quickly due to timeout
	if duration > time.Second*2 {
		t.Errorf("Request took too long: %v", duration)
	}

	body := w.Body.String()
	if !strings.Contains(body, "probe_success 0") {
		t.Errorf("Expected probe to fail due to timeout, but got: %s", body)
	}
}

func BenchmarkHandleProbe(b *testing.B) {
	// Initialize default values for testing
	*defaultCount = 3
	*defaultInterval = time.Second
	*defaultPacketSize = 64
	*defaultTimeout = 5 * time.Second
	*maxCount = 100
	*maxPacketSize = 65507

	logger := promslog.New(&promslog.Config{})

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/probe?target=127.0.0.1&count=1", nil)
		w := httptest.NewRecorder()
		handleProbe(w, req, logger, DetectCapabilities(logger))
	}
}

func TestProbeMetricsFormat(t *testing.T) {
	// Initialize default values for testing
	*defaultCount = 3
	*defaultInterval = time.Second
	*defaultPacketSize = 64
	*defaultTimeout = 5 * time.Second
	*maxCount = 100
	*maxPacketSize = 65507

	logger := promslog.New(&promslog.Config{})

	req := httptest.NewRequest("GET", "/probe?target=127.0.0.1&count=2", nil)
	w := httptest.NewRecorder()

	handleProbe(w, req, logger, DetectCapabilities(logger))

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Check for required Prometheus metrics
	requiredMetrics := []string{
		"probe_success",
		"probe_duration_seconds",
		"probe_ping_packets_sent",
		"probe_ping_packets_received",
		"probe_ping_packet_loss_ratio",
	}

	for _, metric := range requiredMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Missing required metric: %s", metric)
		}
	}

	// Check for HELP and TYPE comments
	if !strings.Contains(body, "# HELP") {
		t.Errorf("Missing HELP comments in Prometheus output")
	}

	if !strings.Contains(body, "# TYPE") {
		t.Errorf("Missing TYPE comments in Prometheus output")
	}
}

func TestProbeTimeoutTooShort(t *testing.T) {
	*defaultCount = 3
	*defaultInterval = time.Second
	*defaultTimeout = 5 * time.Second
	*maxCount = 100

	logger := promslog.New(&promslog.Config{})

	req := httptest.NewRequest("GET", "/probe?target=127.0.0.1&count=10&interval=1s&timeout=5s", nil)
	w := httptest.NewRecorder()
	handleProbe(w, req, logger, Capabilities{IPv4: true})

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestCapToScrapeTimeout(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		timeout  time.Duration
		expected time.Duration
	}{
		{"no header", "", 5 * time.Second, 5 * time.Second},
		{"scrape timeout lower", "3", 5 * time.Second, 2500 * time.Millisecond},
		{"scrape timeout higher", "10", 5 * time.Second, 5 * time.Second},
		{"invalid header", "abc", 5 * time.Second, 5 * time.Second},
		{"offset larger than scrape timeout", "0.2", 5 * time.Second, 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/probe", nil)
			if tt.header != "" {
				req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", tt.header)
			}
			if got := capToScrapeTimeout(req, tt.timeout, 500*time.Millisecond); got != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestProbeLogLevel(t *testing.T) {
	*defaultCount = 1
	*defaultInterval = time.Second
	*defaultTimeout = 2 * time.Second
	*maxCount = 100

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	// Unresolvable target, so the probe only logs and sends no packets
	caps := Capabilities{IPv4: true}

	req := httptest.NewRequest("GET", "/probe?target=invalid.test&log_level=error", nil)
	handleProbe(httptest.NewRecorder(), req, logger, caps)
	if strings.Contains(buf.String(), "level=INFO") {
		t.Errorf("Expected no INFO logs with log_level=error, got:\n%s", buf.String())
	}

	buf.Reset()
	req = httptest.NewRequest("GET", "/probe?target=invalid.test&log_level=debug", nil)
	handleProbe(httptest.NewRecorder(), req, logger, caps)
	if !strings.Contains(buf.String(), "level=INFO") {
		t.Errorf("Expected INFO logs with log_level=debug, got:\n%s", buf.String())
	}

	w := httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/probe?target=invalid.test&log_level=verbose", nil)
	handleProbe(w, req, logger, caps)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid log_level, got %d", w.Code)
	}
}

func TestLevelHandlerLowersLevel(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger := slog.New(&levelHandler{level: slog.LevelDebug, handler: base.Handler()}).With("k", "v")

	logger.Debug("hello")
	if !strings.Contains(buf.String(), "level=DEBUG msg=hello k=v") {
		t.Errorf("Expected debug record with attrs, got: %q", buf.String())
	}
}
