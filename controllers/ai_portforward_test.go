package controllers

import (
	"context"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	httpstreamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
)

func TestAIPortForwardAbandonedHandshake(t *testing.T) {
	started, closed := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(closed)
	}))
	defer func() { server.CloseClientConnections(); server.Close() }()
	transport := podPortTransport(&rest.Config{Host: server.URL, Timeout: 100 * time.Millisecond}, "ai", "ollama-0", ollamaPort)
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://ollama/api/tags", nil)
	result := make(chan error, 1)
	go func() { _, err := transport.RoundTrip(req); result <- err }()
	awaitAI(t, started)
	cancel()
	if err := awaitAI(t, result); err == nil {
		t.Fatal("canceled discovery succeeded")
	}
	awaitAI(t, closed)
}

func TestAIPortForwardStreamLifecycle(t *testing.T) {
	for _, stall := range []bool{false, true} {
		name := "established"
		if stall {
			name = "waiting-for-stream"
		}
		t.Run(name, func(t *testing.T) {
			started, release, stop, closed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/prefix/api/v1/namespaces/ai/pods/ollama-0/portforward" || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("incorrect port-forward request: %s %s", r.Method, r.URL.Path)
				}
				if _, err := httpstream.Handshake(r, w, []string{portforward.PortForwardProtocolV1Name}); err != nil {
					return
				}
				conn := httpstreamspdy.NewResponseUpgrader().UpgradeResponse(w, r, func(stream httpstream.Stream, ready <-chan struct{}) error {
					if stall {
						close(started)
						<-release
						return nil
					}
					go func() {
						<-ready
						defer stream.Close()
						if stream.Headers().Get(corev1.StreamType) == corev1.StreamTypeData {
							io.Copy(stream, stream)
						}
					}()
					return nil
				})
				if conn == nil {
					return
				}
				defer conn.Close()
				select {
				case <-stop:
				case <-conn.CloseChan():
				}
				close(closed)
			}))
			server.StartTLS()
			defer func() { close(release); close(stop); server.CloseClientConnections(); server.Close() }()
			cfg := &rest.Config{Host: server.URL + "/prefix", BearerToken: "test-token", Timeout: 200 * time.Millisecond,
				TLSClientConfig: rest.TLSClientConfig{CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})}}
			var usedProxy bool
			cfg.Proxy = func(*http.Request) (*url.URL, error) { usedProxy = true; return nil, nil }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if stall {
				result := make(chan error, 1)
				go func() {
					conn, err := dialPodPort(ctx, cfg, "ai", "ollama-0", ollamaPort)
					if conn != nil {
						conn.Close()
					}
					result <- err
				}()
				awaitAI(t, started)
				cancel()
				if err := awaitAI(t, result); err == nil {
					t.Fatal("canceled stream setup succeeded")
				}
			} else {
				conn, err := dialPodPort(ctx, cfg, "ai", "ollama-0", ollamaPort)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				cancel()
				time.Sleep(250 * time.Millisecond)
				if _, err := io.WriteString(conn, "ping"); err != nil {
					t.Fatal(err)
				}
				buf := make([]byte, 4)
				if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
					t.Fatalf("established stream: %q, %v", buf, err)
				}
				conn.Close()
			}
			if !usedProxy {
				t.Error("configured proxy was not consulted")
			}
			awaitAI(t, closed)
		})
	}
}
