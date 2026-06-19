package controllers

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
)

type volumeSummary struct {
	ClaimName string `json:"claimName"`
	MountPath string `json:"mountPath"`
}

type volumeRequest struct {
	MountPath string `json:"mountPath"`
	Size      string `json:"size"`
}

func pvcNameForVolume(deployName string, idx int) string {
	return fmt.Sprintf("%s-vol-%d", deployName, idx)
}

// extractVolumes reads PVC-backed mounts from the first container of a Deployment.
func extractVolumes(d appsv1.Deployment) []volumeSummary {
	result := []volumeSummary{}
	if len(d.Spec.Template.Spec.Containers) == 0 {
		return result
	}
	mountsByName := map[string]string{}
	for _, vm := range d.Spec.Template.Spec.Containers[0].VolumeMounts {
		mountsByName[vm.Name] = vm.MountPath
	}
	for _, v := range d.Spec.Template.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			if mp, ok := mountsByName[v.Name]; ok {
				result = append(result, volumeSummary{
					ClaimName: v.PersistentVolumeClaim.ClaimName,
					MountPath: mp,
				})
			}
		}
	}
	return result
}

// buildPodVolumes converts volume requests into k8s Volume + VolumeMount pairs.
func buildPodVolumes(deployName string, reqs []volumeRequest) ([]corev1.Volume, []corev1.VolumeMount) {
	var podVolumes []corev1.Volume
	var mounts []corev1.VolumeMount
	for i, v := range reqs {
		volName := fmt.Sprintf("vol-%d", i)
		podVolumes = append(podVolumes, corev1.Volume{
			Name: volName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcNameForVolume(deployName, i),
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: v.MountPath,
		})
	}
	return podVolumes, mounts
}

// ensureDeploymentPVCs creates a PVC for each volume request that does not already exist.
func ensureDeploymentPVCs(cfg *rest.Config, namespace, deployName string, reqs []volumeRequest) error {
	for i, v := range reqs {
		if v.MountPath == "" {
			continue
		}
		size := v.Size
		if size == "" {
			size = "1Gi"
		}
		storageQty, err := resource.ParseQuantity(size)
		if err != nil {
			return fmt.Errorf("invalid storage size %q: %w", size, err)
		}
		pvcName := pvcNameForVolume(deployName, i)
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: pvcName, Namespace: namespace},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: storageQty},
				},
			},
		}
		if _, err := object.AddPersistentVolumeClaim(cfg, pvc); err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create PVC %s: %w", pvcName, err)
		}
	}
	return nil
}
