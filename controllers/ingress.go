package controllers

import (
	"encoding/json"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type ingressRule struct {
	Host        string `json:"host"`
	Path        string `json:"path"`
	PathType    string `json:"pathType"`
	ServiceName string `json:"serviceName"`
	ServicePort int32  `json:"servicePort"`
}

type ingressSummary struct {
	Namespace       string        `json:"namespace"`
	Name            string        `json:"name"`
	IngressClass    string        `json:"ingressClass"`
	Rules           []ingressRule `json:"rules"`
	TLSEnabled      bool          `json:"tlsEnabled"`
	ClusterIssuer   string        `json:"clusterIssuer"`
	TLSSecretName   string        `json:"tlsSecretName"`
	CreatedAt       string        `json:"createdAt"`
	ResourceVersion string        `json:"resourceVersion"`
}

func toIngressSummary(ing networkingv1.Ingress) ingressSummary {
	var rules []ingressRule
	for _, r := range ing.Spec.Rules {
		if r.HTTP == nil {
			continue
		}
		for _, p := range r.HTTP.Paths {
			pt := ""
			if p.PathType != nil {
				pt = string(*p.PathType)
			}
			port := int32(0)
			if p.Backend.Service != nil {
				port = p.Backend.Service.Port.Number
			}
			svcName := ""
			if p.Backend.Service != nil {
				svcName = p.Backend.Service.Name
			}
			rules = append(rules, ingressRule{
				Host:        r.Host,
				Path:        p.Path,
				PathType:    pt,
				ServiceName: svcName,
				ServicePort: port,
			})
		}
	}
	cls := ""
	if ing.Spec.IngressClassName != nil {
		cls = *ing.Spec.IngressClassName
	}
	tlsEnabled := len(ing.Spec.TLS) > 0
	clusterIssuer := ing.Annotations["cert-manager.io/cluster-issuer"]
	tlsSecretName := ""
	if tlsEnabled {
		tlsSecretName = ing.Spec.TLS[0].SecretName
	}
	return ingressSummary{
		Namespace:       ing.Namespace,
		Name:            ing.Name,
		IngressClass:    cls,
		Rules:           rules,
		TLSEnabled:      tlsEnabled,
		ClusterIssuer:   clusterIssuer,
		TLSSecretName:   tlsSecretName,
		CreatedAt:       ing.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: ing.ResourceVersion,
	}
}

type ingressRequest struct {
	Namespace       string        `json:"namespace"`
	Name            string        `json:"name"`
	IngressClass    string        `json:"ingressClass"`
	Rules           []ingressRule `json:"rules"`
	TLSEnabled      bool          `json:"tlsEnabled"`
	ClusterIssuer   string        `json:"clusterIssuer"`
	ResourceVersion string        `json:"resourceVersion"`
}

func buildIngressSpec(req ingressRequest) networkingv1.IngressSpec {
	var specRules []networkingv1.IngressRule
	hostMap := map[string][]networkingv1.HTTPIngressPath{}
	for _, r := range req.Rules {
		pt := networkingv1.PathTypePrefix
		if r.PathType == "Exact" {
			pt = networkingv1.PathTypeExact
		} else if r.PathType == "ImplementationSpecific" {
			pt = networkingv1.PathTypeImplementationSpecific
		}
		port := networkingv1.ServiceBackendPort{Number: r.ServicePort}
		hostMap[r.Host] = append(hostMap[r.Host], networkingv1.HTTPIngressPath{
			Path:     r.Path,
			PathType: &pt,
			Backend: networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: r.ServiceName,
					Port: port,
				},
			},
		})
	}
	for host, paths := range hostMap {
		specRules = append(specRules, networkingv1.IngressRule{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{Paths: paths},
			},
		})
	}
	spec := networkingv1.IngressSpec{Rules: specRules}
	if req.IngressClass != "" {
		cls := req.IngressClass
		spec.IngressClassName = &cls
	}
	if req.TLSEnabled {
		hostSet := map[string]struct{}{}
		for _, r := range req.Rules {
			if r.Host != "" {
				hostSet[r.Host] = struct{}{}
			}
		}
		hosts := make([]string, 0, len(hostSet))
		for h := range hostSet {
			hosts = append(hosts, h)
		}
		spec.TLS = []networkingv1.IngressTLS{{
			Hosts:      hosts,
			SecretName: req.Name + "-tls",
		}}
	}
	return spec
}

// GetIngresses
// @router /api/get-ingresses [get]
func (c *ApiController) GetIngresses() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	list, err := object.GetIngresses(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]ingressSummary, 0, len(list))
	for _, ing := range list {
		result = append(result, toIngressSummary(ing))
	}
	c.ResponseOk(result)
}

// GetIngress
// @router /api/get-ingress [get]
func (c *ApiController) GetIngress() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	ing, err := object.GetIngress(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toIngressSummary(*ing))
}

// AddIngress
// @router /api/add-ingress [post]
func (c *ApiController) AddIngress() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req ingressRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: buildIngressSpec(req),
	}
	if req.TLSEnabled && req.ClusterIssuer != "" {
		ing.Annotations = map[string]string{
			"cert-manager.io/cluster-issuer": req.ClusterIssuer,
		}
	}
	created, err := object.AddIngress(cfg, ing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toIngressSummary(*created))
}

// UpdateIngress
// @router /api/update-ingress [post]
func (c *ApiController) UpdateIngress() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req ingressRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	annotations := map[string]string{}
	if req.TLSEnabled && req.ClusterIssuer != "" {
		annotations["cert-manager.io/cluster-issuer"] = req.ClusterIssuer
	}
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            req.Name,
			Namespace:       req.Namespace,
			ResourceVersion: req.ResourceVersion,
			Annotations:     annotations,
		},
		Spec: buildIngressSpec(req),
	}
	updated, err := object.UpdateIngress(cfg, ing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toIngressSummary(*updated))
}

// GetIngressCertStatus returns the TLS certificate expiry for an Ingress managed
// by cert-manager. Response: {status: "ready"|"pending"|"no-tls", expiry?: "YYYY-MM-DD"}
// @router /api/get-ingress-cert-status [get]
func (c *ApiController) GetIngressCertStatus() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	ing, err := object.GetIngress(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if len(ing.Spec.TLS) == 0 {
		c.ResponseOk(map[string]string{"status": "no-tls"})
		return
	}
	secretName := ing.Spec.TLS[0].SecretName
	expiry, err := object.GetTLSCertExpiry(cfg, namespace, secretName)
	if err != nil {
		c.ResponseOk(map[string]string{"status": "pending"})
		return
	}
	c.ResponseOk(map[string]string{"status": "ready", "expiry": expiry})
}

// DeleteIngress
// @router /api/delete-ingress [post]
func (c *ApiController) DeleteIngress() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req ingressRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteIngress(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
