# Ping Exporter Helm Chart

This Helm chart deploys the [ping_exporter](https://github.com/zebbra/ping_exporter) on Kubernetes. The ping exporter allows probing of network targets using ICMP ping with configurable packet counts and detailed statistics reporting for Prometheus monitoring.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+
- Cluster nodes must support ICMP (ping) operations

## Installing the Chart

To install the chart with the name `ping-exporter`:

```bash
helm install ping-exporter ./chart/ping_exporter
```

To install from a Helm repository (if published):

```bash
helm repo add zebbra https://charts.zebbra.com
helm install ping-exporter zebbra/ping_exporter
```

## Uninstalling the Chart

To uninstall/delete the `ping-exporter` deployment:

```bash
helm uninstall ping-exporter
```

## Configuration

The following table lists the configurable parameters of the ping_exporter chart and their default values.

### Basic Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Container image repository | `quay.io/zebbra/ping_exporter` |
| `image.tag` | Container image tag | `""` (uses appVersion) |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `imagePullSecrets` | Image pull secrets | `[]` |
| `nameOverride` | Override name of the chart | `""` |
| `fullnameOverride` | Override full name of the chart | `""` |

### Service Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Service port | `9115` |

### Security Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.name` | Service account name | `""` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `podSecurityContext` | Pod security context | `{}` |
| `securityContext.capabilities.add` | Container capabilities | `["NET_RAW"]` |

### Resource Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `100m` |
| `resources.limits.memory` | Memory limit | `128Mi` |
| `resources.requests.cpu` | CPU request | `50m` |
| `resources.requests.memory` | Memory request | `64Mi` |

### Health Checks

| Parameter | Description | Default |
|-----------|-------------|---------|
| `livenessProbe.httpGet.path` | Liveness probe path | `/metrics` |
| `livenessProbe.initialDelaySeconds` | Liveness probe initial delay | `30` |
| `livenessProbe.periodSeconds` | Liveness probe period | `30` |
| `readinessProbe.httpGet.path` | Readiness probe path | `/metrics` |
| `readinessProbe.initialDelaySeconds` | Readiness probe initial delay | `5` |
| `readinessProbe.periodSeconds` | Readiness probe period | `10` |

### Ping Exporter Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `pingExporter.args.logLevel` | Logging level (debug, info, warn, error) | `info` |
| `pingExporter.args.logFormat` | Log format (logfmt, json) | `logfmt` |
| `pingExporter.args.webListenAddress` | Web server listen address | `:9115` |
| `pingExporter.args.defaultCount` | Default number of ping packets | `3` |
| `pingExporter.args.defaultInterval` | Default interval between packets | `1s` |
| `pingExporter.args.defaultPacketSize` | Default packet size in bytes | `64` |
| `pingExporter.args.defaultTimeout` | Default probe timeout | `5s` |
| `pingExporter.args.maxCount` | Maximum allowed packet count | `100` |
| `pingExporter.args.maxPacketSize` | Maximum allowed packet size | `65507` |

### Web Configuration (TLS/Auth)

| Parameter | Description | Default |
|-----------|-------------|---------|
| `pingExporter.webConfig.enabled` | Enable web configuration file | `false` |
| `pingExporter.webConfig.config` | Web configuration content | `{}` |

### Ingress Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable ingress | `false` |
| `ingress.className` | Ingress class name | `""` |
| `ingress.annotations` | Ingress annotations | `{}` |
| `ingress.hosts` | Ingress hosts configuration | See values.yaml |
| `ingress.tls` | Ingress TLS configuration | `[]` |

### Monitoring Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceMonitor.enabled` | Create ServiceMonitor for Prometheus Operator | `false` |
| `serviceMonitor.labels` | Additional labels for ServiceMonitor | `{}` |
| `serviceMonitor.interval` | Scrape interval | `30s` |
| `serviceMonitor.scrapeTimeout` | Scrape timeout | `10s` |
| `prometheusRule.enabled` | Create PrometheusRule for alerting | `false` |
| `prometheusRule.labels` | Additional labels for PrometheusRule | `{}` |
| `prometheusRule.rules` | Alerting rules | `[]` |

### Other Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable horizontal pod autoscaling | `false` |
| `autoscaling.minReplicas` | Minimum number of replicas | `1` |
| `autoscaling.maxReplicas` | Maximum number of replicas | `3` |
| `nodeSelector` | Node selector | `{}` |
| `tolerations` | Tolerations | `[]` |
| `affinity` | Affinity | `{}` |

## Examples

### Basic Installation

```bash
helm install ping-exporter ./chart/ping_exporter
```

### Custom Configuration

```bash
helm install ping-exporter ./chart/ping_exporter \
  --set replicaCount=2 \
  --set pingExporter.args.logLevel=debug \
  --set resources.requests.memory=128Mi
```

### With Ingress

```bash
helm install ping-exporter ./chart/ping_exporter \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=ping-exporter.example.com \
  --set ingress.hosts[0].paths[0].path=/ \
  --set ingress.hosts[0].paths[0].pathType=Prefix
```

### With ServiceMonitor

```bash
helm install ping-exporter ./chart/ping_exporter \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.interval=15s
```

### With TLS Configuration

Create a values file with web configuration:

```yaml
# values-tls.yaml
pingExporter:
  webConfig:
    enabled: true
    config:
      tls_server_config:
        cert_file: /etc/certs/tls.crt
        key_file: /etc/certs/tls.key
      basic_auth_users:
        admin: $2y$10$...

volumes:
- name: certs
  secret:
    secretName: ping-exporter-tls

volumeMounts:
- name: certs
  mountPath: /etc/certs
  readOnly: true
```

```bash
helm install ping-exporter ./chart/ping_exporter -f values-tls.yaml
```

## Usage

After installation, the ping exporter will be available at the service endpoint. You can test it using port-forwarding:

```bash
kubectl port-forward svc/ping-exporter 9115:9115
```

Then access the endpoints:

- Metrics: `http://localhost:9115/metrics`
- Ping probe: `http://localhost:9115/probe?target=google.com`
- Debug probe: `http://localhost:9115/probe?target=8.8.8.8&count=5&debug=true`

## Prometheus Integration

### Manual Configuration

Add the following to your Prometheus configuration:

```yaml
scrape_configs:
  # Scrape the exporter itself
  - job_name: 'ping-exporter'
    static_configs:
    - targets: ['ping-exporter:9115']

  # Scrape ping probes
  - job_name: 'ping-probes'
    metrics_path: /probe
    params:
      count: ['5']
      interval: ['1s']
    static_configs:
    - targets:
      - google.com
      - 8.8.8.8
      - github.com
    relabel_configs:
    - source_labels: [__address__]
      target_label: __param_target
    - source_labels: [__param_target]
      target_label: instance
    - target_label: __address__
      replacement: ping-exporter:9115
```

### Prometheus Operator

Enable ServiceMonitor for automatic discovery:

```yaml
serviceMonitor:
  enabled: true
  interval: 30s
  labels:
    prometheus: kube-prometheus
```

## Alerting Rules Example

```yaml
prometheusRule:
  enabled: true
  rules:
  - alert: PingExporterDown
    expr: up{job="ping-exporter"} == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Ping Exporter is down"
      description: "Ping Exporter has been down for more than 5 minutes."

  - alert: HighPingPacketLoss
    expr: probe_ping_packet_loss_ratio > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High packet loss detected"
      description: "{{ $labels.instance }} has {{ $value | humanizePercentage }} packet loss."

  - alert: HighPingLatency
    expr: probe_ping_rtt_seconds{type="mean"} > 0.1
    for: 2m
    labels:
      severity: warning
    annotations:
      summary: "High ping latency detected"
      description: "{{ $labels.instance }} has mean RTT of {{ $value | humanizeDuration }}."
```

## Security Considerations

The ping exporter requires the `NET_RAW` capability to send ICMP packets. This is automatically configured in the chart's security context. In restricted environments, you may need to:

1. Use a privileged security context
2. Configure appropriate Pod Security Policies/Standards
3. Use network policies to restrict access

## Troubleshooting

### Permission Issues

If you see permission denied errors, ensure:
- The container has `NET_RAW` capability
- Pod Security Policies allow the capability
- The node supports ICMP operations

### Network Issues

- Verify target hosts are reachable from the cluster
- Check firewall rules for ICMP traffic
- Test with simple targets like `8.8.8.8` first

### Resource Issues

- Monitor CPU/memory usage under load
- Adjust resource requests/limits as needed
- Consider horizontal scaling for high-volume probing

## Contributing

Contributions are welcome! Please see the main project repository at https://github.com/zebbra/ping_exporter for guidelines.

## License

This chart is licensed under the Apache License 2.0, same as the ping_exporter project.