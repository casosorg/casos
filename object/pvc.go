package object

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetPersistentVolumeClaims(cfg *rest.Config, namespace string) ([]corev1.PersistentVolumeClaim, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	list, err := client.CoreV1().PersistentVolumeClaims(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetPersistentVolumeClaim(cfg *rest.Config, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().PersistentVolumeClaims(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func AddPersistentVolumeClaim(cfg *rest.Config, pvc *corev1.PersistentVolumeClaim) (*corev1.PersistentVolumeClaim, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().PersistentVolumeClaims(pvc.Namespace).Create(context.Background(), pvc, metav1.CreateOptions{})
}

func DeletePersistentVolumeClaim(cfg *rest.Config, namespace, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.CoreV1().PersistentVolumeClaims(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}
