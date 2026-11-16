package controllers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/casosorg/casos/proxy"
)

type aiProvider struct {
	Name            string   `json:"name"`
	BaseURL         string   `json:"baseUrl"`
	Models          []string `json:"models"`
	ResourceVersion string   `json:"resourceVersion,omitempty"`
	HasAPIKey       bool     `json:"hasApiKey"`
	apiKey          string
}

func aiProviderOwner(owner string) string {
	sum := sha256.Sum256([]byte(owner))
	return hex.EncodeToString(sum[:20])
}

func aiProviderSecretName(owner, name string) string {
	return "casos-ai-" + aiProviderOwner(owner+"/"+name)
}

func aiProviderSecrets() (typedcorev1.SecretInterface, error) {
	cfg := getAdminRestConfig()
	if cfg == nil {
		return nil, fmt.Errorf("apiserver not ready")
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().Secrets(metav1.NamespaceSystem), nil
}

func getAIProviders(ctx context.Context, owner string) ([]aiProvider, error) {
	secrets, err := aiProviderSecrets()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, aiTagsTimeout)
	defer cancel()
	list, err := secrets.List(ctx, metav1.ListOptions{LabelSelector: "casos.io/ai-provider=true,casos.io/ai-owner=" + aiProviderOwner(owner)})
	if err != nil {
		return nil, err
	}
	providers := []aiProvider{}
	for _, secret := range list.Items {
		if secret.Annotations["casos.io/owner"] != owner {
			continue
		}
		var provider aiProvider
		if err := json.Unmarshal(secret.Data["provider"], &provider); err != nil {
			return nil, fmt.Errorf("invalid AI provider configuration")
		}
		provider.ResourceVersion = secret.ResourceVersion
		provider.apiKey = string(secret.Data["api-key"])
		provider.HasAPIKey = provider.apiKey != ""
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	return providers, nil
}

func (c *ApiController) GetAiProviders() {
	if c.RequireSignedIn() {
		return
	}
	providers, err := getAIProviders(c.Ctx.Request.Context(), c.GetSessionUser().Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(providers)
}

func (c *ApiController) SaveAiProvider() {
	if c.RequireSignedIn() {
		return
	}
	var req struct {
		aiProvider
		APIKey string `json:"apiKey"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid provider configuration")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	target, err := url.Parse(req.BaseURL)
	if len(validation.IsDNS1123Label(req.Name)) != 0 || err != nil || target.Hostname() == "" ||
		(target.Scheme != "http" && target.Scheme != "https") || target.User != nil || target.RawQuery != "" || target.Fragment != "" ||
		len(req.BaseURL) > 2048 || len(req.APIKey) > 4096 || strings.ContainsAny(req.APIKey, "\r\n") {
		c.ResponseError("use a lowercase provider name and an HTTP(S) base URL without credentials, query or fragment")
		return
	}
	models := []string{}
	seen := map[string]bool{}
	for _, model := range req.Models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 200 || strings.ContainsAny(model, "\r\n") {
			c.ResponseError("provide non-empty model IDs, one per line")
			return
		}
		if !seen[model] {
			models = append(models, model)
			seen[model] = true
		}
	}
	if len(models) == 0 || len(models) > 100 {
		c.ResponseError("provide between 1 and 100 model IDs")
		return
	}
	req.Models = models
	secrets, err := aiProviderSecrets()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	owner := c.GetSessionUser().Name
	ctx, cancel := context.WithTimeout(c.Ctx.Request.Context(), aiTagsTimeout)
	defer cancel()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            aiProviderSecretName(owner, req.Name),
			ResourceVersion: req.ResourceVersion,
			Labels:          map[string]string{"casos.io/ai-provider": "true", "casos.io/ai-owner": aiProviderOwner(owner)},
			Annotations:     map[string]string{"casos.io/owner": owner},
		},
		Type: corev1.SecretTypeOpaque,
	}
	if req.ResourceVersion != "" && req.APIKey == "" {
		previous, err := secrets.Get(ctx, secret.Name, metav1.GetOptions{})
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		req.APIKey = string(previous.Data["api-key"])
	}
	req.ResourceVersion = ""
	req.HasAPIKey = req.APIKey != ""
	data, err := json.Marshal(req.aiProvider)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	secret.Data = map[string][]byte{"provider": data, "api-key": []byte(req.APIKey)}
	if secret.ResourceVersion == "" {
		_, err = secrets.Create(ctx, secret, metav1.CreateOptions{})
	} else {
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	}
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

func (c *ApiController) DeleteAiProvider() {
	if c.RequireSignedIn() {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil || len(validation.IsDNS1123Label(req.Name)) != 0 {
		c.ResponseError("invalid provider name")
		return
	}
	secrets, err := aiProviderSecrets()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Ctx.Request.Context(), aiTagsTimeout)
	defer cancel()
	if err := secrets.Delete(ctx, aiProviderSecretName(c.GetSessionUser().Name, req.Name), metav1.DeleteOptions{}); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

var aiProviderTransport = sync.OnceValue(func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport)
	if configured, ok := proxy.HTTPClient().Transport.(*http.Transport); ok {
		transport = configured
	}
	transport = transport.Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return transport
})

func proxyAIProvider(w http.ResponseWriter, r *http.Request, provider aiProvider, model string, body []byte) {
	path := strings.TrimPrefix(r.URL.Path, AIGatewayPrefix)
	if path != "chat/completions" && path != "responses" && path != "embeddings" {
		writeAIError(w, http.StatusNotFound, "invalid_request_error", "", "unsupported provider endpoint")
		return
	}
	target, err := url.Parse(provider.BaseURL)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		writeAIError(w, http.StatusBadGateway, "server_error", "", "invalid AI provider URL")
		return
	}
	var payload map[string]json.RawMessage
	_ = json.Unmarshal(body, &payload)
	payload["model"], _ = json.Marshal(model)
	body, _ = json.Marshal(payload)
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.URL.Path = strings.TrimRight(target.Path, "/") + "/" + path
			pr.Out.URL.RawPath = ""
			pr.Out.URL.RawQuery = ""
			pr.Out.Header = make(http.Header)
			pr.Out.Header.Set("Content-Type", "application/json")
			for _, header := range []string{"Accept", "OpenAI-Beta"} {
				if value := pr.In.Header.Get(header); value != "" {
					pr.Out.Header.Set(header, value)
				}
			}
			if provider.apiKey != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+provider.apiKey)
			}
		},
		Transport:     aiProviderTransport(),
		FlushInterval: -1,
		ModifyResponse: func(response *http.Response) error {
			response.Header.Del("Set-Cookie")
			response.Header.Del("Access-Control-Allow-Origin")
			response.Header.Del("Access-Control-Allow-Credentials")
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeAIError(w, http.StatusBadGateway, "server_error", "", "the AI provider did not answer")
		},
	}
	started := time.Now()
	recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	proxy.ServeHTTP(recorder, r)
	logs.Info("ai gateway: provider %s model %s: %d in %s", provider.Name, model, recorder.status, time.Since(started).Round(time.Millisecond))
}
