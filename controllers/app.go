package controllers

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

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
	// The rest is what the launchpad form adds on top of a one-click install:
	// how much the container may use, what it runs, the files and credentials
	// it needs, and how it is reached and scaled. All optional, so an install
	// that sends none of it behaves exactly as it did before.
	resourceRequest
	Command []string `json:"command"`
	Args    []string `json:"args"`
	// Pointers, for the same reason the quantities above are: an install that
	// says nothing about config files or a domain must leave the ones the app
	// already has alone, while an empty list means "remove them".
	ConfigFiles *[]appConfigFile  `json:"configFiles"`
	Registry    *appRegistryLogin `json:"registry"`
	Hpa         *appHpaRequest    `json:"hpa"`
	Domains     *[]appDomain      `json:"domains"`
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

	configVolume, configMounts, pullSecret, err := reconcileAppExtras(cfg, req, labels, nil)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	container := &depl.Spec.Template.Spec.Containers[0]
	if err := applyAppContainer(container, req); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if len(req.Ports) > 0 {
		container.Ports = containerPortsFor(req.Ports)
	}
	if configVolume != nil {
		depl.Spec.Template.Spec.Volumes = append(depl.Spec.Template.Spec.Volumes, *configVolume)
		container.VolumeMounts = append(container.VolumeMounts, configMounts...)
	}
	applyImagePullSecret(&depl.Spec.Template.Spec, pullSecret)

	// An autoscaled app starts at the floor it will be held to; leaving the
	// form's replica count would have the autoscaler undo it a minute later.
	if req.Hpa != nil && req.Hpa.Enabled && req.Hpa.MinReplicas > 0 {
		minReplicas := req.Hpa.MinReplicas
		depl.Spec.Replicas = &minReplicas
	}

	createdDepl, err := object.AddDeployment(cfg, depl)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	result := deployAppResult{Deployment: toDeploymentSummary(*createdDepl)}

	if err := reconcileAppNetworkAndScaling(cfg, req, labels); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if svc, svcErr := object.GetService(cfg, req.Namespace, req.Name); svcErr == nil {
		summary := toSvcSummary(*svc)
		result.Service = &summary
	}

	c.ResponseOk(result)
}
