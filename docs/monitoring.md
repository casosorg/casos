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
| `pvc` | `storage`, `storage_used_bytes`, `storage_capacity_bytes` | percent or bytes |

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

PromQL definitions are centralized in `object/monitor_metrics.go` and
`object/monitor_resource.go`; controllers and frontend code never construct
PromQL.

## Resource detail monitoring

The generic metric endpoint remains available for debugging and reusable
queries. Resource detail pages use aggregate endpoints that return Kubernetes
metadata, multiple metric results, per-metric status, and relationship data in
one response:

| Endpoint | Resource |
| --- | --- |
| `GET /api/get-node-monitor-overview?name=<node>` | Node detail |
| `GET /api/get-pod-monitor-overview?namespace=<ns>&name=<pod>&mode=total|container` | Pod detail |
| `GET /api/get-workload-monitor-overview?kind=Deployment|StatefulSet|DaemonSet&namespace=<ns>&name=<name>&mode=total|pod` | Workload detail |
| `GET /api/get-pvc-monitor-overview?namespace=<ns>&name=<pvc>` | PVC detail |
| `GET /api/get-monitor-resource-inventory` | Monitor Center resource inventory |
| `GET /api/get-monitor-top?resource=node|pod|workload&metric=cpu|memory&limit=5` | Cluster Top N |
| `GET /api/get-monitor-resource-events?kind=<kind>&namespace=<ns>&name=<name>` | Resource Events tab |

Each overview response contains a `metrics` array. Every metric has independent
`status`, `error.code`, and `data.series` fields. A failed chart does not clear
other successful charts, and Kubernetes metadata still loads when Prometheus is
disabled or unavailable.

Structured monitor error codes are:

| Code | Meaning |
| --- | --- |
| `invalid_params` | Request validation failed |
| `prometheus_not_configured` | `prometheusAddress` is empty |
| `prometheus_unavailable` | Prometheus could not be reached or returned a server-side failure |
| `prometheus_timeout` | The Prometheus request exceeded `prometheusQueryTimeout` |
| `query_error` | Prometheus rejected the query or returned an invalid result |
| `empty` | The query succeeded but returned no usable samples |

CasOS cannot reliably distinguish "metric family missing" from "valid query
with no samples" without additional Prometheus metadata checks, so both are
reported as `empty` on successful queries.

## Kubernetes ownership and resource relationships

CasOS does not model Kubernetes resources as a single linear tree. Node and
Workload are parallel relationships for Pods:

- Node detail lists Pods using Kubernetes field selector
  `spec.nodeName=<node>`.
- Pod detail shows both `Scheduled On` (Node) and `Controlled By` (Workload).
- Deployment Pod ownership is resolved as
  `Deployment -> ReplicaSet -> Pod` using controller ownerReferences and UID
  checks.
- StatefulSet and DaemonSet Pods are resolved through controller
  ownerReferences and UID checks.
- Job and CronJob ownership is resolved for Pod relationship display and
  Workload Top N aggregation when present.

CasOS does not infer ownership from Pod name prefixes.

## Workload monitoring semantics

This phase does not require kube-state-metrics. Workload monitoring uses the
Kubernetes API to list the Workload's current Pods, then queries Prometheus for
those Pod names:

```text
Current Workload Pods -> safe pod=~"^(...|...)$" matcher -> Prometheus range query
```

The resulting Workload trends are therefore **current Pods historical trends**,
not complete Workload history across the whole selected time range. If a
Deployment rolled out earlier and old Pods were deleted, those deleted Pods are
not included unless future kube-state-metrics joins or recording rules are
added.

`mode=total` returns one aggregate line by default. `mode=pod` is opt-in and is
limited to 10 lines by default with a hard cap of 20 selected Pod series. Pod
names are safely regex-escaped before building PromQL, and oversized matchers
are rejected.

CPU and memory request/limit values come from the current Kubernetes
configuration. If they are shown as references alongside usage, they do not
represent historical request/limit changes.

## Node identity mapping

Node detail monitoring handles common node_exporter label conventions in this
order:

1. Prometheus `node="<node name>"`;
2. Prometheus `nodename="<node name>"`;
3. `instance` matching Kubernetes Node InternalIP or ExternalIP with optional
   port;
4. `instance` matching the Kubernetes Node name with optional port.

If none of these match, the metric query returns empty data for that Node rather
than matching another Node.

## Container and storage limitations

Pod CPU and memory support `Total` and `By Container`. Pod network is shown only
as Pod total because `container_network_*` metrics commonly represent the shared
Pod network namespace and can be double-counted if split by container.

PVC storage uses kubelet volume stats:

- `kubelet_volume_stats_used_bytes`
- `kubelet_volume_stats_capacity_bytes`

PVC IOPS and read/write throughput are not displayed by default. They require a
CSI driver metric source or another storage exporter with reliable volume
labels.

## Query limits

CasOS applies the following limits before sending queries to Prometheus:

- Maximum time range: 90 days.
- Maximum requested points per series: 11,000.
- Minimum step: 1 second.
- Rate window: `max(5m, step * 4)` capped at 1 hour.
- Overview Prometheus query concurrency: 4.
- Default Workload By Pod series limit: 10.
- Hard series and selected-Pod limit: 20.
- Maximum Pod regex matcher length: 4,096 characters.
- Top N default limit: 5; maximum: 20.
- Prometheus HTTP response size: 32 MiB.

Frontend monitoring tabs cancel old requests with `AbortController`, ignore
stale responses with a request id, refresh only the active Monitoring tab, pause
automatic refresh when `document.hidden`, and do not auto-refresh fixed custom
history ranges.

## UI behavior and limitations

The Monitor Center provides the last 1 hour, 6 hours, 24 hours, 7 days, and a
custom range. Manual refresh is always available, and automatic refresh runs
every 60 seconds when enabled.

- Node detail monitoring supports `node`, `nodename`, Node IP, and compatible
  `instance` matching as described above. The generic `node` scope still uses
  the historical `instance` matcher for backwards compatibility.
- Pod and PVC queries rely on the conventional `namespace`, `pod`, and
  `persistentvolumeclaim` labels.
- The Node disk chart shows the most-used eligible filesystem per Node. Virtual
  and container-runtime filesystems are excluded.
- Monitor Center uses Kubernetes API objects as the primary navigation source:
  Nodes, Workloads, and PVCs are listed only when those objects exist in the
  current CasOS Kubernetes API. Prometheus is used only to add current CPU,
  memory, network, disk, and PVC usage columns to those rows.
- Node rows can be expanded to show Pods scheduled on that Node. Workload rows
  can be expanded to show Pods currently owned by that Workload. Missing
  resource groups are not rendered.
- The Top N API remains available as an auxiliary discovery/debug endpoint, but
  Prometheus-only objects should not be used as primary detail navigation when
  they are not present in the current Kubernetes API.
- Authentication, custom CA bundles, and per-tenant Prometheus data-source
  configuration are not included in this phase.
