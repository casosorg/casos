package controllers

import (
	"encoding/json"
	"fmt"
	"sync"

	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
)

// certStatus tracks the state of an in-flight or completed cert request.
type certStatus struct {
	Status string `json:"status"` // pending | verifying | issued | failed
	Error  string `json:"error,omitempty"`
	Expiry string `json:"expiry,omitempty"`
}

var (
	certStatusMu sync.Mutex
	certStatuses = map[string]*certStatus{} // key: "namespace/ingressName"
)

func setCertStatus(key string, s *certStatus) {
	certStatusMu.Lock()
	defer certStatusMu.Unlock()
	certStatuses[key] = s
}

func getCertStatusFromMap(key string) *certStatus {
	certStatusMu.Lock()
	defer certStatusMu.Unlock()
	return certStatuses[key]
}

// ServeACMEChallenge responds to Let's Encrypt HTTP-01 verification requests.
// Route: GET /.well-known/acme-challenge/:token  (public, no auth)
func (c *ApiController) ServeACMEChallenge() {
	token := c.Ctx.Input.Param(":token")
	if token == "" {
		c.Ctx.Output.SetStatus(404)
		return
	}
	keyAuth, ok := object.GetACMEChallenge(token)
	if !ok {
		c.Ctx.Output.SetStatus(404)
		return
	}
	c.Ctx.Output.Header("Content-Type", "text/plain")
	c.Ctx.Output.Body([]byte(keyAuth))
}

// RequestLECert initiates an async Let's Encrypt HTTP-01 certificate request.
// @router /api/request-le-cert [post]
func (c *ApiController) RequestLECert() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	var req struct {
		Namespace        string `json:"namespace"`
		IngressName      string `json:"ingressName"`
		Domain           string `json:"domain"`
		CasosServiceName string `json:"casosServiceName"`
		CasosServicePort int32  `json:"casosServicePort"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" || req.IngressName == "" || req.Domain == "" {
		c.ResponseError("namespace, ingressName and domain are required")
		return
	}

	status, err := startLECertRequest(cfg, req.Namespace, req.IngressName, req.Domain, req.CasosServiceName, req.CasosServicePort)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(map[string]string{"status": status})
}

// startLECertRequest kicks off an HTTP-01 issuance in the background and
// reports what it did: "pending" for a request it started, "already_running"
// for one that was already in flight. Both the certificate page and an app
// deployment that asked for HTTPS come through here.
func startLECertRequest(cfg *rest.Config, namespace, ingressName, domain, serviceName string, servicePort int32) (string, error) {
	if serviceName == "" {
		serviceName = conf.GetConfigStringDefault("casosServiceName", "casos")
	}
	if servicePort == 0 {
		servicePort = int32(conf.GetConfigIntDefault("casosServicePort", conf.GetConfigIntDefault("httpport", 20080)))
	}

	srvCfg := getServerConfig()
	if srvCfg == nil {
		return "", fmt.Errorf("server config not ready")
	}
	backend := object.CasosBackend{
		ServiceName: serviceName,
		ServicePort: servicePort,
		HostIP:      srvCfg.AdvertiseAddress,
	}

	key := namespace + "/" + ingressName

	// Prevent duplicate concurrent requests.
	certStatusMu.Lock()
	if s := certStatuses[key]; s != nil && (s.Status == "pending" || s.Status == "verifying") {
		certStatusMu.Unlock()
		return "already_running", nil
	}
	certStatuses[key] = &certStatus{Status: "pending"}
	certStatusMu.Unlock()

	go func() {
		setCertStatus(key, &certStatus{Status: "verifying"})
		if err := object.ObtainLECert(cfg, namespace, ingressName, domain, backend); err != nil {
			setCertStatus(key, &certStatus{Status: "failed", Error: err.Error()})
			return
		}
		expiry, _ := object.GetTLSCertExpiry(cfg, namespace, ingressName+"-tls")
		setCertStatus(key, &certStatus{Status: "issued", Expiry: expiry})
	}()

	return "pending", nil
}

// UploadCert accepts a manually supplied PEM certificate + key, stores them as
// a TLS Secret, and attaches TLS to the Ingress.
// @router /api/upload-cert [post]
func (c *ApiController) UploadCert() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	var req struct {
		Namespace   string `json:"namespace"`
		IngressName string `json:"ingressName"`
		CertPEM     string `json:"certPEM"`
		KeyPEM      string `json:"keyPEM"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" || req.IngressName == "" || req.CertPEM == "" || req.KeyPEM == "" {
		c.ResponseError("namespace, ingressName, certPEM and keyPEM are required")
		return
	}

	secretName := req.IngressName + "-tls"
	if err := object.StoreTLSSecret(cfg, req.Namespace, secretName, req.CertPEM, req.KeyPEM); err != nil {
		c.ResponseError("store tls secret: " + err.Error())
		return
	}
	if err := object.AttachTLSToIngress(cfg, req.Namespace, req.IngressName, secretName); err != nil {
		c.ResponseError("attach tls to ingress: " + err.Error())
		return
	}

	expiry, _ := object.GetTLSCertExpiry(cfg, req.Namespace, secretName)
	key := req.Namespace + "/" + req.IngressName
	setCertStatus(key, &certStatus{Status: "issued", Expiry: expiry})

	c.ResponseOk(map[string]string{"status": "issued", "expiry": expiry})
}

// GetCertStatus returns the certificate status for an Ingress.
// It checks the in-memory status map first, then falls back to reading the TLS Secret.
// @router /api/get-cert-status [get]
func (c *ApiController) GetCertStatus() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	namespace := c.GetString("namespace")
	ingressName := c.GetString("ingressName")
	key := namespace + "/" + ingressName

	// Check in-memory status (covers pending/verifying/failed states).
	if s := getCertStatusFromMap(key); s != nil {
		c.ResponseOk(s)
		return
	}

	// Fall back to checking whether the TLS Secret already exists.
	secretName := ingressName + "-tls"
	expiry, err := object.GetTLSCertExpiry(cfg, namespace, secretName)
	if err != nil {
		c.ResponseOk(&certStatus{Status: "none"})
		return
	}
	c.ResponseOk(&certStatus{Status: "issued", Expiry: expiry})
}
