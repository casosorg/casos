package server

import (
	"context"
	"regexp"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeServiceLBLeaderElector func(context.Context)

func (run fakeServiceLBLeaderElector) Run(ctx context.Context) {
	run(ctx)
}

func TestRunServiceLBLeaderElectionRetriesAfterLeadershipLoss(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runs := 0
	factory := func() (serviceLBLeaderElector, error) {
		return fakeServiceLBLeaderElector(func(context.Context) {
			runs++
			if runs == 2 {
				cancel()
			}
		}), nil
	}

	runServiceLBLeaderElection(ctx, 0, factory)

	if runs != 2 {
		t.Fatalf("leader election runs = %d, want 2", runs)
	}
}

func TestReadyProxyNodeAddressesPreferExternalIPsGlobally(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "public-worker"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
				Addresses: []corev1.NodeAddress{
					{Type: corev1.NodeExternalIP, Address: "203.0.113.10"},
					{Type: corev1.NodeInternalIP, Address: "10.0.0.10"},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "private-worker"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
				Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.20"}},
			},
		},
	}
	ready := map[string]struct{}{"public-worker": {}, "private-worker": {}}

	got := readyProxyNodeAddresses(nodes, ready, []corev1.IPFamily{corev1.IPv4Protocol})

	if len(got) != 1 || got[0] != "203.0.113.10" {
		t.Fatalf("readyProxyNodeAddresses() = %v, want only the external IP", got)
	}
}

func TestServiceLBManagesExpectedClasses(t *testing.T) {
	casosClass := serviceLBClass
	otherClass := "example.com/other-lb"
	tests := []struct {
		name    string
		service *corev1.Service
		want    bool
	}{
		{name: "classless LoadBalancer", service: &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}, want: true},
		{name: "CasOS class", service: &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &casosClass}}, want: true},
		{name: "other class", service: &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, LoadBalancerClass: &otherClass}}, want: false},
		{name: "disabled", service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{serviceLBDisabledAnnotation: "true"}}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}, want: false},
		{name: "ClusterIP", service: &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceLBManages(tt.service); got != tt.want {
				t.Fatalf("serviceLBManages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServiceLBReadyProxyNodeIPsHonorsLocalEndpoints(t *testing.T) {
	service := serviceLBTestService()
	service.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeLocal
	service.Spec.Ports[0].NodePort = 30080
	publicNode := readyServiceLBTestNode("public-worker", corev1.NodeAddress{Type: corev1.NodeExternalIP, Address: "203.0.113.10"})
	privateNode := readyServiceLBTestNode("private-worker", corev1.NodeAddress{Type: corev1.NodeInternalIP, Address: "10.0.0.20"})
	ready := true
	endpointNode := privateNode.Name
	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: service.Namespace,
			Name:      "app-v4",
			Labels:    map[string]string{discoveryv1.LabelServiceName: service.Name},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{{
			Addresses:  []string{"10.42.0.20"},
			NodeName:   &endpointNode,
			Conditions: discoveryv1.EndpointConditions{Ready: &ready},
		}},
	}
	client := fake.NewSimpleClientset(
		service,
		&publicNode,
		&privateNode,
		readyServiceLBTestPod(service, publicNode.Name, "10.42.0.10"),
		readyServiceLBTestPod(service, privateNode.Name, "10.42.0.20"),
		endpointSlice,
	)

	got, err := serviceLBReadyProxyNodeIPs(context.Background(), client, service, []corev1.Node{publicNode, privateNode})
	if err != nil {
		t.Fatalf("serviceLBReadyProxyNodeIPs() error = %v", err)
	}
	if len(got) != 1 || got[0] != "10.0.0.20" {
		t.Fatalf("serviceLBReadyProxyNodeIPs() = %v, want only the node with a ready local endpoint", got)
	}
}

func TestReconcileServiceLBCreatesAndCleansManagedDaemonSet(t *testing.T) {
	service := serviceLBTestService()
	client := fake.NewSimpleClientset(service)

	if err := reconcileServiceLBWithImage(context.Background(), client, defaultServiceLBImage); err != nil {
		t.Fatalf("reconcileServiceLBWithImage() error = %v", err)
	}
	name := serviceLBDaemonSetName(service)
	if _, err := client.AppsV1().DaemonSets(serviceLBNamespace).Get(context.Background(), name, metav1.GetOptions{}); err != nil {
		t.Fatalf("managed DaemonSet was not created: %v", err)
	}

	updated, err := client.CoreV1().Services(service.Namespace).Get(context.Background(), service.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get Service: %v", err)
	}
	updated.Annotations = map[string]string{serviceLBDisabledAnnotation: "true"}
	if _, err := client.CoreV1().Services(service.Namespace).Update(context.Background(), updated, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("disable ServiceLB: %v", err)
	}
	if err := reconcileServiceLBWithImage(context.Background(), client, defaultServiceLBImage); err != nil {
		t.Fatalf("cleanup reconcile error = %v", err)
	}
	if daemonSets, err := client.AppsV1().DaemonSets(serviceLBNamespace).List(context.Background(), metav1.ListOptions{}); err != nil {
		t.Fatalf("list DaemonSets: %v", err)
	} else if len(daemonSets.Items) != 0 {
		t.Fatalf("managed DaemonSets after opt-out = %v", daemonSetNames(daemonSets.Items))
	}
}

func TestReconcileServiceLBUpdatesDaemonSetAndCleansOrphan(t *testing.T) {
	service := serviceLBTestService()
	current, err := buildServiceLBDaemonSet(service, "example.com/old/service-lb:v1")
	if err != nil {
		t.Fatalf("build current DaemonSet: %v", err)
	}
	orphan := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Namespace: serviceLBNamespace,
		Name:      "svclb-orphan",
		Labels: map[string]string{
			serviceLBManagedByLabel: "casos",
			serviceLBComponentLabel: "service-lb",
		},
	}}
	client := fake.NewSimpleClientset(service, current, orphan)

	if err := reconcileServiceLBWithImage(context.Background(), client, defaultServiceLBImage); err != nil {
		t.Fatalf("reconcileServiceLBWithImage() error = %v", err)
	}
	updated, err := client.AppsV1().DaemonSets(serviceLBNamespace).Get(context.Background(), current.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get updated DaemonSet: %v", err)
	}
	if got := updated.Spec.Template.Spec.Containers[0].Image; got != defaultServiceLBImage {
		t.Fatalf("updated image = %q, want %q", got, defaultServiceLBImage)
	}
	if _, err := client.AppsV1().DaemonSets(serviceLBNamespace).Get(context.Background(), orphan.Name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("orphaned DaemonSet still exists, get error = %v", err)
	}
}

func serviceLBTestService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "app", UID: types.UID("app-uid")},
		Spec: corev1.ServiceSpec{
			Type:       corev1.ServiceTypeLoadBalancer,
			ClusterIP:  "10.43.0.10",
			ClusterIPs: []string{"10.43.0.10"},
			IPFamilies: []corev1.IPFamily{corev1.IPv4Protocol},
			Ports:      []corev1.ServicePort{{Name: "http", Protocol: corev1.ProtocolTCP, Port: 8080}},
		},
	}
}

func readyServiceLBTestNode(name string, addresses ...corev1.NodeAddress) corev1.Node {
	return corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			Addresses:  addresses,
		},
	}
}

func readyServiceLBTestPod(service *corev1.Service, nodeName, podIP string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: serviceLBNamespace,
			Name:      "svclb-test-" + nodeName,
			Labels:    serviceLBLabels(service),
		},
		Spec: corev1.PodSpec{NodeName: nodeName},
		Status: corev1.PodStatus{
			PodIP:      podIP,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
}

func daemonSetNames(daemonSets []appsv1.DaemonSet) []string {
	names := make([]string, 0, len(daemonSets))
	for i := range daemonSets {
		names = append(names, daemonSets[i].Name)
	}
	return names
}

func TestServiceLBDaemonSetName(t *testing.T) {
	tests := []struct {
		name    string
		service *corev1.Service
		want    string
	}{
		{
			name: "uses uid for the stable suffix",
			service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name: "api",
				UID:  types.UID("uid-1"),
			}},
			want: "svclb-api-4a49acf8a6bd",
		},
		{
			name: "uses namespace and name before the api assigns a uid",
			service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "api",
			}},
			want: "svclb-api-d53b356d3e1e",
		},
		{
			name: "trims boundary hyphens",
			service: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name: "---",
			}},
			want: "svclb-service-ff24f66688f3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceLBDaemonSetName(tt.service); got != tt.want {
				t.Fatalf("serviceLBDaemonSetName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestServiceLBDaemonSetNameFitsDNSLabel(t *testing.T) {
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Namespace: "default",
		Name:      strings.Repeat("a", 43) + "-" + strings.Repeat("b", 19),
	}}

	got := serviceLBDaemonSetName(service)
	if len(got) > 63 {
		t.Fatalf("serviceLBDaemonSetName() length = %d, want at most 63: %q", len(got), got)
	}
	if matched := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`).MatchString(got); !matched {
		t.Fatalf("serviceLBDaemonSetName() = %q, want a DNS label", got)
	}
}

func TestServiceLBDaemonSetNameSeparatesNamespacesWithoutUID(t *testing.T) {
	first := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}}
	second := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Namespace: "other", Name: "api"}}

	firstName := serviceLBDaemonSetName(first)
	secondName := serviceLBDaemonSetName(second)
	if firstName == secondName {
		t.Fatalf("serviceLBDaemonSetName() reused %q across namespaces", firstName)
	}
	if secondName != "svclb-api-25dcc26e6d02" {
		t.Fatalf("serviceLBDaemonSetName() = %q, want %q", secondName, "svclb-api-25dcc26e6d02")
	}
}
