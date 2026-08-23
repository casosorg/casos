package controllers

import (
	"encoding/json"

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
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	// The app this workload belongs to, and which part of it this is. Both are
	// empty for an app deployed on its own; a companion database names the app
	// that asked for it, so that uninstalling the app takes its database with it.
	Owner       string           `json:"owner"`
	Component   string           `json:"component"`
	Replicas    *int32           `json:"replicas"`
	Ports       []appPortRequest `json:"ports"`
	EnvVars     []envVarRequest  `json:"envVars"`
	Volumes     []volumeRequest  `json:"volumes"`
	ServiceType string           `json:"serviceType"`
}

type deployAppResult struct {
	Deployment deploymentSummary `json:"deployment"`
	Service    *serviceSummary   `json:"service,omitempty"`
}

func containerPortsFor(ports []appPortRequest) []corev1.ContainerPort {
	result := make([]corev1.ContainerPort, 0, len(ports))
	for _, p := range ports {
		proto := corev1.ProtocolTCP
		if p.Protocol == "UDP" {
			proto = corev1.ProtocolUDP
		}
		result = append(result, corev1.ContainerPort{
			Name:          p.Name,
			ContainerPort: p.ContainerPort,
			Protocol:      proto,
		})
	}
	return result
}

// DeployApp creates a Deployment and a matching ClusterIP/NodePort Service in one call.
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
	owner := req.Owner
	if owner == "" {
		owner = req.Name
	}
	labels := appOwnershipLabels(owner, req.Component)

	deplReq := deploymentRequest{
		Namespace: req.Namespace,
		Name:      req.Name,
		Replicas:  req.Replicas,
		Image:     req.Image,
		EnvVars:   req.EnvVars,
		Volumes:   req.Volumes,
	}
	depl, err := buildDeployment(deplReq)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	applyLabels(&depl.ObjectMeta, labels)
	applyLabels(&depl.Spec.Template.ObjectMeta, labels)
	applyAnnotation(&depl.ObjectMeta, appImageAnnotation, req.Image)

	if err := ensureDeploymentPVCs(cfg, req.Namespace, req.Name, req.Volumes, labels); err != nil {
		c.ResponseError(err.Error())
		return
	}

	if len(req.Ports) > 0 && len(depl.Spec.Template.Spec.Containers) > 0 {
		depl.Spec.Template.Spec.Containers[0].Ports = containerPortsFor(req.Ports)
	}

	createdDepl, err := object.AddDeployment(cfg, depl)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	result := deployAppResult{Deployment: toDeploymentSummary(*createdDepl)}

	if len(req.Ports) > 0 {
		svcType := req.ServiceType
		if svcType == "" {
			svcType = "NodePort"
		}
		svcReq := serviceRequest{
			Namespace: req.Namespace,
			Name:      req.Name,
			Type:      svcType,
			Selector:  map[string]string{"app": req.Name},
			Ports:     servicePortsFor(req.Ports),
		}
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      svcReq.Name,
				Namespace: svcReq.Namespace,
			},
			Spec: buildServiceSpec(svcReq),
		}
		applyLabels(&svc.ObjectMeta, labels)
		createdSvc, svcErr := object.AddService(cfg, svc)
		if svcErr != nil {
			c.ResponseError("deployment created but service failed: " + svcErr.Error())
			return
		}
		s := toSvcSummary(*createdSvc)
		result.Service = &s
	}

	c.ResponseOk(result)
}
