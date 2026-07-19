package server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	clusterDNSNamespace = "kube-system"
	clusterDNSName      = "coredns"
	clusterDNSServiceIP = "10.43.0.10"
	// coreDNSRolloutRev forces a rollout when the managed CoreDNS pod template changes.
	coreDNSRolloutRev = "4"
)

func ensureClusterDNS(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	if err := ensureNamespace(ctx, client, clusterDNSNamespace); err != nil {
		return err
	}
	managed, err := isCoreDNSServiceManaged(ctx, client)
	if err != nil {
		return err
	}
	if !managed {
		logrus.Warn("Skipping CoreDNS bootstrap because kube-system/kube-dns is not managed by casos")
		return nil
	}
	if err := ensureCoreDNSServiceAccount(ctx, client); err != nil {
		return err
	}
	if err := ensureCoreDNSClusterRole(ctx, client); err != nil {
		return err
	}
	if err := ensureCoreDNSClusterRoleBinding(ctx, client); err != nil {
		return err
	}
	if err := ensureCoreDNSConfigMap(ctx, client); err != nil {
		return err
	}
	if err := ensureCoreDNSService(ctx, client); err != nil {
		return err
	}
	return ensureCoreDNSDeployment(ctx, client, cfg)
}

func ensureCoreDNSServiceAccount(ctx context.Context, client kubernetes.Interface) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterDNSName,
			Namespace: clusterDNSNamespace,
			Labels:    coreDNSLabels(),
		},
	}
	return createOrUpdateServiceAccount(ctx, client, sa)
}

func ensureCoreDNSClusterRole(ctx context.Context, client kubernetes.Interface) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "system:coredns",
			Labels: coreDNSLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "services", "pods", "namespaces"},
				Verbs:     []string{"list", "watch"},
			},
			{
				APIGroups: []string{"discovery.k8s.io"},
				Resources: []string{"endpointslices"},
				Verbs:     []string{"list", "watch"},
			},
		},
	}
	return createOrUpdateClusterRole(ctx, client, role)
}

func ensureCoreDNSClusterRoleBinding(ctx context.Context, client kubernetes.Interface) error {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "system:coredns",
			Labels: coreDNSLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:coredns",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      clusterDNSName,
				Namespace: clusterDNSNamespace,
			},
		},
	}
	return createOrUpdateClusterRoleBinding(ctx, client, binding)
}

func ensureCoreDNSConfigMap(ctx context.Context, client kubernetes.Interface) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterDNSName,
			Namespace: clusterDNSNamespace,
			Labels:    coreDNSLabels(),
		},
		Data: map[string]string{
			"Corefile": `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
        ttl 30
    }
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}
`,
		},
	}
	return createOrUpdateConfigMap(ctx, client, cm)
}

func ensureCoreDNSService(ctx context.Context, client kubernetes.Interface) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-dns",
			Namespace: clusterDNSNamespace,
			Labels:    coreDNSLabels(),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: clusterDNSServiceIP,
			Selector:  coreDNSSelectorLabels(),
			Ports: []corev1.ServicePort{
				{Name: "dns", Port: 53, Protocol: corev1.ProtocolUDP, TargetPort: intstr.FromInt(53)},
				{Name: "dns-tcp", Port: 53, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(53)},
				{Name: "metrics", Port: 9153, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt(9153)},
			},
		},
	}
	current, err := client.CoreV1().Services(clusterDNSNamespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if err == nil && current.Spec.ClusterIP != "" && current.Spec.ClusterIP != clusterDNSServiceIP {
		if err := client.CoreV1().Services(clusterDNSNamespace).Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("replace CoreDNS service %s/%s: %w", clusterDNSNamespace, svc.Name, err)
		}
		if err := waitForServiceDeleted(ctx, client, clusterDNSNamespace, svc.Name); err != nil {
			return fmt.Errorf("wait to replace CoreDNS service %s/%s: %w", clusterDNSNamespace, svc.Name, err)
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("get CoreDNS service %s/%s: %w", clusterDNSNamespace, svc.Name, err)
	}
	return createOrUpdateService(ctx, client, svc)
}

func waitForServiceDeleted(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	return wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		_, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

func ensureCoreDNSDeployment(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	replicas := int32(1)
	maxSurge := intstr.FromInt(1)
	maxUnavailable := intstr.FromInt(0)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterDNSName,
			Namespace: clusterDNSNamespace,
			Labels:    coreDNSLabels(),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: coreDNSSelectorLabels()},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: coreDNSLabels(),
					Annotations: map[string]string{
						"prometheus.io/port":   "9153",
						"prometheus.io/scrape": "true",
						"casos.io/rollout-rev": coreDNSRolloutRev,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: clusterDNSName,
					DNSPolicy:          corev1.DNSDefault,
					Tolerations: []corev1.Toleration{
						{Key: "CriticalAddonsOnly", Operator: corev1.TolerationOpExists},
						{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node-role.kubernetes.io/master", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "casos.io/bootstrap", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{
						{
							Name:            clusterDNSName,
							Image:           cfg.CoreDNSImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            []string{"-conf", "/etc/coredns/Corefile"},
							Env:             coreDNSEnv(cfg),
							Ports: []corev1.ContainerPort{
								{Name: "dns", ContainerPort: 53, Protocol: corev1.ProtocolUDP},
								{Name: "dns-tcp", ContainerPort: 53, Protocol: corev1.ProtocolTCP},
								{Name: "metrics", ContainerPort: 9153, Protocol: corev1.ProtocolTCP},
								{Name: "health", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
								{Name: "ready", ContainerPort: 8181, Protocol: corev1.ProtocolTCP},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/health",
										Port:   intstr.FromInt(8080),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/ready",
										Port:   intstr.FromInt(8181),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 10,
								TimeoutSeconds:      5,
								PeriodSeconds:       10,
								FailureThreshold:    3,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("70Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("170Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr(false),
								ReadOnlyRootFilesystem:   ptr(true),
								RunAsNonRoot:             ptr(true),
								RunAsUser:                ptr(int64(65534)),
								RunAsGroup:               ptr(int64(65534)),
								Capabilities: &corev1.Capabilities{
									Add:  []corev1.Capability{"NET_BIND_SERVICE"},
									Drop: []corev1.Capability{"ALL"},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "config-volume", MountPath: "/etc/coredns", ReadOnly: true},
								{Name: "tmp", MountPath: "/tmp"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config-volume",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: clusterDNSName},
									Items: []corev1.KeyToPath{
										{Key: "Corefile", Path: "Corefile"},
									},
								},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
	return createOrUpdateDeployment(ctx, client, deployment)
}

func coreDNSEnv(cfg Config) []corev1.EnvVar {
	if cfg.AdvertiseAddress == "" || cfg.ApiserverPort <= 0 {
		return nil
	}
	return []corev1.EnvVar{
		{Name: "KUBERNETES_SERVICE_HOST", Value: cfg.AdvertiseAddress},
		{Name: "KUBERNETES_SERVICE_PORT", Value: strconv.Itoa(cfg.ApiserverPort)},
		{Name: "KUBERNETES_SERVICE_PORT_HTTPS", Value: strconv.Itoa(cfg.ApiserverPort)},
	}
}

func coreDNSLabels() map[string]string {
	labels := coreDNSSelectorLabels()
	labels["k8s-app"] = "kube-dns"
	return labels
}

func coreDNSSelectorLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "coredns",
		"app.kubernetes.io/managed-by": "casos",
	}
}

func isCoreDNSServiceManaged(ctx context.Context, client kubernetes.Interface) (bool, error) {
	current, err := client.CoreV1().Services(clusterDNSNamespace).Get(ctx, "kube-dns", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("get service %s/kube-dns: %w", clusterDNSNamespace, err)
	}
	return current.Labels["app.kubernetes.io/managed-by"] == "casos", nil
}

func createOrUpdateService(ctx context.Context, client kubernetes.Interface, svc *corev1.Service) error {
	current, err := client.CoreV1().Services(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = client.CoreV1().Services(svc.Namespace).Create(ctx, svc, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create service %s/%s: %w", svc.Namespace, svc.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	svc.Labels = mergeStringMap(current.Labels, svc.Labels)
	svc.Annotations = mergeStringMap(current.Annotations, svc.Annotations)
	svc.ResourceVersion = current.ResourceVersion
	svc.Spec.Type = current.Spec.Type
	if current.Spec.ClusterIP != "" {
		svc.Spec.ClusterIP = current.Spec.ClusterIP
	}
	if len(current.Spec.ClusterIPs) > 0 {
		svc.Spec.ClusterIPs = current.Spec.ClusterIPs
	}
	for i, currentPort := range current.Spec.Ports {
		for j, desiredPort := range svc.Spec.Ports {
			if currentPort.Name == desiredPort.Name &&
				currentPort.Port == desiredPort.Port &&
				currentPort.Protocol == desiredPort.Protocol &&
				currentPort.NodePort != 0 {
				svc.Spec.Ports[j].NodePort = current.Spec.Ports[i].NodePort
				break
			}
		}
	}
	if len(current.Spec.IPFamilies) > 0 {
		svc.Spec.IPFamilies = current.Spec.IPFamilies
	}
	if current.Spec.IPFamilyPolicy != nil {
		svc.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
	}
	if current.Spec.HealthCheckNodePort != 0 {
		svc.Spec.HealthCheckNodePort = current.Spec.HealthCheckNodePort
	}
	svc.Spec.ExternalIPs = current.Spec.ExternalIPs
	svc.Spec.SessionAffinity = current.Spec.SessionAffinity
	svc.Spec.ExternalTrafficPolicy = current.Spec.ExternalTrafficPolicy
	svc.Spec.InternalTrafficPolicy = current.Spec.InternalTrafficPolicy
	svc.Spec.LoadBalancerIP = current.Spec.LoadBalancerIP
	svc.Spec.LoadBalancerClass = current.Spec.LoadBalancerClass
	svc.Spec.LoadBalancerSourceRanges = current.Spec.LoadBalancerSourceRanges
	svc.Spec.AllocateLoadBalancerNodePorts = current.Spec.AllocateLoadBalancerNodePorts
	if apiequality.Semantic.DeepEqual(current.Labels, svc.Labels) &&
		apiequality.Semantic.DeepEqual(current.Annotations, svc.Annotations) &&
		apiequality.Semantic.DeepEqual(current.Spec, svc.Spec) {
		return nil
	}
	if _, err := client.CoreV1().Services(svc.Namespace).Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update service %s/%s: %w", svc.Namespace, svc.Name, err)
	}
	return nil
}
