package object

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/casosorg/casos/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestParseResourceMonitorRequest(t *testing.T) {
	request, err := ParseResourceMonitorRequest(ResourceMonitorQueryParams{
		ResourceKind: "Pod",
		Namespace:    "default",
		Name:         "api",
		Mode:         "container",
		Start:        "2026-07-15T00:00:00Z",
		End:          "2026-07-15T01:00:00Z",
		Step:         "30s",
		PodLimit:     "50",
		SelectedPods: "api-2,api-1,api-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Mode != "container" || request.PodLimit != monitorOverviewMaxSeriesLimit {
		t.Fatalf("unexpected request mode/limit: %#v", request)
	}
	if strings.Join(request.SelectedPods, ",") != "api-1,api-2" {
		t.Fatalf("selected Pods were not normalized: %#v", request.SelectedPods)
	}
}

func TestParseResourceMonitorRequestValidation(t *testing.T) {
	cases := []ResourceMonitorQueryParams{
		{ResourceKind: "Pod", Namespace: "default"},
		{ResourceKind: "Pod", Namespace: "default", Name: "api", Mode: "network"},
		{ResourceKind: "Pod", Namespace: "default", Name: "api", Start: "2026-07-15T00:00:00Z"},
		{ResourceKind: "Pod", Namespace: "default", Name: "api", Start: "2026-07-15T01:00:00Z", End: "2026-07-15T00:00:00Z"},
		{ResourceKind: "Pod", Namespace: "default", Name: "api", Start: "2026-07-15T00:00:00Z", End: "2026-07-15T01:00:00Z", Step: "500ms"},
	}
	for _, params := range cases {
		if _, err := ParseResourceMonitorRequest(params); err == nil {
			t.Fatalf("expected validation error for %#v", params)
		}
	}
}

func TestMonitorMetricRateWindowIsCapped(t *testing.T) {
	query := MonitorMetricQuery{IsRange: true, Step: 30 * time.Minute}
	if got := monitorMetricRateWindow(query); got != "3600s" {
		t.Fatalf("rate window = %s, want 3600s", got)
	}
	query.Step = 15 * time.Second
	if got := monitorMetricRateWindow(query); got != "300s" {
		t.Fatalf("short range window = %s, want 300s", got)
	}
}

func TestPodRegexMatcherEscapesInput(t *testing.T) {
	matcher, ok := podRegexMatcher([]string{`api"} or vector(1)`, "web.1"})
	if !ok {
		t.Fatal("matcher was rejected")
	}
	if strings.Contains(matcher, `"} or vector`) || !strings.Contains(matcher, `api\"\\}`) || !strings.Contains(matcher, `web\\.1`) {
		t.Fatalf("matcher was not safely escaped: %s", matcher)
	}
}

func TestPodMetricQueryGroups(t *testing.T) {
	matchers := []string{`namespace="default"`, `pod=~"^(api)$"`, `container!=""`}
	total := podMetricQuery("container_cpu_usage_seconds_total", matchers, "5m", "total", "rate")
	if strings.Contains(total, " by ") {
		t.Fatalf("total query unexpectedly groups: %s", total)
	}
	byPod := podMetricQuery("container_cpu_usage_seconds_total", matchers, "5m", "pod", "rate")
	if !strings.Contains(byPod, "sum by (namespace, pod)") || strings.Contains(byPod, "container)") {
		t.Fatalf("pod query grouped incorrectly: %s", byPod)
	}
	byContainer := podMetricQuery("container_cpu_usage_seconds_total", matchers, "5m", "container", "rate")
	if !strings.Contains(byContainer, "sum by (namespace, pod, container)") {
		t.Fatalf("container query grouped incorrectly: %s", byContainer)
	}
}

func TestClassifyMonitorMetricError(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{ErrPrometheusNotConfigured, MonitorErrorPrometheusNotConfigured},
		{prometheus.ErrTimeout, MonitorErrorPrometheusTimeout},
		{prometheus.ErrUnavailable, MonitorErrorPrometheusUnavailable},
		{prometheus.ErrQuery, MonitorErrorQuery},
		{errors.New("other"), MonitorErrorQuery},
	}
	for _, test := range cases {
		if got := classifyMonitorMetricError(test.err); got.Code != test.code {
			t.Fatalf("classify(%v) = %s, want %s", test.err, got.Code, test.code)
		}
	}
}

func TestResourceMonitorModeCompatibility(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "node by pod",
			call: func() error {
				_, err := GetNodeMonitorOverview(ctx, nil, ResourceMonitorQueryParams{Name: "worker-1", Mode: "pod"})
				return err
			},
			want: "only total",
		},
		{
			name: "pod by pod",
			call: func() error {
				_, err := GetPodMonitorOverview(ctx, nil, ResourceMonitorQueryParams{Namespace: "default", Name: "api", Mode: "pod"})
				return err
			},
			want: "total or container",
		},
		{
			name: "workload by container",
			call: func() error {
				_, err := GetWorkloadMonitorOverview(ctx, nil, ResourceMonitorQueryParams{ResourceKind: "deployment", Namespace: "default", Name: "api", Mode: "container"})
				return err
			},
			want: "total or pod",
		},
		{
			name: "PVC by pod",
			call: func() error {
				_, err := GetPVCMonitorOverview(ctx, nil, ResourceMonitorQueryParams{Namespace: "default", Name: "data", Mode: "pod"})
				return err
			},
			want: "only total",
		},
	}
	for _, test := range cases {
		err := test.call()
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s error = %v, want %q", test.name, err, test.want)
		}
	}
}

func TestMonitorTopUnits(t *testing.T) {
	if got := monitorTopUnit("node", "cpu"); got != "percent" {
		t.Fatalf("node cpu unit = %s, want percent", got)
	}
	if got := monitorTopUnit("pod", "cpu"); got != "cores" {
		t.Fatalf("pod cpu unit = %s, want cores", got)
	}
	if got := monitorTopUnit("workload", "memory"); got != "bytes" {
		t.Fatalf("workload memory unit = %s, want bytes", got)
	}
}

func TestPodMonitorTopValidatesCurrentPods(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", UID: types.UID("api-uid")}},
	)
	promClient := &fakePrometheusQuerier{series: []prometheus.Series{
		{
			Labels:  map[string]string{"namespace": "default", "pod": "api"},
			Samples: []prometheus.Sample{{Timestamp: 1, Value: 2}},
		},
		{
			Labels:  map[string]string{"namespace": "default", "pod": "deleted"},
			Samples: []prometheus.Sample{{Timestamp: 1, Value: 3}},
		},
	}}

	items, err := getPodMonitorTop(context.Background(), client, promClient, "cpu", "default", 5, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items length = %d, want 2: %#v", len(items), items)
	}
	if items[0].Resource.Name != "deleted" || items[0].Resource.Validated {
		t.Fatalf("historical Pod should remain unvalidated: %#v", items[0])
	}
	if items[1].Resource.Name != "api" || !items[1].Resource.Validated || items[1].Resource.UID != "api-uid" {
		t.Fatalf("current Pod was not validated: %#v", items[1])
	}
}

func TestMonitorContainerStatusesIncludesSpecContainersWithoutStatus(t *testing.T) {
	statuses := monitorContainerStatuses(corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "api",
				Image: "example/api:v1",
			}},
		},
	})
	if len(statuses) != 1 {
		t.Fatalf("statuses length = %d, want 1", len(statuses))
	}
	if statuses[0].Name != "api" || statuses[0].Image != "example/api:v1" {
		t.Fatalf("unexpected container metadata: %#v", statuses[0])
	}
	if statuses[0].State != "Unknown" || statuses[0].Reason != "StatusNotReported" {
		t.Fatalf("unexpected missing status marker: %#v", statuses[0])
	}
}

func TestMonitorResourceInventoryUsesKubernetesObjects(t *testing.T) {
	deployUID := types.UID("deploy-uid")
	rsUID := types.UID("rs-uid")
	nodeUID := types.UID("node-uid")
	pvcUID := types.UID("pvc-uid")
	replicas := int32(1)
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}
	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1", UID: nodeUID},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
				Addresses:  []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.1"}},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", UID: deployUID},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: selector,
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels}},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "api-rs",
				Namespace:       "default",
				UID:             rsUID,
				Labels:          selector.MatchLabels,
				OwnerReferences: []metav1.OwnerReference{controllerOwner("Deployment", "api", deployUID)},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "api-rs-pod",
				Namespace:       "default",
				Labels:          selector.MatchLabels,
				OwnerReferences: []metav1.OwnerReference{controllerOwner("ReplicaSet", "api-rs", rsUID)},
			},
			Spec: corev1.PodSpec{
				NodeName:   "worker-1",
				Containers: []corev1.Container{{Name: "api", Image: "example/api:v1"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default", UID: pvcUID},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		},
	)

	inventory, err := getMonitorResourceInventoryFromClient(context.Background(), client, nil, ErrPrometheusNotConfigured, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if inventory.MetricsAvailable || inventory.Error == nil || inventory.Error.Code != MonitorErrorPrometheusNotConfigured {
		t.Fatalf("unexpected Prometheus state: %#v", inventory)
	}
	if len(inventory.Nodes) != 1 || inventory.Nodes[0].Resource.Name != "worker-1" || inventory.Nodes[0].PodCount != 1 {
		t.Fatalf("unexpected nodes: %#v", inventory.Nodes)
	}
	if len(inventory.Nodes[0].Pods) != 1 || inventory.Nodes[0].Pods[0].Owner == nil || inventory.Nodes[0].Pods[0].Owner.Kind != "Deployment" {
		t.Fatalf("unexpected node Pods: %#v", inventory.Nodes[0].Pods)
	}
	if len(inventory.Workloads) != 1 || inventory.Workloads[0].Resource.Kind != "Deployment" || inventory.Workloads[0].CurrentPodCount != 1 {
		t.Fatalf("unexpected workloads: %#v", inventory.Workloads)
	}
	if len(inventory.PVCs) != 1 || inventory.PVCs[0].Resource.Name != "data" || inventory.PVCs[0].Status != string(corev1.ClaimBound) {
		t.Fatalf("unexpected PVCs: %#v", inventory.PVCs)
	}
}

func TestMonitorResourceInventoryAllowsNoPVCs(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	})

	inventory, err := getMonitorResourceInventoryFromClient(context.Background(), client, nil, ErrPrometheusNotConfigured, time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Nodes) != 1 {
		t.Fatalf("nodes length = %d, want 1", len(inventory.Nodes))
	}
	if len(inventory.PVCs) != 0 {
		t.Fatalf("PVCs length = %d, want 0: %#v", len(inventory.PVCs), inventory.PVCs)
	}
}
