package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func aiTestBackend(t *testing.T, key string, handler http.HandlerFunc) *aiBackend {
	t.Helper()
	server := httptest.NewServer(handler)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	t.Cleanup(server.Close)
	t.Cleanup(transport.CloseIdleConnections)
	return &aiBackend{key: key, transport: transport, inFlight: &atomic.Int64{}, models: []ollamaModel{{Name: "old:latest"}}}
}

func TestAICacheRefreshDoesNotBlockReaders(t *testing.T) {
	old := aiTestBackend(t, "old", nil)
	cache := aiBackendCache{list: []*aiBackend{old}, fetched: time.Now()}
	started, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := cache.current(ctx, true, func(ctx context.Context, _ []*aiBackend) ([]*aiBackend, error) {
			close(started)
			select {
			case <-release:
				return []*aiBackend{old}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})
		finished <- err
	}()
	<-started
	readerCtx, stopReader := context.WithTimeout(context.Background(), time.Second)
	defer stopReader()
	list, err := cache.current(readerCtx, false, nil)
	if err != nil || len(list) != 1 || list[0] != old {
		t.Fatalf("cached read blocked or changed: %v, %v", list, err)
	}
	waitCtx, stopWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stopWait()
	if _, err := cache.current(waitCtx, true, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled waiter: %v", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestAICacheCancellationPreservesSnapshot(t *testing.T) {
	old := aiTestBackend(t, "old", nil)
	fetched := time.Now().Add(-aiBackendTTL)
	cache := aiBackendCache{list: []*aiBackend{old}, fetched: fetched}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := cache.current(ctx, true, func(context.Context, []*aiBackend) ([]*aiBackend, error) {
		cancel()
		return []*aiBackend{}, nil
	})
	if !errors.Is(err, context.Canceled) || cache.list[0] != old || cache.fetched != fetched {
		t.Fatalf("cancellation changed cache: %v", err)
	}
	failure := errors.New("apiserver unavailable")
	_, err = cache.current(context.Background(), true, func(context.Context, []*aiBackend) ([]*aiBackend, error) {
		return nil, failure
	})
	if !errors.Is(err, failure) || cache.list[0] != old || cache.fetched != fetched {
		t.Fatalf("failed refresh changed cache: %v", err)
	}
}

func TestAIProbePartialFailure(t *testing.T) {
	good := aiTestBackend(t, "good", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path = %s", r.URL.Path)
		}
		io.WriteString(w, `{"models":[{"name":"new:latest"}]}`)
	})
	bad := aiTestBackend(t, "bad", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	found := []*aiBackend{{key: good.key}, {key: bad.key}}
	list, err := probeAIBackends(context.Background(), nil, found, []*aiBackend{good, bad})
	if err == nil || len(list) != 1 || pickAIBackend(list, "new:latest") == nil {
		t.Fatalf("healthy backend lost or failure hidden: %v, %v", list, err)
	}
	if !good.has("old:latest") || good.has("new:latest") || list[0] == good || list[0].inFlight != good.inFlight {
		t.Fatal("refresh mutated the old snapshot or lost its in-flight counter")
	}
	cache := aiBackendCache{}
	_, _ = cache.current(context.Background(), true, func(context.Context, []*aiBackend) ([]*aiBackend, error) { return list, err })
	cached, cachedErr := cache.current(context.Background(), false, nil)
	if cachedErr == nil || pickAIBackend(cached, "new:latest") == nil {
		t.Fatal("cached partial result lost its failure or healthy backend")
	}
	list, err = probeAIBackends(context.Background(), nil, []*aiBackend{{key: bad.key}}, []*aiBackend{bad})
	if list != nil || err == nil {
		t.Fatalf("total failure: %v, %v", list, err)
	}
}

func TestAIProbeBoundsConcurrencyAndCancels(t *testing.T) {
	var active, maximum atomic.Int64
	started := make(chan struct{}, aiDiscoveryConcurrency*2)
	backend := aiTestBackend(t, "shared", func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		<-r.Context().Done()
	})
	var previous, found []*aiBackend
	for i := 0; i < aiDiscoveryConcurrency*2; i++ {
		key := fmt.Sprint(i)
		previous = append(previous, &aiBackend{key: key, transport: backend.transport, inFlight: backend.inFlight})
		found = append(found, &aiBackend{key: key})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := probeAIBackends(ctx, nil, found, previous); finished <- err }()
	for i := 0; i < aiDiscoveryConcurrency; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("probes did not start")
		}
	}
	if maximum.Load() > aiDiscoveryConcurrency {
		t.Fatalf("concurrent probes: %d", maximum.Load())
	}
	cancel()
	if err := awaitAI(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestAIDiscoveryCancelsKubernetesList(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := findOllamaPods(ctx, &rest.Config{Host: server.URL}); finished <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("list request did not start")
	}
	cancel()
	if err := awaitAI(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("list cancellation: %v", err)
	}
}

func TestAICacheClosesAbandonedTransport(t *testing.T) {
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"models":[]}`)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	server.Start()
	defer server.Close()
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	backend := &aiBackend{transport: transport}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := aiBackendCache{}
	_, err := cache.current(ctx, true, func(ctx context.Context, _ []*aiBackend) ([]*aiBackend, error) {
		if _, err := fetchOllamaModels(ctx, backend); err != nil {
			return nil, err
		}
		cancel()
		return []*aiBackend{backend}, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled refresh: %v", err)
	}
	awaitAI(t, closed)
}

func TestAIDiscoveryDistinguishesReplacementPod(t *testing.T) {
	var uid atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/services":
			io.WriteString(w, `{"apiVersion":"v1","kind":"ServiceList","items":[{"metadata":{"name":"ollama","namespace":"ai"},"spec":{"selector":{"app":"ollama"},"ports":[{"port":11434}]}}]}`)
		case "/api/v1/pods":
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":"ollama-0","namespace":"ai","uid":"%d","labels":{"app":"ollama"}},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}]}`, uid.Add(1))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := &rest.Config{Host: server.URL}
	first, err := findOllamaPods(context.Background(), cfg)
	if err != nil || len(first) != 1 {
		t.Fatalf("first discovery: %v, %v", first, err)
	}
	second, err := findOllamaPods(context.Background(), cfg)
	if err != nil || len(second) != 1 {
		t.Fatalf("second discovery: %v, %v", second, err)
	}
	if first[0].key == second[0].key || first[0].pod != second[0].pod {
		t.Fatal("same-name replacement reused the old backend identity")
	}
}

func TestAIModelsPartialDiscovery(t *testing.T) {
	previous := aiBackends
	defer func() { aiBackends = previous }()
	backend := aiTestBackend(t, "good", nil)
	aiBackends = &aiBackendCache{
		list: []*aiBackend{backend}, fetched: time.Now(), err: errors.New("another pod is unavailable"),
	}
	for _, tc := range []struct {
		id     string
		status int
	}{
		{"", http.StatusOK}, {"old", http.StatusOK}, {"missing", http.StatusServiceUnavailable},
	} {
		recorder := httptest.NewRecorder()
		serveAIModels(recorder, httptest.NewRequest(http.MethodGet, "/v1/models/"+tc.id, nil), tc.id)
		if recorder.Code != tc.status {
			t.Errorf("model %q: status %d, want %d", tc.id, recorder.Code, tc.status)
		}
	}
	aiBackends.err = nil
	recorder := httptest.NewRecorder()
	serveAIModels(recorder, httptest.NewRequest(http.MethodGet, "/v1/models/missing", nil), "missing")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("confirmed missing model: status %d", recorder.Code)
	}
}

func awaitAI[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(time.Second):
		t.Fatal("asynchronous operation did not finish")
		return *new(T)
	}
}
