package object

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
)

func GetDeployments(cfg *rest.Config, namespace string) ([]appsv1.Deployment, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	list, err := client.AppsV1().Deployments(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetDeployment(cfg *rest.Config, namespace, name string) (*appsv1.Deployment, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func AddDeployment(cfg *rest.Config, deploy *appsv1.Deployment) (*appsv1.Deployment, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().Deployments(deploy.Namespace).Create(context.Background(), deploy, metav1.CreateOptions{})
}

func UpdateDeployment(cfg *rest.Config, deploy *appsv1.Deployment) (*appsv1.Deployment, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.AppsV1().Deployments(deploy.Namespace).Update(context.Background(), deploy, metav1.UpdateOptions{})
}

func DeleteDeployment(cfg *rest.Config, namespace, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.AppsV1().Deployments(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}

func RestartDeployment(cfg *rest.Config, namespace, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{
						"casos.io/restartedAt": time.Now().UTC().Format(time.RFC3339),
					},
				},
			},
		},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = client.AppsV1().Deployments(namespace).Patch(
		context.Background(), name, types.MergePatchType, data, metav1.PatchOptions{},
	)
	return err
}

// revisionAnnotation is where the Deployment controller records which revision
// a ReplicaSet is. It is the only ordering a rollback can trust: names are
// hashes and creation timestamps tie at second resolution.
const revisionAnnotation = "deployment.kubernetes.io/revision"

// DeploymentRevisions returns the ReplicaSets a Deployment has owned, newest
// revision first. These are the versions a rollback can return to; how many
// there are is the Deployment's own revisionHistoryLimit.
func DeploymentRevisions(cfg *rest.Config, namespace, name string) (*appsv1.Deployment, []appsv1.ReplicaSet, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, nil, err
	}
	deploy, err := client.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, err
	}

	list, err := client.AppsV1().ReplicaSets(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, nil, err
	}

	owned := make([]appsv1.ReplicaSet, 0, len(list.Items))
	for _, rs := range list.Items {
		for _, ref := range rs.OwnerReferences {
			if ref.UID == deploy.UID {
				owned = append(owned, rs)
				break
			}
		}
	}
	sort.Slice(owned, func(i, j int) bool {
		return revisionOf(owned[i]) > revisionOf(owned[j])
	})
	return deploy, owned, nil
}

func revisionOf(rs appsv1.ReplicaSet) int64 {
	value, err := strconv.ParseInt(rs.Annotations[revisionAnnotation], 10, 64)
	if err != nil {
		return 0
	}
	return value
}

// RevisionNumber exposes a ReplicaSet's revision to callers outside this package.
func RevisionNumber(rs appsv1.ReplicaSet) int64 {
	return revisionOf(rs)
}

// IsCurrentRevision reports whether a ReplicaSet is the one the Deployment is
// currently set to.
//
// The Deployment's own revision annotation is written by its controller and so
// lags a change by a moment; the template is what the reader just asked for and
// is true immediately. The hash label is the controller's bookkeeping about the
// template rather than part of it, which is why it comes off before comparing.
func IsCurrentRevision(deploy *appsv1.Deployment, rs appsv1.ReplicaSet) bool {
	template := *rs.Spec.Template.DeepCopy()
	delete(template.Labels, "pod-template-hash")
	return apiequality.Semantic.DeepEqual(template, deploy.Spec.Template)
}

// RollbackDeployment puts a past revision's pod template back on the
// Deployment, which the Deployment controller then rolls out as a new revision.
// Rolling back is a roll forward to an older template — the history is never
// rewritten, so a rollback can itself be rolled back.
func RollbackDeployment(cfg *rest.Config, namespace, name string, revision int64) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	deploy, revisions, err := DeploymentRevisions(cfg, namespace, name)
	if err != nil {
		return err
	}

	var target *appsv1.ReplicaSet
	for i := range revisions {
		if revisionOf(revisions[i]) == revision {
			target = &revisions[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("revision %d is no longer kept", revision)
	}

	template := *target.Spec.Template.DeepCopy()
	// The hash labels and annotations belong to the ReplicaSet that was made
	// from the template, not to the template itself; carrying them back would
	// make the Deployment claim a hash it has not computed.
	delete(template.Labels, "pod-template-hash")
	delete(template.Annotations, revisionAnnotation)

	deploy.Spec.Template = template
	_, err = client.AppsV1().Deployments(namespace).Update(context.Background(), deploy, metav1.UpdateOptions{})
	return err
}
