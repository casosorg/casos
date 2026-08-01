package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	coordinationclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/retry"
	appsinternal "k8s.io/kubernetes/pkg/apis/apps/v1"

	"github.com/sirupsen/logrus"
)

const (
	serviceLBNamespace             = "kube-system"
	serviceLBManagedIPsAnnotation  = "casos.io/service-lb-managed-ips"
	serviceLBDisabledAnnotation    = "casos.io/service-lb-disabled"
	serviceLBManagedByLabel        = "app.kubernetes.io/managed-by"
	serviceLBComponentLabel        = "app.kubernetes.io/component"
	serviceLBServiceNameLabel      = "casos.io/service-lb-service-name"
	serviceLBServiceNamespaceLabel = "casos.io/service-lb-service-namespace"
	serviceLBServiceUIDLabel       = "casos.io/service-lb-service-uid"
	serviceLBClass                 = "casos.io/service-lb"
	serviceLBLeaderLease           = "casos-service-lb"
	defaultServiceLBImage          = "docker.io/rancher/klipper-lb:v0.4.17"
)

type serviceLBLeaderElector interface {
	Run(context.Context)
}

type serviceLBLeaderElectorFactory func() (serviceLBLeaderElector, error)

// StartServiceLB starts the built-in bare-metal LoadBalancer controller. Each
// managed Service gets a hostPort proxy DaemonSet; only nodes with Ready proxy
// Pods are published in status.loadBalancer. User-owned Service spec is not
// used as the steady-state data plane.
func StartServiceLB(ctx context.Context, cfg *rest.Config, srvCfg Config) error {
	if cfg == nil {
		return fmt.Errorf("apiserver rest config is required")
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("service load balancer client: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	coordination, err := coordinationclient.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("service load balancer coordination client: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("service load balancer identity: %w", err)
	}
	identity := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	newElector := func() (serviceLBLeaderElector, error) {
		return leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
			Lock: &resourcelock.LeaseLock{
				LeaseMeta:  metav1.ObjectMeta{Name: serviceLBLeaderLease, Namespace: serviceLBNamespace},
				Client:     coordination,
				LockConfig: resourcelock.ResourceLockConfig{Identity: identity},
			},
			LeaseDuration:   15 * time.Second,
			RenewDeadline:   10 * time.Second,
			RetryPeriod:     2 * time.Second,
			ReleaseOnCancel: true,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(leaderCtx context.Context) {
					runServiceLB(leaderCtx, client, srvCfg.ServiceLBImage)
				},
				OnStoppedLeading: func() {},
			},
		})
	}
	if _, err := newElector(); err != nil {
		return fmt.Errorf("service load balancer leader election: %w", err)
	}
	go runServiceLBLeaderElection(ctx, 2*time.Second, newElector)
	return nil
}

func runServiceLBLeaderElection(ctx context.Context, retryDelay time.Duration, newElector serviceLBLeaderElectorFactory) {
	for ctx.Err() == nil {
		elector, err := newElector()
		if err != nil {
			logrus.Errorf("create service load balancer leader elector: %v", err)
		} else {
			elector.Run(ctx)
			if ctx.Err() != nil {
				return
			}
			logrus.Warn("service load balancer leadership lost; rejoining election")
		}
		if !waitForServiceLBRetry(ctx, retryDelay) {
			return
		}
	}
}

func waitForServiceLBRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func runServiceLB(ctx context.Context, client kubernetes.Interface, image string) {
	const interval = 5 * time.Second
	for {
		if err := reconcileServiceLBWithImage(ctx, client, image); err != nil && ctx.Err() == nil {
			logrus.Warnf("service load balancer reconciliation failed: %v", err)
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func reconcileServiceLBWithImage(ctx context.Context, client kubernetes.Interface, image string) error {
	if strings.TrimSpace(image) == "" {
		image = defaultServiceLBImage
	}
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	services, err := client.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list LoadBalancer services: %w", err)
	}
	existingDaemonSets, err := listServiceLBDaemonSets(ctx, client)
	if err != nil {
		return err
	}
	existingDaemonSetNames := make(map[string]struct{}, len(existingDaemonSets))
	for i := range existingDaemonSets {
		existingDaemonSetNames[existingDaemonSets[i].Name] = struct{}{}
	}

	desiredDaemonSets := make(map[string]struct{})
	reconcileErrors := make([]error, 0)
	for i := range services.Items {
		service := &services.Items[i]
		if !serviceLBManages(service) {
			_, hadDaemonSet := existingDaemonSetNames[serviceLBDaemonSetName(service)]
			if hadDaemonSet || serviceLBWasExplicitlyOwned(service) || service.Annotations[serviceLBManagedIPsAnnotation] != "" {
				if err := cleanupLoadBalancerService(ctx, client, service); err != nil {
					reconcileErrors = append(reconcileErrors, fmt.Errorf("clean up LoadBalancer service %s/%s: %w", service.Namespace, service.Name, err))
				}
			}
			continue
		}

		desiredDaemonSets[serviceLBDaemonSetName(service)] = struct{}{}
		desired, err := buildServiceLBDaemonSet(service, image)
		if err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("build ServiceLB DaemonSet for %s/%s: %w", service.Namespace, service.Name, err))
			continue
		}
		if err := createOrUpdateServiceLBDaemonSet(ctx, client, desired); err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("reconcile ServiceLB DaemonSet for %s/%s: %w", service.Namespace, service.Name, err))
			continue
		}
		nodeIPs, err := serviceLBReadyProxyNodeIPs(ctx, client, service, nodes.Items)
		if err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("select ServiceLB nodes for %s/%s: %w", service.Namespace, service.Name, err))
			continue
		}
		if err := reconcileLoadBalancerService(ctx, client, service, nodeIPs); err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("reconcile LoadBalancer status for %s/%s: %w", service.Namespace, service.Name, err))
		}
	}
	if err := cleanupOrphanedServiceLBDaemonSets(ctx, client, desiredDaemonSets); err != nil {
		reconcileErrors = append(reconcileErrors, err)
	}
	return errors.Join(reconcileErrors...)
}

func cleanupServiceLB(ctx context.Context, client kubernetes.Interface) error {
	daemonSets, err := listServiceLBDaemonSets(ctx, client)
	if err != nil {
		return err
	}
	daemonSetNames := make(map[string]struct{}, len(daemonSets))
	for i := range daemonSets {
		daemonSetNames[daemonSets[i].Name] = struct{}{}
	}
	services, err := client.CoreV1().Services(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list services for ServiceLB cleanup: %w", err)
	}
	errs := make([]error, 0)
	for i := range services.Items {
		service := &services.Items[i]
		_, hadDaemonSet := daemonSetNames[serviceLBDaemonSetName(service)]
		if !hadDaemonSet && !serviceLBWasExplicitlyOwned(service) && service.Annotations[serviceLBManagedIPsAnnotation] == "" {
			continue
		}
		if err := cleanupLoadBalancerService(ctx, client, service); err != nil {
			errs = append(errs, fmt.Errorf("clean up ServiceLB state for %s/%s: %w", service.Namespace, service.Name, err))
		}
	}
	for i := range daemonSets {
		if err := client.AppsV1().DaemonSets(serviceLBNamespace).Delete(ctx, daemonSets[i].Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete ServiceLB DaemonSet %s/%s: %w", serviceLBNamespace, daemonSets[i].Name, err))
		}
	}
	return errors.Join(errs...)
}

func serviceLBManages(service *corev1.Service) bool {
	if service == nil || service.Spec.Type != corev1.ServiceTypeLoadBalancer || service.Annotations[serviceLBDisabledAnnotation] == "true" {
		return false
	}
	if service.Spec.LoadBalancerClass == nil {
		return true
	}
	return *service.Spec.LoadBalancerClass == serviceLBClass
}

func serviceLBWasExplicitlyOwned(service *corev1.Service) bool {
	return service != nil && service.Spec.LoadBalancerClass != nil && *service.Spec.LoadBalancerClass == serviceLBClass
}

func serviceLBDaemonSetName(service *corev1.Service) string {
	identity := string(service.UID)
	if identity == "" {
		identity = service.Namespace + "/" + service.Name
	}
	sum := sha256.Sum256([]byte(identity))
	suffix := hex.EncodeToString(sum[:6])
	name := strings.Trim(service.Name, "-")
	const prefix = "svclb-"
	maxNameLen := 63 - len(prefix) - 1 - len(suffix)
	if len(name) > maxNameLen {
		name = strings.TrimRight(name[:maxNameLen], "-")
	}
	if name == "" {
		name = "service"
	}
	return prefix + name + "-" + suffix
}

func serviceLBLabels(service *corev1.Service) map[string]string {
	return map[string]string{
		serviceLBManagedByLabel:        "casos",
		serviceLBComponentLabel:        "service-lb",
		serviceLBServiceNameLabel:      service.Name,
		serviceLBServiceNamespaceLabel: service.Namespace,
		serviceLBServiceUIDLabel:       string(service.UID),
	}
}

func buildServiceLBDaemonSet(service *corev1.Service, image string) (*appsv1.DaemonSet, error) {
	if service == nil {
		return nil, fmt.Errorf("service is required")
	}
	if len(service.Spec.Ports) == 0 {
		return nil, fmt.Errorf("service has no ports")
	}
	if strings.TrimSpace(image) == "" {
		image = defaultServiceLBImage
	}
	name := serviceLBDaemonSetName(service)
	labels := serviceLBLabels(service)
	labels["app.kubernetes.io/name"] = name
	selectorLabels := map[string]string{"app.kubernetes.io/name": name}
	maxUnavailable := intstr.FromInt(1)
	automountToken := false
	sourceRanges := append([]string{}, service.Spec.LoadBalancerSourceRanges...)
	if len(sourceRanges) == 0 {
		sourceRanges = []string{"0.0.0.0/0"}
		if serviceUsesIPFamily(service, corev1.IPv6Protocol) {
			sourceRanges = append(sourceRanges, "::/0")
		}
	}
	sort.Strings(sourceRanges)

	podSecurityContext := &corev1.PodSecurityContext{}
	if serviceUsesIPFamily(service, corev1.IPv4Protocol) {
		podSecurityContext.Sysctls = append(podSecurityContext.Sysctls, corev1.Sysctl{Name: "net.ipv4.ip_forward", Value: "1"})
	}
	if serviceUsesIPFamily(service, corev1.IPv6Protocol) {
		podSecurityContext.Sysctls = append(podSecurityContext.Sysctls, corev1.Sysctl{Name: "net.ipv6.conf.all.forwarding", Value: "1"})
	}

	containers := make([]corev1.Container, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		if port.Port <= 0 {
			return nil, fmt.Errorf("service port %q has invalid port %d", port.Name, port.Port)
		}
		container := corev1.Container{
			Name:            fmt.Sprintf("lb-%s-%d", strings.ToLower(string(port.Protocol)), port.Port),
			Image:           image,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Ports: []corev1.ContainerPort{{
				Name:          fmt.Sprintf("lb-%s-%d", strings.ToLower(string(port.Protocol)), port.Port),
				ContainerPort: port.Port,
				HostPort:      port.Port,
				Protocol:      port.Protocol,
			}},
			Env: []corev1.EnvVar{
				{Name: "SRC_PORT", Value: strconv.Itoa(int(port.Port))},
				{Name: "SRC_RANGES", Value: strings.Join(sourceRanges, ",")},
				{Name: "DEST_PROTO", Value: string(port.Protocol)},
			},
			SecurityContext: &corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN"}}},
		}
		if service.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal {
			if port.NodePort == 0 {
				return nil, fmt.Errorf("service port %q requires a NodePort for externalTrafficPolicy=Local", port.Name)
			}
			container.Env = append(
				container.Env,
				corev1.EnvVar{Name: "DEST_PORT", Value: strconv.Itoa(int(port.NodePort))},
				corev1.EnvVar{Name: "DEST_IPS", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.hostIPs"}}},
			)
		} else {
			clusterIPs := serviceClusterIPs(service)
			if len(clusterIPs) == 0 {
				return nil, fmt.Errorf("service has no routable ClusterIP")
			}
			container.Env = append(
				container.Env,
				corev1.EnvVar{Name: "DEST_PORT", Value: strconv.Itoa(int(port.Port))},
				corev1.EnvVar{Name: "DEST_IPS", Value: strings.Join(clusterIPs, ",")},
			)
		}
		containers = append(containers, container)
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: serviceLBNamespace, Labels: labels},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type:          appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxUnavailable: &maxUnavailable},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: mergeStringMap(labels, selectorLabels)},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &automountToken,
					PriorityClassName:            "system-node-critical",
					SecurityContext:              podSecurityContext,
					Affinity: &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{
							{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.NodeSelectorOpDoesNotExist},
							{Key: "node-role.kubernetes.io/master", Operator: corev1.NodeSelectorOpDoesNotExist},
						}}},
					}}},
					Tolerations: []corev1.Toleration{
						{Key: "CriticalAddonsOnly", Operator: corev1.TolerationOpExists},
						{Key: "casos.io/bootstrap", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
					},
					Containers: containers,
				},
			},
		},
	}, nil
}

func createOrUpdateServiceLBDaemonSet(ctx context.Context, client kubernetes.Interface, desired *appsv1.DaemonSet) error {
	daemonSets := client.AppsV1().DaemonSets(desired.Namespace)
	current, err := daemonSets.Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = daemonSets.Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if current.Labels[serviceLBManagedByLabel] != "casos" || current.Labels[serviceLBComponentLabel] != "service-lb" {
		return fmt.Errorf("DaemonSet %s/%s exists and is not managed by CasOS", desired.Namespace, desired.Name)
	}
	for _, key := range []string{serviceLBServiceNameLabel, serviceLBServiceNamespaceLabel, serviceLBServiceUIDLabel} {
		if current.Labels[key] != desired.Labels[key] {
			return fmt.Errorf("DaemonSet %s/%s belongs to a different Service", desired.Namespace, desired.Name)
		}
	}
	desired.Labels = mergeStringMap(current.Labels, desired.Labels)
	desired.Annotations = mergeStringMap(current.Annotations, desired.Annotations)
	currentDefaulted := current.DeepCopy()
	desiredDefaulted := desired.DeepCopy()
	appsinternal.SetObjectDefaults_DaemonSet(currentDefaulted)
	appsinternal.SetObjectDefaults_DaemonSet(desiredDefaulted)
	if apiequality.Semantic.DeepEqual(currentDefaulted.Labels, desiredDefaulted.Labels) &&
		apiequality.Semantic.DeepEqual(currentDefaulted.Annotations, desiredDefaulted.Annotations) &&
		apiequality.Semantic.DeepEqual(currentDefaulted.Spec, desiredDefaulted.Spec) {
		return nil
	}
	desired.ResourceVersion = current.ResourceVersion
	_, err = daemonSets.Update(ctx, desired, metav1.UpdateOptions{})
	return err
}

func cleanupOrphanedServiceLBDaemonSets(ctx context.Context, client kubernetes.Interface, desired map[string]struct{}) error {
	daemonSets, err := listServiceLBDaemonSets(ctx, client)
	if err != nil {
		return err
	}
	errs := make([]error, 0)
	for i := range daemonSets {
		daemonSet := &daemonSets[i]
		if _, ok := desired[daemonSet.Name]; ok {
			continue
		}
		if err := client.AppsV1().DaemonSets(daemonSet.Namespace).Delete(ctx, daemonSet.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("delete orphaned ServiceLB DaemonSet %s/%s: %w", daemonSet.Namespace, daemonSet.Name, err))
		}
	}
	return errors.Join(errs...)
}

func listServiceLBDaemonSets(ctx context.Context, client kubernetes.Interface) ([]appsv1.DaemonSet, error) {
	daemonSets, err := client.AppsV1().DaemonSets(serviceLBNamespace).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(labels.Set{
		serviceLBManagedByLabel: "casos",
		serviceLBComponentLabel: "service-lb",
	}).String()})
	if err != nil {
		return nil, fmt.Errorf("list ServiceLB DaemonSets: %w", err)
	}
	return daemonSets.Items, nil
}

func serviceLBReadyProxyNodeIPs(ctx context.Context, client kubernetes.Interface, service *corev1.Service, nodes []corev1.Node) ([]string, error) {
	selector := labels.SelectorFromSet(labels.Set{
		serviceLBServiceNameLabel:      service.Name,
		serviceLBServiceNamespaceLabel: service.Namespace,
		serviceLBServiceUIDLabel:       string(service.UID),
	}).String()
	pods, err := client.CoreV1().Pods(serviceLBNamespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list ServiceLB proxy Pods: %w", err)
	}
	readyProxyNodes := make(map[string]struct{})
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName == "" || pod.Status.PodIP == "" || !isReadyPod(pod) {
			continue
		}
		readyProxyNodes[pod.Spec.NodeName] = struct{}{}
	}
	if service.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal {
		localNodes, err := serviceReadyEndpointNodes(ctx, client, service)
		if err != nil {
			return nil, err
		}
		for nodeName := range readyProxyNodes {
			if _, ok := localNodes[nodeName]; !ok {
				delete(readyProxyNodes, nodeName)
			}
		}
	}
	return readyProxyNodeAddresses(nodes, readyProxyNodes, service.Spec.IPFamilies), nil
}

func serviceReadyEndpointNodes(ctx context.Context, client kubernetes.Interface, service *corev1.Service) (map[string]struct{}, error) {
	selector := labels.SelectorFromSet(labels.Set{discoveryv1.LabelServiceName: service.Name}).String()
	endpointSlices, err := client.DiscoveryV1().EndpointSlices(service.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list EndpointSlices: %w", err)
	}
	nodes := make(map[string]struct{})
	for _, endpointSlice := range endpointSlices.Items {
		for _, endpoint := range endpointSlice.Endpoints {
			if endpoint.NodeName == nil || (endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready) {
				continue
			}
			nodes[*endpoint.NodeName] = struct{}{}
		}
	}
	return nodes, nil
}

func readyProxyNodeAddresses(nodes []corev1.Node, readyProxyNodes map[string]struct{}, families []corev1.IPFamily) []string {
	external := make([]string, 0)
	internal := make([]string, 0)
	for _, node := range nodes {
		if _, ok := readyProxyNodes[node.Name]; !ok || !isReadyNode(node) || isControlPlaneNode(node) {
			continue
		}
		for _, address := range node.Status.Addresses {
			if !serviceLBAddressMatchesFamilies(address.Address, families) {
				continue
			}
			switch address.Type {
			case corev1.NodeExternalIP:
				external = append(external, address.Address)
			case corev1.NodeInternalIP:
				internal = append(internal, address.Address)
			}
		}
	}
	result := internal
	if len(external) > 0 {
		result = external
	}
	sort.Strings(result)
	return uniqueStrings(result)
}

func serviceLBAddressMatchesFamilies(address string, families []corev1.IPFamily) bool {
	ip := net.ParseIP(address)
	if ip == nil {
		return false
	}
	if len(families) == 0 {
		return true
	}
	want := corev1.IPv6Protocol
	if ip.To4() != nil {
		want = corev1.IPv4Protocol
	}
	for _, family := range families {
		if family == want {
			return true
		}
	}
	return false
}

func isReadyPod(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isReadyNode(node corev1.Node) bool {
	if node.Spec.Unschedulable {
		return false
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func isControlPlaneNode(node corev1.Node) bool {
	_, controlPlane := node.Labels["node-role.kubernetes.io/control-plane"]
	_, master := node.Labels["node-role.kubernetes.io/master"]
	return controlPlane || master
}

func reconcileLoadBalancerService(ctx context.Context, client kubernetes.Interface, service *corev1.Service, nodeIPs []string) error {
	if service == nil || service.DeletionTimestamp != nil {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := client.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.DeletionTimestamp != nil || !serviceLBManages(current) {
			return nil
		}
		return reconcileLoadBalancerServiceOnce(ctx, client, current, nodeIPs)
	})
}

func reconcileLoadBalancerServiceOnce(ctx context.Context, client kubernetes.Interface, service *corev1.Service, nodeIPs []string) error {
	previousManagedIPs, migrationErr := serviceLBManagedIPs(service.Annotations)
	current := service
	var err error
	if migrationErr == nil {
		current, err = removeLegacyServiceLBExternalIPs(ctx, client, service, previousManagedIPs)
		if err != nil {
			return err
		}
	}
	desiredManagedIPs := uniqueStrings(append([]string{}, nodeIPs...))
	desiredStatus := current.Status.DeepCopy()
	desiredStatus.LoadBalancer.Ingress = nil
	for _, ip := range desiredManagedIPs {
		desiredStatus.LoadBalancer.Ingress = append(desiredStatus.LoadBalancer.Ingress, corev1.LoadBalancerIngress{IP: ip})
	}
	if !apiequality.Semantic.DeepEqual(current.Status.LoadBalancer, desiredStatus.LoadBalancer) {
		statusUpdate := current.DeepCopy()
		statusUpdate.Status = *desiredStatus
		current, err = client.CoreV1().Services(current.Namespace).UpdateStatus(ctx, statusUpdate, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update LoadBalancer service status: %w", err)
		}
	}
	if migrationErr != nil {
		return fmt.Errorf("preserve malformed legacy ServiceLB state: %w", migrationErr)
	}
	return removeLegacyServiceLBManagedIPsAnnotation(ctx, client, current.Namespace, current.Name)
}

func cleanupLoadBalancerService(ctx context.Context, client kubernetes.Interface, service *corev1.Service) error {
	if service == nil || service.DeletionTimestamp != nil {
		return nil
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := client.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		status := current.Status.DeepCopy()
		status.LoadBalancer = corev1.LoadBalancerStatus{}
		if !apiequality.Semantic.DeepEqual(current.Status.LoadBalancer, status.LoadBalancer) {
			statusUpdate := current.DeepCopy()
			statusUpdate.Status = *status
			current, err = client.CoreV1().Services(current.Namespace).UpdateStatus(ctx, statusUpdate, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
		managedIPs, err := serviceLBManagedIPs(current.Annotations)
		if err != nil {
			return fmt.Errorf("preserve malformed legacy ServiceLB state: %w", err)
		}
		current, err = removeLegacyServiceLBExternalIPs(ctx, client, current, managedIPs)
		if err != nil {
			return err
		}
		return removeLegacyServiceLBManagedIPsAnnotation(ctx, client, current.Namespace, current.Name)
	})
}

func removeLegacyServiceLBExternalIPs(ctx context.Context, client kubernetes.Interface, service *corev1.Service, managedIPs []string) (*corev1.Service, error) {
	if len(managedIPs) == 0 || len(service.Spec.ExternalIPs) == 0 {
		return service, nil
	}
	managed := make(map[string]struct{}, len(managedIPs))
	for _, ip := range managedIPs {
		managed[ip] = struct{}{}
	}
	desired := service.DeepCopy()
	desired.Spec.ExternalIPs = desired.Spec.ExternalIPs[:0]
	for _, ip := range service.Spec.ExternalIPs {
		if _, ok := managed[ip]; !ok {
			desired.Spec.ExternalIPs = append(desired.Spec.ExternalIPs, ip)
		}
	}
	if reflect.DeepEqual(service.Spec.ExternalIPs, desired.Spec.ExternalIPs) {
		return service, nil
	}
	updated, err := client.CoreV1().Services(service.Namespace).Update(ctx, desired, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("remove legacy ServiceLB externalIPs: %w", err)
	}
	return updated, nil
}

func removeLegacyServiceLBManagedIPsAnnotation(ctx context.Context, client kubernetes.Interface, namespace, name string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if _, ok := current.Annotations[serviceLBManagedIPsAnnotation]; !ok {
			return nil
		}
		updated := current.DeepCopy()
		delete(updated.Annotations, serviceLBManagedIPsAnnotation)
		_, err = client.CoreV1().Services(namespace).Update(ctx, updated, metav1.UpdateOptions{})
		return err
	})
}

func serviceLBManagedIPs(annotations map[string]string) ([]string, error) {
	value := annotations[serviceLBManagedIPsAnnotation]
	if value == "" {
		return nil, nil
	}
	var ips []string
	if err := json.Unmarshal([]byte(value), &ips); err != nil {
		return nil, fmt.Errorf("decode managed LoadBalancer IPs: %w", err)
	}
	return uniqueStrings(ips), nil
}

func serviceClusterIPs(service *corev1.Service) []string {
	clusterIPs := append([]string{}, service.Spec.ClusterIPs...)
	if len(clusterIPs) == 0 && service.Spec.ClusterIP != "" {
		clusterIPs = append(clusterIPs, service.Spec.ClusterIP)
	}
	result := make([]string, 0, len(clusterIPs))
	for _, ip := range clusterIPs {
		if ip != "" && !strings.EqualFold(ip, corev1.ClusterIPNone) {
			result = append(result, ip)
		}
	}
	return uniqueStrings(result)
}

func serviceUsesIPFamily(service *corev1.Service, family corev1.IPFamily) bool {
	if len(service.Spec.IPFamilies) == 0 {
		return family == corev1.IPv4Protocol
	}
	for _, current := range service.Spec.IPFamilies {
		if current == family {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !containsString(result, value) {
			result = append(result, value)
		}
	}
	return result
}
