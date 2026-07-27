package object

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ResourceReference struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid,omitempty"`
	Validated bool   `json:"validated"`
}

type WorkloadPodSet struct {
	Kind          string
	Namespace     string
	Name          string
	UID           types.UID
	Selector      string
	Replicas      int32
	ReadyReplicas int32
	Template      corev1.PodTemplateSpec
	Pods          []corev1.Pod
}

func ResolvePodOwner(cfg *rest.Config, pod *corev1.Pod) (ResourceReference, error) {
	client, err := newClient(cfg)
	if err != nil {
		return ResourceReference{}, err
	}
	return ResolvePodOwnerFromClient(context.Background(), client, pod)
}

func ResolvePodOwnerFromClient(ctx context.Context, client kubernetes.Interface, pod *corev1.Pod) (ResourceReference, error) {
	if pod == nil {
		return ResourceReference{}, nil
	}
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return ResourceReference{}, nil
	}
	base := ownerReferenceToResource(pod.Namespace, ref, false)
	switch strings.ToLower(ref.Kind) {
	case "replicaset":
		rs, err := client.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil || rs.UID != ref.UID {
			return base, nil
		}
		replicaSetRef := ResourceReference{Kind: "ReplicaSet", Namespace: pod.Namespace, Name: rs.Name, UID: string(rs.UID), Validated: true}
		deployRef := metav1.GetControllerOf(rs)
		if deployRef == nil || !strings.EqualFold(deployRef.Kind, "Deployment") {
			return replicaSetRef, nil
		}
		deployment, err := client.AppsV1().Deployments(pod.Namespace).Get(ctx, deployRef.Name, metav1.GetOptions{})
		if err != nil || deployment.UID != deployRef.UID {
			return replicaSetRef, nil
		}
		return ResourceReference{Kind: "Deployment", Namespace: pod.Namespace, Name: deployment.Name, UID: string(deployment.UID), Validated: true}, nil
	case "deployment":
		deployment, err := client.AppsV1().Deployments(pod.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err == nil && deployment.UID == ref.UID {
			base.Validated = true
		}
		return base, nil
	case "statefulset":
		sts, err := client.AppsV1().StatefulSets(pod.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err == nil && sts.UID == ref.UID {
			base.Validated = true
		}
		return base, nil
	case "daemonset":
		ds, err := client.AppsV1().DaemonSets(pod.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err == nil && ds.UID == ref.UID {
			base.Validated = true
		}
		return base, nil
	case "job":
		job, err := client.BatchV1().Jobs(pod.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil || job.UID != ref.UID {
			return base, nil
		}
		jobRef := ResourceReference{Kind: "Job", Namespace: pod.Namespace, Name: job.Name, UID: string(job.UID), Validated: true}
		cronRef := metav1.GetControllerOf(job)
		if cronRef == nil || !strings.EqualFold(cronRef.Kind, "CronJob") {
			return jobRef, nil
		}
		cronJob, err := client.BatchV1().CronJobs(pod.Namespace).Get(ctx, cronRef.Name, metav1.GetOptions{})
		if err != nil || cronJob.UID != cronRef.UID {
			return jobRef, nil
		}
		return ResourceReference{Kind: "CronJob", Namespace: pod.Namespace, Name: cronJob.Name, UID: string(cronJob.UID), Validated: true}, nil
	default:
		return base, nil
	}
}

func ListPodsForNode(cfg *rest.Config, nodeName string) ([]corev1.Pod, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return ListPodsForNodeFromClient(context.Background(), client, nodeName)
}

func ListPodsForNodeFromClient(ctx context.Context, client kubernetes.Interface, nodeName string) ([]corev1.Pod, error) {
	if strings.TrimSpace(nodeName) == "" {
		return nil, fmt.Errorf("node name is required")
	}
	list, err := client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
	})
	if err != nil {
		return nil, err
	}
	pods := append([]corev1.Pod(nil), list.Items...)
	sortPodsStable(pods)
	return pods, nil
}

func ListPodsForWorkload(cfg *rest.Config, kind, namespace, name string) (WorkloadPodSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return WorkloadPodSet{}, err
	}
	return ListPodsForWorkloadFromClient(context.Background(), client, kind, namespace, name)
}

func ListPodsForWorkloadFromClient(ctx context.Context, client kubernetes.Interface, kind, namespace, name string) (WorkloadPodSet, error) {
	normalizedKind, err := NormalizeWorkloadKind(kind)
	if err != nil {
		return WorkloadPodSet{}, err
	}
	if strings.TrimSpace(namespace) == "" {
		return WorkloadPodSet{}, fmt.Errorf("namespace is required")
	}
	if strings.TrimSpace(name) == "" {
		return WorkloadPodSet{}, fmt.Errorf("name is required")
	}

	switch normalizedKind {
	case "Deployment":
		return listPodsForDeployment(ctx, client, namespace, name)
	case "StatefulSet":
		return listPodsForStatefulSet(ctx, client, namespace, name)
	case "DaemonSet":
		return listPodsForDaemonSet(ctx, client, namespace, name)
	default:
		return WorkloadPodSet{}, fmt.Errorf("unsupported workload kind %q", kind)
	}
}

func NormalizeWorkloadKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "deployment", "deployments":
		return "Deployment", nil
	case "statefulset", "statefulsets":
		return "StatefulSet", nil
	case "daemonset", "daemonsets":
		return "DaemonSet", nil
	default:
		return "", fmt.Errorf("unsupported workload kind %q", kind)
	}
}

func listPodsForDeployment(ctx context.Context, client kubernetes.Interface, namespace, name string) (WorkloadPodSet, error) {
	deployment, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return WorkloadPodSet{}, err
	}
	selector, err := workloadSelectorString(deployment.Spec.Selector)
	if err != nil {
		return WorkloadPodSet{}, err
	}
	replicaSets, err := client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return WorkloadPodSet{}, err
	}
	replicaSetUIDs := map[types.UID]struct{}{}
	for _, rs := range replicaSets.Items {
		if controllerMatches(metav1.GetControllerOf(&rs), "Deployment", deployment.Name, deployment.UID) {
			replicaSetUIDs[rs.UID] = struct{}{}
		}
	}
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return WorkloadPodSet{}, err
	}
	selected := make([]corev1.Pod, 0, len(pods.Items))
	for _, pod := range pods.Items {
		ref := metav1.GetControllerOf(&pod)
		if ref == nil || !strings.EqualFold(ref.Kind, "ReplicaSet") {
			continue
		}
		if _, ok := replicaSetUIDs[ref.UID]; ok {
			selected = append(selected, pod)
		}
	}
	sortPodsStable(selected)
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	return WorkloadPodSet{
		Kind:          "Deployment",
		Namespace:     deployment.Namespace,
		Name:          deployment.Name,
		UID:           deployment.UID,
		Selector:      selector,
		Replicas:      replicas,
		ReadyReplicas: deployment.Status.ReadyReplicas,
		Template:      deployment.Spec.Template,
		Pods:          selected,
	}, nil
}

func listPodsForStatefulSet(ctx context.Context, client kubernetes.Interface, namespace, name string) (WorkloadPodSet, error) {
	sts, err := client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return WorkloadPodSet{}, err
	}
	selector, err := workloadSelectorString(sts.Spec.Selector)
	if err != nil {
		return WorkloadPodSet{}, err
	}
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return WorkloadPodSet{}, err
	}
	selected := selectPodsByController(pods.Items, "StatefulSet", sts.Name, sts.UID)
	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	return WorkloadPodSet{
		Kind:          "StatefulSet",
		Namespace:     sts.Namespace,
		Name:          sts.Name,
		UID:           sts.UID,
		Selector:      selector,
		Replicas:      replicas,
		ReadyReplicas: sts.Status.ReadyReplicas,
		Template:      sts.Spec.Template,
		Pods:          selected,
	}, nil
}

func listPodsForDaemonSet(ctx context.Context, client kubernetes.Interface, namespace, name string) (WorkloadPodSet, error) {
	ds, err := client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return WorkloadPodSet{}, err
	}
	selector, err := workloadSelectorString(ds.Spec.Selector)
	if err != nil {
		return WorkloadPodSet{}, err
	}
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return WorkloadPodSet{}, err
	}
	selected := selectPodsByController(pods.Items, "DaemonSet", ds.Name, ds.UID)
	return WorkloadPodSet{
		Kind:          "DaemonSet",
		Namespace:     ds.Namespace,
		Name:          ds.Name,
		UID:           ds.UID,
		Selector:      selector,
		Replicas:      ds.Status.DesiredNumberScheduled,
		ReadyReplicas: ds.Status.NumberReady,
		Template:      ds.Spec.Template,
		Pods:          selected,
	}, nil
}

func selectPodsByController(pods []corev1.Pod, kind, name string, uid types.UID) []corev1.Pod {
	selected := make([]corev1.Pod, 0, len(pods))
	for _, pod := range pods {
		if controllerMatches(metav1.GetControllerOf(&pod), kind, name, uid) {
			selected = append(selected, pod)
		}
	}
	sortPodsStable(selected)
	return selected
}

func workloadSelectorString(selector *metav1.LabelSelector) (string, error) {
	if selector == nil {
		return "", fmt.Errorf("workload selector is empty")
	}
	parsed, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return "", err
	}
	if parsed.Empty() {
		return "", fmt.Errorf("workload selector is empty")
	}
	return parsed.String(), nil
}

func controllerMatches(ref *metav1.OwnerReference, kind, name string, uid types.UID) bool {
	return ref != nil && strings.EqualFold(ref.Kind, kind) && ref.Name == name && ref.UID == uid
}

func ownerReferenceToResource(namespace string, ref *metav1.OwnerReference, validated bool) ResourceReference {
	if ref == nil {
		return ResourceReference{}
	}
	return ResourceReference{
		Kind:      ref.Kind,
		Namespace: namespace,
		Name:      ref.Name,
		UID:       string(ref.UID),
		Validated: validated,
	}
}

func sortPodsStable(pods []corev1.Pod) {
	sort.SliceStable(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
	})
}

func podTemplateContainers(template corev1.PodTemplateSpec) []corev1.Container {
	return append([]corev1.Container(nil), template.Spec.Containers...)
}

func workloadTemplateContainers(workload WorkloadPodSet) []corev1.Container {
	return podTemplateContainers(workload.Template)
}

func workloadKindForObject(obj interface{}) string {
	switch obj.(type) {
	case *appsv1.Deployment, appsv1.Deployment:
		return "Deployment"
	case *appsv1.StatefulSet, appsv1.StatefulSet:
		return "StatefulSet"
	case *appsv1.DaemonSet, appsv1.DaemonSet:
		return "DaemonSet"
	case *batchv1.Job, batchv1.Job:
		return "Job"
	case *batchv1.CronJob, batchv1.CronJob:
		return "CronJob"
	default:
		return ""
	}
}
