package object

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func GetIngresses(cfg *rest.Config, namespace string) ([]networkingv1.Ingress, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	list, err := client.NetworkingV1().Ingresses(ns).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func GetIngress(cfg *rest.Config, namespace, name string) (*networkingv1.Ingress, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.NetworkingV1().Ingresses(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func AddIngress(cfg *rest.Config, ing *networkingv1.Ingress) (*networkingv1.Ingress, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.NetworkingV1().Ingresses(ing.Namespace).Create(context.Background(), ing, metav1.CreateOptions{})
}

func UpdateIngress(cfg *rest.Config, ing *networkingv1.Ingress) (*networkingv1.Ingress, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	return client.NetworkingV1().Ingresses(ing.Namespace).Update(context.Background(), ing, metav1.UpdateOptions{})
}

func DeleteIngress(cfg *rest.Config, namespace, name string) error {
	client, err := newClient(cfg)
	if err != nil {
		return err
	}
	return client.NetworkingV1().Ingresses(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}

// GetTLSCertExpiry reads the TLS secret created by cert-manager and returns
// the certificate's NotAfter date (YYYY-MM-DD). Returns an error if the secret
// does not yet exist (cert still being issued).
func GetTLSCertExpiry(cfg *rest.Config, namespace, secretName string) (string, error) {
	client, err := newClient(cfg)
	if err != nil {
		return "", err
	}
	secret, err := client.CoreV1().Secrets(namespace).Get(context.Background(), secretName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	certPEM, ok := secret.Data["tls.crt"]
	if !ok {
		return "", fmt.Errorf("tls.crt not found in secret %s", secretName)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM in secret %s", secretName)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	return cert.NotAfter.UTC().Format("2006-01-02"), nil
}
