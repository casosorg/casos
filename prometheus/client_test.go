package prometheus

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientQuery(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prometheus/api/v1/query" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "up" {
			t.Fatalf("query = %q", got)
		}
		if got := r.URL.Query().Get("time"); got != "1700000000" {
			t.Fatalf("time = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"__name__":"up","instance":"node-1"},"value":[1700000000,"1"]}]}}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/prometheus/", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	series, err := client.Query(context.Background(), "up", at)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].Labels["instance"] != "node-1" {
		t.Fatalf("unexpected series: %#v", series)
	}
	if len(series[0].Samples) != 1 || series[0].Samples[0].Value != 1 {
		t.Fatalf("unexpected samples: %#v", series[0].Samples)
	}
}

func TestClientQueryRange(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	end := start.Add(2 * time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		values := r.URL.Query()
		if values.Get("start") != "1700000000" || values.Get("end") != "1700000120" || values.Get("step") != "30" {
			t.Fatalf("unexpected range query: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"instance":"node-1"},"values":[[1700000000,"12.5"],[1700000030,"13.25"]]}]}}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	series, err := client.QueryRange(context.Background(), "node_metric", Range{Start: start, End: end, Step: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || len(series[0].Samples) != 2 || series[0].Samples[1].Value != 13.25 {
		t.Fatalf("unexpected result: %#v", series)
	}
}

func TestClientQueryErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"status":"error","errorType":"bad_data","error":"invalid parameter query"}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Query(context.Background(), "broken", time.Time{})
	if !errors.Is(err, ErrQuery) {
		t.Fatalf("error = %v, want ErrQuery", err)
	}
}

func TestClientEmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	series, err := client.QueryRange(context.Background(), "missing_metric", Range{
		Start: time.Now().Add(-time.Hour),
		End:   time.Now(),
		Step:  time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 0 {
		t.Fatalf("series = %#v, want empty", series)
	}
}

func TestClientQueryTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(200 * time.Millisecond):
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Query(context.Background(), "up", time.Time{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}
}
