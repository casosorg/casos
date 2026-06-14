package controllers

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type serviceAccountSummary struct {
	Namespace        string   `json:"namespace"`
	Name             string   `json:"name"`
	Secrets          int      `json:"secrets"`
	ImagePullSecrets []string `json:"imagePullSecrets"`
	CreatedAt        string   `json:"createdAt"`
	ResourceVersion  string   `json:"resourceVersion"`
}

func toSaSummary(sa corev1.ServiceAccount) serviceAccountSummary {
	ips := make([]string, 0, len(sa.ImagePullSecrets))
	for _, r := range sa.ImagePullSecrets {
		ips = append(ips, r.Name)
	}
	return serviceAccountSummary{
		Namespace:        sa.Namespace,
		Name:             sa.Name,
		Secrets:          len(sa.Secrets),
		ImagePullSecrets: ips,
		CreatedAt:        sa.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion:  sa.ResourceVersion,
	}
}

// GetServiceAccounts
// @router /api/get-serviceaccounts [get]
func (c *ApiController) GetServiceAccounts() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	sas, err := object.GetServiceAccounts(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]serviceAccountSummary, 0, len(sas))
	for _, sa := range sas {
		result = append(result, toSaSummary(sa))
	}
	c.ResponseOk(result)
}

// GetServiceAccount
// @router /api/get-serviceaccount [get]
func (c *ApiController) GetServiceAccount() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	sa, err := object.GetServiceAccount(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSaSummary(*sa))
}

type serviceAccountRequest struct {
	Namespace        string   `json:"namespace"`
	Name             string   `json:"name"`
	ImagePullSecrets []string `json:"imagePullSecrets"`
	ResourceVersion  string   `json:"resourceVersion"`
}

// AddServiceAccount
// @router /api/add-serviceaccount [post]
func (c *ApiController) AddServiceAccount() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceAccountRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	ips := make([]corev1.LocalObjectReference, 0, len(req.ImagePullSecrets))
	for _, s := range req.ImagePullSecrets {
		if s != "" {
			ips = append(ips, corev1.LocalObjectReference{Name: s})
		}
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta:       metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
		ImagePullSecrets: ips,
	}
	created, err := object.AddServiceAccount(cfg, sa)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSaSummary(*created))
}

// UpdateServiceAccount
// @router /api/update-serviceaccount [post]
func (c *ApiController) UpdateServiceAccount() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceAccountRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	ips := make([]corev1.LocalObjectReference, 0, len(req.ImagePullSecrets))
	for _, s := range req.ImagePullSecrets {
		if s != "" {
			ips = append(ips, corev1.LocalObjectReference{Name: s})
		}
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta:       metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, ResourceVersion: req.ResourceVersion},
		ImagePullSecrets: ips,
	}
	updated, err := object.UpdateServiceAccount(cfg, sa)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSaSummary(*updated))
}

// DeleteServiceAccount
// @router /api/delete-serviceaccount [post]
func (c *ApiController) DeleteServiceAccount() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req serviceAccountRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteServiceAccount(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
