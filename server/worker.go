package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// tryParsePrivateKey attempts RSA (PKCS#1) first, then falls back to PKCS#8
// and EC, matching whatever format ensureCerts wrote.
func tryParsePrivateKey(der []byte) (interface{}, error) {
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		return k, nil
	}
	if k, err := x509.ParseECPrivateKey(der); err == nil {
		return k, nil
	}
	return nil, fmt.Errorf("unsupported private key format")
}

// WorkerKubeconfig holds the kubeconfig content and the node cert files for a
// worker node, all as PEM strings (base64-encoded inside the kubeconfig).
type WorkerKubeconfig struct {
	Kubeconfig string // YAML content, ready to write to disk
	NodeName   string
}

// GenerateWorkerKubeconfig signs a node client certificate for the given
// nodeName (CN=system:node:<nodeName>, O=system:nodes) using the cluster CA,
// then returns a kubeconfig embedding all cert data as base64.
//
// The returned kubeconfig is intended for kubelet's --kubeconfig flag.
func GenerateWorkerKubeconfig(cfg Config, nodeName string) (*WorkerKubeconfig, error) {
	apiserverURL := fmt.Sprintf("https://127.0.0.1:%d", cfg.ApiserverPort)
	return GenerateWorkerKubeconfigForServer(cfg, nodeName, apiserverURL)
}

// GenerateWorkerKubeconfigForServer signs a node client certificate and embeds
// the provided apiserver URL as the cluster server endpoint.
func GenerateWorkerKubeconfigForServer(cfg Config, nodeName, apiserverURL string) (*WorkerKubeconfig, error) {
	certDir := filepath.Join(cfg.DataDir, "tls")

	// Load cluster CA.
	caKeyPEM, err := os.ReadFile(filepath.Join(certDir, "ca.key"))
	if err != nil {
		return nil, fmt.Errorf("read ca.key: %w", err)
	}
	caCertPEM, err := os.ReadFile(filepath.Join(certDir, "ca.crt"))
	if err != nil {
		return nil, fmt.Errorf("read ca.crt: %w", err)
	}

	block, _ := pem.Decode(caKeyPEM)
	caKeyRaw, err := tryParsePrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse ca.key: %w", err)
	}
	block, _ = pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse ca.crt: %w", err)
	}

	// Generate node client key and cert.
	nodeKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	nodeTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   "system:node:" + nodeName,
			Organization: []string{"system:nodes"},
		},
		NotBefore:   time.Now().Add(-time.Minute),
		NotAfter:    time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	nodeCertDER, err := x509.CreateCertificate(rand.Reader, nodeTemplate, caCert, &nodeKey.PublicKey, caKeyRaw)
	if err != nil {
		return nil, err
	}

	// Encode to PEM.
	nodeKeyDER, err := x509.MarshalECPrivateKey(nodeKey)
	if err != nil {
		return nil, err
	}
	nodeCertPEM := pemEncode("CERTIFICATE", nodeCertDER)
	nodeKeyPEM := pemEncode("EC PRIVATE KEY", nodeKeyDER)

	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
preferences: {}
clusters:
- cluster:
    certificate-authority-data: %s
    server: %s
  name: casos
contexts:
- context:
    cluster: casos
    user: system:node:%s
  name: %s@casos
current-context: %s@casos
users:
- name: system:node:%s
  user:
    client-certificate-data: %s
    client-key-data: %s
`,
		base64.StdEncoding.EncodeToString(caCertPEM),
		apiserverURL,
		nodeName, nodeName, nodeName, nodeName,
		base64.StdEncoding.EncodeToString(nodeCertPEM),
		base64.StdEncoding.EncodeToString(nodeKeyPEM),
	)

	return &WorkerKubeconfig{
		Kubeconfig: kubeconfig,
		NodeName:   nodeName,
	}, nil
}

func pemEncode(typ string, der []byte) []byte {
	block := &pem.Block{Type: typ, Bytes: der}
	return pem.EncodeToMemory(block)
}
