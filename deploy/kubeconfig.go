package deploy

import (
	"encoding/base64"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type workerKubeconfigCluster struct {
	Cluster struct {
		CertificateAuthorityData string `yaml:"certificate-authority-data"`
	} `yaml:"cluster"`
}

type workerKubeconfigFile struct {
	Clusters []workerKubeconfigCluster `yaml:"clusters"`
}

func extractCertificateAuthority(kubeconfig string) (string, error) {
	var cfg workerKubeconfigFile
	if err := yaml.Unmarshal([]byte(kubeconfig), &cfg); err != nil {
		return "", fmt.Errorf("parse worker kubeconfig: %w", err)
	}
	if len(cfg.Clusters) != 1 {
		return "", fmt.Errorf("expected exactly 1 cluster in worker kubeconfig, got %d", len(cfg.Clusters))
	}
	raw := strings.TrimSpace(cfg.Clusters[0].Cluster.CertificateAuthorityData)
	if raw == "" {
		return "", fmt.Errorf("certificate-authority-data is empty")
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("decode certificate-authority-data: %w", err)
	}
	return string(data), nil
}
