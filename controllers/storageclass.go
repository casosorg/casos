package controllers

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
)

const (
	defaultStorageClassAnnotation     = "storageclass.kubernetes.io/is-default-class"
	defaultStorageClassAnnotationBeta = "storageclass.beta.kubernetes.io/is-default-class"
)

type storageClassSummary struct {
	Name                 string            `json:"name"`
	Provisioner          string            `json:"provisioner"`
	ReclaimPolicy        string            `json:"reclaimPolicy"`
	VolumeBindingMode    string            `json:"volumeBindingMode"`
	AllowVolumeExpansion bool              `json:"allowVolumeExpansion"`
	Parameters           map[string]string `json:"parameters"`
	IsDefault            bool              `json:"isDefault"`
	CreatedAt            string            `json:"createdAt"`
	ResourceVersion      string            `json:"resourceVersion"`
}

func isDefaultStorageClass(sc storagev1.StorageClass) bool {
	return sc.Annotations[defaultStorageClassAnnotation] == "true" ||
		sc.Annotations[defaultStorageClassAnnotationBeta] == "true"
}

func toScSummary(sc storagev1.StorageClass) storageClassSummary {
	reclaimPolicy := ""
	if sc.ReclaimPolicy != nil {
		reclaimPolicy = string(*sc.ReclaimPolicy)
	}
	volumeBindingMode := ""
	if sc.VolumeBindingMode != nil {
		volumeBindingMode = string(*sc.VolumeBindingMode)
	}
	allowVolumeExpansion := false
	if sc.AllowVolumeExpansion != nil {
		allowVolumeExpansion = *sc.AllowVolumeExpansion
	}
	return storageClassSummary{
		Name:                 sc.Name,
		Provisioner:          sc.Provisioner,
		ReclaimPolicy:        reclaimPolicy,
		VolumeBindingMode:    volumeBindingMode,
		AllowVolumeExpansion: allowVolumeExpansion,
		Parameters:           sc.Parameters,
		IsDefault:            isDefaultStorageClass(sc),
		CreatedAt:            sc.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion:      sc.ResourceVersion,
	}
}

// GetStorageClasses
// @router /api/get-storageclasses [get]
func (c *ApiController) GetStorageClasses() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	scs, err := object.GetStorageClasses(cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]storageClassSummary, 0, len(scs))
	for _, sc := range scs {
		result = append(result, toScSummary(sc))
	}
	c.ResponseOk(result)
}

// GetStorageClass
// @router /api/get-storageclass [get]
func (c *ApiController) GetStorageClass() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	name := c.GetString("name")
	sc, err := object.GetStorageClass(cfg, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toScSummary(*sc))
}

type storageClassRequest struct {
	Name                 string            `json:"name"`
	Provisioner          string            `json:"provisioner"`
	ReclaimPolicy        string            `json:"reclaimPolicy"`
	VolumeBindingMode    string            `json:"volumeBindingMode"`
	AllowVolumeExpansion bool              `json:"allowVolumeExpansion"`
	Parameters           map[string]string `json:"parameters"`
	IsDefault            bool              `json:"isDefault"`
	ResourceVersion      string            `json:"resourceVersion"`
}

func defaultAnnotations(isDefault bool) map[string]string {
	if !isDefault {
		return nil
	}
	return map[string]string{
		defaultStorageClassAnnotation:     "true",
		defaultStorageClassAnnotationBeta: "true",
	}
}

// clearOtherDefaultStorageClasses unsets the default annotation on every StorageClass
// other than exceptName, so at most one StorageClass is ever marked default.
func clearOtherDefaultStorageClasses(cfg *rest.Config, exceptName string) error {
	scs, err := object.GetStorageClasses(cfg)
	if err != nil {
		return err
	}
	for _, sc := range scs {
		if sc.Name == exceptName || !isDefaultStorageClass(sc) {
			continue
		}
		delete(sc.Annotations, defaultStorageClassAnnotation)
		delete(sc.Annotations, defaultStorageClassAnnotationBeta)
		if _, err := object.UpdateStorageClass(cfg, &sc); err != nil {
			return err
		}
	}
	return nil
}

// AddStorageClass
// @router /api/add-storageclass [post]
func (c *ApiController) AddStorageClass() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req storageClassRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	reclaimPolicy := corev1.PersistentVolumeReclaimPolicy(req.ReclaimPolicy)
	if reclaimPolicy == "" {
		reclaimPolicy = corev1.PersistentVolumeReclaimDelete
	}
	volumeBindingMode := storagev1.VolumeBindingMode(req.VolumeBindingMode)
	if volumeBindingMode == "" {
		volumeBindingMode = storagev1.VolumeBindingImmediate
	}
	allowVolumeExpansion := req.AllowVolumeExpansion
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        req.Name,
			Annotations: defaultAnnotations(req.IsDefault),
		},
		Provisioner:          req.Provisioner,
		Parameters:           req.Parameters,
		ReclaimPolicy:        &reclaimPolicy,
		VolumeBindingMode:    &volumeBindingMode,
		AllowVolumeExpansion: &allowVolumeExpansion,
	}
	created, err := object.AddStorageClass(cfg, sc)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if req.IsDefault {
		if err := clearOtherDefaultStorageClasses(cfg, req.Name); err != nil {
			c.ResponseError(err.Error())
			return
		}
	}
	c.ResponseOk(toScSummary(*created))
}

// UpdateStorageClass updates the mutable fields of a StorageClass; provisioner,
// parameters and volumeBindingMode are immutable after creation.
// @router /api/update-storageclass [post]
func (c *ApiController) UpdateStorageClass() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req storageClassRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	existing, err := object.GetStorageClass(cfg, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	reclaimPolicy := corev1.PersistentVolumeReclaimPolicy(req.ReclaimPolicy)
	if reclaimPolicy == "" {
		reclaimPolicy = corev1.PersistentVolumeReclaimDelete
	}
	allowVolumeExpansion := req.AllowVolumeExpansion
	existing.ReclaimPolicy = &reclaimPolicy
	existing.AllowVolumeExpansion = &allowVolumeExpansion
	if existing.Annotations == nil {
		existing.Annotations = map[string]string{}
	}
	if req.IsDefault {
		existing.Annotations[defaultStorageClassAnnotation] = "true"
		existing.Annotations[defaultStorageClassAnnotationBeta] = "true"
	} else {
		delete(existing.Annotations, defaultStorageClassAnnotation)
		delete(existing.Annotations, defaultStorageClassAnnotationBeta)
	}
	existing.ResourceVersion = req.ResourceVersion
	updated, err := object.UpdateStorageClass(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if req.IsDefault {
		if err := clearOtherDefaultStorageClasses(cfg, req.Name); err != nil {
			c.ResponseError(err.Error())
			return
		}
	}
	c.ResponseOk(toScSummary(*updated))
}

// DeleteStorageClass
// @router /api/delete-storageclass [post]
func (c *ApiController) DeleteStorageClass() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req storageClassRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if err := object.DeleteStorageClass(cfg, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
