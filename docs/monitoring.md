# Prometheus metrics monitoring

CasOS can query an existing Prometheus server to show historical CPU, memory,
network, node filesystem, and PVC usage in the Monitor Center. Prometheus is an
optional integration: CasOS does not collect or persist these metrics itself,
and the existing Kubernetes health checks, Events, issues, and diagnosis APIs
continue to work when Prometheus is disabled or unavailable.

## Configuration

Set the Prometheus base URL and query timeout in `conf/app.conf`:

```ini
prometheusAddress      = http://prometheus.monitoring.svc:9090
prometheusQueryTimeout = 10s
```

The same keys can be supplied as environment variables through the existing
CasOS configuration loader:

```bash
export prometheusAddress=http://127.0.0.1:9090
export prometheusQueryTimeout=10s
```

Leave `prometheusAddress` empty to disable time-series metrics. The timeout is a
Go duration such as `5s` or `1m`; a positive number is also accepted as seconds.

## Required metric sources

Prometheus must already scrape the following metric families. A typical
`kube-prometheus-stack` installation provides them, but equivalent exporters
are also supported.

| Source | Metric families used | CasOS views |
| --- | --- | --- |
| node_exporter | `node_cpu_seconds_total`, `node_memory_*`, `node_network_*`, `node_filesystem_*` | Cluster and Node CPU, memory, network, and disk |
| kubelet / cAdvisor | `container_cpu_usage_seconds_total`, `container_memory_working_set_bytes`, `container_network_*` | Pod CPU, memory, and network |
| kubelet volume stats | `kubelet_volume_stats_used_bytes`, `kubelet_volume_stats_capacity_bytes` | PVC storage usage |

CasOS does not install Prometheus or exporters. Missing metric families return a
successful response with an empty `series` array so the UI can distinguish
"no data" from a failed query.

## Supported queries

The endpoint is:

```text
GET /api/get-monitor-metrics
```

Supported scope and metric combinations are:

| Scope | Metrics | Unit |
| --- | --- | --- |
| `cluster` | `cpu`, `memory`, `network_receive`, `network_transmit`, `disk` | percent or bytes/second |
| `node` | `cpu`, `memory`, `network_receive`, `network_transmit`, `disk` | percent or bytes/second |
| `pod` | `cpu`, `memory`, `network_receive`, `network_transmit` | cores, bytes, or bytes/second |
| `pvc` | `storage` | percent |

Query parameters:

| Parameter | Required | Description |
| --- | --- | --- |
| `scope` | yes | `cluster`, `node`, `pod`, or `pvc` |
| `metric` | yes | One metric supported by the selected scope |
| `namespace` | no | Pod/PVC namespace filter; required when their `name` is set |
| `name` | no | Node, Pod, or PVC name filter; omitted to return all matching series |
| `start` | range only | RFC3339 timestamp or Unix seconds |
| `end` | range only | RFC3339 timestamp or Unix seconds |
| `step` | no | Duration such as `30s`/`5m`, or seconds; CasOS chooses a default when omitted |

Supplying neither `start` nor `end` performs an instant query. Supplying both
performs a range query. A range may span at most 90 days and may contain at most
11,000 requested points per series.

Example range query:

```bash
curl -G 'http://localhost:9000/api/get-monitor-metrics' \
  --data-urlencode 'scope=cluster' \
  --data-urlencode 'metric=cpu' \
  --data-urlencode 'start=2026-07-15T00:00:00Z' \
  --data-urlencode 'end=2026-07-15T01:00:00Z' \
  --data-urlencode 'step=15s'
```

Authenticated deployments must include the normal CasOS session credentials.

Successful responses use the standard CasOS envelope. Metric samples are kept
as parallel timestamp and value arrays:

```json
{
  "status": "ok",
  "data": {
    "scope": "node",
    "metric": "disk",
    "unit": "percent",
    "start": "2026-07-15T00:00:00Z",
    "end": "2026-07-15T01:00:00Z",
    "step": 60,
    "series": [
      {
        "metric": "disk",
        "object": "worker-1:9100",
        "labels": {"instance": "worker-1:9100"},
        "timestamps": [1752537600, 1752537660],
        "values": [42.1, 42.3]
      }
    ]
  }
}
```

PromQL definitions are centralized in `object/monitor_metrics.go`; controllers
and frontend code never construct PromQL.

## UI behavior and limitations

The Monitor Center provides the last 1 hour, 6 hours, 24 hours, 7 days, and a
custom range. Manual refresh is always available, and automatic refresh runs
every 60 seconds when enabled.

- Node series rely on the common node_exporter `instance` label. A name filter
  matches `node-name` and `node-name:<port>`; installations with different
  relabeling conventions may need matching Prometheus relabel rules.
- Pod and PVC queries rely on the conventional `namespace`, `pod`, and
  `persistentvolumeclaim` labels.
- The Node disk chart shows the most-used eligible filesystem per Node. Virtual
  and container-runtime filesystems are excluded.
- Querying all Nodes or PVCs can produce many lines in large clusters. Object
  selectors and additional UI filtering can be added in a later phase.
- Authentication, custom CA bundles, and per-tenant Prometheus data-source
  configuration are not included in this phase.
