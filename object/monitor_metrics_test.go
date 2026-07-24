package object

import (
	"context"
	"strings"
	"testing"
	"time"

	prometheusapi "github.com/casosorg/casos/prometheus"
)

func TestParseMonitorMetricQueryValidation(t *testing.T) {
	tests := []struct {
		name   string
		params MonitorMetricQueryParams
		want   string
	}{
		{name: "scope required", params: MonitorMetricQueryParams{Metric: "cpu"}, want: "scope is required"},
		{name: "unsupported scope", params: MonitorMetricQueryParams{Scope: "deployment", Metric: "cpu"}, want: "unsupported scope"},
		{name: "metric required", params: MonitorMetricQueryParams{Scope: "cluster"}, want: "metric is required"},
		{name: "unsupported metric", params: MonitorMetricQueryParams{Scope: "node", Metric: "storage"}, want: "not supported"},
		{name: "cluster selector", params: MonitorMetricQueryParams{Scope: "cluster", Metric: "cpu", Name: "node-1"}, want: "not valid for cluster"},
		{name: "node namespace", params: MonitorMetricQueryParams{Scope: "node", Metric: "cpu", Namespace: "default"}, want: "not valid for node"},
		{name: "pod name without namespace", params: MonitorMetricQueryParams{Scope: "pod", Metric: "cpu", Name: "api"}, want: "namespace is required"},
		{name: "partial range", params: MonitorMetricQueryParams{Scope: "cluster", Metric: "cpu", Start: "2026-07-15T00:00:00Z"}, want: "provided together"},
		{name: "step without range", params: MonitorMetricQueryParams{Scope: "cluster", Metric: "cpu", Step: "1m"}, want: "step requires"},
		{name: "reverse range", params: MonitorMetricQueryParams{Scope: "cluster", Metric: "cpu", Start: "2026-07-15T01:00:00Z", End: "2026-07-15T00:00:00Z"}, want: "start must be before end"},
		{name: "small step", params: MonitorMetricQueryParams{Scope: "cluster", Metric: "cpu", Start: "2026-07-15T00:00:00Z", End: "2026-07-15T01:00:00Z", Step: "500ms"}, want: "at least 1s"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseMonitorMetricQuery(test.params)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestParseMonitorMetricQueryRange(t *testing.T) {
	query, err := ParseMonitorMetricQuery(MonitorMetricQueryParams{
		Scope:     " POD ",
		Metric:    " CPU ",
		Namespace: "default",
		Name:      "api",
		Start:     "2026-07-15T00:00:00Z",
		End:       "2026-07-15T06:00:00Z",
		Step:      "60",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !query.IsRange || query.Scope != "pod" || query.Metric != "cpu" {
		t.Fatalf("unexpected query: %#v", query)
	}
	if query.End.Sub(query.Start) != 6*time.Hour || query.Step != time.Minute {
		t.Fatalf("unexpected time range: %#v", query)
	}

	query, err = ParseMonitorMetricQuery(MonitorMetricQueryParams{
		Scope:  "cluster",
		Metric: "memory",
		Start:  "1700000000",
		End:    "1700003600",
	})
	if err != nil {
		t.Fatal(err)
	}
	if query.Step != 15*time.Second {
		t.Fatalf("default step = %s, want 15s", query.Step)
	}
}

func TestMonitorMetricDefinitions(t *testing.T) {
	for scope, definitions := range monitorMetricDefinitions {
		for metric, definition := range definitions {
			t.Run(scope+"/"+metric, func(t *testing.T) {
				query := MonitorMetricQuery{
					Scope: scope, Metric: metric, Namespace: "default", Name: "api",
					IsRange: true, Step: time.Minute,
				}
				if definition.unit == "" || definition.buildQuery(query) == "" {
					t.Fatalf("incomplete definition: %#v", definition)
				}
			})
		}
	}

	query := MonitorMetricQuery{Scope: "pod", Metric: "cpu", Namespace: "default", Name: `api"} or vector(1)`}
	promQL := monitorMetricDefinitions["pod"]["cpu"].buildQuery(query)
	if !strings.Contains(promQL, `pod="api\"} or vector(1)"`) {
		t.Fatalf("pod selector was not safely quoted: %s", promQL)
	}
}

type fakePrometheusQuerier struct {
	queryCalled      bool
	queryRangeCalled bool
	rangeValue       prometheusapi.Range
	series           []prometheusapi.Series
	err              error
}

func (f *fakePrometheusQuerier) Query(_ context.Context, _ string, _ time.Time) ([]prometheusapi.Series, error) {
	f.queryCalled = true
	return f.series, f.err
}

func (f *fakePrometheusQuerier) QueryRange(_ context.Context, _ string, queryRange prometheusapi.Range) ([]prometheusapi.Series, error) {
	f.queryRangeCalled = true
	f.rangeValue = queryRange
	return f.series, f.err
}

func TestQueryMonitorMetricsRangeResponse(t *testing.T) {
	start := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	client := &fakePrometheusQuerier{series: []prometheusapi.Series{{
		Labels: map[string]string{"namespace": "default", "pod": "api"},
		Samples: []prometheusapi.Sample{
			{Timestamp: 1_752_537_600, Value: 0.25},
			{Timestamp: 1_752_537_660, Value: 0.5},
		},
	}}}
	query := MonitorMetricQuery{
		Scope: "pod", Metric: "cpu", Namespace: "default", Name: "api",
		Start: start, End: start.Add(time.Hour), Step: time.Minute, IsRange: true,
	}

	response, err := queryMonitorMetrics(context.Background(), client, query)
	if err != nil {
		t.Fatal(err)
	}
	if !client.queryRangeCalled || client.queryCalled || client.rangeValue.Step != time.Minute {
		t.Fatalf("unexpected client calls: %#v", client)
	}
	if response.Unit != "cores" || response.Step != 60 || len(response.Series) != 1 {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Series[0].Object != "default/api" || len(response.Series[0].Values) != 2 {
		t.Fatalf("unexpected series: %#v", response.Series[0])
	}
}

func TestQueryMonitorMetricsEmptyResponse(t *testing.T) {
	client := &fakePrometheusQuerier{series: []prometheusapi.Series{}}
	response, err := queryMonitorMetrics(context.Background(), client, MonitorMetricQuery{Scope: "cluster", Metric: "cpu"})
	if err != nil {
		t.Fatal(err)
	}
	if !client.queryCalled || response.Series == nil || len(response.Series) != 0 {
		t.Fatalf("unexpected response: %#v", response)
	}
}
