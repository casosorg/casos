package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beego/beego/logs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

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

	ollamaPort        = 11434
	aiBackendTTL      = 15 * time.Second
	aiTagsTimeout     = 5 * time.Second
	aiMaxRequestBytes = 64 << 20
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

var aiBackends struct {
	mu      sync.Mutex
	list    []*aiBackend
	fetched time.Time
}

// currentAIBackends returns the Ollama servers that answered, with the models
// each one has. force skips the cache, for a model pulled a moment ago.
func currentAIBackends(ctx context.Context, force bool) ([]*aiBackend, error) {
	aiBackends.mu.Lock()
	defer aiBackends.mu.Unlock()
	if !force && aiBackends.list != nil && time.Since(aiBackends.fetched) < aiBackendTTL {
		return aiBackends.list, nil
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		return nil, fmt.Errorf("the casos apiserver is not ready yet; try again in a moment")
	}
	found, err := findOllamaPods(cfg)
	if err != nil {
		return nil, err
	}

	previous := map[string]*aiBackend{}
	for _, backend := range aiBackends.list {
		previous[backend.key] = backend
	}
	var wg sync.WaitGroup
	for _, backend := range found {
		// Shared across refreshes, so open connections survive and a request
		// still in flight is counted where it ends.
		if old, ok := previous[backend.key]; ok {
			backend.transport = old.transport
			backend.inFlight = old.inFlight
			delete(previous, backend.key)
		} else {
			backend.transport = podPortTransport(cfg, backend.namespace, backend.pod, ollamaPort)
			backend.inFlight = &atomic.Int64{}
		}
		wg.Add(1)
		go func(backend *aiBackend) {
			defer wg.Done()
			models, err := fetchOllamaModels(ctx, backend)
			if err != nil {
				logs.Warning("ai gateway: list the models of %s/%s: %v", backend.namespace, backend.pod, err)
				return
			}
			backend.models = models
		}(backend)
	}
	wg.Wait()
	for _, gone := range previous {
		gone.transport.CloseIdleConnections()
	}

	aiBackends.list = found
	aiBackends.fetched = time.Now()
	return found, nil
}

// findOllamaPods finds Ollama by the port it listens on, so an instance of any
// template counts, not only casos's own Private AI.
func findOllamaPods(cfg *rest.Config) ([]*aiBackend, error) {
	services, err := object.GetServices(cfg, "")
	if err != nil {
		return nil, err
	}
	pods, err := object.GetPods(cfg, "")
	if err != nil {
		return nil, err
	}
	var result []*aiBackend
	seen := map[string]bool{}
	for _, svc := range services {
		if len(svc.Spec.Selector) == 0 || !servesOllama(svc) {
			continue
		}
		selector := labels.SelectorFromSet(svc.Spec.Selector)
		for _, pod := range pods {
			key := pod.Namespace + "/" + pod.Name
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
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return dialPodPort(cfg, namespace, pod, port)
		},
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
}

func dialPodPort(cfg *rest.Config, namespace, pod string, port int) (net.Conn, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	target := clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod).SubResource("portforward").URL()
	roundTripper, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return nil, err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: roundTripper}, http.MethodPost, target)
	conn, _, err := dialer.Dial(portforward.PortForwardProtocolV1Name)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set(corev1.StreamType, corev1.StreamTypeError)
	headers.Set(corev1.PortHeader, strconv.Itoa(port))
	headers.Set(corev1.PortForwardRequestIDHeader, strconv.FormatInt(podPortRequestID.Add(1), 10))
	errorStream, err := conn.CreateStream(headers)
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
	dataStream, err := conn.CreateStream(headers)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &podPortConn{Stream: dataStream, conn: conn}, nil
}

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
	if err != nil {
		writeAIError(w, http.StatusServiceUnavailable, "server_error", "", err.Error())
		return
	}
	backend := pickAIBackend(backends, model)
	if backend == nil {
		if backends, err = currentAIBackends(r.Context(), true); err == nil {
			backend = pickAIBackend(backends, model)
		}
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
	if err != nil {
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
	if err != nil {
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
