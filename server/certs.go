package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/rest"
)

func apiServerServingDNSNames() []string {
	return []string{"localhost", "kubernetes", "kubernetes.default", "kubernetes.default.svc"}
}

// ensureCerts generates a self-signed CA, apiserver cert/key, and admin client
// cert/key if absent.
func ensureCerts(dir, ip, advertiseIP string) error {
	caKeyFile := filepath.Join(dir, "ca.key")
	caCertFile := filepath.Join(dir, "ca.crt")
	srvKeyFile := filepath.Join(dir, "apiserver.key")
	srvCrtFile := filepath.Join(dir, "apiserver.crt")
	admKeyFile := filepath.Join(dir, "admin.key")
	admCrtFile := filepath.Join(dir, "admin.crt")
	kubeletKeyFile := filepath.Join(dir, "apiserver-kubelet-client.key")
	kubeletCrtFile := filepath.Join(dir, "apiserver-kubelet-client.crt")

	// kube-apiserver fails ECDSA signature verification on self-signed ECDSA CAs.
	// If the existing CA key is ECDSA, remove all derived certs so they are
	// regenerated as RSA below.
	if fileExists(caKeyFile) && caKeyIsECDSA(caKeyFile) {
		for _, f := range []string{
			caKeyFile, caCertFile, srvKeyFile, srvCrtFile,
			admKeyFile, admCrtFile, kubeletKeyFile, kubeletCrtFile,
		} {
			_ = os.Remove(f)
		}
	}

	var caKey *rsa.PrivateKey
	var caCert *x509.Certificate
	if fileExists(caCertFile) && fileExists(caKeyFile) {
		keyPEM, err := os.ReadFile(caKeyFile)
		if err != nil {
			return err
		}
		block, _ := pem.Decode(keyPEM)
		caKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
		certPEM, err := os.ReadFile(caCertFile)
		if err != nil {
			return err
		}
		block, _ = pem.Decode(certPEM)
		caCert, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			return err
		}
	} else {
		var err error
		caKey, err = rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return err
		}
		caTemplate := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "casos-ca"},
			NotBefore:             time.Now().Add(-time.Minute),
			NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
			IsCA:                  true,
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
		}
		caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
		if err != nil {
			return err
		}
		if err := writePEM(caCertFile, "CERTIFICATE", caDER); err != nil {
			return err
		}
		caKeyDER := x509.MarshalPKCS1PrivateKey(caKey)
		if err := writePEM(caKeyFile, "RSA PRIVATE KEY", caKeyDER); err != nil {
			return err
		}
		caCert, err = x509.ParseCertificate(caDER)
		if err != nil {
			return err
		}
	}

	serverIPs := desiredAPIServerIPs(ip, advertiseIP)
	if !apiServerServingCertUsable(srvCrtFile, srvKeyFile, serverIPs, caCert) {
		if err := removeIfExists(srvKeyFile); err != nil {
			return err
		}
		if err := removeIfExists(srvCrtFile); err != nil {
			return err
		}
	}

	if !fileExists(srvCrtFile) || !fileExists(srvKeyFile) {
		srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		srvTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject:      pkix.Name{CommonName: "kube-apiserver"},
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			IPAddresses:  serverIPs,
			DNSNames:     apiServerServingDNSNames(),
		}
		srvDER, err := x509.CreateCertificate(rand.Reader, srvTemplate, caCert, &srvKey.PublicKey, caKey)
		if err != nil {
			return err
		}
		if err := writePEM(srvCrtFile, "CERTIFICATE", srvDER); err != nil {
			return err
		}
		srvKeyDER, _ := x509.MarshalECPrivateKey(srvKey)
		if err := writePEM(srvKeyFile, "EC PRIVATE KEY", srvKeyDER); err != nil {
			return err
		}
	}

	if !fileExists(admCrtFile) {
		admKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		admTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(3),
			Subject:      pkix.Name{CommonName: "admin", Organization: []string{"system:masters"}},
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		admDER, err := x509.CreateCertificate(rand.Reader, admTemplate, caCert, &admKey.PublicKey, caKey)
		if err != nil {
			return err
		}
		if err := writePEM(admCrtFile, "CERTIFICATE", admDER); err != nil {
			return err
		}
		admKeyDER, _ := x509.MarshalECPrivateKey(admKey)
		if err := writePEM(admKeyFile, "EC PRIVATE KEY", admKeyDER); err != nil {
			return err
		}
	}

	if !fileExists(kubeletCrtFile) {
		kubeletKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		kubeletTemplate := &x509.Certificate{
			SerialNumber: big.NewInt(4),
			Subject:      pkix.Name{CommonName: "kube-apiserver-kubelet-client", Organization: []string{"system:masters"}},
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		kubeletDER, err := x509.CreateCertificate(rand.Reader, kubeletTemplate, caCert, &kubeletKey.PublicKey, caKey)
		if err != nil {
			return err
		}
		if err := writePEM(kubeletCrtFile, "CERTIFICATE", kubeletDER); err != nil {
			return err
		}
		kubeletKeyDER, _ := x509.MarshalECPrivateKey(kubeletKey)
		if err := writePEM(kubeletKeyFile, "EC PRIVATE KEY", kubeletKeyDER); err != nil {
			return err
		}
	}

	return nil
}

// EnsureWebhookCert generates a TLS cert for the admission webhook server
// signed by the casos CA, valid for 127.0.0.1.
func EnsureWebhookCert(certDir string) error {
	certFile := filepath.Join(certDir, "webhook.crt")
	keyFile := filepath.Join(certDir, "webhook.key")
	if fileExists(certFile) && fileExists(keyFile) {
		// Verify the existing cert is still valid under the current CA.
		// If the CA was regenerated, the old webhook cert will fail verification
		// and must be replaced.
		if webhookCertValidUnderCA(certFile, filepath.Join(certDir, "ca.crt")) {
			return nil
		}
		_ = os.Remove(certFile)
		_ = os.Remove(keyFile)
	}

	caKeyPEM, err := os.ReadFile(filepath.Join(certDir, "ca.key"))
	if err != nil {
		return fmt.Errorf("read ca.key: %w", err)
	}
	block, _ := pem.Decode(caKeyPEM)
	caKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse ca key: %w", err)
	}
	caCertPEM, err := os.ReadFile(filepath.Join(certDir, "ca.crt"))
	if err != nil {
		return fmt.Errorf("read ca.crt: %w", err)
	}
	block, _ = pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse ca cert: %w", err)
	}

	whKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	whTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(10),
		Subject:      pkix.Name{CommonName: "casos-webhook"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	whDER, err := x509.CreateCertificate(rand.Reader, whTemplate, caCert, &whKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writePEM(certFile, "CERTIFICATE", whDER); err != nil {
		return err
	}
	whKeyDER := x509.MarshalPKCS1PrivateKey(whKey)
	return writePEM(keyFile, "RSA PRIVATE KEY", whKeyDER)
}

// EnsureAuthzWebhookConfig writes (once) the kubeconfig that the apiserver
// uses to reach the Casbin authorization webhook.
func EnsureAuthzWebhookConfig(certDir string, webhookPort int) (string, error) {
	path := filepath.Join(certDir, "authz-webhook.kubeconfig")
	if fileExists(path) {
		return path, nil
	}

	caData, err := os.ReadFile(filepath.Join(certDir, "ca.crt"))
	if err != nil {
		return "", fmt.Errorf("read ca.crt: %w", err)
	}
	certData, err := os.ReadFile(filepath.Join(certDir, "admin.crt"))
	if err != nil {
		return "", fmt.Errorf("read admin.crt: %w", err)
	}
	keyData, err := os.ReadFile(filepath.Join(certDir, "admin.key"))
	if err != nil {
		return "", fmt.Errorf("read admin.key: %w", err)
	}

	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: authz-webhook
  cluster:
    certificate-authority-data: %s
    server: https://127.0.0.1:%d/authorization/authorize
users:
- name: casos
  user:
    client-certificate-data: %s
    client-key-data: %s
contexts:
- context:
    cluster: authz-webhook
    user: casos
  name: default
current-context: default
`,
		base64.StdEncoding.EncodeToString(caData),
		webhookPort,
		base64.StdEncoding.EncodeToString(certData),
		base64.StdEncoding.EncodeToString(keyData),
	)

	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// ensureServiceAccountKey generates an RSA key pair for service-account token
// signing/verification if not already present.
func ensureServiceAccountKey(dir string) error {
	keyFile := filepath.Join(dir, "sa.key")
	pubFile := filepath.Join(dir, "sa.pub")
	if fileExists(keyFile) && fileExists(pubFile) {
		return nil
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	keyDER := x509.MarshalPKCS1PrivateKey(key)
	if err := writePEM(keyFile, "RSA PRIVATE KEY", keyDER); err != nil {
		return err
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	return writePEM(pubFile, "PUBLIC KEY", pubDER)
}

// ensureComponentKubeconfig writes <name>.kubeconfig inside certDir, embedding
// admin client certs as base64 to avoid Windows path escaping issues.
func ensureComponentKubeconfig(certDir, apiserverURL, name string) (string, error) {
	path := filepath.Join(certDir, name+".kubeconfig")
	if fileExists(path) {
		return path, nil
	}

	caData, err := os.ReadFile(filepath.Join(certDir, "ca.crt"))
	if err != nil {
		return "", err
	}
	certData, err := os.ReadFile(filepath.Join(certDir, "admin.crt"))
	if err != nil {
		return "", err
	}
	keyData, err := os.ReadFile(filepath.Join(certDir, "admin.key"))
	if err != nil {
		return "", err
	}

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
    user: %s
  name: %s@casos
current-context: %s@casos
users:
- name: %s
  user:
    client-certificate-data: %s
    client-key-data: %s
`,
		base64.StdEncoding.EncodeToString(caData),
		apiserverURL,
		name, name, name, name,
		base64.StdEncoding.EncodeToString(certData),
		base64.StdEncoding.EncodeToString(keyData),
	)

	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// AdminRestConfig returns a rest.Config that authenticates to the apiserver
// using the generated admin client certificate (system:masters group).
func AdminRestConfig(cfg Config) *rest.Config {
	certDir := filepath.Join(cfg.DataDir, "tls")
	return &rest.Config{
		Host: fmt.Sprintf("https://127.0.0.1:%d", cfg.ApiserverPort),
		TLSClientConfig: rest.TLSClientConfig{
			CAFile:   filepath.Join(certDir, "ca.crt"),
			CertFile: filepath.Join(certDir, "admin.crt"),
			KeyFile:  filepath.Join(certDir, "admin.key"),
		},
	}
}

func writePEM(path, typ string, der []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: typ, Bytes: der})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func uniqueIPs(addrs ...string) []net.IP {
	seen := map[string]bool{}
	var result []net.IP
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil || seen[ip.String()] {
			continue
		}
		seen[ip.String()] = true
		result = append(result, ip)
	}
	return result
}

func desiredAPIServerIPs(ip, advertiseIP string) []net.IP {
	return uniqueIPs(append([]string{"127.0.0.1", ip, advertiseIP}, allInterfaceIPs()...)...)
}

func apiServerServingCertUsable(certFile, keyFile string, desiredIPs []net.IP, caCert *x509.Certificate) bool {
	if caCert == nil || !fileExists(certFile) || !fileExists(keyFile) {
		return false
	}
	tlsCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil || len(tlsCert.Certificate) == 0 {
		return false
	}
	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return false
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		return false
	}
	for _, desiredIP := range desiredIPs {
		if !certHasIP(cert.IPAddresses, desiredIP) {
			return false
		}
	}
	for _, dnsName := range apiServerServingDNSNames() {
		if !certHasDNS(cert.DNSNames, dnsName) {
			return false
		}
	}
	return true
}

func certHasIP(certIPs []net.IP, desiredIP net.IP) bool {
	if desiredIP == nil {
		return false
	}
	desired := desiredIP
	desired4 := desiredIP.To4()
	if desired4 != nil {
		desired = desired4
	}
	for _, certIP := range certIPs {
		candidate := certIP
		if desired4 != nil {
			candidate = certIP.To4()
		}
		if candidate != nil && candidate.Equal(desired) {
			return true
		}
	}
	return false
}

func certHasDNS(certDNS []string, desiredDNS string) bool {
	for _, name := range certDNS {
		if name == desiredDNS {
			return true
		}
	}
	return false
}

// webhookCertValidUnderCA returns true if the PEM cert at certFile verifies
// against the PEM CA at caFile. Returns false on any read, parse, or verify error.
func webhookCertValidUnderCA(certFile, caFile string) bool {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return false
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return false
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	_, err = cert.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	return err == nil
}

// caKeyIsECDSA reports whether the PEM key file at path contains an EC private
// key. Used to detect legacy ECDSA CAs that must be replaced with RSA to avoid
// kube-apiserver x509 ECDSA verification failures on self-signed CA certs.
func caKeyIsECDSA(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	return block != nil && block.Type == "EC PRIVATE KEY"
}

// allInterfaceIPs returns all unicast IP addresses assigned to local network
// interfaces, so the apiserver cert is valid regardless of which interface
// a client (e.g. a WSL2 worker) uses to reach the host.
func allInterfaceIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ips []string
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips
}
