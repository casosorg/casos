package server

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	ingressControllerNamespace    = "kube-system"
	ingressControllerName         = "traefik"
	ingressControllerClass        = "traefik"
	ingressControllerID           = "traefik.io/ingress-controller"
	defaultIngressControllerImage = "docker.io/library/traefik:v3.3.4"
)

func ingressControllerLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       ingressControllerName,
		"app.kubernetes.io/managed-by": "casos",
	}
}

func validateIngressControllerOwnership(object metav1.Object) error {
	if object.GetLabels()["app.kubernetes.io/managed-by"] != "casos" {
		return fmt.Errorf("Ingress controller %T %s already exists and is not managed by CasOS", object, object.GetName())
	}
	return nil
}

func ensureIngressController(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	if err := ensureIngressControllerOwnership(ctx, client); err != nil {
		return err
	}
	if err := ensureNamespace(ctx, client, ingressControllerNamespace); err != nil {
		return err
	}
	if err := createOrUpdateServiceAccount(ctx, client, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: ingressControllerNamespace,
			Labels:    ingressControllerLabels(),
		},
	}, validateIngressControllerOwnership); err != nil {
		return err
	}
	if err := ensureIngressControllerClusterRole(ctx, client); err != nil {
		return err
	}
	if err := ensureIngressControllerClusterRoleBinding(ctx, client); err != nil {
		return err
	}
	if err := ensureIngressClass(ctx, client); err != nil {
		return err
	}
	if err := ensureIngressControllerService(ctx, client); err != nil {
		return err
	}
	return ensureIngressControllerDeployment(ctx, client, cfg)
}

func cleanupIngressController(ctx context.Context, client kubernetes.Interface) error {
	resources := []struct {
		kind   string
		get    func() (metav1.Object, error)
		delete func() error
	}{
		{kind: "Deployment", get: func() (metav1.Object, error) {
			return client.AppsV1().Deployments(ingressControllerNamespace).Get(ctx, ingressControllerName, metav1.GetOptions{})
		}, delete: func() error {
			return client.AppsV1().Deployments(ingressControllerNamespace).Delete(ctx, ingressControllerName, metav1.DeleteOptions{})
		}},
		{kind: "Service", get: func() (metav1.Object, error) {
			return client.CoreV1().Services(ingressControllerNamespace).Get(ctx, ingressControllerName, metav1.GetOptions{})
		}, delete: func() error {
			return client.CoreV1().Services(ingressControllerNamespace).Delete(ctx, ingressControllerName, metav1.DeleteOptions{})
		}},
		{kind: "IngressClass", get: func() (metav1.Object, error) {
			return client.NetworkingV1().IngressClasses().Get(ctx, ingressControllerClass, metav1.GetOptions{})
		}, delete: func() error {
			return client.NetworkingV1().IngressClasses().Delete(ctx, ingressControllerClass, metav1.DeleteOptions{})
		}},
		{kind: "ClusterRoleBinding", get: func() (metav1.Object, error) {
			return client.RbacV1().ClusterRoleBindings().Get(ctx, "casos:traefik-ingress-controller", metav1.GetOptions{})
		}, delete: func() error {
			return client.RbacV1().ClusterRoleBindings().Delete(ctx, "casos:traefik-ingress-controller", metav1.DeleteOptions{})
		}},
		{kind: "ClusterRole", get: func() (metav1.Object, error) {
			return client.RbacV1().ClusterRoles().Get(ctx, "casos:traefik-ingress-controller", metav1.GetOptions{})
		}, delete: func() error {
			return client.RbacV1().ClusterRoles().Delete(ctx, "casos:traefik-ingress-controller", metav1.DeleteOptions{})
		}},
		{kind: "ServiceAccount", get: func() (metav1.Object, error) {
			return client.CoreV1().ServiceAccounts(ingressControllerNamespace).Get(ctx, ingressControllerName, metav1.GetOptions{})
		}, delete: func() error {
			return client.CoreV1().ServiceAccounts(ingressControllerNamespace).Delete(ctx, ingressControllerName, metav1.DeleteOptions{})
		}},
	}
	errs := make([]error, 0)
	for _, resource := range resources {
		object, err := resource.get()
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("get Ingress controller %s for cleanup: %w", resource.kind, err))
			continue
		}
		if object.GetLabels()["app.kubernetes.io/managed-by"] != "casos" {
			continue
		}
		if err := resource.delete(); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete Ingress controller %s %s: %w", resource.kind, object.GetName(), err))
		}
	}
	return errors.Join(errs...)
}

func ensureIngressControllerOwnership(ctx context.Context, client kubernetes.Interface) error {
	checks := []struct {
		kind string
		get  func() (metav1.Object, error)
	}{
		{kind: "ServiceAccount", get: func() (metav1.Object, error) {
			return client.CoreV1().ServiceAccounts(ingressControllerNamespace).Get(ctx, ingressControllerName, metav1.GetOptions{})
		}},
		{kind: "Service", get: func() (metav1.Object, error) {
			return client.CoreV1().Services(ingressControllerNamespace).Get(ctx, ingressControllerName, metav1.GetOptions{})
		}},
		{kind: "Deployment", get: func() (metav1.Object, error) {
			return client.AppsV1().Deployments(ingressControllerNamespace).Get(ctx, ingressControllerName, metav1.GetOptions{})
		}},
		{kind: "ClusterRole", get: func() (metav1.Object, error) {
			return client.RbacV1().ClusterRoles().Get(ctx, "casos:traefik-ingress-controller", metav1.GetOptions{})
		}},
		{kind: "ClusterRoleBinding", get: func() (metav1.Object, error) {
			return client.RbacV1().ClusterRoleBindings().Get(ctx, "casos:traefik-ingress-controller", metav1.GetOptions{})
		}},
		{kind: "IngressClass", get: func() (metav1.Object, error) {
			return client.NetworkingV1().IngressClasses().Get(ctx, ingressControllerClass, metav1.GetOptions{})
		}},
	}
	for _, check := range checks {
		object, err := check.get()
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("check Ingress controller %s ownership: %w", check.kind, err)
		}
		if err := validateIngressControllerOwnership(object); err != nil {
			return fmt.Errorf("check Ingress controller %s ownership: %w", check.kind, err)
		}
	}
	return nil
}

func ensureIngressControllerClusterRole(ctx context.Context, client kubernetes.Interface) error {
	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "casos:traefik-ingress-controller",
			Labels: ingressControllerLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints", "nodes", "secrets", "namespaces"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"discovery.k8s.io"},
				Resources: []string{"endpointslices"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses", "ingressclasses"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "update", "patch"},
			},
			{
				APIGroups: []string{"networking.k8s.io"},
				Resources: []string{"ingresses/status"},
				Verbs:     []string{"update", "patch"},
			},
		},
	}
	return createOrUpdateClusterRole(ctx, client, role, validateIngressControllerOwnership)
}

func ensureIngressControllerClusterRoleBinding(ctx context.Context, client kubernetes.Interface) error {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "casos:traefik-ingress-controller",
			Labels: ingressControllerLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "casos:traefik-ingress-controller",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      ingressControllerName,
			Namespace: ingressControllerNamespace,
		}},
	}
	return createOrUpdateClusterRoleBinding(ctx, client, binding, validateIngressControllerOwnership)
}

func ensureIngressClass(ctx context.Context, client kubernetes.Interface) error {
	classes := client.NetworkingV1().IngressClasses()
	defaultClassExists := false
	existingClasses, err := classes.List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list IngressClasses: %w", err)
	}
	for i := range existingClasses.Items {
		class := &existingClasses.Items[i]
		if class.Name != ingressControllerClass && class.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
			defaultClassExists = true
			break
		}
	}
	desired := &networkingv1.IngressClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressControllerClass,
			Labels:      ingressControllerLabels(),
			Annotations: map[string]string{},
		},
		Spec: networkingv1.IngressClassSpec{Controller: ingressControllerID},
	}
	if !defaultClassExists {
		desired.Annotations["ingressclass.kubernetes.io/is-default-class"] = "true"
	}
	current, err := classes.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := classes.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create IngressClass %s: %w", desired.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get IngressClass %s: %w", desired.Name, err)
	}
	if current.Spec.Controller != desired.Spec.Controller {
		return fmt.Errorf("IngressClass %s is owned by controller %s", desired.Name, current.Spec.Controller)
	}
	if current.Labels["app.kubernetes.io/managed-by"] != "casos" {
		return fmt.Errorf("IngressClass %s already exists and is not managed by CasOS", desired.Name)
	}
	desired.Labels = mergeStringMap(current.Labels, desired.Labels)
	desired.Annotations = mergeStringMap(current.Annotations, desired.Annotations)
	if defaultClassExists {
		delete(desired.Annotations, "ingressclass.kubernetes.io/is-default-class")
	}
	if apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) &&
		apiequality.Semantic.DeepEqual(current.Annotations, desired.Annotations) &&
		apiequality.Semantic.DeepEqual(current.Spec, desired.Spec) {
		return nil
	}
	desired.ResourceVersion = current.ResourceVersion
	if _, err := classes.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update IngressClass %s: %w", desired.Name, err)
	}
	return nil
}

func ensureIngressControllerService(ctx context.Context, client kubernetes.Interface) error {
	desired := buildIngressControllerService()
	services := client.CoreV1().Services(ingressControllerNamespace)
	current, err := services.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := services.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create Ingress controller Service: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get Ingress controller Service: %w", err)
	}
	if current.Labels["app.kubernetes.io/managed-by"] != "casos" {
		return fmt.Errorf("Ingress controller Service %s/%s already exists and is not managed by CasOS", ingressControllerNamespace, ingressControllerName)
	}
	desired.Labels = mergeStringMap(current.Labels, desired.Labels)
	desired.Annotations = mergeStringMap(current.Annotations, desired.Annotations)
	desired.Spec.ClusterIP = current.Spec.ClusterIP
	desired.Spec.ClusterIPs = current.Spec.ClusterIPs
	desired.Spec.IPFamilies = current.Spec.IPFamilies
	desired.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
	desired.Spec.ExternalIPs = current.Spec.ExternalIPs
	desired.Spec.SessionAffinity = current.Spec.SessionAffinity
	desired.Spec.InternalTrafficPolicy = current.Spec.InternalTrafficPolicy
	desired.Spec.LoadBalancerIP = current.Spec.LoadBalancerIP
	desired.Spec.LoadBalancerClass = current.Spec.LoadBalancerClass
	desired.Spec.LoadBalancerSourceRanges = current.Spec.LoadBalancerSourceRanges
	desired.Spec.AllocateLoadBalancerNodePorts = current.Spec.AllocateLoadBalancerNodePorts
	desired.Spec.HealthCheckNodePort = current.Spec.HealthCheckNodePort
	for i := range desired.Spec.Ports {
		for _, currentPort := range current.Spec.Ports {
			if desired.Spec.Ports[i].Name == currentPort.Name {
				desired.Spec.Ports[i].NodePort = currentPort.NodePort
				break
			}
		}
	}
	if apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) &&
		apiequality.Semantic.DeepEqual(current.Annotations, desired.Annotations) &&
		apiequality.Semantic.DeepEqual(current.Spec, desired.Spec) {
		return nil
	}
	desired.ResourceVersion = current.ResourceVersion
	if _, err := services.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update Ingress controller Service: %w", err)
	}
	return nil
}

func buildIngressControllerService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: ingressControllerNamespace,
			Labels:    ingressControllerLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type:                  corev1.ServiceTypeLoadBalancer,
			Selector:              ingressControllerLabels(),
			ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeCluster,
			LoadBalancerClass:     ptr(serviceLBClass),
			Ports: []corev1.ServicePort{
				{Name: "web", Port: 80, TargetPort: intstr.FromInt(8000), Protocol: corev1.ProtocolTCP},
				{Name: "websecure", Port: 443, TargetPort: intstr.FromInt(8443), Protocol: corev1.ProtocolTCP},
			},
		},
	}
}

func ensureIngressControllerDeployment(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	return createOrUpdateDeployment(ctx, client, buildIngressControllerDeployment(cfg), validateIngressControllerOwnership)
}

func buildIngressControllerDeployment(cfg Config) *appsv1.Deployment {
	replicas := int32(1)
	image := cfg.IngressControllerImage
	if image == "" {
		image = defaultIngressControllerImage
	}
	labels := ingressControllerLabels()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressControllerName,
			Namespace: ingressControllerNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: ingressControllerName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   ptr(true),
						RunAsUser:      ptr(int64(65532)),
						RunAsGroup:     ptr(int64(65532)),
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Tolerations: []corev1.Toleration{
						{Key: "CriticalAddonsOnly", Operator: corev1.TolerationOpExists},
						{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "node-role.kubernetes.io/master", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Key: "casos.io/bootstrap", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: []corev1.Container{{
						Name:            ingressControllerName,
						Image:           image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args: []string{
							"--providers.kubernetesingress=true",
							"--providers.kubernetesingress.ingressclass=" + ingressControllerClass,
							"--providers.kubernetesingress.ingressendpoint.publishedservice=" + ingressControllerNamespace + "/" + ingressControllerName,
							"--entrypoints.web.address=:8000",
							"--entrypoints.websecure.address=:8443",
							"--ping=true",
						},
						Ports: []corev1.ContainerPort{
							{Name: "web", ContainerPort: 8000, Protocol: corev1.ProtocolTCP},
							{Name: "websecure", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
							{Name: "ping", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
								Path: "/ping", Port: intstr.FromInt(8080), Scheme: corev1.URISchemeHTTP,
							}},
							InitialDelaySeconds: 5, PeriodSeconds: 10, TimeoutSeconds: 5, FailureThreshold: 6,
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler:        corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/ping", Port: intstr.FromInt(8080), Scheme: corev1.URISchemeHTTP}},
							InitialDelaySeconds: 15, PeriodSeconds: 10, TimeoutSeconds: 5, FailureThreshold: 6,
						},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: ptr(false),
							ReadOnlyRootFilesystem:   ptr(true),
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "tmp", MountPath: "/tmp"}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
							Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
						},
					}},
					Volumes: []corev1.Volume{{Name: "tmp", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}},
				},
			},
		},
	}
	return deployment
}
