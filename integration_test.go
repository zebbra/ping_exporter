//go:build integration
// +build integration

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	testPort    = "19115"
	testAddress = "localhost:" + testPort
	testBaseURL = "http://" + testAddress
)

var serverPID int

func TestMain(m *testing.M) {
	// Start the ping exporter server
	if err := startServer(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
		os.Exit(1)
	}

	// Wait for server to be ready
	if err := waitForServer(); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed to start: %v\n", err)
		stopServer()
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Cleanup
	stopServer()
	os.Exit(code)
}

func startServer() error {
	cmd := exec.Command("./dist/ping_exporter", "--web.listen-address="+testAddress, "--log.level=warn")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	serverPID = cmd.Process.Pid
	return nil
}

func stopServer() {
	if serverPID > 0 {
		if proc, err := os.FindProcess(serverPID); err == nil {
			proc.Signal(syscall.SIGTERM)
			proc.Wait()
		}
	}
}

func waitForServer() error {
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for server to start")
		case <-ticker.C:
			resp, err := http.Get(testBaseURL + "/-/healthy")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

func TestIntegrationHomePage(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/")
	if err != nil {
		t.Fatalf("Failed to get home page: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Ping Exporter") {
		t.Error("Home page does not contain expected title")
	}

	if !strings.Contains(bodyStr, "probe?target=") {
		t.Error("Home page does not contain probe links")
	}
}

func TestIntegrationMetricsEndpoint(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/metrics")
	if err != nil {
		t.Fatalf("Failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	expectedMetrics := []string{
		"go_gc_duration_seconds",
		"go_goroutines",
		"go_memstats_",
		"promhttp_metric_handler_requests_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(bodyStr, metric) {
			t.Errorf("Metrics endpoint missing expected metric: %s", metric)
		}
	}
}

func TestIntegrationHealthEndpoint(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/-/healthy")
	if err != nil {
		t.Fatalf("Failed to get health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if strings.TrimSpace(string(body)) != "Healthy" {
		t.Errorf("Expected 'Healthy', got '%s'", string(body))
	}
}

func TestIntegrationPingLocalhost(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/probe?target=127.0.0.1&count=2")
	if err != nil {
		t.Fatalf("Failed to probe localhost: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	requiredMetrics := []string{
		"probe_success",
		"probe_duration_seconds",
		"probe_ping_packets_sent",
		"probe_ping_packets_received",
		"probe_ping_packet_loss_ratio",
	}

	for _, metric := range requiredMetrics {
		if !strings.Contains(bodyStr, metric) {
			t.Errorf("Probe response missing required metric: %s", metric)
		}
	}

	// Check that packets were sent
	if !strings.Contains(bodyStr, "probe_ping_packets_sent 2") {
		t.Error("Expected 2 packets to be sent")
	}
}

func TestIntegrationPingWithDebug(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/probe?target=127.0.0.1&count=1&debug=true")
	if err != nil {
		t.Fatalf("Failed to probe with debug: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	debugSections := []string{
		"Logs for the probe:",
		"Target: 127.0.0.1",
		"Count: 1",
		"Metrics that would have been returned:",
		"probe_success",
	}

	for _, section := range debugSections {
		if !strings.Contains(bodyStr, section) {
			t.Errorf("Debug output missing expected section: %s", section)
		}
	}
}

func TestIntegrationPingWithCustomParameters(t *testing.T) {
	tests := []struct {
		name   string
		params string
		check  func(body string) bool
	}{
		{
			name:   "custom packet count",
			params: "target=127.0.0.1&count=5",
			check: func(body string) bool {
				return strings.Contains(body, "probe_ping_packets_sent 5")
			},
		},
		{
			name:   "custom packet size",
			params: "target=127.0.0.1&packet_size=128",
			check: func(body string) bool {
				return strings.Contains(body, "probe_success")
			},
		},
		{
			name:   "custom interval",
			params: "target=127.0.0.1&count=2&interval=200ms",
			check: func(body string) bool {
				return strings.Contains(body, "probe_ping_packets_sent 2")
			},
		},
		{
			name:   "custom timeout",
			params: "target=127.0.0.1&timeout=10s",
			check: func(body string) bool {
				return strings.Contains(body, "probe_success")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get(testBaseURL + "/probe?" + tt.params)
			if err != nil {
				t.Fatalf("Failed to probe: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed to read response body: %v", err)
			}

			if !tt.check(string(body)) {
				t.Errorf("Check failed for test %s. Body: %s", tt.name, string(body))
			}
		})
	}
}

func TestIntegrationPingInvalidTarget(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/probe?target=invalid.nonexistent.domain.test")
	if err != nil {
		t.Fatalf("Failed to probe invalid target: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "probe_success 0") {
		t.Error("Expected probe to fail for invalid target")
	}
}

func TestIntegrationPingMissingTarget(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/probe")
	if err != nil {
		t.Fatalf("Failed to probe without target: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if !strings.Contains(string(body), "Target parameter is missing") {
		t.Error("Expected error message about missing target parameter")
	}
}

func TestIntegrationRTTMetrics(t *testing.T) {
	resp, err := http.Get(testBaseURL + "/probe?target=127.0.0.1&count=3")
	if err != nil {
		t.Fatalf("Failed to probe: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	
	// Check if we got successful pings (RTT metrics are only present when packets are received)
	if strings.Contains(bodyStr, "probe_ping_packets_received") && 
	   !strings.Contains(bodyStr, "probe_ping_packets_received 0") {
		
		rttMetrics := []string{
			"probe_ping_rtt_seconds{type=\"best\"}",
			"probe_ping_rtt_seconds{type=\"worst\"}",
			"probe_ping_rtt_seconds{type=\"mean\"}",
			"probe_ping_rtt_seconds{type=\"sum\"}",
			"probe_ping_rtt_seconds{type=\"range\"}",
		}

		for _, metric := range rttMetrics {
			if !strings.Contains(bodyStr, metric) {
				t.Errorf("Missing RTT metric: %s", metric)
			}
		}
	} else {
		t.Log("No packets received, skipping RTT metrics check")
	}
}

func TestIntegrationConcurrentRequests(t *testing.T) {
	concurrency := 5
	done := make(chan bool, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			resp, err := http.Get(fmt.Sprintf("%s/probe?target=127.0.0.1&count=1", testBaseURL))
			if err != nil {
				t.Errorf("Goroutine %d failed: %v", id, err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Goroutine %d got status %d", id, resp.StatusCode)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < concurrency; i++ {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("Timeout waiting for concurrent requests to complete")
		}
	}
}