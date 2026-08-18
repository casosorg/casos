package controllers

import (
	"encoding/json"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type configMapSummary struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	DataKeys        int               `json:"dataKeys"`
	Data            map[string]string `json:"data,omitempty"`
	CreatedAt       string            `json:"createdAt"`
	ResourceVersion string            `json:"resourceVersion"`
}

func toListSummary(cm corev1.ConfigMap) configMapSummary {
	summary := toSummary(cm)
	summary.Data = nil
	return summary
}

type configMapPage struct {
	Items              []configMapSummary `json:"items"`
	ContinueToken      string             `json:"continueToken"`
	RemainingItemCount *int64             `json:"remainingItemCount,omitempty"`
}

func parseConfigMapLimit(value string) (int64, bool, error) {
	if value == "" {
		return 0, false, nil
	}
	limit, err := strconv.ParseInt(value, 10, 64)
	if err != nil || limit < 1 || limit > 100 {
		return 0, false, fmt.Errorf("limit must be an integer between 1 and 100")
	}
	return limit, true, nil
}

func toSummary(cm corev1.ConfigMap) configMapSummary {
	return configMapSummary{
		Namespace:       cm.Namespace,
		Name:            cm.Name,
		DataKeys:        len(cm.Data),
		Data:            cm.Data,
		CreatedAt:       cm.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: cm.ResourceVersion,
	}
}

// GetConfigMaps
// @router /api/get-configmaps [get]
func (c *ApiController) GetConfigMaps() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	limit, paginated, err := parseConfigMapLimit(c.GetString("limit"))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if paginated {
		page, err := object.GetConfigMapPage(cfg, namespace, limit, c.GetString("continue"))
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		items := make([]configMapSummary, 0, len(page.Items))
		for _, cm := range page.Items {
			items = append(items, toListSummary(cm))
		}
		c.ResponseOk(configMapPage{
			Items:              items,
			ContinueToken:      page.Continue,
			RemainingItemCount: page.RemainingItemCount,
		})
		return
	}
	cms, err := object.GetConfigMaps(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]configMapSummary, 0, len(cms))
	for _, cm := range cms {
		result = append(result, toSummary(cm))
	}
	c.ResponseOk(result)
}

// GetConfigMap
// @router /api/get-configmap [get]
func (c *ApiController) GetConfigMap() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	name := c.GetString("name")
	cm, err := object.GetConfigMap(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSummary(*cm))
}

type configMapRequest struct {
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	Data            map[string]string `json:"data"`
	ResourceVersion string            `json:"resourceVersion"`
}

// AddConfigMap
// @router /api/add-configmap [post]
func (c *ApiController) AddConfigMap() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req configMapRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Data: req.Data,
	}
	created, err := object.AddConfigMap(cfg, cm)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSummary(*created))
}

// UpdateConfigMap
// @router /api/update-configmap [post]
func (c *ApiController) UpdateConfigMap() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req configMapRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            req.Name,
			Namespace:       req.Namespace,
			ResourceVersion: req.ResourceVersion,
		},
		Data: req.Data,
	}
	updated, err := object.UpdateConfigMap(cfg, cm)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toSummary(*updated))
}

// DeleteConfigMap
// @router /api/delete-configmap [post]
func (c *ApiController) DeleteConfigMap() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req configMapRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeleteConfigMap(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
