package controllers

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Helm stamps this annotation onto every resource it creates, independently of
// what the chart author wrote, which makes it the one dependable way to say
// which release a Service, Ingress or volume belongs to. The instance label is
// the fallback for resources that carry the conventional chart labels but were
// not stamped — an older release, or one adopted from outside Helm.
const (
	helmReleaseNameAnnotation = "meta.helm.sh/release-name"
	helmInstanceLabel         = "app.kubernetes.io/instance"
)

func helmReleaseOf(meta metav1.ObjectMeta) string {
	if name := meta.Annotations[helmReleaseNameAnnotation]; name != "" {
		return name
	}
	return meta.Labels[helmInstanceLabel]
}
