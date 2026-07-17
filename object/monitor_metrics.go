package object

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/casosorg/casos/conf"
	prometheusapi "github.com/casosorg/casos/prometheus"
)

const (
	defaultPrometheusQueryTimeout = 10 * time.Second
	maxMonitorMetricRange         = 90 * 24 * time.Hour
	maxMonitorMetricSamples       = 11000
)

var ErrPrometheusNotConfigured = errors.New("Prometheus is not configured")

type MonitorMetricQueryParams struct {
	Scope     string
	Metric    string
	Namespace string
	Name      string
	Start     string
	End       string
	Step      string
}

type MonitorMetricQuery struct {
	Scope     string
	Metric    string
	Namespace string
	Name      string
	Start     time.Time
	End       time.Time
	Step      time.Duration
	IsRange   bool
}

type MonitorMetricSeries struct {
	Metric     string            `json:"metric"`
	Object     string            `json:"object"`
	Labels     map[string]string `json:"labels"`
	Timestamps []float64         `json:"timestamps"`
	Values     []float64         `json:"values"`
}

type MonitorMetricResponse struct {
	Scope  string                `json:"scope"`
	Metric string                `json:"metric"`
	Unit   string                `json:"unit"`
	Start  string                `json:"start,omitempty"`
	End    string                `json:"end,omitempty"`
	Step   float64               `json:"step,omitempty"`
	Series []MonitorMetricSeries `json:"series"`
}

type monitorMetricDefinition struct {
	unit       string
	buildQuery func(MonitorMetricQuery) string
}

type prometheusQuerier interface {
	Query(context.Context, string, time.Time) ([]prometheusapi.Series, error)
	QueryRange(context.Context, string, prometheusapi.Range) ([]prometheusapi.Series, error)
}

var monitorMetricDefinitions = map[string]map[string]monitorMetricDefinition{
	"cluster": {
		"cpu": {
			unit: "percent",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[%s])))`, monitorMetricRateWindow(query))
			},
		},
		"memory": {
			unit: "percent",
			buildQuery: func(MonitorMetricQuery) string {
				return `100 * (1 - sum(node_memory_MemAvailable_bytes) / sum(node_memory_MemTotal_bytes))`
			},
		},
		"network_receive": {
			unit: "bytes_per_second",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`sum(rate(%s[%s]))`, promSelector("node_network_receive_bytes_total", nodeNetworkMatchers("")), monitorMetricRateWindow(query))
			},
		},
		"network_transmit": {
			unit: "bytes_per_second",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`sum(rate(%s[%s]))`, promSelector("node_network_transmit_bytes_total", nodeNetworkMatchers("")), monitorMetricRateWindow(query))
			},
		},
		"disk": {
			unit: "percent",
			buildQuery: func(MonitorMetricQuery) string {
				available := promSelector("node_filesystem_avail_bytes", nodeFilesystemMatchers(""))
				size := promSelector("node_filesystem_size_bytes", nodeFilesystemMatchers(""))
				return fmt.Sprintf(`100 * (1 - sum(%s) / sum(%s))`, available, size)
			},
		},
	},
	"node": {
		"cpu": {
			unit: "percent",
			buildQuery: func(query MonitorMetricQuery) string {
				matchers := append([]string{`mode="idle"`}, nodeInstanceMatchers(query.Name)...)
				return fmt.Sprintf(`100 * (1 - avg by (instance) (rate(%s[%s])))`, promSelector("node_cpu_seconds_total", matchers), monitorMetricRateWindow(query))
			},
		},
		"memory": {
			unit: "percent",
			buildQuery: func(query MonitorMetricQuery) string {
				available := promSelector("node_memory_MemAvailable_bytes", nodeInstanceMatchers(query.Name))
				total := promSelector("node_memory_MemTotal_bytes", nodeInstanceMatchers(query.Name))
				return fmt.Sprintf(`100 * (1 - max by (instance) (%s) / max by (instance) (%s))`, available, total)
			},
		},
		"network_receive": {
			unit: "bytes_per_second",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`sum by (instance) (rate(%s[%s]))`, promSelector("node_network_receive_bytes_total", nodeNetworkMatchers(query.Name)), monitorMetricRateWindow(query))
			},
		},
		"network_transmit": {
			unit: "bytes_per_second",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`sum by (instance) (rate(%s[%s]))`, promSelector("node_network_transmit_bytes_total", nodeNetworkMatchers(query.Name)), monitorMetricRateWindow(query))
			},
		},
		"disk": {
			unit: "percent",
			buildQuery: func(query MonitorMetricQuery) string {
				available := promSelector("node_filesystem_avail_bytes", nodeFilesystemMatchers(query.Name))
				size := promSelector("node_filesystem_size_bytes", nodeFilesystemMatchers(query.Name))
				return fmt.Sprintf(`max by (instance) (100 * (1 - %s / %s))`, available, size)
			},
		},
	},
	"pod": {
		"cpu": {
			unit: "cores",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`sum by (namespace, pod) (rate(%s[%s]))`, promSelector("container_cpu_usage_seconds_total", podContainerMatchers(query)), monitorMetricRateWindow(query))
			},
		},
		"memory": {
			unit: "bytes",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`sum by (namespace, pod) (%s)`, promSelector("container_memory_working_set_bytes", podContainerMatchers(query)))
			},
		},
		"network_receive": {
			unit: "bytes_per_second",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`sum by (namespace, pod) (rate(%s[%s]))`, promSelector("container_network_receive_bytes_total", podNetworkMatchers(query)), monitorMetricRateWindow(query))
			},
		},
		"network_transmit": {
			unit: "bytes_per_second",
			buildQuery: func(query MonitorMetricQuery) string {
				return fmt.Sprintf(`sum by (namespace, pod) (rate(%s[%s]))`, promSelector("container_network_transmit_bytes_total", podNetworkMatchers(query)), monitorMetricRateWindow(query))
			},
		},
	},
	"pvc": {
		"storage": {
			unit: "percent",
			buildQuery: func(query MonitorMetricQuery) string {
				matchers := pvcMatchers(query)
				used := promSelector("kubelet_volume_stats_used_bytes", matchers)
				capacity := promSelector("kubelet_volume_stats_capacity_bytes", matchers)
				return fmt.Sprintf(`100 * max by (namespace, persistentvolumeclaim) (%s) / max by (namespace, persistentvolumeclaim) (%s > 0)`, used, capacity)
			},
		},
	},
}

func ParseMonitorMetricQuery(params MonitorMetricQueryParams) (MonitorMetricQuery, error) {
	query := MonitorMetricQuery{
		Scope:     strings.ToLower(strings.TrimSpace(params.Scope)),
		Metric:    strings.ToLower(strings.TrimSpace(params.Metric)),
		Namespace: strings.TrimSpace(params.Namespace),
		Name:      strings.TrimSpace(params.Name),
	}
	if query.Scope == "" {
		return MonitorMetricQuery{}, errors.New("scope is required")
	}
	definitions, ok := monitorMetricDefinitions[query.Scope]
	if !ok {
		return MonitorMetricQuery{}, fmt.Errorf("unsupported scope %q; expected cluster, node, pod, or pvc", query.Scope)
	}
	if query.Metric == "" {
		return MonitorMetricQuery{}, errors.New("metric is required")
	}
	if _, ok := definitions[query.Metric]; !ok {
		return MonitorMetricQuery{}, fmt.Errorf("metric %q is not supported for scope %q; supported metrics: %s", query.Metric, query.Scope, strings.Join(sortedMetricNames(definitions), ", "))
	}
	if query.Scope == "cluster" && (query.Namespace != "" || query.Name != "") {
		return MonitorMetricQuery{}, errors.New("namespace and name are not valid for cluster metrics")
	}
	if query.Scope == "node" && query.Namespace != "" {
		return MonitorMetricQuery{}, errors.New("namespace is not valid for node metrics")
	}
	if (query.Scope == "pod" || query.Scope == "pvc") && query.Name != "" && query.Namespace == "" {
		return MonitorMetricQuery{}, errors.New("namespace is required when name is specified for pod or pvc metrics")
	}

	startText := strings.TrimSpace(params.Start)
	endText := strings.TrimSpace(params.End)
	stepText := strings.TrimSpace(params.Step)
	if startText == "" && endText == "" {
		if stepText != "" {
			return MonitorMetricQuery{}, errors.New("step requires start and end")
		}
		return query, nil
	}
	if startText == "" || endText == "" {
		return MonitorMetricQuery{}, errors.New("start and end must be provided together")
	}

	start, err := parseMonitorMetricTimestamp(startText)
	if err != nil {
		return MonitorMetricQuery{}, fmt.Errorf("invalid start: %w", err)
	}
	end, err := parseMonitorMetricTimestamp(endText)
	if err != nil {
		return MonitorMetricQuery{}, fmt.Errorf("invalid end: %w", err)
	}
	if !start.Before(end) {
		return MonitorMetricQuery{}, errors.New("start must be before end")
	}
	duration := end.Sub(start)
	if duration > maxMonitorMetricRange {
		return MonitorMetricQuery{}, fmt.Errorf("time range must not exceed %s", maxMonitorMetricRange)
	}

	step := defaultMonitorMetricStep(duration)
	if stepText != "" {
		step, err = parseMonitorMetricDuration(stepText)
		if err != nil {
			return MonitorMetricQuery{}, fmt.Errorf("invalid step: %w", err)
		}
	}
	if step < time.Second {
		return MonitorMetricQuery{}, errors.New("step must be at least 1s")
	}
	if math.Ceil(duration.Seconds()/step.Seconds())+1 > maxMonitorMetricSamples {
		return MonitorMetricQuery{}, fmt.Errorf("time range and step exceed the %d sample limit", maxMonitorMetricSamples)
	}

	query.Start = start
	query.End = end
	query.Step = step
	query.IsRange = true
	return query, nil
}

func GetMonitorMetrics(ctx context.Context, query MonitorMetricQuery) (MonitorMetricResponse, error) {
	address := strings.TrimSpace(conf.GetConfigString("prometheusAddress"))
	if address == "" {
		return MonitorMetricResponse{}, ErrPrometheusNotConfigured
	}
	timeout := defaultPrometheusQueryTimeout
	if configured := strings.TrimSpace(conf.GetConfigString("prometheusQueryTimeout")); configured != "" {
		parsed, err := parseMonitorMetricDuration(configured)
		if err != nil || parsed <= 0 {
			return MonitorMetricResponse{}, fmt.Errorf("invalid prometheusQueryTimeout %q", configured)
		}
		timeout = parsed
	}
	client, err := prometheusapi.NewClient(address, timeout)
	if err != nil {
		return MonitorMetricResponse{}, err
	}
	return queryMonitorMetrics(ctx, client, query)
}

func queryMonitorMetrics(ctx context.Context, client prometheusQuerier, query MonitorMetricQuery) (MonitorMetricResponse, error) {
	definitions, ok := monitorMetricDefinitions[query.Scope]
	if !ok {
		return MonitorMetricResponse{}, fmt.Errorf("unsupported scope %q", query.Scope)
	}
	definition, ok := definitions[query.Metric]
	if !ok {
		return MonitorMetricResponse{}, fmt.Errorf("metric %q is not supported for scope %q", query.Metric, query.Scope)
	}
	promQL := definition.buildQuery(query)

	var (
		promSeries []prometheusapi.Series
		err        error
	)
	if query.IsRange {
		promSeries, err = client.QueryRange(ctx, promQL, prometheusapi.Range{Start: query.Start, End: query.End, Step: query.Step})
	} else {
		promSeries, err = client.Query(ctx, promQL, time.Now().UTC())
	}
	if err != nil {
		return MonitorMetricResponse{}, err
	}

	response := MonitorMetricResponse{
		Scope:  query.Scope,
		Metric: query.Metric,
		Unit:   definition.unit,
		Series: []MonitorMetricSeries{},
	}
	if query.IsRange {
		response.Start = query.Start.UTC().Format(time.RFC3339Nano)
		response.End = query.End.UTC().Format(time.RFC3339Nano)
		response.Step = query.Step.Seconds()
	}
	for _, series := range promSeries {
		if len(series.Samples) == 0 {
			continue
		}
		labels := series.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		metricSeries := MonitorMetricSeries{
			Metric:     query.Metric,
			Object:     monitorMetricObjectName(query.Scope, labels),
			Labels:     labels,
			Timestamps: make([]float64, 0, len(series.Samples)),
			Values:     make([]float64, 0, len(series.Samples)),
		}
		for _, sample := range series.Samples {
			metricSeries.Timestamps = append(metricSeries.Timestamps, sample.Timestamp)
			metricSeries.Values = append(metricSeries.Values, sample.Value)
		}
		response.Series = append(response.Series, metricSeries)
	}
	return response, nil
}

func monitorMetricObjectName(scope string, labels map[string]string) string {
	switch scope {
	case "cluster":
		return "cluster"
	case "node":
		for _, key := range []string{"node", "nodename", "instance"} {
			if labels[key] != "" {
				return labels[key]
			}
		}
	case "pod":
		return monitorNamespacedMetricObject(labels["namespace"], labels["pod"])
	case "pvc":
		return monitorNamespacedMetricObject(labels["namespace"], labels["persistentvolumeclaim"])
	}
	return scope
}

func monitorNamespacedMetricObject(namespace, name string) string {
	if namespace == "" {
		return name
	}
	if name == "" {
		return namespace
	}
	return namespace + "/" + name
}

func monitorMetricRateWindow(query MonitorMetricQuery) string {
	window := 5 * time.Minute
	if query.IsRange && query.Step*4 > window {
		window = query.Step * 4
	}
	return formatPrometheusDuration(window)
}

func nodeInstanceMatchers(name string) []string {
	if name == "" {
		return nil
	}
	pattern := "^" + regexp.QuoteMeta(name) + "(:[0-9]+)?$"
	return []string{"instance=~" + strconv.Quote(pattern)}
}

func nodeNetworkMatchers(name string) []string {
	matchers := []string{`device!~"^(lo|veth.*|docker.*|cni.*|flannel.*|cali.*)$"`}
	return append(matchers, nodeInstanceMatchers(name)...)
}

func nodeFilesystemMatchers(name string) []string {
	matchers := []string{
		`fstype!~"^(tmpfs|devtmpfs|overlay|squashfs|nsfs|tracefs|proc|sysfs)$"`,
		`mountpoint!~"^/(run|var/lib/(docker|containerd|kubelet)/pods)($|/)"`,
	}
	return append(matchers, nodeInstanceMatchers(name)...)
}

func podContainerMatchers(query MonitorMetricQuery) []string {
	matchers := []string{`container!=""`, `container!="POD"`, `image!=""`}
	return append(matchers, namespacedMetricMatchers(query, "pod")...)
}

func podNetworkMatchers(query MonitorMetricQuery) []string {
	matchers := []string{`interface!="lo"`}
	return append(matchers, namespacedMetricMatchers(query, "pod")...)
}

func pvcMatchers(query MonitorMetricQuery) []string {
	return namespacedMetricMatchers(query, "persistentvolumeclaim")
}

func namespacedMetricMatchers(query MonitorMetricQuery, nameLabel string) []string {
	matchers := []string{}
	if query.Namespace != "" {
		matchers = append(matchers, "namespace="+strconv.Quote(query.Namespace))
	}
	if query.Name != "" {
		matchers = append(matchers, nameLabel+"="+strconv.Quote(query.Name))
	}
	return matchers
}

func promSelector(metric string, matchers []string) string {
	if len(matchers) == 0 {
		return metric
	}
	return metric + "{" + strings.Join(matchers, ",") + "}"
}

func parseMonitorMetricTimestamp(value string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), nil
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return time.Time{}, errors.New("expected RFC3339 or Unix seconds")
	}
	whole, fraction := math.Modf(seconds)
	return time.Unix(int64(whole), int64(fraction*float64(time.Second))).UTC(), nil
}

func parseMonitorMetricDuration(value string) (time.Duration, error) {
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed, nil
	}
	seconds, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return 0, errors.New("expected a positive duration such as 30s or seconds as a number")
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func defaultMonitorMetricStep(duration time.Duration) time.Duration {
	switch {
	case duration <= time.Hour:
		return 15 * time.Second
	case duration <= 6*time.Hour:
		return time.Minute
	case duration <= 24*time.Hour:
		return 5 * time.Minute
	case duration <= 7*24*time.Hour:
		return 30 * time.Minute
	default:
		return time.Hour
	}
}

func formatPrometheusDuration(value time.Duration) string {
	return strconv.FormatInt(int64(math.Ceil(value.Seconds())), 10) + "s"
}

func sortedMetricNames(definitions map[string]monitorMetricDefinition) []string {
	result := make([]string, 0, len(definitions))
	for metric := range definitions {
		result = append(result, metric)
	}
	sort.Strings(result)
	return result
}
