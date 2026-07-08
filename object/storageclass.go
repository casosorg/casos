package object

import (
	"context"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetStorageClasses(cfg *rest.Config) ([]storagev1.StorageClass, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	list, err := client.StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetStorageClass(cfg *rest.Config, name string) (*storagev1.StorageClass, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.StorageV1().StorageClasses().Get(context.Background(), name, metav1.GetOptions{})
}

func AddStorageClass(cfg *rest.Config, sc *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.StorageV1().StorageClasses().Create(context.Background(), sc, metav1.CreateOptions{})
}

func UpdateStorageClass(cfg *rest.Config, sc *storagev1.StorageClass) (*storagev1.StorageClass, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.StorageV1().StorageClasses().Update(context.Background(), sc, metav1.UpdateOptions{})
}

func DeleteStorageClass(cfg *rest.Config, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.StorageV1().StorageClasses().Delete(context.Background(), name, metav1.DeleteOptions{})
}
