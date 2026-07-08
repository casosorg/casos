package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	localPathNamespace                 = "local-path-storage"
	localPathProvisionerName           = "casos.io/local-path-provisioner"
	localPathStorageClass              = "local-path"
	localPathManagedSpecHashAnnotation = "casos.io/managed-spec-hash"
	localPathPreserveSpecAnnotation    = "casos.io/preserve-user-spec"
)

type localPathProvisionerConfig struct {
	NodePathMap []localPathNodePathMap `json:"nodePathMap"`
}

type localPathNodePathMap struct {
	Node  string   `json:"node"`
	Paths []string `json:"paths"`
}

func ensureDefaultStorageProvisioner(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	if !path.IsAbs(cfg.DataDir) {
		return fmt.Errorf("dataDir must be absolute to enable local-path storage: %s", cfg.DataDir)
	}
	rootDir := path.Join(cfg.DataDir, "local-path-provisioner")
	configData, err := localPathConfigData(rootDir, cfg.LocalPathHelperImage)
	if err != nil {
		return err
	}
	if err := ensureNamespace(ctx, client, localPathNamespace); err != nil {
		return err
	}
	if err := ensureLocalPathServiceAccount(ctx, client); err != nil {
		return err
	}
	if err := ensureLocalPathClusterRole(ctx, client); err != nil {
		return err
	}
	if err := ensureLocalPathClusterRoleBinding(ctx, client); err != nil {
		return err
	}
	if err := ensureLocalPathConfigMap(ctx, client, configData); err != nil {
		return err
	}
	if err := ensureLocalPathDeployment(ctx, client, hashConfigData(configData), cfg.LocalPathProvisionerImage); err != nil {
		return err
	}
	return ensureLocalPathStorageClass(ctx, client)
}

func ensureNamespace(ctx context.Context, client kubernetes.Interface, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	_, err := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}
	return nil
}

func ensureLocalPathServiceAccount(ctx context.Context, client kubernetes.Interface) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-path-provisioner-service-account",
			Namespace: localPathNamespace,
			Labels:    localPathLabels(),
		},
	}
	return createOrUpdateServiceAccount(ctx, client, sa)
}

func ensureLocalPathClusterRole(ctx context.Context, client kubernetes.Interface) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "local-path-provisioner-role",
			Labels: localPathLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes", "configmaps", "pods", "pods/log"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumeclaims"},
				Verbs:     []string{"get", "list", "watch", "update", "patch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumes"},
				Verbs:     []string{"get", "list", "watch", "create", "patch", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch", "update"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"storageclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			},
		},
	}
	return createOrUpdateClusterRole(ctx, client, role)
}

func ensureLocalPathClusterRoleBinding(ctx context.Context, client kubernetes.Interface) error {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "local-path-provisioner-bind",
			Labels: localPathLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "local-path-provisioner-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "local-path-provisioner-service-account",
				Namespace: localPathNamespace,
			},
		},
	}
	return createOrUpdateClusterRoleBinding(ctx, client, binding)
}

func localPathConfigData(rootDir, helperImage string) (map[string]string, error) {
	quotedRootDir := shellQuote(rootDir)
	configBytes, err := json.Marshal(localPathProvisionerConfig{
		NodePathMap: []localPathNodePathMap{
			{Node: "DEFAULT_PATH_FOR_NON_LISTED_NODES", Paths: []string{rootDir}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal local path provisioner config: %w", err)
	}
	return map[string]string{
		"config.json": string(configBytes),
		"helperPod.yaml": fmt.Sprintf(`apiVersion: v1
kind: Pod
metadata:
  name: helper-pod
spec:
  restartPolicy: Never
  automountServiceAccountToken: false
  tolerations:
    - key: node.kubernetes.io/disk-pressure
      operator: Exists
      effect: NoSchedule
  containers:
    - name: helper-pod
      image: %s
      imagePullPolicy: IfNotPresent
      securityContext:
        allowPrivilegeEscalation: false
        privileged: false
        readOnlyRootFilesystem: true
        runAsUser: 0
        capabilities:
          drop:
            - ALL
      resources:
        requests:
          cpu: 10m
          memory: 32Mi
        limits:
          cpu: 100m
          memory: 128Mi
`, helperImage),
		"setup": `#!/bin/sh
set -eu
SAFE_ROOT=` + quotedRootDir + `
case "${VOL_DIR:-}" in
  "$SAFE_ROOT"/*) ;;
  *) echo "refusing unsafe volume path: ${VOL_DIR:-<empty>}" >&2; exit 1 ;;
esac
mkdir -p "$VOL_DIR"
chmod 0777 "$VOL_DIR"
`,
		"teardown": `#!/bin/sh
set -eu
SAFE_ROOT=` + quotedRootDir + `
case "${VOL_DIR:-}" in
  "$SAFE_ROOT"/*) ;;
  *) echo "refusing unsafe volume path: ${VOL_DIR:-<empty>}" >&2; exit 1 ;;
esac
rm -rf "$VOL_DIR"
`,
	}, nil
}

func ensureLocalPathConfigMap(ctx context.Context, client kubernetes.Interface, data map[string]string) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-path-config",
			Namespace: localPathNamespace,
			Labels:    localPathLabels(),
		},
		Data: data,
	}
	return createOrUpdateConfigMap(ctx, client, cm)
}

func ensureLocalPathDeployment(ctx context.Context, client kubernetes.Interface, configHash, provisionerImage string) error {
	replicas := int32(1)
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true
	runAsNonRoot := true
	runAsUser := int64(65534)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-path-provisioner",
			Namespace: localPathNamespace,
			Labels:    localPathLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: localPathSelectorLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: localPathLabels(),
					Annotations: map[string]string{
						"casos.io/local-path-config-hash": configHash,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "local-path-provisioner-service-account",
					Tolerations: []corev1.Toleration{
						{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node-role.kubernetes.io/master", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{
						{
							Name:            "local-path-provisioner",
							Image:           provisionerImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command: []string{
								"local-path-provisioner",
								"--debug",
								"start",
								"--config",
								"/etc/config/config.json",
								"--helper-pod-file",
								"/etc/config/helperPod.yaml",
								"--service-account-name",
								"local-path-provisioner-service-account",
								"--provisioner-name",
								localPathProvisionerName,
							},
							Env: []corev1.EnvVar{
								{
									Name: "POD_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
									},
								},
								{
									Name:  "CONFIG_MOUNT_PATH",
									Value: "/etc/config/",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("50m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivilegeEscalation,
								ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
								RunAsNonRoot:             &runAsNonRoot,
								RunAsUser:                &runAsUser,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "config-volume", MountPath: "/etc/config/"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "local-path-config"},
								},
							},
						},
					},
				},
			},
		},
	}
	return reconcileLocalPathDeployment(ctx, client, deployment)
}

func ensureLocalPathStorageClass(ctx context.Context, client kubernetes.Interface) error {
	bindingMode := storagev1.VolumeBindingWaitForFirstConsumer
	allowExpansion := true
	hasDefault, err := hasDefaultStorageClass(ctx, client, localPathStorageClass)
	if err != nil {
		return err
	}
	class := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        localPathStorageClass,
			Labels:      localPathLabels(),
			Annotations: localPathStorageClassAnnotations(!hasDefault),
		},
		Provisioner:          localPathProvisionerName,
		ReclaimPolicy:        ptr(corev1.PersistentVolumeReclaimDelete),
		VolumeBindingMode:    &bindingMode,
		AllowVolumeExpansion: &allowExpansion,
	}
	return createOrPatchStorageClassDefaultAnnotations(ctx, client, class)
}

func localPathLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "local-path-provisioner",
		"app.kubernetes.io/managed-by": "casos",
	}
}

func localPathSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": "local-path-provisioner",
	}
}

func localPathStorageClassAnnotations(isDefault bool) map[string]string {
	if !isDefault {
		return nil
	}
	return map[string]string{
		"storageclass.kubernetes.io/is-default-class":      "true",
		"storageclass.beta.kubernetes.io/is-default-class": "true",
	}
}

func hashConfigData(data map[string]string) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(data[key])
		b.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", sum[:])
}

func createOrUpdateServiceAccount(ctx context.Context, client kubernetes.Interface, sa *corev1.ServiceAccount) error {
	current, err := client.CoreV1().ServiceAccounts(sa.Namespace).Get(ctx, sa.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.CoreV1().ServiceAccounts(sa.Namespace).Create(ctx, sa, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create serviceaccount %s/%s: %w", sa.Namespace, sa.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get serviceaccount %s/%s: %w", sa.Namespace, sa.Name, err)
	}
	sa.Labels = mergeStringMap(current.Labels, sa.Labels)
	sa.Annotations = mergeStringMap(current.Annotations, sa.Annotations)
	sa.Secrets = current.Secrets
	sa.ImagePullSecrets = current.ImagePullSecrets
	sa.AutomountServiceAccountToken = current.AutomountServiceAccountToken
	sa.ResourceVersion = current.ResourceVersion
	if _, err := client.CoreV1().ServiceAccounts(sa.Namespace).Update(ctx, sa, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update serviceaccount %s/%s: %w", sa.Namespace, sa.Name, err)
	}
	return nil
}

func createOrUpdateClusterRole(ctx context.Context, client kubernetes.Interface, role *rbacv1.ClusterRole) error {
	current, err := client.RbacV1().ClusterRoles().Get(ctx, role.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.RbacV1().ClusterRoles().Create(ctx, role, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create clusterrole %s: %w", role.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get clusterrole %s: %w", role.Name, err)
	}
	role.Labels = mergeStringMap(current.Labels, role.Labels)
	role.Annotations = mergeStringMap(current.Annotations, role.Annotations)
	role.ResourceVersion = current.ResourceVersion
	if _, err := client.RbacV1().ClusterRoles().Update(ctx, role, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update clusterrole %s: %w", role.Name, err)
	}
	return nil
}

func createOrUpdateClusterRoleBinding(ctx context.Context, client kubernetes.Interface, binding *rbacv1.ClusterRoleBinding) error {
	current, err := client.RbacV1().ClusterRoleBindings().Get(ctx, binding.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create clusterrolebinding %s: %w", binding.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get clusterrolebinding %s: %w", binding.Name, err)
	}
	binding.Labels = mergeStringMap(current.Labels, binding.Labels)
	binding.Annotations = mergeStringMap(current.Annotations, binding.Annotations)
	binding.ResourceVersion = current.ResourceVersion
	if _, err := client.RbacV1().ClusterRoleBindings().Update(ctx, binding, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update clusterrolebinding %s: %w", binding.Name, err)
	}
	return nil
}

func createOrUpdateConfigMap(ctx context.Context, client kubernetes.Interface, cm *corev1.ConfigMap) error {
	current, err := client.CoreV1().ConfigMaps(cm.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.CoreV1().ConfigMaps(cm.Namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create configmap %s/%s: %w", cm.Namespace, cm.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get configmap %s/%s: %w", cm.Namespace, cm.Name, err)
	}
	cm.Labels = mergeStringMap(current.Labels, cm.Labels)
	cm.Annotations = mergeStringMap(current.Annotations, cm.Annotations)
	cm.ResourceVersion = current.ResourceVersion
	if _, err := client.CoreV1().ConfigMaps(cm.Namespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update configmap %s/%s: %w", cm.Namespace, cm.Name, err)
	}
	return nil
}

func createOrUpdateDeployment(ctx context.Context, client kubernetes.Interface, deployment *appsv1.Deployment) error {
	current, err := client.AppsV1().Deployments(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}
	deployment.Labels = mergeStringMap(current.Labels, deployment.Labels)
	deployment.Annotations = mergeStringMap(current.Annotations, deployment.Annotations)
	deployment.ResourceVersion = current.ResourceVersion
	if _, err := client.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}
	return nil
}

func reconcileLocalPathDeployment(ctx context.Context, client kubernetes.Interface, deployment *appsv1.Deployment) error {
	desiredHash, err := hashLocalPathDeploymentSpec(deployment.Spec)
	if err != nil {
		return fmt.Errorf("hash deployment %s/%s spec: %w", deployment.Namespace, deployment.Name, err)
	}
	current, err := client.AppsV1().Deployments(deployment.Namespace).Get(ctx, deployment.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		deployment.Annotations = mergeStringMap(deployment.Annotations, map[string]string{
			localPathManagedSpecHashAnnotation: desiredHash,
		})
		_, err := client.AppsV1().Deployments(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}

	if current.Annotations[localPathPreserveSpecAnnotation] == "true" {
		updated := current.DeepCopy()
		updated.Labels = mergeStringMap(updated.Labels, deployment.Labels)
		updated.Annotations = mergeStringMap(updated.Annotations, deployment.Annotations)
		if _, err := client.AppsV1().Deployments(updated.Namespace).Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("patch deployment %s/%s metadata: %w", deployment.Namespace, deployment.Name, err)
		}
		return nil
	}

	currentHash, err := hashLocalPathDeploymentSpec(current.Spec)
	if err != nil {
		return fmt.Errorf("hash current deployment %s/%s spec: %w", deployment.Namespace, deployment.Name, err)
	}
	if currentHash == desiredHash && current.Annotations[localPathManagedSpecHashAnnotation] == desiredHash {
		return nil
	}

	deployment.Annotations = mergeStringMap(deployment.Annotations, map[string]string{
		localPathManagedSpecHashAnnotation: desiredHash,
	})
	deployment.ResourceVersion = current.ResourceVersion
	deployment.Labels = mergeStringMap(current.Labels, deployment.Labels)
	deployment.Annotations = mergeStringMap(current.Annotations, deployment.Annotations)
	if _, err := client.AppsV1().Deployments(deployment.Namespace).Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update deployment %s/%s: %w", deployment.Namespace, deployment.Name, err)
	}
	return nil
}

func hashLocalPathDeploymentSpec(spec appsv1.DeploymentSpec) (string, error) {
	managed := struct {
		Replicas *int32
		Selector *metav1.LabelSelector
		Template struct {
			Labels      map[string]string
			Annotations map[string]string
			Spec        struct {
				ServiceAccountName string
				Tolerations        []corev1.Toleration
				Containers         []corev1.Container
				Volumes            []corev1.Volume
			}
		}
	}{
		Replicas: spec.Replicas,
		Selector: spec.Selector,
	}
	managed.Template.Labels = spec.Template.Labels
	managed.Template.Annotations = spec.Template.Annotations
	managed.Template.Spec.ServiceAccountName = spec.Template.Spec.ServiceAccountName
	managed.Template.Spec.Tolerations = spec.Template.Spec.Tolerations
	managed.Template.Spec.Containers = spec.Template.Spec.Containers
	managed.Template.Spec.Volumes = spec.Template.Spec.Volumes
	return hashJSON(managed)
}

func createOrPatchStorageClassDefaultAnnotations(ctx context.Context, client kubernetes.Interface, class *storagev1.StorageClass) error {
	current, err := client.StorageV1().StorageClasses().Get(ctx, class.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.StorageV1().StorageClasses().Create(ctx, class, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create storageclass %s: %w", class.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get storageclass %s: %w", class.Name, err)
	}

	// The StorageClass CRUD UI can change mutable fields such as reclaimPolicy
	// and allowVolumeExpansion. On restart, only reconcile default annotations.
	copied := current.DeepCopy()
	if copied.Annotations == nil {
		copied.Annotations = map[string]string{}
	}
	desiredDefault := class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true"
	if desiredDefault {
		copied.Annotations["storageclass.kubernetes.io/is-default-class"] = "true"
		copied.Annotations["storageclass.beta.kubernetes.io/is-default-class"] = "true"
	} else {
		delete(copied.Annotations, "storageclass.kubernetes.io/is-default-class")
		delete(copied.Annotations, "storageclass.beta.kubernetes.io/is-default-class")
	}
	if _, err := client.StorageV1().StorageClasses().Update(ctx, copied, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update storageclass %s: %w", class.Name, err)
	}
	return nil
}

func mergeStringMap(base map[string]string, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	merged := make(map[string]string, len(base)+len(overlay))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func hashJSON(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:]), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func hasDefaultStorageClass(ctx context.Context, client kubernetes.Interface, ignore string) (bool, error) {
	classes, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("list storageclasses: %w", err)
	}
	for _, class := range classes.Items {
		if class.Name == ignore {
			continue
		}
		if class.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			class.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true" {
			return true, nil
		}
	}
	return false, nil
}

func ptr[T any](value T) *T {
	return &value
}
