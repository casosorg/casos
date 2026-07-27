package object

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/casosorg/casos/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	MonitorErrorInvalidParams            = "invalid_params"
	MonitorErrorPrometheusNotConfigured  = "prometheus_not_configured"
	MonitorErrorPrometheusUnavailable    = "prometheus_unavailable"
	MonitorErrorPrometheusTimeout        = "prometheus_timeout"
	MonitorErrorQuery                    = "query_error"
	MonitorErrorEmpty                    = "empty"
	monitorOverviewMaxConcurrency        = 4
	monitorOverviewDefaultSeriesLimit    = 10
	monitorOverviewMaxSeriesLimit        = 20
	monitorOverviewMaxPodMatcherLength   = 4096
	monitorOverviewDefaultTopLimit       = 5
	monitorOverviewMaxTopLimit           = 20
	monitorOverviewCurrentPodHistoryNote = "Metrics are calculated from the workload's current Pods. Deleted Pods from earlier rollouts are not included unless kube-state-metrics support is added later."
)

type ResourceMonitorQueryParams struct {
	ResourceKind string
	Namespace    string
	Name         string
	Mode         string
	Start        string
	End          string
	Step         string
	PodLimit     string
	SelectedPods string
}

type ResourceMonitorRequest struct {
	ResourceKind string
	Namespace    string
	Name         string
	Mode         string
	Start        time.Time
	End          time.Time
	Step         time.Duration
	PodLimit     int
	SelectedPods []string
	IsRange      bool
}

type MonitorStructuredError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type MonitorOverviewTimeRange struct {
	Start      string  `json:"start"`
	End        string  `json:"end"`
	Step       float64 `json:"step"`
	RateWindow string  `json:"rateWindow"`
}

type MonitorOverviewLimits struct {
	SeriesLimit      int `json:"seriesLimit"`
	PodLimit         int `json:"podLimit"`
	HardSeriesLimit  int `json:"hardSeriesLimit"`
	TopLimit         int `json:"topLimit,omitempty"`
	MaxTopLimit      int `json:"maxTopLimit,omitempty"`
	MaxSamples       int `json:"maxSamples"`
	MaxRangeSeconds  int `json:"maxRangeSeconds"`
	MaxMatcherLength int `json:"maxMatcherLength"`
	MaxQueryParallel int `json:"maxQueryParallel"`
}

type MonitorOverviewMetric struct {
	Key         string                  `json:"key"`
	Title       string                  `json:"title"`
	Unit        string                  `json:"unit"`
	Status      string                  `json:"status"`
	Error       *MonitorStructuredError `json:"error,omitempty"`
	Data        MonitorMetricResponse   `json:"data"`
	SeriesLimit int                     `json:"seriesLimit,omitempty"`
	Truncated   bool                    `json:"truncated,omitempty"`
}

type MonitorResourceOverview struct {
	Resource   ResourceReference        `json:"resource"`
	Mode       string                   `json:"mode"`
	TimeRange  MonitorOverviewTimeRange `json:"timeRange"`
	Limits     MonitorOverviewLimits    `json:"limits"`
	Metadata   map[string]interface{}   `json:"metadata,omitempty"`
	Metrics    []MonitorOverviewMetric  `json:"metrics"`
	Pods       []MonitorPodSummary      `json:"pods,omitempty"`
	Containers []MonitorContainerStatus `json:"containers,omitempty"`
	Owner      *ResourceReference       `json:"owner,omitempty"`
	Node       *ResourceReference       `json:"node,omitempty"`
	Notes      []string                 `json:"notes,omitempty"`
}

type MonitorPodSummary struct {
	Namespace          string                   `json:"namespace"`
	Name               string                   `json:"name"`
	Phase              string                   `json:"phase"`
	Status             string                   `json:"status"`
	NodeName           string                   `json:"nodeName,omitempty"`
	Owner              *ResourceReference       `json:"owner,omitempty"`
	RestartCount       int32                    `json:"restartCount"`
	CurrentCPUCores    float64                  `json:"currentCpuCores,omitempty"`
	CurrentMemoryBytes float64                  `json:"currentMemoryBytes,omitempty"`
	MetricsAvailable   bool                     `json:"metricsAvailable"`
	Containers         []MonitorContainerStatus `json:"containers,omitempty"`
	CreatedAt          string                   `json:"createdAt"`
}

type MonitorContainerStatus struct {
	Name         string `json:"name"`
	Image        string `json:"image,omitempty"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	LastState    string `json:"lastState,omitempty"`
	LastReason   string `json:"lastReason,omitempty"`
	LastExitCode int32  `json:"lastExitCode,omitempty"`
	OOMKilled    bool   `json:"oomKilled"`
}

type MonitorTopQueryParams struct {
	Resource  string
	Metric    string
	Namespace string
	Limit     string
}

type MonitorTopResponse struct {
	Resource string           `json:"resource"`
	Metric   string           `json:"metric"`
	Unit     string           `json:"unit"`
	Limit    int              `json:"limit"`
	Items    []MonitorTopItem `json:"items"`
	Time     string           `json:"time"`
	Notes    []string         `json:"notes,omitempty"`
}

type MonitorTopItem struct {
	Resource ResourceReference `json:"resource"`
	Value    float64           `json:"value"`
	Unit     string            `json:"unit"`
}

type MonitorResourceInventory struct {
	Time             string                     `json:"time"`
	MetricsAvailable bool                       `json:"metricsAvailable"`
	Error            *MonitorStructuredError    `json:"error,omitempty"`
	Nodes            []MonitorInventoryNode     `json:"nodes"`
	Workloads        []MonitorInventoryWorkload `json:"workloads"`
	PVCs             []MonitorInventoryPVC      `json:"pvcs"`
}

type MonitorInventoryNode struct {
	Resource                      ResourceReference   `json:"resource"`
	Status                        string              `json:"status"`
	InternalIP                    string              `json:"internalIP,omitempty"`
	ExternalIP                    string              `json:"externalIP,omitempty"`
	PodCount                      int                 `json:"podCount"`
	CPUPercent                    *float64            `json:"cpuPercent,omitempty"`
	MemoryPercent                 *float64            `json:"memoryPercent,omitempty"`
	NetworkReceiveBytesPerSecond  *float64            `json:"networkReceiveBytesPerSecond,omitempty"`
	NetworkTransmitBytesPerSecond *float64            `json:"networkTransmitBytesPerSecond,omitempty"`
	DiskUsagePercent              *float64            `json:"diskUsagePercent,omitempty"`
	Pods                          []MonitorPodSummary `json:"pods,omitempty"`
}

type MonitorInventoryWorkload struct {
	Resource                      ResourceReference      `json:"resource"`
	Replicas                      int32                  `json:"replicas"`
	ReadyReplicas                 int32                  `json:"readyReplicas"`
	CurrentPodCount               int                    `json:"currentPodCount"`
	CPUCores                      *float64               `json:"cpuCores,omitempty"`
	MemoryBytes                   *float64               `json:"memoryBytes,omitempty"`
	NetworkReceiveBytesPerSecond  *float64               `json:"networkReceiveBytesPerSecond,omitempty"`
	NetworkTransmitBytesPerSecond *float64               `json:"networkTransmitBytesPerSecond,omitempty"`
	ResourceConfiguration         map[string]interface{} `json:"resourceConfiguration,omitempty"`
	Pods                          []MonitorPodSummary    `json:"pods,omitempty"`
}

type MonitorInventoryPVC struct {
	Resource         ResourceReference `json:"resource"`
	Status           string            `json:"status"`
	StorageClassName string            `json:"storageClassName,omitempty"`
	VolumeName       string            `json:"volumeName,omitempty"`
	RequestedStorage string            `json:"requestedStorage,omitempty"`
	UsedBytes        *float64          `json:"usedBytes,omitempty"`
	CapacityBytes    *float64          `json:"capacityBytes,omitempty"`
	UsagePercent     *float64          `json:"usagePercent,omitempty"`
}

type monitorOverviewQuerySpec struct {
	Key        string
	Title      string
	Unit       string
	Scope      string
	Metric     string
	PromQL     string
	ObjectName func(map[string]string) string
}

type nodeMetricIdentity struct {
	Name      string
	Addresses []string
}

func ParseResourceMonitorRequest(params ResourceMonitorQueryParams) (ResourceMonitorRequest, error) {
	request := ResourceMonitorRequest{
		ResourceKind: strings.TrimSpace(params.ResourceKind),
		Namespace:    strings.TrimSpace(params.Namespace),
		Name:         strings.TrimSpace(params.Name),
		Mode:         strings.ToLower(strings.TrimSpace(params.Mode)),
		PodLimit:     monitorOverviewDefaultSeriesLimit,
	}
	if request.ResourceKind == "" {
		return ResourceMonitorRequest{}, fmt.Errorf("resource kind is required")
	}
	if request.Name == "" {
		return ResourceMonitorRequest{}, fmt.Errorf("name is required")
	}
	if request.Mode == "" {
		request.Mode = "total"
	}
	if request.Mode != "total" && request.Mode != "pod" && request.Mode != "container" {
		return ResourceMonitorRequest{}, fmt.Errorf("unsupported mode %q", request.Mode)
	}
	if params.PodLimit != "" {
		limit, err := strconv.Atoi(strings.TrimSpace(params.PodLimit))
		if err != nil || limit <= 0 {
			return ResourceMonitorRequest{}, fmt.Errorf("podLimit must be a positive integer")
		}
		request.PodLimit = limit
	}
	if request.PodLimit > monitorOverviewMaxSeriesLimit {
		request.PodLimit = monitorOverviewMaxSeriesLimit
	}
	request.SelectedPods = parseSelectedMonitorPods(params.SelectedPods)
	if len(request.SelectedPods) > monitorOverviewMaxSeriesLimit {
		return ResourceMonitorRequest{}, fmt.Errorf("selectedPods must not exceed %d", monitorOverviewMaxSeriesLimit)
	}

	start, end, step, err := parseResourceMonitorRange(params.Start, params.End, params.Step)
	if err != nil {
		return ResourceMonitorRequest{}, err
	}
	request.Start = start
	request.End = end
	request.Step = step
	request.IsRange = true
	return request, nil
}

func GetNodeMonitorOverview(ctx context.Context, cfg *rest.Config, params ResourceMonitorQueryParams) (MonitorResourceOverview, error) {
	params.ResourceKind = "Node"
	request, err := ParseResourceMonitorRequest(params)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	if request.Mode != "total" {
		return MonitorResourceOverview{}, fmt.Errorf("node monitoring supports only total mode")
	}
	client, err := newClient(cfg)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	node, err := client.CoreV1().Nodes().Get(ctx, request.Name, metav1.GetOptions{})
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	pods, err := ListPodsForNodeFromClient(ctx, client, node.Name)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	promClient, promErr := newConfiguredPrometheusClient()
	podSummaries := buildMonitorPodSummaries(ctx, client, pods)
	if promErr == nil {
		applyPodInstantMetrics(ctx, promClient, podSummaries, pods, request.End)
	}

	identity := nodeIdentityFromNode(*node)
	metrics := runOverviewMetricQueries(ctx, promClient, promErr, request, nodeOverviewMetricSpecs(request, identity), monitorOverviewDefaultSeriesLimit)
	return MonitorResourceOverview{
		Resource:  ResourceReference{Kind: "Node", Name: node.Name, UID: string(node.UID), Validated: true},
		Mode:      request.Mode,
		TimeRange: monitorOverviewTimeRange(request),
		Limits:    monitorOverviewLimits(request),
		Metadata: map[string]interface{}{
			"status":        nodeStatus(*node),
			"internalIP":    nodeAddress(*node, corev1.NodeInternalIP),
			"externalIP":    nodeAddress(*node, corev1.NodeExternalIP),
			"kubelet":       node.Status.NodeInfo.KubeletVersion,
			"os":            node.Status.NodeInfo.OperatingSystem,
			"arch":          node.Status.NodeInfo.Architecture,
			"podCount":      len(pods),
			"unschedulable": node.Spec.Unschedulable,
		},
		Metrics: metrics,
		Pods:    podSummaries,
	}, nil
}

func GetPodMonitorOverview(ctx context.Context, cfg *rest.Config, params ResourceMonitorQueryParams) (MonitorResourceOverview, error) {
	params.ResourceKind = "Pod"
	request, err := ParseResourceMonitorRequest(params)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	if request.Namespace == "" {
		return MonitorResourceOverview{}, fmt.Errorf("namespace is required")
	}
	if request.Mode == "pod" {
		return MonitorResourceOverview{}, fmt.Errorf("pod monitoring supports total or container mode")
	}
	client, err := newClient(cfg)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	pod, err := client.CoreV1().Pods(request.Namespace).Get(ctx, request.Name, metav1.GetOptions{})
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	owner, _ := ResolvePodOwnerFromClient(ctx, client, pod)
	var ownerPtr *ResourceReference
	if owner.Name != "" {
		ownerPtr = &owner
	}
	var nodePtr *ResourceReference
	if pod.Spec.NodeName != "" {
		nodePtr = &ResourceReference{Kind: "Node", Name: pod.Spec.NodeName, Validated: true}
	}
	promClient, promErr := newConfiguredPrometheusClient()
	specs := podOverviewMetricSpecs(request)
	metrics := runOverviewMetricQueries(ctx, promClient, promErr, request, specs, monitorOverviewMaxSeriesLimit)
	notes := []string{"Network metrics are shown at Pod total level only. Container network breakdown is not enabled because container_network_* series may duplicate the Pod network namespace."}
	return MonitorResourceOverview{
		Resource:   ResourceReference{Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, UID: string(pod.UID), Validated: true},
		Mode:       request.Mode,
		TimeRange:  monitorOverviewTimeRange(request),
		Limits:     monitorOverviewLimits(request),
		Metadata:   podMetadata(*pod),
		Metrics:    metrics,
		Pods:       buildMonitorPodSummaries(ctx, client, []corev1.Pod{*pod}),
		Containers: monitorContainerStatuses(*pod),
		Owner:      ownerPtr,
		Node:       nodePtr,
		Notes:      notes,
	}, nil
}

func GetWorkloadMonitorOverview(ctx context.Context, cfg *rest.Config, params ResourceMonitorQueryParams) (MonitorResourceOverview, error) {
	request, err := ParseResourceMonitorRequest(params)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	if request.Namespace == "" {
		return MonitorResourceOverview{}, fmt.Errorf("namespace is required")
	}
	if request.Mode == "container" {
		return MonitorResourceOverview{}, fmt.Errorf("workload monitoring supports total or pod mode")
	}
	workload, err := ListPodsForWorkload(cfg, request.ResourceKind, request.Namespace, request.Name)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	client, err := newClient(cfg)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	pods := selectMonitorPods(workload.Pods, request.SelectedPods, request.PodLimit)
	podNames := make([]string, 0, len(pods))
	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	if request.Mode == "pod" && len(pods) == 0 && len(workload.Pods) > 0 {
		return MonitorResourceOverview{}, fmt.Errorf("selectedPods did not match current workload Pods")
	}
	promClient, promErr := newConfiguredPrometheusClient()
	specs := workloadOverviewMetricSpecs(request, podNames)
	metrics := runOverviewMetricQueries(ctx, promClient, promErr, request, specs, request.PodLimit)
	podSummaries := buildMonitorPodSummaries(ctx, client, workload.Pods)
	if promErr == nil {
		applyPodInstantMetrics(ctx, promClient, podSummaries, workload.Pods, request.End)
	}
	resourceConfig := workloadResourceConfig(workload)
	return MonitorResourceOverview{
		Resource:  ResourceReference{Kind: workload.Kind, Namespace: workload.Namespace, Name: workload.Name, UID: string(workload.UID), Validated: true},
		Mode:      request.Mode,
		TimeRange: monitorOverviewTimeRange(request),
		Limits:    monitorOverviewLimits(request),
		Metadata: map[string]interface{}{
			"replicas":                  workload.Replicas,
			"readyReplicas":             workload.ReadyReplicas,
			"selector":                  workload.Selector,
			"currentPodCount":           len(workload.Pods),
			"queriedPodCount":           len(pods),
			"selectedPods":              podNames,
			"currentPodsHistoryNote":    monitorOverviewCurrentPodHistoryNote,
			"resourceConfiguration":     resourceConfig,
			"requestLimitCurrentConfig": "Request and limit references come from the current Kubernetes spec, not historical configuration.",
		},
		Metrics: metrics,
		Pods:    podSummaries,
		Notes:   []string{monitorOverviewCurrentPodHistoryNote, "CPU and memory request/limit references use the current workload configuration."},
	}, nil
}

func GetPVCMonitorOverview(ctx context.Context, cfg *rest.Config, params ResourceMonitorQueryParams) (MonitorResourceOverview, error) {
	params.ResourceKind = "PersistentVolumeClaim"
	request, err := ParseResourceMonitorRequest(params)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	if request.Namespace == "" {
		return MonitorResourceOverview{}, fmt.Errorf("namespace is required")
	}
	if request.Mode != "total" {
		return MonitorResourceOverview{}, fmt.Errorf("PVC monitoring supports only total mode")
	}
	client, err := newClient(cfg)
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	pvc, err := client.CoreV1().PersistentVolumeClaims(request.Namespace).Get(ctx, request.Name, metav1.GetOptions{})
	if err != nil {
		return MonitorResourceOverview{}, err
	}
	promClient, promErr := newConfiguredPrometheusClient()
	specs := []monitorOverviewQuerySpec{
		genericMetricSpec("storage", "Storage Usage", "pvc", "storage", "percent", request.Namespace, request.Name),
		genericMetricSpec("usedBytes", "Used Bytes", "pvc", "storage_used_bytes", "bytes", request.Namespace, request.Name),
		genericMetricSpec("capacityBytes", "Capacity", "pvc", "storage_capacity_bytes", "bytes", request.Namespace, request.Name),
	}
	metrics := runOverviewMetricQueries(ctx, promClient, promErr, request, specs, monitorOverviewDefaultSeriesLimit)
	return MonitorResourceOverview{
		Resource:  ResourceReference{Kind: "PersistentVolumeClaim", Namespace: pvc.Namespace, Name: pvc.Name, UID: string(pvc.UID), Validated: true},
		Mode:      request.Mode,
		TimeRange: monitorOverviewTimeRange(request),
		Limits:    monitorOverviewLimits(request),
		Metadata: map[string]interface{}{
			"status":           string(pvc.Status.Phase),
			"storageClassName": pvcStorageClassName(*pvc),
			"volumeName":       pvc.Spec.VolumeName,
			"requestedStorage": pvcRequestedStorage(*pvc),
			"accessModes":      pvcAccessModes(*pvc),
			"unsupportedMetrics": []string{
				"IOPS requires CSI or an additional storage exporter.",
				"Read/write throughput requires CSI or an additional storage exporter.",
			},
		},
		Metrics: metrics,
		Notes:   []string{"PVC storage usage uses kubelet_volume_stats_* metrics. IOPS and throughput are not shown unless a reliable CSI/exporter source is available."},
	}, nil
}

func GetMonitorTop(ctx context.Context, cfg *rest.Config, params MonitorTopQueryParams) (MonitorTopResponse, error) {
	resource := strings.ToLower(strings.TrimSpace(params.Resource))
	metric := strings.ToLower(strings.TrimSpace(params.Metric))
	namespace := strings.TrimSpace(params.Namespace)
	limit := normalizeMonitorTopLimit(params.Limit)
	if resource != "node" && resource != "pod" && resource != "workload" {
		return MonitorTopResponse{}, fmt.Errorf("resource must be node, pod, or workload")
	}
	if metric != "cpu" && metric != "memory" {
		return MonitorTopResponse{}, fmt.Errorf("metric must be cpu or memory")
	}
	client, err := newClient(cfg)
	if err != nil {
		return MonitorTopResponse{}, err
	}
	promClient, err := newConfiguredPrometheusClient()
	if err != nil {
		return MonitorTopResponse{}, err
	}
	now := time.Now().UTC()
	unit := monitorTopUnit(resource, metric)
	var items []MonitorTopItem
	switch resource {
	case "node":
		items, err = getNodeMonitorTop(ctx, client, promClient, metric, limit, now)
	case "pod":
		items, err = getPodMonitorTop(ctx, client, promClient, metric, namespace, limit, now)
	case "workload":
		items, err = getWorkloadMonitorTop(ctx, client, promClient, metric, namespace, limit, now)
	}
	if err != nil {
		return MonitorTopResponse{}, err
	}
	return MonitorTopResponse{
		Resource: resource,
		Metric:   metric,
		Unit:     unit,
		Limit:    limit,
		Items:    items,
		Time:     now.Format(time.RFC3339Nano),
		Notes:    monitorTopNotes(resource),
	}, nil
}

func GetMonitorResourceInventory(ctx context.Context, cfg *rest.Config) (MonitorResourceInventory, error) {
	client, err := newClient(cfg)
	if err != nil {
		return MonitorResourceInventory{}, err
	}
	promClient, promErr := newConfiguredPrometheusClient()
	return getMonitorResourceInventoryFromClient(ctx, client, promClient, promErr, time.Now().UTC())
}

func getMonitorResourceInventoryFromClient(ctx context.Context, client kubernetes.Interface, promClient prometheusQuerier, promErr error, at time.Time) (MonitorResourceInventory, error) {
	inventory := MonitorResourceInventory{
		Time:             at.Format(time.RFC3339Nano),
		MetricsAvailable: promErr == nil,
		Nodes:            []MonitorInventoryNode{},
		Workloads:        []MonitorInventoryWorkload{},
		PVCs:             []MonitorInventoryPVC{},
	}
	if promErr != nil {
		inventory.Error = classifyMonitorMetricError(promErr)
	}

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return MonitorResourceInventory{}, err
	}
	for _, node := range nodes.Items {
		pods, err := ListPodsForNodeFromClient(ctx, client, node.Name)
		if err != nil {
			return MonitorResourceInventory{}, err
		}
		podSummaries := buildMonitorPodSummaries(ctx, client, pods)
		if promErr == nil {
			applyPodInstantMetrics(ctx, promClient, podSummaries, pods, at)
		}
		row := MonitorInventoryNode{
			Resource:   ResourceReference{Kind: "Node", Name: node.Name, UID: string(node.UID), Validated: true},
			Status:     nodeStatus(node),
			InternalIP: nodeAddress(node, corev1.NodeInternalIP),
			ExternalIP: nodeAddress(node, corev1.NodeExternalIP),
			PodCount:   len(pods),
			Pods:       podSummaries,
		}
		if promErr == nil {
			applyNodeInventoryMetrics(ctx, promClient, &row, node, at)
		}
		inventory.Nodes = append(inventory.Nodes, row)
	}
	sort.SliceStable(inventory.Nodes, func(i, j int) bool {
		return inventory.Nodes[i].Resource.Name < inventory.Nodes[j].Resource.Name
	})

	workloads, err := listMonitorInventoryWorkloads(ctx, client)
	if err != nil {
		return MonitorResourceInventory{}, err
	}
	for _, workload := range workloads {
		podSummaries := buildMonitorPodSummaries(ctx, client, workload.Pods)
		if promErr == nil {
			applyPodInstantMetrics(ctx, promClient, podSummaries, workload.Pods, at)
		}
		row := MonitorInventoryWorkload{
			Resource:              ResourceReference{Kind: workload.Kind, Namespace: workload.Namespace, Name: workload.Name, UID: string(workload.UID), Validated: true},
			Replicas:              workload.Replicas,
			ReadyReplicas:         workload.ReadyReplicas,
			CurrentPodCount:       len(workload.Pods),
			ResourceConfiguration: workloadResourceConfig(workload),
			Pods:                  podSummaries,
		}
		if promErr == nil {
			applyWorkloadInventoryMetrics(ctx, promClient, &row, workload.Pods, at)
		}
		inventory.Workloads = append(inventory.Workloads, row)
	}
	sort.SliceStable(inventory.Workloads, func(i, j int) bool {
		left := inventory.Workloads[i].Resource
		right := inventory.Workloads[j].Resource
		return left.Kind+"/"+left.Namespace+"/"+left.Name < right.Kind+"/"+right.Namespace+"/"+right.Name
	})

	pvcs, err := client.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return MonitorResourceInventory{}, err
	}
	pvcMetrics := monitorPVCInventoryMetrics{}
	if promErr == nil {
		pvcMetrics = queryPVCInventoryMetrics(ctx, promClient, at)
	}
	for _, pvc := range pvcs.Items {
		key := pvc.Namespace + "/" + pvc.Name
		row := MonitorInventoryPVC{
			Resource:         ResourceReference{Kind: "PersistentVolumeClaim", Namespace: pvc.Namespace, Name: pvc.Name, UID: string(pvc.UID), Validated: true},
			Status:           string(pvc.Status.Phase),
			StorageClassName: pvcStorageClassName(pvc),
			VolumeName:       pvc.Spec.VolumeName,
			RequestedStorage: pvcRequestedStorage(pvc),
			UsedBytes:        pvcMetrics.used[key],
			CapacityBytes:    pvcMetrics.capacity[key],
			UsagePercent:     pvcMetrics.usage[key],
		}
		inventory.PVCs = append(inventory.PVCs, row)
	}
	sort.SliceStable(inventory.PVCs, func(i, j int) bool {
		left := inventory.PVCs[i].Resource
		right := inventory.PVCs[j].Resource
		return left.Namespace+"/"+left.Name < right.Namespace+"/"+right.Name
	})

	return inventory, nil
}

func GetMonitorResourceEvents(cfg *rest.Config, kind, namespace, name string, limit int) ([]MonitorEvent, error) {
	if cfg == nil {
		return nil, errors.New("apiserver not ready")
	}
	kind = strings.TrimSpace(kind)
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if kind == "" || name == "" {
		return nil, fmt.Errorf("kind and name are required")
	}
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, err := listMonitorEventsForObject(ctx, client, kind, namespace, name, limit)
	if err != nil {
		return nil, err
	}
	return toMonitorEvents(events), nil
}

func parseResourceMonitorRange(startText, endText, stepText string) (time.Time, time.Time, time.Duration, error) {
	startText = strings.TrimSpace(startText)
	endText = strings.TrimSpace(endText)
	stepText = strings.TrimSpace(stepText)
	if startText == "" && endText == "" {
		end := time.Now().UTC()
		return end.Add(-time.Hour), end, 15 * time.Second, nil
	}
	if startText == "" || endText == "" {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("start and end must be provided together")
	}
	start, err := parseMonitorMetricTimestamp(startText)
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid start: %w", err)
	}
	end, err := parseMonitorMetricTimestamp(endText)
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid end: %w", err)
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("start must be before end")
	}
	duration := end.Sub(start)
	if duration > maxMonitorMetricRange {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("time range must not exceed %s", maxMonitorMetricRange)
	}
	step := defaultMonitorMetricStep(duration)
	if stepText != "" {
		step, err = parseMonitorMetricDuration(stepText)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid step: %w", err)
		}
	}
	if step < time.Second {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("step must be at least 1s")
	}
	if math.Ceil(duration.Seconds()/step.Seconds())+1 > maxMonitorMetricSamples {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("time range and step exceed the %d sample limit", maxMonitorMetricSamples)
	}
	return start, end, step, nil
}

func runOverviewMetricQueries(ctx context.Context, client prometheusQuerier, clientErr error, request ResourceMonitorRequest, specs []monitorOverviewQuerySpec, seriesLimit int) []MonitorOverviewMetric {
	results := make([]MonitorOverviewMetric, len(specs))
	if clientErr != nil {
		for i, spec := range specs {
			results[i] = overviewMetricError(spec, clientErr, seriesLimit)
		}
		return results
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, monitorOverviewMaxConcurrency)
	for i, spec := range specs {
		i, spec := i, spec
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = queryOverviewMetric(ctx, client, request, spec, seriesLimit)
		}()
	}
	wg.Wait()
	return results
}

func queryOverviewMetric(ctx context.Context, client prometheusQuerier, request ResourceMonitorRequest, spec monitorOverviewQuerySpec, seriesLimit int) MonitorOverviewMetric {
	query := MonitorMetricQuery{
		Scope:     spec.Scope,
		Metric:    spec.Metric,
		Namespace: request.Namespace,
		Name:      request.Name,
		Start:     request.Start,
		End:       request.End,
		Step:      request.Step,
		IsRange:   true,
	}
	promQL := spec.PromQL
	if promQL == "" {
		definition := monitorMetricDefinitions[spec.Scope][spec.Metric]
		promQL = definition.buildQuery(query)
	}
	promSeries, err := client.QueryRange(ctx, promQL, prometheus.Range{Start: request.Start, End: request.End, Step: request.Step})
	if err != nil {
		return overviewMetricError(spec, err, seriesLimit)
	}
	objectName := spec.ObjectName
	if objectName == nil {
		objectName = func(labels map[string]string) string { return monitorMetricObjectName(spec.Scope, labels) }
	}
	data := monitorMetricResponseFromPromSeries(query, spec.Unit, promSeries, objectName)
	metric := MonitorOverviewMetric{
		Key:         spec.Key,
		Title:       spec.Title,
		Unit:        spec.Unit,
		Status:      "ok",
		Data:        data,
		SeriesLimit: seriesLimit,
	}
	if len(metric.Data.Series) == 0 {
		metric.Status = "empty"
		metric.Error = &MonitorStructuredError{Code: MonitorErrorEmpty, Message: "no metric data"}
		return metric
	}
	if seriesLimit > 0 && len(metric.Data.Series) > seriesLimit {
		metric.Data.Series = metric.Data.Series[:seriesLimit]
		metric.Truncated = true
	}
	return metric
}

func overviewMetricError(spec monitorOverviewQuerySpec, err error, seriesLimit int) MonitorOverviewMetric {
	return MonitorOverviewMetric{
		Key:         spec.Key,
		Title:       spec.Title,
		Unit:        spec.Unit,
		Status:      "error",
		Error:       classifyMonitorMetricError(err),
		Data:        MonitorMetricResponse{Scope: spec.Scope, Metric: spec.Metric, Unit: spec.Unit, Series: []MonitorMetricSeries{}},
		SeriesLimit: seriesLimit,
	}
}

func classifyMonitorMetricError(err error) *MonitorStructuredError {
	if err == nil {
		return nil
	}
	code := MonitorErrorQuery
	switch {
	case errors.Is(err, ErrPrometheusNotConfigured):
		code = MonitorErrorPrometheusNotConfigured
	case errors.Is(err, prometheus.ErrTimeout):
		code = MonitorErrorPrometheusTimeout
	case errors.Is(err, prometheus.ErrUnavailable):
		code = MonitorErrorPrometheusUnavailable
	case errors.Is(err, prometheus.ErrQuery), errors.Is(err, prometheus.ErrInvalidAddress):
		code = MonitorErrorQuery
	}
	return &MonitorStructuredError{Code: code, Message: err.Error()}
}

func nodeOverviewMetricSpecs(request ResourceMonitorRequest, identity nodeMetricIdentity) []monitorOverviewQuerySpec {
	window := monitorMetricRateWindow(resourceMonitorRequestAsMetricQuery(request, "node", "cpu"))
	return []monitorOverviewQuerySpec{
		{Key: "cpu", Title: "CPU Usage", Unit: "percent", Scope: "node", Metric: "cpu", PromQL: fmt.Sprintf(`100 * (1 - avg(%s))`, nodeRateExpr("node_cpu_seconds_total", []string{`mode="idle"`}, identity, window)), ObjectName: func(map[string]string) string { return identity.Name }},
		{Key: "memory", Title: "Memory Usage", Unit: "percent", Scope: "node", Metric: "memory", PromQL: fmt.Sprintf(`100 * (1 - max(%s) / max(%s))`, nodeSelectorUnion("node_memory_MemAvailable_bytes", nil, identity), nodeSelectorUnion("node_memory_MemTotal_bytes", nil, identity)), ObjectName: func(map[string]string) string { return identity.Name }},
		{Key: "networkReceive", Title: "Network Receive", Unit: "bytes_per_second", Scope: "node", Metric: "network_receive", PromQL: fmt.Sprintf(`sum(%s)`, nodeRateExpr("node_network_receive_bytes_total", []string{`device!~"^(lo|veth.*|docker.*|cni.*|flannel.*|cali.*)$"`}, identity, window)), ObjectName: func(map[string]string) string { return identity.Name }},
		{Key: "networkTransmit", Title: "Network Transmit", Unit: "bytes_per_second", Scope: "node", Metric: "network_transmit", PromQL: fmt.Sprintf(`sum(%s)`, nodeRateExpr("node_network_transmit_bytes_total", []string{`device!~"^(lo|veth.*|docker.*|cni.*|flannel.*|cali.*)$"`}, identity, window)), ObjectName: func(map[string]string) string { return identity.Name }},
		{Key: "disk", Title: "Disk Usage", Unit: "percent", Scope: "node", Metric: "disk", PromQL: fmt.Sprintf(`100 * (1 - sum(%s) / sum(%s))`, nodeSelectorUnion("node_filesystem_avail_bytes", nodeFilesystemBaseMatchers(), identity), nodeSelectorUnion("node_filesystem_size_bytes", nodeFilesystemBaseMatchers(), identity)), ObjectName: func(map[string]string) string { return identity.Name }},
	}
}

func podOverviewMetricSpecs(request ResourceMonitorRequest) []monitorOverviewQuerySpec {
	matchers := []string{"namespace=" + strconv.Quote(request.Namespace), "pod=" + strconv.Quote(request.Name)}
	window := monitorMetricRateWindow(resourceMonitorRequestAsMetricQuery(request, "pod", "cpu"))
	specs := []monitorOverviewQuerySpec{
		{Key: "cpu", Title: "Pod CPU", Unit: "cores", Scope: "pod", Metric: "cpu", PromQL: podMetricQuery("container_cpu_usage_seconds_total", append([]string{`container!=""`, `container!="POD"`, `image!=""`}, matchers...), window, monitorPodGroup(request.Mode == "container"), "rate")},
		{Key: "memory", Title: "Pod Memory", Unit: "bytes", Scope: "pod", Metric: "memory", PromQL: podMetricQuery("container_memory_working_set_bytes", append([]string{`container!=""`, `container!="POD"`, `image!=""`}, matchers...), "", monitorPodGroup(request.Mode == "container"), "gauge")},
		{Key: "networkReceive", Title: "Pod Network Receive", Unit: "bytes_per_second", Scope: "pod", Metric: "network_receive", PromQL: podMetricQuery("container_network_receive_bytes_total", append([]string{`interface!="lo"`}, matchers...), window, "total", "rate")},
		{Key: "networkTransmit", Title: "Pod Network Transmit", Unit: "bytes_per_second", Scope: "pod", Metric: "network_transmit", PromQL: podMetricQuery("container_network_transmit_bytes_total", append([]string{`interface!="lo"`}, matchers...), window, "total", "rate")},
	}
	return specs
}

func workloadOverviewMetricSpecs(request ResourceMonitorRequest, podNames []string) []monitorOverviewQuerySpec {
	podMatcher, matcherOK := podRegexMatcher(podNames)
	matchers := []string{"namespace=" + strconv.Quote(request.Namespace), podMatcher}
	window := monitorMetricRateWindow(resourceMonitorRequestAsMetricQuery(request, "pod", "cpu"))
	byPod := request.Mode == "pod"
	if !matcherOK {
		return []monitorOverviewQuerySpec{
			emptyQuerySpec("cpu", "Total CPU", "cores", "pod", "cpu"),
			emptyQuerySpec("memory", "Total Memory", "bytes", "pod", "memory"),
			emptyQuerySpec("networkReceive", "Network Receive", "bytes_per_second", "pod", "network_receive"),
			emptyQuerySpec("networkTransmit", "Network Transmit", "bytes_per_second", "pod", "network_transmit"),
		}
	}
	return []monitorOverviewQuerySpec{
		{Key: "cpu", Title: "Total CPU", Unit: "cores", Scope: "pod", Metric: "cpu", PromQL: podMetricQuery("container_cpu_usage_seconds_total", append([]string{`container!=""`, `container!="POD"`, `image!=""`}, matchers...), window, monitorWorkloadGroup(byPod), "rate")},
		{Key: "memory", Title: "Total Memory", Unit: "bytes", Scope: "pod", Metric: "memory", PromQL: podMetricQuery("container_memory_working_set_bytes", append([]string{`container!=""`, `container!="POD"`, `image!=""`}, matchers...), "", monitorWorkloadGroup(byPod), "gauge")},
		{Key: "networkReceive", Title: "Network Receive", Unit: "bytes_per_second", Scope: "pod", Metric: "network_receive", PromQL: podMetricQuery("container_network_receive_bytes_total", append([]string{`interface!="lo"`}, matchers...), window, monitorWorkloadGroup(byPod), "rate")},
		{Key: "networkTransmit", Title: "Network Transmit", Unit: "bytes_per_second", Scope: "pod", Metric: "network_transmit", PromQL: podMetricQuery("container_network_transmit_bytes_total", append([]string{`interface!="lo"`}, matchers...), window, monitorWorkloadGroup(byPod), "rate")},
	}
}

func genericMetricSpec(key, title, scope, metric, unit, namespace, name string) monitorOverviewQuerySpec {
	return monitorOverviewQuerySpec{Key: key, Title: title, Scope: scope, Metric: metric, Unit: unit, ObjectName: func(labels map[string]string) string { return monitorMetricObjectName(scope, labels) }}
}

func emptyQuerySpec(key, title, unit, scope, metric string) monitorOverviewQuerySpec {
	return monitorOverviewQuerySpec{Key: key, Title: title, Unit: unit, Scope: scope, Metric: metric, PromQL: `vector(0) unless vector(0)`}
}

func podMetricQuery(metric string, matchers []string, window string, group string, metricType string) string {
	selector := promSelector(metric, matchers)
	value := selector
	if metricType == "rate" {
		value = fmt.Sprintf("rate(%s[%s])", selector, window)
	}
	switch group {
	case "container":
		return fmt.Sprintf("sum by (namespace, pod, container) (%s)", value)
	case "pod":
		return fmt.Sprintf("sum by (namespace, pod) (%s)", value)
	default:
		return fmt.Sprintf("sum(%s)", value)
	}
}

func monitorPodGroup(byContainer bool) string {
	if byContainer {
		return "container"
	}
	return "total"
}

func monitorWorkloadGroup(byPod bool) string {
	if byPod {
		return "pod"
	}
	return "total"
}

func resourceMonitorRequestAsMetricQuery(request ResourceMonitorRequest, scope, metric string) MonitorMetricQuery {
	return MonitorMetricQuery{Scope: scope, Metric: metric, Namespace: request.Namespace, Name: request.Name, Start: request.Start, End: request.End, Step: request.Step, IsRange: true}
}

func nodeIdentityFromNode(node corev1.Node) nodeMetricIdentity {
	candidates := []string{node.Name}
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP || address.Type == corev1.NodeExternalIP || address.Type == corev1.NodeHostName {
			candidates = append(candidates, address.Address)
		}
	}
	return nodeMetricIdentity{Name: node.Name, Addresses: uniqueNonEmptyStrings(candidates)}
}

func nodeSelectorUnion(metric string, baseMatchers []string, identity nodeMetricIdentity) string {
	return "(" + strings.Join(nodeSelectorVariants(metric, baseMatchers, identity), " or ") + ")"
}

func nodeRateExpr(metric string, baseMatchers []string, identity nodeMetricIdentity, window string) string {
	var parts []string
	for _, selector := range nodeSelectorVariants(metric, baseMatchers, identity) {
		parts = append(parts, fmt.Sprintf("rate(%s[%s])", selector, window))
	}
	return "(" + strings.Join(parts, " or ") + ")"
}

func nodeSelectorVariants(metric string, baseMatchers []string, identity nodeMetricIdentity) []string {
	var variants []string
	if identity.Name != "" {
		variants = append(variants, promSelector(metric, append(append([]string{}, baseMatchers...), "node="+strconv.Quote(identity.Name))))
		variants = append(variants, promSelector(metric, append(append([]string{}, baseMatchers...), "nodename="+strconv.Quote(identity.Name))))
	}
	if len(identity.Addresses) > 0 {
		patterns := make([]string, 0, len(identity.Addresses))
		for _, candidate := range identity.Addresses {
			patterns = append(patterns, regexp.QuoteMeta(candidate))
		}
		pattern := `^(` + strings.Join(patterns, "|") + `)(:[0-9]+)?$`
		variants = append(variants, promSelector(metric, append(append([]string{}, baseMatchers...), "instance=~"+strconv.Quote(pattern))))
	}
	return uniqueNonEmptyStrings(variants)
}

func nodeFilesystemBaseMatchers() []string {
	return []string{
		`fstype!~"^(tmpfs|devtmpfs|overlay|squashfs|nsfs|tracefs|proc|sysfs)$"`,
		`mountpoint!~"^/(run|var/lib/(docker|containerd|kubelet)/pods)($|/)"`,
	}
}

func podRegexMatcher(podNames []string) (string, bool) {
	if len(podNames) == 0 {
		return `pod=~"^$"`, false
	}
	parts := make([]string, 0, len(podNames))
	for _, pod := range podNames {
		parts = append(parts, regexp.QuoteMeta(pod))
	}
	pattern := `^(` + strings.Join(parts, "|") + `)$`
	if len(pattern) > monitorOverviewMaxPodMatcherLength {
		return "", false
	}
	return "pod=~" + strconv.Quote(pattern), true
}

func buildMonitorPodSummaries(ctx context.Context, client kubernetes.Interface, pods []corev1.Pod) []MonitorPodSummary {
	summaries := make([]MonitorPodSummary, 0, len(pods))
	for _, pod := range pods {
		owner, _ := ResolvePodOwnerFromClient(ctx, client, &pod)
		var ownerPtr *ResourceReference
		if owner.Name != "" {
			ownerPtr = &owner
		}
		summaries = append(summaries, MonitorPodSummary{
			Namespace:    pod.Namespace,
			Name:         pod.Name,
			Phase:        string(pod.Status.Phase),
			Status:       podStatusReason(pod),
			NodeName:     pod.Spec.NodeName,
			Owner:        ownerPtr,
			RestartCount: podRestartCount(pod),
			Containers:   monitorContainerStatuses(pod),
			CreatedAt:    pod.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		})
	}
	return summaries
}

func applyPodInstantMetrics(ctx context.Context, client prometheusQuerier, summaries []MonitorPodSummary, pods []corev1.Pod, at time.Time) {
	podNamesByNamespace := map[string][]string{}
	for _, pod := range pods {
		podNamesByNamespace[pod.Namespace] = append(podNamesByNamespace[pod.Namespace], pod.Name)
	}
	cpuValues := map[string]float64{}
	memoryValues := map[string]float64{}
	for namespace, names := range podNamesByNamespace {
		matcher, ok := podRegexMatcher(names)
		if !ok {
			continue
		}
		base := []string{"namespace=" + strconv.Quote(namespace), matcher}
		cpuQuery := podMetricQuery("container_cpu_usage_seconds_total", append([]string{`container!=""`, `container!="POD"`, `image!=""`}, base...), "5m", "pod", "rate")
		memQuery := podMetricQuery("container_memory_working_set_bytes", append([]string{`container!=""`, `container!="POD"`, `image!=""`}, base...), "", "pod", "gauge")
		for key, value := range queryInstantValuesByPod(ctx, client, cpuQuery, at) {
			cpuValues[key] = value
		}
		for key, value := range queryInstantValuesByPod(ctx, client, memQuery, at) {
			memoryValues[key] = value
		}
	}
	for i := range summaries {
		key := summaries[i].Namespace + "/" + summaries[i].Name
		if value, ok := cpuValues[key]; ok {
			summaries[i].CurrentCPUCores = value
			summaries[i].MetricsAvailable = true
		}
		if value, ok := memoryValues[key]; ok {
			summaries[i].CurrentMemoryBytes = value
			summaries[i].MetricsAvailable = true
		}
	}
}

func queryInstantValuesByPod(ctx context.Context, client prometheusQuerier, query string, at time.Time) map[string]float64 {
	result := map[string]float64{}
	series, err := client.Query(ctx, query, at)
	if err != nil {
		return result
	}
	for _, item := range series {
		if len(item.Samples) == 0 {
			continue
		}
		namespace := item.Labels["namespace"]
		pod := item.Labels["pod"]
		if namespace == "" || pod == "" {
			continue
		}
		result[namespace+"/"+pod] = item.Samples[len(item.Samples)-1].Value
	}
	return result
}

func listMonitorInventoryWorkloads(ctx context.Context, client kubernetes.Interface) ([]WorkloadPodSet, error) {
	var result []WorkloadPodSet
	deployments, err := client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, deployment := range deployments.Items {
		workload, err := ListPodsForWorkloadFromClient(ctx, client, "deployment", deployment.Namespace, deployment.Name)
		if err != nil {
			return nil, err
		}
		result = append(result, workload)
	}
	statefulSets, err := client.AppsV1().StatefulSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, statefulSet := range statefulSets.Items {
		workload, err := ListPodsForWorkloadFromClient(ctx, client, "statefulset", statefulSet.Namespace, statefulSet.Name)
		if err != nil {
			return nil, err
		}
		result = append(result, workload)
	}
	daemonSets, err := client.AppsV1().DaemonSets(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, daemonSet := range daemonSets.Items {
		workload, err := ListPodsForWorkloadFromClient(ctx, client, "daemonset", daemonSet.Namespace, daemonSet.Name)
		if err != nil {
			return nil, err
		}
		result = append(result, workload)
	}
	return result, nil
}

func applyNodeInventoryMetrics(ctx context.Context, client prometheusQuerier, row *MonitorInventoryNode, node corev1.Node, at time.Time) {
	identity := nodeIdentityFromNode(node)
	window := "5m"
	row.CPUPercent = queryInstantSingleValue(ctx, client, fmt.Sprintf(`100 * (1 - avg(%s))`, nodeRateExpr("node_cpu_seconds_total", []string{`mode="idle"`}, identity, window)), at)
	row.MemoryPercent = queryInstantSingleValue(ctx, client, fmt.Sprintf(`100 * (1 - max(%s) / max(%s))`, nodeSelectorUnion("node_memory_MemAvailable_bytes", nil, identity), nodeSelectorUnion("node_memory_MemTotal_bytes", nil, identity)), at)
	row.NetworkReceiveBytesPerSecond = queryInstantSingleValue(ctx, client, fmt.Sprintf(`sum(%s)`, nodeRateExpr("node_network_receive_bytes_total", []string{`device!~"^(lo|veth.*|docker.*|cni.*|flannel.*|cali.*)$"`}, identity, window)), at)
	row.NetworkTransmitBytesPerSecond = queryInstantSingleValue(ctx, client, fmt.Sprintf(`sum(%s)`, nodeRateExpr("node_network_transmit_bytes_total", []string{`device!~"^(lo|veth.*|docker.*|cni.*|flannel.*|cali.*)$"`}, identity, window)), at)
	row.DiskUsagePercent = queryInstantSingleValue(ctx, client, fmt.Sprintf(`100 * (1 - sum(%s) / sum(%s))`, nodeSelectorUnion("node_filesystem_avail_bytes", nodeFilesystemBaseMatchers(), identity), nodeSelectorUnion("node_filesystem_size_bytes", nodeFilesystemBaseMatchers(), identity)), at)
}

func applyWorkloadInventoryMetrics(ctx context.Context, client prometheusQuerier, row *MonitorInventoryWorkload, pods []corev1.Pod, at time.Time) {
	if len(pods) == 0 {
		return
	}
	podsByNamespace := map[string][]string{}
	for _, pod := range pods {
		podsByNamespace[pod.Namespace] = append(podsByNamespace[pod.Namespace], pod.Name)
	}
	var cpu, memory, receive, transmit float64
	var hasCPU, hasMemory, hasReceive, hasTransmit bool
	for namespace, podNames := range podsByNamespace {
		podMatcher, matcherOK := podRegexMatcher(podNames)
		if !matcherOK {
			continue
		}
		matchers := []string{"namespace=" + strconv.Quote(namespace), podMatcher}
		if value := queryInstantSingleValue(ctx, client, podMetricQuery("container_cpu_usage_seconds_total", append([]string{`container!=""`, `container!="POD"`, `image!=""`}, matchers...), "5m", "total", "rate"), at); value != nil {
			cpu += *value
			hasCPU = true
		}
		if value := queryInstantSingleValue(ctx, client, podMetricQuery("container_memory_working_set_bytes", append([]string{`container!=""`, `container!="POD"`, `image!=""`}, matchers...), "", "total", "gauge"), at); value != nil {
			memory += *value
			hasMemory = true
		}
		if value := queryInstantSingleValue(ctx, client, podMetricQuery("container_network_receive_bytes_total", append([]string{`interface!="lo"`}, matchers...), "5m", "total", "rate"), at); value != nil {
			receive += *value
			hasReceive = true
		}
		if value := queryInstantSingleValue(ctx, client, podMetricQuery("container_network_transmit_bytes_total", append([]string{`interface!="lo"`}, matchers...), "5m", "total", "rate"), at); value != nil {
			transmit += *value
			hasTransmit = true
		}
	}
	if hasCPU {
		row.CPUCores = float64Ptr(cpu)
	}
	if hasMemory {
		row.MemoryBytes = float64Ptr(memory)
	}
	if hasReceive {
		row.NetworkReceiveBytesPerSecond = float64Ptr(receive)
	}
	if hasTransmit {
		row.NetworkTransmitBytesPerSecond = float64Ptr(transmit)
	}
}

type monitorPVCInventoryMetrics struct {
	used     map[string]*float64
	capacity map[string]*float64
	usage    map[string]*float64
}

func queryPVCInventoryMetrics(ctx context.Context, client prometheusQuerier, at time.Time) monitorPVCInventoryMetrics {
	used := queryInstantValuesByPVC(ctx, client, `max by (namespace, persistentvolumeclaim) (kubelet_volume_stats_used_bytes)`, at)
	capacity := queryInstantValuesByPVC(ctx, client, `max by (namespace, persistentvolumeclaim) (kubelet_volume_stats_capacity_bytes)`, at)
	usage := map[string]*float64{}
	for key, usedValue := range used {
		capacityValue, ok := capacity[key]
		if !ok || capacityValue == nil || *capacityValue <= 0 || usedValue == nil {
			continue
		}
		usage[key] = float64Ptr(100 * (*usedValue) / (*capacityValue))
	}
	return monitorPVCInventoryMetrics{used: used, capacity: capacity, usage: usage}
}

func queryInstantValuesByPVC(ctx context.Context, client prometheusQuerier, query string, at time.Time) map[string]*float64 {
	result := map[string]*float64{}
	series, err := client.Query(ctx, query, at)
	if err != nil {
		return result
	}
	for _, item := range series {
		if len(item.Samples) == 0 {
			continue
		}
		namespace := item.Labels["namespace"]
		name := item.Labels["persistentvolumeclaim"]
		if namespace == "" || name == "" {
			continue
		}
		value := item.Samples[len(item.Samples)-1].Value
		result[namespace+"/"+name] = float64Ptr(value)
	}
	return result
}

func queryInstantSingleValue(ctx context.Context, client prometheusQuerier, query string, at time.Time) *float64 {
	series, err := client.Query(ctx, query, at)
	if err != nil {
		return nil
	}
	for _, item := range series {
		if len(item.Samples) == 0 {
			continue
		}
		return float64Ptr(item.Samples[len(item.Samples)-1].Value)
	}
	return nil
}

func float64Ptr(value float64) *float64 {
	return &value
}

func monitorContainerStatuses(pod corev1.Pod) []MonitorContainerStatus {
	imageByName := map[string]string{}
	for _, container := range pod.Spec.Containers {
		imageByName[container.Name] = container.Image
	}
	result := make([]MonitorContainerStatus, 0, len(pod.Spec.Containers))
	seen := map[string]struct{}{}
	for _, status := range pod.Status.ContainerStatuses {
		state, reason := containerStateReason(status.State)
		lastState, lastReason, lastExitCode, oomKilled := containerLastState(status.LastTerminationState)
		seen[status.Name] = struct{}{}
		result = append(result, MonitorContainerStatus{
			Name:         status.Name,
			Image:        imageByName[status.Name],
			Ready:        status.Ready,
			RestartCount: status.RestartCount,
			State:        state,
			Reason:       reason,
			LastState:    lastState,
			LastReason:   lastReason,
			LastExitCode: lastExitCode,
			OOMKilled:    oomKilled || lastReason == "OOMKilled",
		})
	}
	for _, container := range pod.Spec.Containers {
		if _, ok := seen[container.Name]; ok {
			continue
		}
		result = append(result, MonitorContainerStatus{
			Name:   container.Name,
			Image:  container.Image,
			State:  "Unknown",
			Reason: "StatusNotReported",
		})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func containerStateReason(state corev1.ContainerState) (string, string) {
	switch {
	case state.Waiting != nil:
		return "Waiting", state.Waiting.Reason
	case state.Running != nil:
		return "Running", ""
	case state.Terminated != nil:
		return "Terminated", state.Terminated.Reason
	default:
		return "Unknown", ""
	}
}

func containerLastState(state corev1.ContainerState) (string, string, int32, bool) {
	if state.Terminated != nil {
		return "Terminated", state.Terminated.Reason, state.Terminated.ExitCode, state.Terminated.Reason == "OOMKilled"
	}
	if state.Waiting != nil {
		return "Waiting", state.Waiting.Reason, 0, false
	}
	if state.Running != nil {
		return "Running", "", 0, false
	}
	return "", "", 0, false
}

func podRestartCount(pod corev1.Pod) int32 {
	var total int32
	for _, status := range pod.Status.ContainerStatuses {
		total += status.RestartCount
	}
	return total
}

func podStatusReason(pod corev1.Pod) string {
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil && status.State.Waiting.Reason != "" {
			return status.State.Waiting.Reason
		}
		if status.State.Terminated != nil && status.State.Terminated.Reason != "" {
			return status.State.Terminated.Reason
		}
	}
	return string(pod.Status.Phase)
}

func podMetadata(pod corev1.Pod) map[string]interface{} {
	return map[string]interface{}{
		"phase":        string(pod.Status.Phase),
		"status":       podStatusReason(pod),
		"nodeName":     pod.Spec.NodeName,
		"restartCount": podRestartCount(pod),
		"podIP":        pod.Status.PodIP,
		"hostIP":       pod.Status.HostIP,
		"labels":       pod.Labels,
	}
}

func workloadResourceConfig(workload WorkloadPodSet) map[string]interface{} {
	var cpuRequest, cpuLimit float64
	var memoryRequest, memoryLimit int64
	for _, container := range workloadTemplateContainers(workload) {
		if q, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
			cpuRequest += float64(q.MilliValue()) / 1000
		}
		if q, ok := container.Resources.Limits[corev1.ResourceCPU]; ok {
			cpuLimit += float64(q.MilliValue()) / 1000
		}
		if q, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
			memoryRequest += q.Value()
		}
		if q, ok := container.Resources.Limits[corev1.ResourceMemory]; ok {
			memoryLimit += q.Value()
		}
	}
	replicas := float64(workload.Replicas)
	if replicas <= 0 {
		replicas = float64(len(workload.Pods))
	}
	return map[string]interface{}{
		"cpuRequestCores":    cpuRequest * replicas,
		"cpuLimitCores":      cpuLimit * replicas,
		"memoryRequestBytes": memoryRequest * int64(replicas),
		"memoryLimitBytes":   memoryLimit * int64(replicas),
		"replicasUsed":       replicas,
	}
}

func selectMonitorPods(pods []corev1.Pod, selected []string, limit int) []corev1.Pod {
	if len(selected) > 0 {
		selectedSet := map[string]struct{}{}
		for _, name := range selected {
			selectedSet[name] = struct{}{}
		}
		result := make([]corev1.Pod, 0, len(selected))
		for _, pod := range pods {
			if _, ok := selectedSet[pod.Name]; ok {
				result = append(result, pod)
			}
		}
		sortPodsStable(result)
		return result
	}
	if limit <= 0 || limit > monitorOverviewMaxSeriesLimit {
		limit = monitorOverviewDefaultSeriesLimit
	}
	result := append([]corev1.Pod(nil), pods...)
	sortPodsStable(result)
	if len(result) > limit {
		return result[:limit]
	}
	return result
}

func parseSelectedMonitorPods(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func monitorOverviewTimeRange(request ResourceMonitorRequest) MonitorOverviewTimeRange {
	return MonitorOverviewTimeRange{
		Start:      request.Start.UTC().Format(time.RFC3339Nano),
		End:        request.End.UTC().Format(time.RFC3339Nano),
		Step:       request.Step.Seconds(),
		RateWindow: monitorMetricRateWindow(resourceMonitorRequestAsMetricQuery(request, "pod", "cpu")),
	}
}

func monitorOverviewLimits(request ResourceMonitorRequest) MonitorOverviewLimits {
	return MonitorOverviewLimits{
		SeriesLimit:      request.PodLimit,
		PodLimit:         request.PodLimit,
		HardSeriesLimit:  monitorOverviewMaxSeriesLimit,
		MaxSamples:       maxMonitorMetricSamples,
		MaxRangeSeconds:  int(maxMonitorMetricRange.Seconds()),
		MaxMatcherLength: monitorOverviewMaxPodMatcherLength,
		MaxQueryParallel: monitorOverviewMaxConcurrency,
	}
}

func getNodeMonitorTop(ctx context.Context, client kubernetes.Interface, promClient prometheusQuerier, metric string, limit int, at time.Time) ([]MonitorTopItem, error) {
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	nodeByInstance := map[string]ResourceReference{}
	for _, node := range nodes.Items {
		ref := ResourceReference{Kind: "Node", Name: node.Name, UID: string(node.UID), Validated: true}
		nodeByInstance[node.Name] = ref
		for _, address := range node.Status.Addresses {
			nodeByInstance[address.Address] = ref
		}
	}
	query := nodeTopQuery(metric, limit)
	series, err := promClient.Query(ctx, query, at)
	if err != nil {
		return nil, err
	}
	items := make([]MonitorTopItem, 0, len(series))
	for _, item := range series {
		if len(item.Samples) == 0 {
			continue
		}
		name := item.Labels["node"]
		if name == "" {
			name = item.Labels["nodename"]
		}
		if name == "" {
			name = stripInstancePort(item.Labels["instance"])
		}
		ref, ok := nodeByInstance[name]
		if !ok {
			ref = ResourceReference{Kind: "Node", Name: name}
		}
		items = append(items, MonitorTopItem{Resource: ref, Value: item.Samples[len(item.Samples)-1].Value, Unit: monitorTopUnit("node", metric)})
	}
	sortMonitorTopItems(items)
	return capMonitorTopItems(items, limit), nil
}

func getPodMonitorTop(ctx context.Context, client kubernetes.Interface, promClient prometheusQuerier, metric, namespace string, limit int, at time.Time) ([]MonitorTopItem, error) {
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	currentPods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	currentPodRefs := map[string]ResourceReference{}
	for _, pod := range currentPods.Items {
		currentPodRefs[pod.Namespace+"/"+pod.Name] = ResourceReference{Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, UID: string(pod.UID), Validated: true}
	}
	series, err := promClient.Query(ctx, podTopQuery(metric, namespace, limit), at)
	if err != nil {
		return nil, err
	}
	items := make([]MonitorTopItem, 0, len(series))
	for _, item := range series {
		if len(item.Samples) == 0 || item.Labels["pod"] == "" {
			continue
		}
		ref := ResourceReference{Kind: "Pod", Namespace: item.Labels["namespace"], Name: item.Labels["pod"]}
		if currentRef, ok := currentPodRefs[ref.Namespace+"/"+ref.Name]; ok {
			ref = currentRef
		}
		items = append(items, MonitorTopItem{
			Resource: ref,
			Value:    item.Samples[len(item.Samples)-1].Value,
			Unit:     monitorTopUnit("pod", metric),
		})
	}
	sortMonitorTopItems(items)
	return capMonitorTopItems(items, limit), nil
}

func getWorkloadMonitorTop(ctx context.Context, client kubernetes.Interface, promClient prometheusQuerier, metric, namespace string, limit int, at time.Time) ([]MonitorTopItem, error) {
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	podList, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	podByKey := map[string]corev1.Pod{}
	for _, pod := range podList.Items {
		podByKey[pod.Namespace+"/"+pod.Name] = pod
	}
	series, err := promClient.Query(ctx, podTopQuery(metric, namespace, monitorOverviewMaxTopLimit*10), at)
	if err != nil {
		return nil, err
	}
	aggregates := map[string]MonitorTopItem{}
	for _, item := range series {
		if len(item.Samples) == 0 {
			continue
		}
		key := item.Labels["namespace"] + "/" + item.Labels["pod"]
		pod, ok := podByKey[key]
		if !ok {
			continue
		}
		owner, _ := ResolvePodOwnerFromClient(ctx, client, &pod)
		if owner.Name == "" || !isTopWorkloadKind(owner.Kind) {
			continue
		}
		ownerKey := owner.Kind + "/" + owner.Namespace + "/" + owner.Name
		current := aggregates[ownerKey]
		if current.Resource.Name == "" {
			current.Resource = owner
			current.Unit = monitorTopUnit("workload", metric)
		}
		current.Value += item.Samples[len(item.Samples)-1].Value
		aggregates[ownerKey] = current
	}
	items := make([]MonitorTopItem, 0, len(aggregates))
	for _, item := range aggregates {
		items = append(items, item)
	}
	sortMonitorTopItems(items)
	return capMonitorTopItems(items, limit), nil
}

func nodeTopQuery(metric string, limit int) string {
	if metric == "memory" {
		return fmt.Sprintf(`topk(%d, 100 * (1 - max by (instance, node, nodename) (node_memory_MemAvailable_bytes) / max by (instance, node, nodename) (node_memory_MemTotal_bytes)))`, limit)
	}
	return fmt.Sprintf(`topk(%d, 100 * (1 - avg by (instance, node, nodename) (rate(node_cpu_seconds_total{mode="idle"}[5m]))))`, limit)
}

func podTopQuery(metric, namespace string, limit int) string {
	matchers := []string{`container!=""`, `container!="POD"`, `image!=""`}
	if namespace != "" {
		matchers = append(matchers, "namespace="+strconv.Quote(namespace))
	}
	if metric == "memory" {
		return fmt.Sprintf(`topk(%d, sum by (namespace, pod) (%s))`, limit, promSelector("container_memory_working_set_bytes", matchers))
	}
	return fmt.Sprintf(`topk(%d, sum by (namespace, pod) (rate(%s[5m])))`, limit, promSelector("container_cpu_usage_seconds_total", matchers))
}

func normalizeMonitorTopLimit(value string) int {
	if strings.TrimSpace(value) == "" {
		return monitorOverviewDefaultTopLimit
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return monitorOverviewDefaultTopLimit
	}
	if parsed > monitorOverviewMaxTopLimit {
		return monitorOverviewMaxTopLimit
	}
	return parsed
}

func monitorTopUnit(resource, metric string) string {
	if metric == "cpu" {
		if resource == "node" {
			return "percent"
		}
		return "cores"
	}
	if metric == "memory" {
		return "bytes"
	}
	return ""
}

func monitorTopNotes(resource string) []string {
	if resource == "workload" {
		return []string{"Workload Top N aggregates current Pods by ownerReference in the backend. Deleted Pods from previous rollouts are not included without kube-state-metrics."}
	}
	return nil
}

func sortMonitorTopItems(items []MonitorTopItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Value != items[j].Value {
			return items[i].Value > items[j].Value
		}
		return items[i].Resource.Kind+"/"+items[i].Resource.Namespace+"/"+items[i].Resource.Name < items[j].Resource.Kind+"/"+items[j].Resource.Namespace+"/"+items[j].Resource.Name
	})
}

func capMonitorTopItems(items []MonitorTopItem, limit int) []MonitorTopItem {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func isTopWorkloadKind(kind string) bool {
	return kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" || kind == "Job" || kind == "CronJob"
}

func stripInstancePort(instance string) string {
	if instance == "" {
		return ""
	}
	if idx := strings.LastIndex(instance, ":"); idx > -1 {
		return instance[:idx]
	}
	return instance
}

func nodeStatus(node corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			if condition.Status == corev1.ConditionTrue {
				return "Ready"
			}
			return "NotReady"
		}
	}
	return "Unknown"
}

func nodeAddress(node corev1.Node, addressType corev1.NodeAddressType) string {
	for _, address := range node.Status.Addresses {
		if address.Type == addressType {
			return address.Address
		}
	}
	return ""
}

func pvcStorageClassName(pvc corev1.PersistentVolumeClaim) string {
	if pvc.Spec.StorageClassName == nil {
		return ""
	}
	return *pvc.Spec.StorageClassName
}

func pvcRequestedStorage(pvc corev1.PersistentVolumeClaim) string {
	if quantity, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		return quantity.String()
	}
	return ""
}

func pvcAccessModes(pvc corev1.PersistentVolumeClaim) []string {
	result := make([]string, 0, len(pvc.Spec.AccessModes))
	for _, mode := range pvc.Spec.AccessModes {
		result = append(result, string(mode))
	}
	return result
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func workloadFromAppsObject(obj interface{}) (WorkloadPodSet, bool) {
	switch workload := obj.(type) {
	case appsv1.Deployment:
		replicas := int32(1)
		if workload.Spec.Replicas != nil {
			replicas = *workload.Spec.Replicas
		}
		return WorkloadPodSet{Kind: "Deployment", Namespace: workload.Namespace, Name: workload.Name, UID: workload.UID, Replicas: replicas, ReadyReplicas: workload.Status.ReadyReplicas, Template: workload.Spec.Template}, true
	case appsv1.StatefulSet:
		replicas := int32(1)
		if workload.Spec.Replicas != nil {
			replicas = *workload.Spec.Replicas
		}
		return WorkloadPodSet{Kind: "StatefulSet", Namespace: workload.Namespace, Name: workload.Name, UID: workload.UID, Replicas: replicas, ReadyReplicas: workload.Status.ReadyReplicas, Template: workload.Spec.Template}, true
	case appsv1.DaemonSet:
		return WorkloadPodSet{Kind: "DaemonSet", Namespace: workload.Namespace, Name: workload.Name, UID: workload.UID, Replicas: workload.Status.DesiredNumberScheduled, ReadyReplicas: workload.Status.NumberReady, Template: workload.Spec.Template}, true
	default:
		return WorkloadPodSet{}, false
	}
}

func monitorStructuredInvalidParams(err error) *MonitorStructuredError {
	if err == nil {
		return nil
	}
	return &MonitorStructuredError{Code: MonitorErrorInvalidParams, Message: err.Error()}
}

func monitorErrorFromString(code, message string) *MonitorStructuredError {
	return &MonitorStructuredError{Code: code, Message: message}
}

func monitorErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
