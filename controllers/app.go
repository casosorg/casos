package controllers

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type appPortRequest struct {
	Name          string `json:"name"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type deployAppRequest struct {
	Namespace   string           `json:"namespace"`
	Name        string           `json:"name"`
	Image       string           `json:"image"`
	Replicas    int32            `json:"replicas"`
	Ports       []appPortRequest `json:"ports"`
	EnvVars     []envVarRequest  `json:"envVars"`
	ServiceType string           `json:"serviceType"`
}

type deployAppResult struct {
	Deployment *deploymentSummary `json:"deployment,omitempty"`
	Service    *serviceSummary    `json:"service,omitempty"`
}

// DeployApp creates a Deployment and a matching ClusterIP/NodePort Service in one call.
// Helm charts are installed through /api/install-helm-chart instead.
// @router /api/deploy-app [post]
func (c *ApiController) DeployApp() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req deployAppRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Replicas <= 0 {
		req.Replicas = 1
	}

	deplReq := deploymentRequest{
		Namespace: req.Namespace,
		Name:      req.Name,
		Replicas:  req.Replicas,
		Image:     req.Image,
		EnvVars:   req.EnvVars,
	}
	depl := buildDeployment(deplReq)

	if len(req.Ports) > 0 && len(depl.Spec.Template.Spec.Containers) > 0 {
		cports := make([]corev1.ContainerPort, 0, len(req.Ports))
		for _, p := range req.Ports {
			proto := corev1.ProtocolTCP
			if p.Protocol == "UDP" {
				proto = corev1.ProtocolUDP
			}
			cports = append(cports, corev1.ContainerPort{
				Name:          p.Name,
				ContainerPort: p.ContainerPort,
				Protocol:      proto,
			})
		}
		depl.Spec.Template.Spec.Containers[0].Ports = cports
	}

	createdDepl, err := object.AddDeployment(cfg, depl)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	deplSummary := toDeploymentSummary(*createdDepl)
	result := deployAppResult{Deployment: &deplSummary}

	if len(req.Ports) > 0 {
		svcPorts := make([]portRequest, 0, len(req.Ports))
		for _, p := range req.Ports {
			proto := p.Protocol
			if proto == "" {
				proto = "TCP"
			}
			svcPorts = append(svcPorts, portRequest{
				Name:       p.Name,
				Protocol:   proto,
				Port:       p.ContainerPort,
				TargetPort: fmt.Sprintf("%d", p.ContainerPort),
			})
		}
		svcType := req.ServiceType
		if svcType == "" {
			svcType = "NodePort"
		}
		svcReq := serviceRequest{
			Namespace: req.Namespace,
			Name:      req.Name,
			Type:      svcType,
			Selector:  map[string]string{"app": req.Name},
			Ports:     svcPorts,
		}
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      svcReq.Name,
				Namespace: svcReq.Namespace,
			},
			Spec: buildServiceSpec(svcReq),
		}
		createdSvc, svcErr := object.AddService(cfg, svc)
		if svcErr != nil {
			c.ResponseError(fmt.Sprintf("deployment created but service failed: %s", svcErr.Error()))
			return
		}
		s := toSvcSummary(*createdSvc)
		result.Service = &s
	}

	c.ResponseOk(result)
}
