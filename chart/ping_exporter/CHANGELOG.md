# Changelog

All notable changes to the ping_exporter Helm chart will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2024-12-05

### Added
- Initial Helm chart for ping_exporter
- Support for all ping_exporter command-line arguments
- Configurable security context with NET_RAW capability
- ServiceMonitor template for Prometheus Operator integration
- PrometheusRule template for alerting rules
- Web configuration support for TLS and basic authentication
- Comprehensive health checks (liveness and readiness probes)
- Horizontal Pod Autoscaler support
- Ingress configuration with customizable annotations
- Resource limits and requests with sensible defaults
- Node selector, tolerations, and affinity support
- Volume and volume mount support
- Comprehensive test suite for chart validation
- Detailed documentation and usage examples
- Multiple example configurations for different use cases

### Configuration Features
- Default ping parameters (count, interval, packet size, timeout)
- Maximum limits for ping parameters
- Logging configuration (level and format)
- Web server listen address configuration
- Image configuration with support for custom repositories and tags
- Service account creation and configuration
- Pod security context and container security context
- Service type and port configuration

### Monitoring Features
- ServiceMonitor for Prometheus Operator with configurable scrape settings
- PrometheusRule for custom alerting rules
- Example alert rules for common scenarios (downtime, packet loss, latency)
- Metrics endpoint exposure for exporter operational metrics

### Security Features
- NET_RAW capability for ICMP ping functionality
- Optional TLS configuration via web config file
- Optional basic authentication support
- Configurable security contexts
- Secret management for web configuration

### Documentation
- Comprehensive README with configuration options
- Usage examples for different deployment scenarios
- Prometheus integration examples
- Troubleshooting guide
- Security considerations
- Multiple example values files for common use cases

### Templates
- Deployment with ping_exporter container configuration
- Service for exposing the exporter
- ServiceAccount with configurable annotations
- Secret for web configuration (when enabled)
- ServiceMonitor for Prometheus Operator (when enabled)
- PrometheusRule for alerting (when enabled)
- Ingress for external access (when enabled)
- HorizontalPodAutoscaler for scaling (when enabled)
- Test pod for chart validation

### Default Values
- Container image: `quay.io/zebbra/ping_exporter:latest`
- Service port: 9115
- Default ping count: 3 packets
- Default ping interval: 1 second
- Default packet size: 64 bytes
- Default timeout: 5 seconds
- Log level: info
- Resource requests: 50m CPU, 64Mi memory
- Resource limits: 100m CPU, 128Mi memory
- Replica count: 1
- Service type: ClusterIP

[Unreleased]: https://github.com/zebbra/ping_exporter/compare/chart-v0.1.0...HEAD
[0.1.0]: https://github.com/zebbra/ping_exporter/releases/tag/chart-v0.1.0