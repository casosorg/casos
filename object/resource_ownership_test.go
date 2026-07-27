package object

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestResolvePodOwnerDeploymentViaReplicaSet(t *testing.T) {
	deployUID := types.UID("deploy-uid")
	rsUID := types.UID("rs-uid")
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", UID: deployUID}},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "api-abc",
				Namespace:       "default",
				UID:             rsUID,
				OwnerReferences: []metav1.OwnerReference{controllerOwner("Deployment", "api", deployUID)},
			},
		},
	)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:            "api-abc-1",
		Namespace:       "default",
		OwnerReferences: []metav1.OwnerReference{controllerOwner("ReplicaSet", "api-abc", rsUID)},
	}}

	owner, err := ResolvePodOwnerFromClient(context.Background(), client, pod)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Kind != "Deployment" || owner.Name != "api" || owner.Namespace != "default" || !owner.Validated {
		t.Fatalf("unexpected owner: %#v", owner)
	}
}

func TestResolvePodOwnerUIDMismatchDoesNotPromote(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-abc", Namespace: "default", UID: types.UID("real-rs")}},
	)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:            "api-abc-1",
		Namespace:       "default",
		OwnerReferences: []metav1.OwnerReference{controllerOwner("ReplicaSet", "api-abc", types.UID("wrong-rs"))},
	}}

	owner, err := ResolvePodOwnerFromClient(context.Background(), client, pod)
	if err != nil {
		t.Fatal(err)
	}
	if owner.Kind != "ReplicaSet" || owner.Validated {
		t.Fatalf("unexpected owner after UID mismatch: %#v", owner)
	}
}

func TestListPodsForWorkloadDeploymentUsesReplicaSetUID(t *testing.T) {
	deployUID := types.UID("deploy-uid")
	rsUID := types.UID("rs-current")
	oldRSUID := types.UID("rs-old")
	replicas := int32(2)
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}}
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", UID: deployUID},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: selector,
				Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels}},
			},
		},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-current", Namespace: "default", UID: rsUID, Labels: selector.MatchLabels, OwnerReferences: []metav1.OwnerReference{controllerOwner("Deployment", "api", deployUID)}}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-old", Namespace: "default", UID: oldRSUID, Labels: selector.MatchLabels}},
		ownedPod("default", "api-current-1", selector.MatchLabels, "ReplicaSet", "api-current", rsUID),
		ownedPod("default", "api-current-2", selector.MatchLabels, "ReplicaSet", "api-current", rsUID),
		ownedPod("default", "api-old-1", selector.MatchLabels, "ReplicaSet", "api-old", oldRSUID),
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "manual", Namespace: "default", Labels: selector.MatchLabels}},
	)

	podSet, err := ListPodsForWorkloadFromClient(context.Background(), client, "deployment", "default", "api")
	if err != nil {
		t.Fatal(err)
	}
	if len(podSet.Pods) != 2 || podSet.Pods[0].Name != "api-current-1" || podSet.Pods[1].Name != "api-current-2" {
		t.Fatalf("unexpected deployment Pods: %#v", podSet.Pods)
	}
}

func TestListPodsForWorkloadStatefulSetAndDaemonSet(t *testing.T) {
	stsUID := types.UID("sts-uid")
	dsUID := types.UID("ds-uid")
	stsSelector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}}
	dsSelector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}}
	client := fake.NewSimpleClientset(
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default", UID: stsUID}, Spec: appsv1.StatefulSetSpec{Selector: stsSelector, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: stsSelector.MatchLabels}}}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "kube-system", UID: dsUID}, Spec: appsv1.DaemonSetSpec{Selector: dsSelector, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: dsSelector.MatchLabels}}}},
		ownedPod("default", "db-0", stsSelector.MatchLabels, "StatefulSet", "db", stsUID),
		ownedPod("kube-system", "agent-worker-1", dsSelector.MatchLabels, "DaemonSet", "agent", dsUID),
	)

	stsPods, err := ListPodsForWorkloadFromClient(context.Background(), client, "statefulset", "default", "db")
	if err != nil {
		t.Fatal(err)
	}
	if len(stsPods.Pods) != 1 || stsPods.Pods[0].Name != "db-0" {
		t.Fatalf("unexpected StatefulSet Pods: %#v", stsPods.Pods)
	}
	dsPods, err := ListPodsForWorkloadFromClient(context.Background(), client, "daemonset", "kube-system", "agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(dsPods.Pods) != 1 || dsPods.Pods[0].Name != "agent-worker-1" {
		t.Fatalf("unexpected DaemonSet Pods: %#v", dsPods.Pods)
	}
}

func TestListPodsForNodeUsesFieldSelector(t *testing.T) {
	client := fake.NewSimpleClientset()
	var gotFieldSelector string
	client.Fake.PrependReactor("list", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		listAction := action.(ktesting.ListAction)
		gotFieldSelector = listAction.GetListRestrictions().Fields.String()
		return true, &corev1.PodList{}, nil
	})
	if _, err := ListPodsForNodeFromClient(context.Background(), client, "worker-1"); err != nil {
		t.Fatal(err)
	}
	if gotFieldSelector != "spec.nodeName=worker-1" {
		t.Fatalf("field selector = %q, want spec.nodeName=worker-1", gotFieldSelector)
	}
}

func controllerOwner(kind, name string, uid types.UID) metav1.OwnerReference {
	controller := true
	return metav1.OwnerReference{APIVersion: "v1", Kind: kind, Name: name, UID: uid, Controller: &controller}
}

func ownedPod(namespace, name string, podLabels labels.Set, ownerKind, ownerName string, ownerUID types.UID) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:            name,
		Namespace:       namespace,
		Labels:          podLabels,
		OwnerReferences: []metav1.OwnerReference{controllerOwner(ownerKind, ownerName, ownerUID)},
	}}
}
