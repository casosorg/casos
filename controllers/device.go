package controllers

import (
	"path/filepath"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/casosorg/casos/object"
)

const (
	// Free means at most this share committed; 25% leaves room for daemonsets.
	deviceFreeCpuPercent = 25
	deviceFreeMemPercent = 25

	deviceStateFree     = "free"
	deviceStateBusy     = "busy"
	deviceStateReserved = "reserved"
	deviceStateCordoned = "cordoned"
	deviceStateOffline  = "offline"
)

type deviceGpu struct {
	Resource  string `json:"resource"`
	Product   string `json:"product"`
	Total     int64  `json:"total"`
	Requested int64  `json:"requested"`
	Free      int64  `json:"free"`
}

type deviceSummary struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Roles  []string `json:"roles"`
	// free, busy, reserved, cordoned or offline.
	State  string   `json:"state"`
	Taints []string `json:"taints"`
	Pods   int      `json:"pods"`

	CpuTotalM     int64 `json:"cpuTotalM"`
	CpuRequestedM int64 `json:"cpuRequestedM"`
	CpuFreeM      int64 `json:"cpuFreeM"`
	CpuPercent    int   `json:"cpuPercent"`

	MemTotalMi     int64 `json:"memTotalMi"`
	MemRequestedMi int64 `json:"memRequestedMi"`
	MemFreeMi      int64 `json:"memFreeMi"`
	MemPercent     int   `json:"memPercent"`

	// Live usage is informational; committed capacity decides whether a device is free.
	CpuUsedM   int64 `json:"cpuUsedM"`
	MemUsedMi  int64 `json:"memUsedMi"`
	UsageKnown bool  `json:"usageKnown"`

	Gpus     []deviceGpu `json:"gpus"`
	GpuTotal int64       `json:"gpuTotal"`
	GpuFree  int64       `json:"gpuFree"`
}

type deviceTotals struct {
	Devices   int   `json:"devices"`
	Free      int   `json:"free"`
	CpuFreeM  int64 `json:"cpuFreeM"`
	MemFreeMi int64 `json:"memFreeMi"`
	GpuTotal  int64 `json:"gpuTotal"`
	GpuFree   int64 `json:"gpuFree"`
}

type deviceReport struct {
	Devices    []deviceSummary `json:"devices"`
	Totals     deviceTotals    `json:"totals"`
	UsageKnown bool            `json:"usageKnown"`
}

// GetDevices
// @router /api/get-devices [get]
func (c *ApiController) GetDevices() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	nodes, err := object.GetNodes(cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	pods, err := object.GetPods(cfg, "")
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	usage := map[string]object.NodeMetric{}
	if srvCfg := getServerConfig(); srvCfg != nil {
		if metrics, mErr := object.GetClusterMetrics(cfg, filepath.Join(srvCfg.DataDir, "tls")); mErr == nil {
			for _, m := range metrics.Nodes {
				usage[m.Name] = m
			}
		}
	}

	report := deviceReport{Devices: []deviceSummary{}}
	for _, node := range nodes {
		device := deviceSummaryOf(node, podsOnNode(pods, node.Name))
		if metric, ok := usage[node.Name]; ok {
			device.CpuUsedM = metric.CPUUsedM
			device.MemUsedMi = metric.MemUsedMi
			device.UsageKnown = true
			report.UsageKnown = true
		}
		report.Devices = append(report.Devices, device)
	}

	sortDevices(report.Devices)
	for _, device := range report.Devices {
		report.Totals.Devices++
		report.Totals.GpuTotal += device.GpuTotal
		if device.State != deviceStateFree {
			continue
		}
		// Tainted or cordoned capacity isn't counted as free.
		report.Totals.Free++
		report.Totals.CpuFreeM += device.CpuFreeM
		report.Totals.MemFreeMi += device.MemFreeMi
		report.Totals.GpuFree += device.GpuFree
	}

	c.ResponseOk(report)
}

func podsOnNode(pods []corev1.Pod, node string) []corev1.Pod {
	result := []corev1.Pod{}
	for i := range pods {
		pod := pods[i]
		if pod.Spec.NodeName != node {
			continue
		}
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result = append(result, pod)
	}
	return result
}

func deviceSummaryOf(node corev1.Node, pods []corev1.Pod) deviceSummary {
	summary := toNodeSummary(node)
	device := deviceSummary{
		Name:       node.Name,
		Status:     summary.Status,
		Roles:      summary.Roles,
		Taints:     deviceTaints(node),
		Pods:       len(pods),
		CpuTotalM:  quantityIn(corev1.ResourceCPU, node.Status.Allocatable[corev1.ResourceCPU]),
		MemTotalMi: quantityIn(corev1.ResourceMemory, node.Status.Allocatable[corev1.ResourceMemory]),
		Gpus:       []deviceGpu{},
	}

	for _, pod := range pods {
		device.CpuRequestedM += podResourceRequest(&pod, corev1.ResourceCPU)
		device.MemRequestedMi += podResourceRequest(&pod, corev1.ResourceMemory)
	}
	device.CpuFreeM = remaining(device.CpuTotalM, device.CpuRequestedM)
	device.MemFreeMi = remaining(device.MemTotalMi, device.MemRequestedMi)
	device.CpuPercent = percentUsed(device.CpuRequestedM, device.CpuTotalM)
	device.MemPercent = percentUsed(device.MemRequestedMi, device.MemTotalMi)

	for _, name := range gpuResourcesOf(node) {
		allocatable := node.Status.Allocatable[name]
		gpu := deviceGpu{
			Resource: string(name),
			Product:  gpuProductOf(node, name),
			Total:    allocatable.Value(),
		}
		for _, pod := range pods {
			gpu.Requested += podResourceRequest(&pod, name)
		}
		gpu.Free = remaining(gpu.Total, gpu.Requested)
		device.Gpus = append(device.Gpus, gpu)
		device.GpuTotal += gpu.Total
		device.GpuFree += gpu.Free
	}

	device.State = deviceStateOf(node, device)
	return device
}

func deviceStateOf(node corev1.Node, device deviceSummary) string {
	switch {
	case device.Status != "Ready":
		return deviceStateOffline
	case node.Spec.Unschedulable:
		return deviceStateCordoned
	case len(device.Taints) > 0:
		return deviceStateReserved
	case device.GpuTotal > 0 && device.GpuFree == 0:
		return deviceStateBusy
	case device.CpuPercent > deviceFreeCpuPercent || device.MemPercent > deviceFreeMemPercent:
		return deviceStateBusy
	default:
		return deviceStateFree
	}
}

func sortDevices(devices []deviceSummary) {
	rank := map[string]int{
		deviceStateFree:     0,
		deviceStateBusy:     1,
		deviceStateReserved: 2,
		deviceStateCordoned: 3,
		deviceStateOffline:  4,
	}
	sort.SliceStable(devices, func(i, j int) bool {
		a, b := devices[i], devices[j]
		if rank[a.State] != rank[b.State] {
			return rank[a.State] < rank[b.State]
		}
		if a.GpuFree != b.GpuFree {
			return a.GpuFree > b.GpuFree
		}
		if a.CpuFreeM != b.CpuFreeM {
			return a.CpuFreeM > b.CpuFreeM
		}
		return a.Name < b.Name
	})
}

func deviceTaints(node corev1.Node) []string {
	taints := []string{}
	for _, taint := range node.Spec.Taints {
		if taint.Effect != corev1.TaintEffectNoSchedule && taint.Effect != corev1.TaintEffectNoExecute {
			continue
		}
		entry := taint.Key
		if taint.Value != "" {
			entry += "=" + taint.Value
		}
		taints = append(taints, entry+":"+string(taint.Effect))
	}
	return taints
}

// GPUs are vendor extended resources, so they are recognized by name.
func gpuResourcesOf(node corev1.Node) []corev1.ResourceName {
	names := []corev1.ResourceName{}
	for name, quantity := range node.Status.Allocatable {
		if !strings.Contains(strings.ToLower(string(name)), "gpu") || quantity.Value() == 0 {
			continue
		}
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func gpuProductOf(node corev1.Node, resourceName corev1.ResourceName) string {
	vendor, _, found := strings.Cut(string(resourceName), "/")
	if !found {
		return ""
	}
	return node.Labels[vendor+"/gpu.product"]
}

// Counted the way the scheduler counts it.
func podResourceRequest(pod *corev1.Pod, name corev1.ResourceName) int64 {
	total := int64(0)
	for i := range pod.Spec.Containers {
		total += containerRequest(&pod.Spec.Containers[i], name)
	}
	initPeak := int64(0)
	for i := range pod.Spec.InitContainers {
		init := &pod.Spec.InitContainers[i]
		// Sidecars add to the total; other init containers only set a floor.
		if init.RestartPolicy != nil && *init.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			total += containerRequest(init, name)
			continue
		}
		if request := containerRequest(init, name); request > initPeak {
			initPeak = request
		}
	}
	if initPeak > total {
		total = initPeak
	}
	if overhead, ok := pod.Spec.Overhead[name]; ok {
		total += quantityIn(name, overhead)
	}
	return total
}

func containerRequest(container *corev1.Container, name corev1.ResourceName) int64 {
	if quantity, ok := container.Resources.Requests[name]; ok {
		return quantityIn(name, quantity)
	}
	// GPUs are often set as a limit only; admission copies it into requests later.
	if quantity, ok := container.Resources.Limits[name]; ok {
		return quantityIn(name, quantity)
	}
	return 0
}

// Millicores for CPU, MiB for memory, whole units otherwise.
func quantityIn(name corev1.ResourceName, quantity resource.Quantity) int64 {
	switch name {
	case corev1.ResourceCPU:
		return quantity.MilliValue()
	case corev1.ResourceMemory:
		return quantity.Value() / (1024 * 1024)
	default:
		return quantity.Value()
	}
}

func remaining(total, used int64) int64 {
	if free := total - used; free > 0 {
		return free
	}
	return 0
}

func percentUsed(used, total int64) int {
	if total <= 0 {
		return 0
	}
	if percent := int(used * 100 / total); percent < 100 {
		return percent
	}
	return 100
}
