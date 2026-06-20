package controllers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/casosorg/casos/object"
)

type portSummary struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

type serviceSummary struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	ClusterIP       string            `json:"clusterIP"`
	ExternalName    string            `json:"externalName"`
	Selector        map[string]string `json:"selector"`
	Ports           []portSummary     `json:"ports"`
	CreatedAt       string            `json:"createdAt"`
	ResourceVersion string            `json:"resourceVersion"`
}

func toSvcSummary(svc corev1.Service) serviceSummary {
	ports := make([]portSummary, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, portSummary{
			Name:       p.Name,
			Protocol:   string(p.Protocol),
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
			NodePort:   p.NodePort,
		})
	}
	return serviceSummary{
		Namespace:       svc.Namespace,
		Name:            svc.Name,
		Type:            string(svc.Spec.Type),
		ClusterIP:       svc.Spec.ClusterIP,
		ExternalName:    svc.Spec.ExternalName,
		Selector:        svc.Spec.Selector,
		Ports:           ports,
		CreatedAt:       svc.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: svc.ResourceVersion,
	}
}

// GetServices
// @router /api/get-services [get]
func (c *ApiController) GetServices() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	svcs, err := object.GetServices(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]serviceSummary, 0, len(svcs))
	for _, svc := range svcs {
		result = append(result, toSvcSummary(svc))
	}
	c.ResponseOk(result)
}

// GetService
// @router /api/get-service [get]
func (c *ApiController) GetService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	svc, err := object.GetService(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSvcSummary(*svc))
}

type portRequest struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

type serviceRequest struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	ExternalName    string            `json:"externalName"`
	Selector        map[string]string `json:"selector"`
	Ports           []portRequest     `json:"ports"`
	ResourceVersion string            `json:"resourceVersion"`
}

func normalizeServiceRequest(req *serviceRequest) error {
	if req.Type == "" {
		req.Type = string(corev1.ServiceTypeClusterIP)
	}
	if req.Type == string(corev1.ServiceTypeExternalName) {
		req.ExternalName = strings.TrimSpace(req.ExternalName)
		if req.ExternalName == "" {
			return fmt.Errorf("ExternalName service requires externalName")
		}
		req.Selector = nil
		req.Ports = nil
	} else {
		req.ExternalName = ""
	}
	if req.Type != string(corev1.ServiceTypeNodePort) && req.Type != string(corev1.ServiceTypeLoadBalancer) {
		for i := range req.Ports {
			req.Ports[i].NodePort = 0
		}
	}
	return nil
}

func buildServiceSpec(req serviceRequest) corev1.ServiceSpec {
	svcType := corev1.ServiceType(req.Type)
	if svcType == "" {
		svcType = corev1.ServiceTypeClusterIP
	}
	ports := make([]corev1.ServicePort, 0, len(req.Ports))
	for _, p := range req.Ports {
		proto := corev1.Protocol(p.Protocol)
		if proto == "" {
			proto = corev1.ProtocolTCP
		}
		sp := corev1.ServicePort{
			Name:     p.Name,
			Protocol: proto,
			Port:     p.Port,
			NodePort: p.NodePort,
		}
		if n, err := strconv.Atoi(p.TargetPort); err == nil {
			sp.TargetPort = intstr.FromInt32(int32(n))
		} else if p.TargetPort != "" {
			sp.TargetPort = intstr.FromString(p.TargetPort)
		} else {
			sp.TargetPort = intstr.FromInt32(p.Port)
		}
		ports = append(ports, sp)
	}
	return corev1.ServiceSpec{
		Type:         svcType,
		Selector:     req.Selector,
		Ports:        ports,
		ExternalName: req.ExternalName,
	}
}

// AddService
// @router /api/add-service [post]
func (c *ApiController) AddService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := normalizeServiceRequest(&req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: buildServiceSpec(req),
	}
	created, err := object.AddService(cfg, svc)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSvcSummary(*created))
}

// UpdateService
// @router /api/update-service [post]
func (c *ApiController) UpdateService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := normalizeServiceRequest(&req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	// Fetch current to preserve immutable fields when still applicable.
	existing, err := object.GetService(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	newSpec := buildServiceSpec(req)
	if req.Type != string(corev1.ServiceTypeExternalName) {
		newSpec.ClusterIP = existing.Spec.ClusterIP
		newSpec.ClusterIPs = existing.Spec.ClusterIPs
	}
	existing.Spec = newSpec
	existing.ResourceVersion = req.ResourceVersion
	updated, err := object.UpdateService(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSvcSummary(*updated))
}

// DeleteService
// @router /api/delete-service [post]
func (c *ApiController) DeleteService() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteService(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
