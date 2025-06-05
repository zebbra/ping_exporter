# Ping Exporter

[![Go Report Card](https://goreportcard.com/badge/github.com/zebbra/ping_exporter)](https://goreportcard.com/report/github.com/zebbra/ping_exporter)
[![Docker Repository on Quay](https://quay.io/repository/zebbra/ping_exporter/status)](https://quay.io/repository/zebbra/ping_exporter)

The ping exporter allows probing of network targets using ICMP ping with configurable packet counts and detailed statistics reporting. It implements the multi-target exporter pattern where targets are specified via URL parameters, similar to the blackbox exporter.

## Features

- **Multi-packet pings**: Send multiple ICMP packets per probe request
- **Comprehensive statistics**: Packet loss, RTT statistics (min, max, avg, stddev), jitter
- **IPv4/IPv6 support**: Configurable IP protocol preference
- **Flexible configuration**: Customizable packet count, timeout, interval, and packet size
- **Multi-target pattern**: Single exporter instance can probe multiple targets
- **Prometheus integration**: Native Prometheus metrics format

## Running this software

### From binaries

Download the most suitable binary from [the releases tab](https://github.com/zebbra/ping_exporter/releases)

Then:

    ./ping_exporter <flags>

### Using the docker image

*Note: You may want to [enable ipv6 in your docker configuration](https://docs.docker.com/v17.09/engine/userguide/networking/default_network/ipv6/)*

    docker run --rm \
      -p 9115:9115 \
      --name ping_exporter \
      --cap-add=NET_RAW \
      quay.io/zebbra/ping_exporter:latest

**Important**: The `--cap-add=NET_RAW` capability is required for ICMP ping functionality.

### Checking the results

Visiting [http://localhost:9115/probe?target=google.com&count=5&interval=1s&packet_size=64](http://localhost:9115/probe?target=google.com&count=5&interval=1s&packet_size=64)
will return metrics for a ping probe against google.com with 5 packets, 1 second interval, and 64-byte packet size. The `probe_success`
metric indicates if the probe succeeded. Adding a `debug=true` parameter
will return debug information for that probe.

Metrics concerning the operation of the exporter itself are available at the
endpoint <http://localhost:9115/metrics>.

### TLS and basic authentication

The Ping Exporter supports TLS and basic authentication. This enables better
control of the various HTTP endpoints.

To use TLS and/or basic authentication, you need to pass a configuration file
using the `--web.config.file` parameter. The format of the file is described
[in the exporter-toolkit repository](https://github.com/prometheus/exporter-toolkit/blob/master/docs/web-configuration.md).

Note that the TLS and basic authentication settings affect all HTTP endpoints:
`/metrics` and `/probe`.

## Controlling log level for probe logs

The Ping Exporter allows you to control the log level for individual probe logs using the `log_level` query parameter.

For example:

    http://localhost:9115/probe?target=example.com&count=3&log_level=debug

This will run the probe with debug logging enabled, which can be useful for troubleshooting probe issues without affecting the global log level of the exporter.

Valid log levels are: `debug`, `info`, `warn`, `error`.

If the log level is not specified, the global log level will be used.

## Metrics

### Probe Metrics

All probe metrics are prefixed with `probe_ping_`.

| Metric | Description |
|--------|-------------|
| `probe_success` | Whether the probe succeeded (1) or failed (0) |
| `probe_duration_seconds` | Total duration of the probe in seconds |
| `probe_ping_packets_sent` | Number of ICMP packets sent |
| `probe_ping_packets_received` | Number of ICMP packets received |
| `probe_ping_packet_loss_ratio` | Packet loss ratio (0.0 to 1.0) |
| `probe_ping_rtt_seconds{type="best"}` | Best (minimum) round-trip time in seconds |
| `probe_ping_rtt_seconds{type="worst"}` | Worst (maximum) round-trip time in seconds |
| `probe_ping_rtt_seconds{type="mean"}` | Mean round-trip time in seconds |
| `probe_ping_rtt_seconds{type="sum"}` | Sum of all round-trip times in seconds |
| `probe_ping_rtt_seconds{type="sd"}` | Squared deviation in seconds |
| `probe_ping_rtt_seconds{type="usd"}` | Standard deviation without correction in seconds |
| `probe_ping_rtt_seconds{type="csd"}` | Standard deviation with correction (Bessel's) in seconds |
| `probe_ping_rtt_seconds{type="range"}` | Range (worst - best) in seconds |

### Example Output

```
# HELP probe_success Displays whether or not the probe was a success
# TYPE probe_success gauge
probe_success 1

# HELP probe_duration_seconds Returns how long the probe took to complete in seconds
# TYPE probe_duration_seconds gauge
probe_duration_seconds 2.003456

# HELP probe_ping_packets_sent Number of ICMP packets sent
# TYPE probe_ping_packets_sent gauge
probe_ping_packets_sent 5

# HELP probe_ping_packets_received Number of ICMP packets received
# TYPE probe_ping_packets_received gauge
probe_ping_packets_received 5

# HELP probe_ping_packet_loss_ratio Packet loss ratio
# TYPE probe_ping_packet_loss_ratio gauge
probe_ping_packet_loss_ratio 0

# HELP probe_ping_rtt_seconds Round-trip time statistics in seconds
# TYPE probe_ping_rtt_seconds gauge
probe_ping_rtt_seconds{type="best"} 0.012345
probe_ping_rtt_seconds{type="worst"} 0.015678
probe_ping_rtt_seconds{type="mean"} 0.013912
probe_ping_rtt_seconds{type="sum"} 0.069560
probe_ping_rtt_seconds{type="sd"} 0.000001523
probe_ping_rtt_seconds{type="usd"} 0.001234
probe_ping_rtt_seconds{type="csd"} 0.001379
probe_ping_rtt_seconds{type="range"} 0.003333
```

## Building the software

### Local Build

    make build

### Building with Docker

    docker build -t ping_exporter .

## CI/CD and Releases

This project uses GitHub Actions for continuous integration and automated releases.

### Automated Releases

Releases are automatically created when a Git tag is pushed:

```bash
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0
```

This triggers:
- Automated testing
- Multi-platform binary builds (Linux, macOS, Windows)
- Docker image builds for `linux/amd64` and `linux/arm64`
- Publication to `quay.io/zebbra/ping_exporter`
- GitHub release creation with binaries and packages

### Docker Images

Official Docker images are available at:
- `quay.io/zebbra/ping_exporter:latest`
- `quay.io/zebbra/ping_exporter:v1.0.0` (version tags)

### Development

For local development and testing:

```bash
# Test GoReleaser configuration
make goreleaser-check

# Build snapshot release locally
make release-snapshot

# Run tests
make test
```

See [CI_CD.md](CI_CD.md) for detailed information about the CI/CD setup.

## Configuration

The ping exporter is configured via URL parameters for ping settings and command-line flags for server configuration. All ping parameters are specified dynamically through the probe URL.

### URL Parameters

| Parameter | Description | Default | Example |
|-----------|-------------|---------|---------|
| `target` | Target hostname or IP address to ping | *required* | `google.com`, `8.8.8.8` |
| `count` | Number of ping packets to send | `3` | `5` |
| `interval` | Time interval between packets | `1s` | `500ms`, `2s` |
| `packet_size` | Size of the ping packet payload in bytes | `64` | `32`, `1024` |
| `timeout` | Maximum duration for the entire probe | `5s` | `10s`, `30s` |
| `ip_protocol` | IP protocol preference: `ip4`, `ip6`, or `auto` | `ip4` | `ip6` |
| `source_ip` | Source IP address for outgoing packets | *auto* | `192.168.1.100` |
| `dont_fragment` | Set the Don't Fragment bit in IPv4 header | `false` | `true` |
| `debug` | Enable debug output | `false` | `true` |
| `log_level` | Override log level for this probe | *global* | `debug`, `info` |

### Example URLs

```
# Basic ping with defaults (3 packets, 1s interval, 64 bytes)
http://localhost:9115/probe?target=google.com

# Custom packet count and interval
http://localhost:9115/probe?target=8.8.8.8&count=10&interval=500ms

# Large packets with longer timeout
http://localhost:9115/probe?target=prometheus.io&count=5&packet_size=1024&timeout=10s

# IPv6 ping
http://localhost:9115/probe?target=2001:4860:4860::8888&ip_protocol=ip6

# Debug mode with specific source IP
http://localhost:9115/probe?target=example.com&source_ip=192.168.1.100&debug=true
```

### Command-line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--web.listen-address` | Address to listen on for web interface and telemetry | `:9115` |
| `--web.config.file` | Path to web configuration file for TLS/auth | `` |
| `--log.level` | Global logging level | `info` |
| `--log.format` | Log format | `logfmt` |
| `--ping.default-count` | Default packet count when not specified | `3` |
| `--ping.default-interval` | Default interval when not specified | `1s` |
| `--ping.default-packet-size` | Default packet size when not specified | `64` |
| `--ping.default-timeout` | Default timeout when not specified | `5s` |
| `--ping.max-count` | Maximum allowed packet count | `100` |
| `--ping.max-packet-size` | Maximum allowed packet size | `65507` |

## Prometheus Configuration

Ping exporter implements the multi-target exporter pattern, so we advise
to read the guide [Understanding and using the multi-target exporter pattern
](https://prometheus.io/docs/guides/multi-target-exporter/) to get the general
idea about the configuration.

The ping exporter needs to be passed the target as a parameter, this can be
done with relabelling.

Example config:
```yaml
scrape_configs:
  - job_name: 'ping'
    metrics_path: /probe
    params:
      count: ['5']        # Send 5 ping packets
      interval: ['1s']    # 1 second between packets
      packet_size: ['64'] # 64-byte packets
    static_configs:
      - targets:
        - google.com        # Target to ping
        - 8.8.8.8          # Target to ping by IP
        - prometheus.io     # Target to ping
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 127.0.0.1:9115  # The ping exporter's real hostname:port.
  - job_name: 'ping_exporter'  # collect ping exporter's operational metrics.
    static_configs:
      - targets: ['127.0.0.1:9115']
```

For dynamic discovery with DNS and different ping configurations:
```yaml
scrape_configs:
  - job_name: ping_dns_fast
    metrics_path: /probe
    params:
      count: ['3']
      interval: ['500ms']
      packet_size: ['32']
    dns_sd_configs:
      - names:
          - example.com
          - prometheus.io
        type: A
        port: 80  # This port is ignored for ping, but required for DNS SD
    relabel_configs:
      - source_labels: [__address__]
        regex: '([^:]+):.*'
        target_label: __param_target
        replacement: '${1}'  # Extract hostname without port
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 127.0.0.1:9115  # The ping exporter's real hostname:port.

  - job_name: ping_dns_thorough
    metrics_path: /probe
    params:
      count: ['10']
      interval: ['1s']
      packet_size: ['1024']
      timeout: ['15s']
    dns_sd_configs:
      - names:
          - critical-service.example.com
        type: A
        port: 80
    relabel_configs:
      - source_labels: [__address__]
        regex: '([^:]+):.*'
        target_label: __param_target
        replacement: '${1}'
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: 127.0.0.1:9115
```

## Permissions

The ping exporter requires elevated privileges to send ICMP packets:

### Linux
Run as root or grant the `CAP_NET_RAW` capability:
```bash
sudo setcap cap_net_raw+ep ./ping_exporter
```

### Docker
Use the `--cap-add=NET_RAW` flag when running the container.

### Windows
Run as Administrator.

### macOS
Run as root or use `sudo`.

## Troubleshooting

### Common Issues

1. **Permission denied errors**: Ensure the binary has the necessary permissions to send ICMP packets (see Permissions section).

2. **No response from target**: Check if the target host responds to ping from the command line first.

3. **IPv6 connectivity issues**: Ensure your system has proper IPv6 configuration if using `ip6` or `auto` protocols.

4. **High packet loss**: Consider increasing the timeout or reducing the packet count/interval for unreliable networks.

### Debug Mode

Add `debug=true` to the probe URL to get detailed debug information:
```
http://localhost:9115/probe?target=example.com&count=5&debug=true
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0, see [LICENSE](LICENSE).
