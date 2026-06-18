package controllers

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"unsafe"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/server"
)

var adminCfg unsafe.Pointer // *rest.Config

func SetAdminRestConfig(cfg *rest.Config) {
	atomic.StorePointer(&adminCfg, unsafe.Pointer(cfg))
}

func getAdminRestConfig() *rest.Config {
	return (*rest.Config)(atomic.LoadPointer(&adminCfg))
}

var srvCfg unsafe.Pointer // *server.Config

func SetServerConfig(cfg *server.Config) {
	atomic.StorePointer(&srvCfg, unsafe.Pointer(cfg))
}

func getServerConfig() *server.Config {
	return (*server.Config)(atomic.LoadPointer(&srvCfg))
}

type podSummary struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Phase           string            `json:"phase"`
	NodeName        string            `json:"nodeName"`
	Image           string            `json:"image"`
	Labels          map[string]string `json:"labels"`
	CreatedAt       string            `json:"createdAt"`
	ResourceVersion string            `json:"resourceVersion"`
	ContainerPorts  []int32           `json:"containerPorts"`
	ExposedPorts    []int32           `json:"exposedPorts"`
}

func toPodSummary(p corev1.Pod) podSummary {
	image := ""
	if len(p.Spec.Containers) > 0 {
		image = p.Spec.Containers[0].Image
	}
	return podSummary{
		Namespace:       p.Namespace,
		Name:            p.Name,
		Phase:           string(p.Status.Phase),
		NodeName:        p.Spec.NodeName,
		Image:           image,
		Labels:          p.Labels,
		CreatedAt:       p.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: p.ResourceVersion,
		ContainerPorts:  object.ContainerPortsFromPod(&p),
		ExposedPorts:    readExposedPortsAnnotation(p.Annotations),
	}
}

func (c *ApiController) GetPods() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	pods, err := object.GetPods(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]podSummary, 0, len(pods))
	for _, p := range pods {
		result = append(result, toPodSummary(p))
	}
	c.ResponseOk(result)
}

func (c *ApiController) GetPod() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	pod, err := object.GetPod(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toPodSummary(*pod))
}

type podRequest struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Image           string            `json:"image"`
	ContainerName   string            `json:"containerName"`
	Labels          map[string]string `json:"labels"`
	ResourceVersion string            `json:"resourceVersion"`
	Ports           []int32           `json:"ports"`
}

func (c *ApiController) AddPod() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req podRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.ContainerName == "" {
		req.ContainerName = "app"
	}
	container := corev1.Container{Name: req.ContainerName, Image: req.Image}
	for _, p := range req.Ports {
		if p > 0 {
			container.Ports = append(container.Ports, corev1.ContainerPort{
				ContainerPort: p,
				Protocol:      corev1.ProtocolTCP,
			})
		}
	}
	annotations := map[string]string{}
	if req.Image != "" {
		if exposed, lookupErr := object.LookupExposedPorts(req.Image); lookupErr == nil {
			writeExposedPortsAnnotation(annotations, exposed)
		}
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Namespace:   req.Namespace,
			Labels:      req.Labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{container},
		},
	}
	created, err := object.AddPod(cfg, pod)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toPodSummary(*created))
}

func (c *ApiController) UpdatePod() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req podRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	existing, err := object.GetPod(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	existing.Labels = req.Labels
	existing.ResourceVersion = req.ResourceVersion
	updated, err := object.UpdatePod(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toPodSummary(*updated))
}

type eventSummary struct {
	Type           string `json:"type"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	Count          int32  `json:"count"`
	LastTimestamp  string `json:"lastTimestamp"`
	FirstTimestamp string `json:"firstTimestamp"`
}

func (c *ApiController) GetPodEvents() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	if namespace == "" {
		namespace = "default"
	}
	events, err := object.GetPodEvents(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	formatTime := func(t metav1.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	result := make([]eventSummary, 0, len(events))
	for _, e := range events {
		result = append(result, eventSummary{
			Type:           e.Type,
			Reason:         e.Reason,
			Message:        e.Message,
			Count:          e.Count,
			LastTimestamp:  formatTime(e.LastTimestamp),
			FirstTimestamp: formatTime(e.FirstTimestamp),
		})
	}
	c.ResponseOk(result)
}

func (c *ApiController) GetPodLogs() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	container := c.GetString("container")
	if namespace == "" {
		namespace = "default"
	}
	var tailLines int64 = 500
	if v := c.GetString("tailLines"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			tailLines = n
		}
	}
	logs, err := object.GetPodLogs(cfg, namespace, name, container, tailLines)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(logs)
}

func (c *ApiController) DeletePod() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req podRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeletePod(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

const exposedPortsAnnotationKey = "casos.io/exposed-ports"

func writeExposedPortsAnnotation(annotations map[string]string, ports []int32) {
	if len(ports) == 0 {
		return
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, strconv.FormatInt(int64(p), 10))
	}
	annotations[exposedPortsAnnotationKey] = strings.Join(parts, ",")
}

func readExposedPortsAnnotation(annotations map[string]string) []int32 {
	raw, ok := annotations[exposedPortsAnnotationKey]
	if !ok || raw == "" {
		return nil
	}
	out := make([]int32, 0)
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		p, err := strconv.ParseInt(tok, 10, 32)
		if err != nil || p <= 0 || p > 65535 {
			continue
		}
		out = append(out, int32(p))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
