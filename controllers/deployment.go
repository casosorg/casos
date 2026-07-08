package controllers

import (
	"encoding/json"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type deploymentSummary struct {
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"readyReplicas"`
	AvailableReplicas int32             `json:"availableReplicas"`
	Image             string            `json:"image"`
	Ports             []portSummary     `json:"ports"`
	Selector          map[string]string `json:"selector"`
	EnvVars           []envVarSummary   `json:"envVars"`
	Volumes           []volumeSummary   `json:"volumes"`
	CreatedAt         string            `json:"createdAt"`
	ResourceVersion   string            `json:"resourceVersion"`
}

func toDeploymentSummary(d appsv1.Deployment) deploymentSummary {
	image := ""
	if len(d.Spec.Template.Spec.Containers) > 0 {
		image = d.Spec.Template.Spec.Containers[0].Image
	}
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	selector := map[string]string{}
	if d.Spec.Selector != nil {
		selector = d.Spec.Selector.MatchLabels
	}
	ports := []portSummary{}
	for _, container := range d.Spec.Template.Spec.Containers {
		for _, p := range container.Ports {
			protocol := string(p.Protocol)
			if protocol == "" {
				protocol = string(corev1.ProtocolTCP)
			}
			ports = append(ports, portSummary{
				Name:       p.Name,
				Protocol:   protocol,
				Port:       p.ContainerPort,
				TargetPort: strconv.FormatInt(int64(p.ContainerPort), 10),
			})
		}
	}
	return deploymentSummary{
		Namespace:         d.Namespace,
		Name:              d.Name,
		Replicas:          replicas,
		ReadyReplicas:     d.Status.ReadyReplicas,
		AvailableReplicas: d.Status.AvailableReplicas,
		Image:             image,
		Ports:             ports,
		Selector:          selector,
		EnvVars:           extractEnvVars(d.Spec.Template.Spec.Containers),
		Volumes:           extractVolumes(d),
		CreatedAt:         d.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion:   d.ResourceVersion,
	}
}

// GetDeployments
// @router /api/get-deployments [get]
func (c *ApiController) GetDeployments() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	deploys, err := object.GetDeployments(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]deploymentSummary, 0, len(deploys))
	for _, d := range deploys {
		result = append(result, toDeploymentSummary(d))
	}
	c.ResponseOk(result)
}

// GetDeployment
// @router /api/get-deployment [get]
func (c *ApiController) GetDeployment() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	d, err := object.GetDeployment(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDeploymentSummary(*d))
}

type deploymentRequest struct {
	Namespace       string          `json:"namespace"`
	Name            string          `json:"name"`
	Replicas        int32           `json:"replicas"`
	ContainerName   string          `json:"containerName"`
	Image           string          `json:"image"`
	CpuRequest      string          `json:"cpuRequest"`
	MemoryRequest   string          `json:"memoryRequest"`
	EnvVars         []envVarRequest `json:"envVars"`
	Volumes         []volumeRequest `json:"volumes"`
	ResourceVersion string          `json:"resourceVersion"`
}

func buildDeployment(req deploymentRequest) *appsv1.Deployment {
	replicas := req.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	containerName := req.ContainerName
	if containerName == "" {
		containerName = req.Name
	}

	container := corev1.Container{
		Name:  containerName,
		Image: req.Image,
		Env:   buildEnvVars(req.EnvVars),
	}

	if req.CpuRequest != "" || req.MemoryRequest != "" {
		reqs := corev1.ResourceList{}
		if req.CpuRequest != "" {
			reqs[corev1.ResourceCPU] = resource.MustParse(req.CpuRequest)
		}
		if req.MemoryRequest != "" {
			reqs[corev1.ResourceMemory] = resource.MustParse(req.MemoryRequest)
		}
		container.Resources = corev1.ResourceRequirements{Requests: reqs}
	}

	podVolumes, mounts := buildPodVolumes(req.Name, req.Volumes)
	container.VolumeMounts = mounts

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            req.Name,
			Namespace:       req.Namespace,
			ResourceVersion: req.ResourceVersion,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": req.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": req.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
					Volumes:    podVolumes,
				},
			},
		},
	}
}

// AddDeployment
// @router /api/add-deployment [post]
func (c *ApiController) AddDeployment() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req deploymentRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := ensureDeploymentPVCs(cfg, req.Namespace, req.Name, req.Volumes); err != nil {
		c.ResponseError(err.Error())
		return
	}
	created, err := object.AddDeployment(cfg, buildDeployment(req))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDeploymentSummary(*created))
}

// UpdateDeployment
// @router /api/update-deployment [post]
func (c *ApiController) UpdateDeployment() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req deploymentRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	// Fetch the existing deployment to preserve fields not exposed in the form
	existing, err := object.GetDeployment(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	replicas := req.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	existing.Spec.Replicas = &replicas
	if len(existing.Spec.Template.Spec.Containers) > 0 {
		existing.Spec.Template.Spec.Containers[0].Image = req.Image
		existing.Spec.Template.Spec.Containers[0].Env = buildEnvVars(req.EnvVars)
	} else {
		containerName := req.ContainerName
		if containerName == "" {
			containerName = req.Name
		}
		existing.Spec.Template.Spec.Containers = []corev1.Container{{
			Name:  containerName,
			Image: req.Image,
			Env:   buildEnvVars(req.EnvVars),
		}}
	}
	existing.ResourceVersion = req.ResourceVersion

	updated, err := object.UpdateDeployment(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toDeploymentSummary(*updated))
}

// DeleteDeployment
// @router /api/delete-deployment [post]
func (c *ApiController) DeleteDeployment() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req deploymentRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteDeployment(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}

	// Best-effort: delete a same-named Service if it exists
	if err := object.DeleteService(cfg, req.Namespace, req.Name); err != nil && !errors.IsNotFound(err) {
		c.ResponseError(err.Error())
		return
	}

	c.ResponseOk()
}

// RestartDeployment triggers a rolling restart by patching the pod template annotation.
// @router /api/restart-deployment [post]
func (c *ApiController) RestartDeployment() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.RestartDeployment(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
