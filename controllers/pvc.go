package controllers

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/casosorg/casos/object"
)

type pvcSummary struct {
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	Status           string `json:"status"`
	StorageClassName string `json:"storageClassName"`
	AccessMode       string `json:"accessMode"`
	Storage          string `json:"storage"`
	VolumeName       string `json:"volumeName"`
	CreatedAt        string `json:"createdAt"`
	ResourceVersion  string `json:"resourceVersion"`
}

func toPvcSummary(p corev1.PersistentVolumeClaim) pvcSummary {
	accessMode := ""
	if len(p.Spec.AccessModes) > 0 {
		accessMode = string(p.Spec.AccessModes[0])
	}
	storage := ""
	if req, ok := p.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		storage = req.String()
	}
	storageClass := ""
	if p.Spec.StorageClassName != nil {
		storageClass = *p.Spec.StorageClassName
	}
	return pvcSummary{
		Namespace:        p.Namespace,
		Name:             p.Name,
		Status:           string(p.Status.Phase),
		StorageClassName: storageClass,
		AccessMode:       accessMode,
		Storage:          storage,
		VolumeName:       p.Spec.VolumeName,
		CreatedAt:        p.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion:  p.ResourceVersion,
	}
}

// GetPersistentVolumeClaims
// @router /api/get-pvcs [get]
func (c *ApiController) GetPersistentVolumeClaims() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	pvcs, err := object.GetPersistentVolumeClaims(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]pvcSummary, 0, len(pvcs))
	for _, p := range pvcs {
		result = append(result, toPvcSummary(p))
	}
	c.ResponseOk(result)
}

type pvcRequest struct {
	Namespace        string `json:"namespace"`
	Name             string `json:"name"`
	StorageClassName string `json:"storageClassName"`
	AccessMode       string `json:"accessMode"`
	Storage          string `json:"storage"`
}

// AddPersistentVolumeClaim
// @router /api/add-pvc [post]
func (c *ApiController) AddPersistentVolumeClaim() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req pvcRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.AccessMode == "" {
		req.AccessMode = string(corev1.ReadWriteOnce)
	}
	if req.Storage == "" {
		req.Storage = "1Gi"
	}

	storageQty, err := resource.ParseQuantity(req.Storage)
	if err != nil {
		c.ResponseError("invalid storage quantity: " + err.Error())
		return
	}

	accessMode := corev1.PersistentVolumeAccessMode(req.AccessMode)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageQty,
				},
			},
		},
	}
	if req.StorageClassName != "" {
		pvc.Spec.StorageClassName = &req.StorageClassName
	}

	created, err := object.AddPersistentVolumeClaim(cfg, pvc)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toPvcSummary(*created))
}

// DeletePersistentVolumeClaim
// @router /api/delete-pvc [post]
func (c *ApiController) DeletePersistentVolumeClaim() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req pvcRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if err := object.DeletePersistentVolumeClaim(cfg, req.Namespace, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
