package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beego/beego/logs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
	httpstreamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"

	"github.com/casosorg/casos/deploy"
	"github.com/casosorg/casos/object"
)

// The model API.
//
// casos answers OpenAI's /v1 API on behalf of the Ollama servers in the
// cluster, so an editor, an SDK or another app on the network can use the
// models on the home GPUs with a casos access token. A request goes to a
// server that has the model it names, the one with the fewest requests in
// flight when several do, which spreads the load over every GPU machine that
// runs a copy. Servers are reached by port-forwarding through the API server,
// because pod and service addresses are not routable from where casos runs,
// such as a Windows host in front of WSL.

const (
	AIGatewayPrefix = "/v1/"

	ollamaPort             = 11434
	aiBackendTTL           = 15 * time.Second
	aiTagsTimeout          = 5 * time.Second
	aiDiscoveryTimeout     = 10 * time.Second
	aiPortConnectTimeout   = 10 * time.Second
	aiDiscoveryConcurrency = 8
	aiMaxRequestBytes      = 64 << 20
)

type ollamaModel struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Details    struct {
		ParameterSize     string `json:"parameter_size"`
		QuantizationLevel string `json:"quantization_level"`
	} `json:"details"`
}

type aiBackend struct {
	key       string
	namespace string
	service   string
	pod       string
	node      string
	models    []ollamaModel
	inFlight  *atomic.Int64
	transport *http.Transport
}

func (b *aiBackend) has(model string) bool {
	for _, m := range b.models {
		if m.Name == model {
			return true
		}
	}
	return false
}

type aiBackendRefresh struct {
	done    chan struct{}
	list    []*aiBackend
	err     error
	aborted bool
}

type aiBackendCache struct {
	mu      sync.Mutex
	list    []*aiBackend
	err     error
	fetched time.Time
	refresh *aiBackendRefresh
}

var aiBackends = &aiBackendCache{}

// Partial discovery returns usable backends together with its error.
func currentAIBackends(ctx context.Context, force bool) ([]*aiBackend, error) {
	return aiBackends.current(ctx, force, discoverAIBackends)
}

func (cache *aiBackendCache) current(ctx context.Context, force bool, discover func(context.Context, []*aiBackend) ([]*aiBackend, error)) ([]*aiBackend, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cache.mu.Lock()
		if !force && cache.list != nil && time.Since(cache.fetched) < aiBackendTTL {
			list, err := cache.list, cache.err
			cache.mu.Unlock()
			return list, err
		}
		if refresh := cache.refresh; refresh != nil {
			cache.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-refresh.done:
				if refresh.aborted {
					continue
				}
				return refresh.list, refresh.err
			}
		}
		refresh := &aiBackendRefresh{done: make(chan struct{})}
		cache.refresh = refresh
		previous := cache.list
		cache.mu.Unlock()

		refreshCtx, cancel := context.WithTimeout(ctx, aiDiscoveryTimeout)
		list, err := discover(refreshCtx, previous)
		if refreshCtx.Err() != nil {
			err = refreshCtx.Err()
			closeNewAITransports(list, previous)
			list = nil
		}
		cancel()

		cache.mu.Lock()
		if list != nil {
			keep := map[*http.Transport]bool{}
			for _, backend := range list {
				keep[backend.transport] = true
			}
			for _, backend := range previous {
				if !keep[backend.transport] {
					backend.transport.CloseIdleConnections()
				}
			}
			cache.list, cache.err, cache.fetched = list, err, time.Now()
		}
		refresh.list, refresh.err, refresh.aborted = list, err, ctx.Err() != nil
		cache.refresh = nil
		close(refresh.done)
		cache.mu.Unlock()
		return list, err
	}
}

func closeNewAITransports(list, previous []*aiBackend) {
	old := map[*http.Transport]bool{}
	for _, backend := range previous {
		old[backend.transport] = true
	}
	for _, backend := range list {
		if !old[backend.transport] {
			backend.transport.CloseIdleConnections()
		}
	}
}

func discoverAIBackends(ctx context.Context, previous []*aiBackend) ([]*aiBackend, error) {
	cfg := getAdminRestConfig()
	if cfg == nil {
		return nil, fmt.Errorf("the casos apiserver is not ready yet; try again in a moment")
	}
	found, err := findOllamaPods(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return probeAIBackends(ctx, cfg, found, previous)
}

func probeAIBackends(ctx context.Context, cfg *rest.Config, found, previous []*aiBackend) ([]*aiBackend, error) {
	old := map[string]*aiBackend{}
	for _, backend := range previous {
		old[backend.key] = backend
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, aiDiscoveryConcurrency)
	probeErrors := make([]error, len(found))
	for i, backend := range found {
		if prior := old[backend.key]; prior != nil {
			backend.transport, backend.inFlight = prior.transport, prior.inFlight
		} else {
			backend.transport = podPortTransport(cfg, backend.namespace, backend.pod, ollamaPort)
			backend.inFlight = &atomic.Int64{}
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			closeNewAITransports(found[:i+1], previous)
			return nil, ctx.Err()
		}
		wg.Add(1)
		go func(i int, backend *aiBackend) {
			defer wg.Done()
			defer func() { <-slots }()
			models, err := fetchOllamaModels(ctx, backend)
			if err != nil {
				probeErrors[i] = fmt.Errorf("cannot discover models on %s: %w", backend.key, err)
				return
			}
			backend.models = models
		}(i, backend)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		closeNewAITransports(found, previous)
		return nil, err
	}
	healthy := make([]*aiBackend, 0, len(found))
	for i, backend := range found {
		if probeErrors[i] == nil {
			healthy = append(healthy, backend)
		} else if old[backend.key] == nil {
			backend.transport.CloseIdleConnections()
		}
	}
	err := errors.Join(probeErrors...)
	if len(found) > 0 && len(healthy) == 0 {
		return nil, err
	}
	return healthy, err
}

// findOllamaPods finds Ollama by the port it listens on, so an instance of any
// template counts, not only casos's own Private AI.
func findOllamaPods(ctx context.Context, cfg *rest.Config) ([]*aiBackend, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	services, err := client.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	var result []*aiBackend
	seen := map[string]bool{}
	for _, svc := range services.Items {
		if len(svc.Spec.Selector) == 0 || !servesOllama(svc) {
			continue
		}
		selector := labels.SelectorFromSet(svc.Spec.Selector)
		for _, pod := range pods.Items {
			key := pod.Namespace + "/" + pod.Name + "/" + string(pod.UID)
			if seen[key] || pod.Namespace != svc.Namespace || !selector.Matches(labels.Set(pod.Labels)) || pod.DeletionTimestamp != nil || !podIsReady(pod) {
				continue
			}
			seen[key] = true
			result = append(result, &aiBackend{key: key, namespace: pod.Namespace, service: svc.Name, pod: pod.Name, node: pod.Spec.NodeName})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].key < result[j].key })
	return result, nil
}

func servesOllama(svc corev1.Service) bool {
	for _, port := range svc.Spec.Ports {
		if port.Port == ollamaPort || port.TargetPort.IntValue() == ollamaPort {
			return true
		}
	}
	return false
}

func fetchOllamaModels(ctx context.Context, backend *aiBackend) ([]ollamaModel, error) {
	ctx, cancel := context.WithTimeout(ctx, aiTagsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://ollama/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := backend.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}
	var tags struct {
		Models []ollamaModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	return tags.Models, nil
}

var podPortRequestID atomic.Int64

// podPortTransport makes each HTTP connection its own port-forward to the
// pod, so keep-alive reuses a forward instead of opening one per request.
func podPortTransport(cfg *rest.Config, namespace, pod string, port int) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialPodPort(ctx, cfg, namespace, pod, port)
		},
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
}

func dialPodPort(ctx context.Context, cfg *rest.Config, namespace, pod string, port int) (net.Conn, error) {
	timeout := aiPortConnectTimeout
	if cfg.Timeout > 0 && cfg.Timeout < timeout {
		timeout = cfg.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	target := clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod).SubResource("portforward").URL()
	tlsConfig, err := rest.TLSConfigFor(cfg)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		tlsConfig = tlsConfig.Clone()
		tlsConfig.NextProtos = []string{"http/1.1"}
	}
	dial := cfg.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}).DialContext
	}
	transport := &http.Transport{
		Proxy: cfg.Proxy, TLSClientConfig: tlsConfig,
		TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return dial(ctx, network, address)
		},
	}
	if transport.Proxy == nil {
		transport.Proxy = http.ProxyFromEnvironment
	}
	defer transport.CloseIdleConnections()
	roundTripper, err := rest.HTTPWrappersForConfig(cfg, transport)
	if err != nil {
		return nil, err
	}
	var raw net.Conn
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { raw = info.Conn },
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(httpstream.HeaderConnection, httpstream.HeaderUpgrade)
	req.Header.Set(httpstream.HeaderUpgrade, httpstreamspdy.HeaderSpdy31)
	req.Header.Set(httpstream.HeaderProtocolVersion, portforward.PortForwardProtocolV1Name)
	// HTTP owns cancellation until the 101 response transfers the connection to SPDY.
	resp, err := roundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, ok := resp.Body.(io.ReadWriteCloser)
	if !ok || raw == nil || resp.StatusCode != http.StatusSwitchingProtocols ||
		!strings.Contains(strings.ToLower(resp.Header.Get(httpstream.HeaderConnection)), "upgrade") ||
		!strings.Contains(strings.ToLower(resp.Header.Get(httpstream.HeaderUpgrade)), strings.ToLower(httpstreamspdy.HeaderSpdy31)) {
		resp.Body.Close()
		return nil, fmt.Errorf("cannot upgrade port-forward: status %s", resp.Status)
	}
	conn, err := httpstreamspdy.NewClientConnectionWithPings(&podPortUpgradeConn{Conn: raw, body: body}, 5*time.Second)
	if err != nil {
		body.Close()
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { raw.Close(); conn.Close() })
	defer stop()

	headers := http.Header{}
	headers.Set(corev1.StreamType, corev1.StreamTypeError)
	headers.Set(corev1.PortHeader, strconv.Itoa(port))
	headers.Set(corev1.PortForwardRequestIDHeader, strconv.FormatInt(podPortRequestID.Add(1), 10))
	errorStream, err := createPodPortStream(ctx, conn, headers)
	if err != nil {
		conn.Close()
		return nil, err
	}
	errorStream.Close()
	go func() {
		// The kubelet reports here when it cannot reach the port, and the data
		// stream would otherwise just hang.
		if message, err := io.ReadAll(errorStream); err == nil && len(message) > 0 {
			logs.Warning("ai gateway: forward to %s/%s:%d: %s", namespace, pod, port, message)
			conn.Close()
		}
	}()

	headers.Set(corev1.StreamType, corev1.StreamTypeData)
	dataStream, err := createPodPortStream(ctx, conn, headers)
	if err != nil {
		conn.Close()
		return nil, err
	}
	stop()
	if err := ctx.Err(); err != nil {
		conn.Close()
		return nil, err
	}
	return &podPortConn{Stream: dataStream, conn: conn}, nil
}

func createPodPortStream(ctx context.Context, conn httpstream.Connection, headers http.Header) (httpstream.Stream, error) {
	type result struct {
		stream httpstream.Stream
		err    error
	}
	// CreateStream has a fixed 30s acknowledgement wait, even after the socket closes.
	done := make(chan result, 1)
	go func() {
		stream, err := conn.CreateStream(headers)
		if ctx.Err() != nil && stream != nil {
			stream.Reset()
		}
		done <- result{stream, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		return result.stream, result.err
	}
}

type podPortUpgradeConn struct {
	net.Conn
	body io.ReadWriteCloser
}

func (c *podPortUpgradeConn) Read(p []byte) (int, error)  { return c.body.Read(p) }
func (c *podPortUpgradeConn) Write(p []byte) (int, error) { return c.body.Write(p) }
func (c *podPortUpgradeConn) Close() error                { return c.body.Close() }

type podPortConn struct {
	httpstream.Stream
	conn httpstream.Connection
}

func (c *podPortConn) Close() error                     { return c.conn.Close() }
func (c *podPortConn) LocalAddr() net.Addr              { return podPortAddr{} }
func (c *podPortConn) RemoteAddr() net.Addr             { return podPortAddr{} }
func (c *podPortConn) SetDeadline(time.Time) error      { return nil }
func (c *podPortConn) SetReadDeadline(time.Time) error  { return nil }
func (c *podPortConn) SetWriteDeadline(time.Time) error { return nil }

type podPortAddr struct{}

func (podPortAddr) Network() string { return "portforward" }
func (podPortAddr) String() string  { return "portforward" }

// Ollama calls a bare model name by its latest tag.
func ollamaModelName(model string) string {
	model = strings.TrimSpace(model)
	if model != "" && !strings.Contains(model, ":") {
		return model + ":latest"
	}
	return model
}

func pickAIBackend(backends []*aiBackend, model string) *aiBackend {
	var best *aiBackend
	for _, backend := range backends {
		if backend.has(model) && (best == nil || backend.inFlight.Load() < best.inFlight.Load()) {
			best = backend
		}
	}
	return best
}

func aiModelNames(backends []*aiBackend) []string {
	seen := map[string]bool{}
	var names []string
	for _, backend := range backends {
		for _, model := range backend.models {
			if !seen[model.Name] {
				seen[model.Name] = true
				names = append(names, model.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// writeAIError answers in OpenAI's error shape, which is what SDKs parse.
func writeAIError(w http.ResponseWriter, status int, kind, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{"message": message, "type": kind, "code": code},
	})
}

// ServeAIGateway answers one /v1 request authenticated by a casos access token.
func ServeAIGateway(w http.ResponseWriter, r *http.Request) {
	// Credentials travel in the Authorization header, never in cookies, so any
	// origin may call; a browser-based chat client needs that.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	caller, ok := authenticateAccessToken(r)
	if !ok {
		writeAIError(w, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key",
			"a casos access token is required as the API key; create one on the AI Agents page")
		return
	}

	if caller.readOnly {
		writeAIError(w, http.StatusForbidden, "invalid_request_error", "permission_denied",
			"read-only access tokens cannot use the model gateway; use a full-access token")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, AIGatewayPrefix)
	if r.Method == http.MethodGet && (path == "models" || strings.HasPrefix(path, "models/")) {
		serveAIModels(w, r, strings.TrimPrefix(strings.TrimPrefix(path, "models"), "/"))
		return
	}
	if r.Method != http.MethodPost {
		writeAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "use POST")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, aiMaxRequestBytes+1))
	if err != nil {
		writeAIError(w, http.StatusBadRequest, "invalid_request_error", "", err.Error())
		return
	}
	if len(body) > aiMaxRequestBytes {
		writeAIError(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "", "the request body is too large")
		return
	}
	var request struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &request); err != nil || strings.TrimSpace(request.Model) == "" {
		writeAIError(w, http.StatusBadRequest, "invalid_request_error", "", "the request body must be JSON naming a model")
		return
	}

	model := ollamaModelName(request.Model)
	backends, err := currentAIBackends(r.Context(), false)
	backend := pickAIBackend(backends, model)
	if backend == nil && backends != nil {
		backends, err = currentAIBackends(r.Context(), true)
		backend = pickAIBackend(backends, model)
	}
	if backend == nil && err != nil {
		writeAIError(w, http.StatusServiceUnavailable, "server_error", "", err.Error())
		return
	}
	if backend == nil {
		message := fmt.Sprintf("the model %s is not available", request.Model)
		if names := aiModelNames(backends); len(names) > 0 {
			message += "; available models: " + strings.Join(names, ", ")
		} else {
			message += "; no Ollama server is running yet, so install Private AI from the dashboard"
		}
		writeAIError(w, http.StatusNotFound, "invalid_request_error", "model_not_found", message)
		return
	}

	backend.inFlight.Add(1)
	defer backend.inFlight.Add(-1)
	started := time.Now()
	recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = "ollama"
			pr.Out.Host = "ollama"
			pr.Out.Header.Del("Authorization")
		},
		Transport: backend.transport,
		// Replies stream token by token.
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeAIError(w, http.StatusBadGateway, "server_error", "", fmt.Sprintf("the Ollama server %s/%s did not answer: %v", backend.namespace, backend.service, err))
		},
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	proxy.ServeHTTP(recorder, r)
	logs.Info("ai gateway: %s (token %s) %s %s on %s/%s: %d in %s", caller.user, caller.token, r.URL.Path, model,
		backend.node, backend.pod, recorder.status, time.Since(started).Round(time.Millisecond))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

// Unwrap lets the reverse proxy flush each streamed chunk through the recorder.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

type openAIModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func serveAIModels(w http.ResponseWriter, r *http.Request, id string) {
	backends, err := currentAIBackends(r.Context(), false)
	if err != nil && len(backends) == 0 {
		writeAIError(w, http.StatusServiceUnavailable, "server_error", "", err.Error())
		return
	}
	models := []openAIModel{}
	seen := map[string]bool{}
	for _, backend := range backends {
		for _, model := range backend.models {
			if seen[model.Name] {
				continue
			}
			seen[model.Name] = true
			models = append(models, openAIModel{ID: model.Name, Object: "model", Created: model.ModifiedAt.Unix(), OwnedBy: "casos"})
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })

	w.Header().Set("Content-Type", "application/json")
	if id == "" {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"object": "list", "data": models})
		return
	}
	name := ollamaModelName(id)
	for _, model := range models {
		if model.ID == name {
			_ = json.NewEncoder(w).Encode(model)
			return
		}
	}
	if err != nil {
		writeAIError(w, http.StatusServiceUnavailable, "server_error", "", err.Error())
		return
	}
	writeAIError(w, http.StatusNotFound, "invalid_request_error", "model_not_found", fmt.Sprintf("the model %s is not available", id))
}

type aiModelSummary struct {
	Name          string   `json:"name"`
	Size          int64    `json:"size"`
	ParameterSize string   `json:"parameterSize"`
	Quantization  string   `json:"quantization"`
	Machines      []string `json:"machines"`
	Gpus          []string `json:"gpus"`
}

// GetAiModels lists the models the /v1 API serves, and the machines each runs on.
// @router /api/get-ai-models [get]
func (c *ApiController) GetAiModels() {
	if c.RequireSignedIn() {
		return
	}
	backends, err := currentAIBackends(c.Ctx.Request.Context(), true)
	if err != nil && len(backends) == 0 {
		c.ResponseError(err.Error())
		return
	}
	gpus := map[string]string{}
	if nodes, err := object.GetNodes(getAdminRestConfig()); err == nil {
		for _, node := range nodes {
			if node.Labels[deploy.NvidiaGPUPresentLabel] == "true" {
				gpus[node.Name] = strings.ReplaceAll(node.Labels["nvidia.com/gpu.product"], "-", " ")
			}
		}
	}

	byName := map[string]*aiModelSummary{}
	var result []*aiModelSummary
	for _, backend := range backends {
		for _, model := range backend.models {
			summary := byName[model.Name]
			if summary == nil {
				summary = &aiModelSummary{
					Name:          model.Name,
					Size:          model.Size,
					ParameterSize: model.Details.ParameterSize,
					Quantization:  model.Details.QuantizationLevel,
					Machines:      []string{},
					Gpus:          []string{},
				}
				byName[model.Name] = summary
				result = append(result, summary)
			}
			summary.Machines = append(summary.Machines, backend.node)
			if gpu := gpus[backend.node]; gpu != "" {
				summary.Gpus = append(summary.Gpus, gpu)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	c.ResponseOk(result, len(backends))
}
