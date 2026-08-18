package object

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func newClient(cfg *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(cfg)
}

func GetConfigMaps(cfg *rest.Config, namespace string) ([]corev1.ConfigMap, error) {
	list, err := getConfigMaps(cfg, namespace, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetConfigMapPage(cfg *rest.Config, namespace string, limit int64, continueToken string) (*corev1.ConfigMapList, error) {
	return getConfigMaps(cfg, namespace, metav1.ListOptions{Limit: limit, Continue: continueToken})
}

func getConfigMaps(cfg *rest.Config, namespace string, options metav1.ListOptions) (*corev1.ConfigMapList, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	list, err := client.CoreV1().ConfigMaps(ns).List(context.Background(), options)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func GetConfigMap(cfg *rest.Config, namespace, name string) (*corev1.ConfigMap, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func AddConfigMap(cfg *rest.Config, cm *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().ConfigMaps(cm.Namespace).Create(context.Background(), cm, metav1.CreateOptions{})
}

func UpdateConfigMap(cfg *rest.Config, cm *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.CoreV1().ConfigMaps(cm.Namespace).Update(context.Background(), cm, metav1.UpdateOptions{})
}

func DeleteConfigMap(cfg *rest.Config, namespace, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.CoreV1().ConfigMaps(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}
