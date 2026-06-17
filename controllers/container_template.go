package controllers

import (
	"encoding/json"
	"math/rand"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/casosorg/casos/object"
)

type containerTemplateSummary struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Image       string `json:"image"`
	DefaultPort int32  `json:"defaultPort"`
}

func toContainerTemplateSummatry(t object.ContainerTemplate) containerTemplateSummary {
	return containerTemplateSummary{
		Name:        t.Name,
		DisplayName: t.DisplayName,
		Description: t.Discription,
		Icon:        t.Icon,
		Image:       t.Image,
		DefaultPort: t.DefaultPort,
	}
}
func (c *ApiController) GetContainerTemplates() {
	templates := object.GetContainerTemplates()
	result := make([]containerTemplateSummary, 0, len(templates))
	for _, t := range templates {
		result = append(result, toContainerTemplateSummatry(t))
	}
	c.ResponseOk(result)
}

type deployContainerTemplateRequest struct {
	TemplateName string `json:"templateName"`
	Namespace    string `json:"namespace"`
}

func (c *ApiController) DeployContainerTemplate() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req deployContainerTemplateRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body:" + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	templates := object.GetContainerTemplates()
	var tpl *object.ContainerTemplate
	for i := range templates {
		if templates[i].Name == req.TemplateName {
			tpl = &templates[i]
			break
		}
	}
	if tpl == nil {
		c.ResponseError("template not found" + req.TemplateName)
		return
	}
	appName := tpl.Name + "-" + randSeq(5)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: req.Namespace,
			Labels: map[string]string{
				"app": appName,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app",
					Image: tpl.Image,
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: tpl.DefaultPort,
						},
					},
				},
			},
		},
	}
	if _, err := object.AddPod(cfg, pod); err != nil {
		c.ResponseError("create pod:" + err.Error())
		return
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: req.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"app": appName,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "app",
					Port:       tpl.DefaultPort,
					TargetPort: intstr.FromInt32(tpl.DefaultPort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	if _, err := object.AddService(cfg, svc); err != nil {
		c.ResponseError("create service:" + err.Error())
		return
	}
	c.ResponseOk(map[string]string{
		"name":      appName,
		"namespace": req.Namespace,
	})

}
func randSeq(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
