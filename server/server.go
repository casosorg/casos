package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/beego/beego"
	"github.com/casosorg/casos/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	"database/sql"

	// kine: MySQL -> etcd v3 gRPC adapter
	"github.com/k3s-io/kine/pkg/endpoint"
	_ "github.com/go-sql-driver/mysql"

	// apiserver in-process entry point (k3s-io/kubernetes fork)
	apiserverapp "k8s.io/kubernetes/cmd/kube-apiserver/app"
	"k8s.io/kubernetes/cmd/kube-apiserver/app/options"

	globalflag "k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/logs"
	"k8s.io/client-go/rest"
)

// Config holds control-plane settings populated from app.conf.
type Config struct {
	DataDir          string
	ApiserverBind    string // actual bind / SAN IP (may be loopback in dev)
	AdvertiseAddress string // non-loopback IP registered as kubernetes service endpoint
	ApiserverPort    int
	DSN              string // MySQL DSN forwarded to kine
}

// ConfigFromAppConf reads server config from the beego app.conf.
func ConfigFromAppConf() (Config, error) {
	dataDir := beego.AppConfig.String("dataDir")
	if dataDir == "" {
		dataDir = "/var/lib/casos"
	}
	bind := beego.AppConfig.String("apiserverBind")
	if bind == "" {
		bind = outboundIP()
	}
	port, _ := beego.AppConfig.Int("apiserverPort")
	if port == 0 {
		port = 6443
	}
	dsn := beego.AppConfig.String("dataSourceName")
	if dsn == "" {
		return Config{}, fmt.Errorf("dataSourceName not set in app.conf")
	}
	dbName := beego.AppConfig.String("dbName")
	if dbName == "" {
		dbName = "casos"
	}
	dsn = injectDBName(dsn, dbName)

	advertise := outboundIP()
	if advertise == "127.0.0.1" || advertise == "::1" {
		advertise = bind
	}

	return Config{
		DataDir:          dataDir,
		ApiserverBind:    bind,
		AdvertiseAddress: advertise,
		ApiserverPort:    port,
		DSN:              dsn,
	}, nil
}

// Start launches kine and the apiserver in-process.
// The returned channel is closed once the apiserver /readyz endpoint responds 200.
func Start(ctx context.Context, cfg Config) (<-chan struct{}, error) {
	certDir := filepath.Join(cfg.DataDir, "tls")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir tls: %w", err)
	}
	if err := ensureCerts(certDir, cfg.ApiserverBind, cfg.AdvertiseAddress); err != nil {
		return nil, fmt.Errorf("certs: %w", err)
	}
	if err := ensureServiceAccountKey(certDir); err != nil {
		return nil, fmt.Errorf("service account key: %w", err)
	}

	// Step 1: start kine (MySQL backend exposed as etcd v3 gRPC on loopback).
	if err := util.StopOldInstance(2379); err != nil {
		logrus.Warnf("failed to stop old instance on port 2379: %v", err)
	}
	// endpoint.Listen returns (ETCDConfig, error) directly — not a struct field.
	etcdCfg, err := endpoint.Listen(ctx, endpoint.Config{
		Endpoint:         "mysql://" + cfg.DSN,
		Listener:         "tcp://127.0.0.1:2379",
		CompactBatchSize: 100,
		NotifyInterval:   time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("kine listen: %w", err)
	}
	logrus.Infof("kine started, etcd endpoint: %v", etcdCfg.Endpoints)

	// Delete stale kubernetes service endpoints from a previous run so the
	// bootstrap controller can recreate them with the current advertise address.
	if err := deleteStaleKubernetesEndpoints(cfg.DSN); err != nil {
		logrus.Warnf("failed to delete stale kubernetes endpoints: %v", err)
	}

	// Step 2: build apiserver options and parse flags.
	// Mirrors what NewAPIServerCommand does: merge NamedFlagSets into one pflag.FlagSet.
	s := options.NewServerRunOptions()
	namedFlagSets := s.Flags()
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), "kube-apiserver", logs.SkipLoggingConfigurationFlags())
	fs := pflag.NewFlagSet("kube-apiserver", pflag.ContinueOnError)
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}
	if err := fs.Parse(buildApiserverArgs(cfg, certDir, etcdCfg.Endpoints[0])); err != nil {
		return nil, fmt.Errorf("apiserver flag parse: %w", err)
	}

	// Step 3: complete and validate options.
	completedOpts, err := s.Complete(ctx)
	if err != nil {
		return nil, fmt.Errorf("apiserver complete: %w", err)
	}
	if errs := completedOpts.Validate(); len(errs) != 0 {
		return nil, fmt.Errorf("apiserver validate: %v", errs)
	}

	// Step 4: run apiserver in a goroutine, then poll /readyz.
	stopCh := make(chan struct{})
	go func() {
		// TODO: launch scheduler and controller-manager in-process using
		// k8s.io/kubernetes/cmd/kube-{scheduler,controller-manager}/app
		if err := apiserverapp.Run(ctx, completedOpts, stopCh); err != nil {
			logrus.Errorf("apiserver exited: %v", err)
		}
	}()

	readyCh := make(chan struct{})
	go func() {
		waitForAPIServer(ctx, fmt.Sprintf("https://127.0.0.1:%d", cfg.ApiserverPort))
		close(readyCh)
	}()

	return readyCh, nil
}

// waitForAPIServer polls /readyz every 2 s until it gets HTTP 200 or ctx is done.
func waitForAPIServer(ctx context.Context, base string) {
	// #nosec G402: self-signed cert, InsecureSkipVerify intentional for milestone 1.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 5 * time.Second,
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resp, err := client.Get(base + "/readyz")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return
				}
			}
		}
	}
}

// injectDBName inserts dbName into a MySQL DSN of the form
// user:pass@tcp(host:port)/ (trailing slash, no database).
// If a database is already present it is replaced.
func injectDBName(dsn, dbName string) string {
	// MySQL DSN format: [user[:pass]@][protocol[(addr)]]/dbname[?params]
	// Find the slash after the closing ')' of the address part.
	idx := strings.LastIndex(dsn, "/")
	if idx < 0 {
		return dsn + dbName
	}
	base := dsn[:idx+1] // everything up to and including the slash
	rest := dsn[idx+1:] // existing dbname + optional ?params
	// Keep query params if present, replace (possibly empty) db name.
	if q := strings.Index(rest, "?"); q >= 0 {
		return base + dbName + rest[q:]
	}
	return base + dbName
}

func buildApiserverArgs(cfg Config, certDir, etcdEndpoint string) []string {
	saKey := filepath.Join(certDir, "sa.key")
	saPub := filepath.Join(certDir, "sa.pub")
	return []string{
		"--advertise-address=" + cfg.AdvertiseAddress,
		"--bind-address=0.0.0.0",
		fmt.Sprintf("--secure-port=%d", cfg.ApiserverPort),
		"--etcd-servers=" + etcdEndpoint,
		"--service-cluster-ip-range=10.43.0.0/16",
		"--allow-privileged=true",
		"--authorization-mode=Node,RBAC",
		"--enable-admission-plugins=NodeRestriction",
		"--tls-cert-file=" + filepath.Join(certDir, "apiserver.crt"),
		"--tls-private-key-file=" + filepath.Join(certDir, "apiserver.key"),
		"--client-ca-file=" + filepath.Join(certDir, "ca.crt"),
		"--service-account-key-file=" + saPub,
		"--service-account-signing-key-file=" + saKey,
		"--service-account-issuer=https://kubernetes.default.svc",
		"--cert-dir=" + certDir,
	}
}

// ensureCerts generates a self-signed CA, apiserver cert/key, and admin client
// cert/key if absent.
func ensureCerts(dir, ip, advertiseIP string) error {
	caKeyFile  := filepath.Join(dir, "ca.key")
	caCertFile := filepath.Join(dir, "ca.crt")
	srvKeyFile := filepath.Join(dir, "apiserver.key")
	srvCrtFile := filepath.Join(dir, "apiserver.crt")
	admKeyFile := filepath.Join(dir, "admin.key")
	admCrtFile := filepath.Join(dir, "admin.crt")

	// Load or generate the CA.
	var caKey *ecdsa.PrivateKey
	var caCert *x509.Certificate
	if fileExists(caCertFile) && fileExists(caKeyFile) {
		// Load existing CA.
		keyPEM, err := os.ReadFile(caKeyFile)
		if err != nil {
			return err
		}
		block, _ := pem.Decode(keyPEM)
		caKey, err = x509.ParseECPrivateKey(block.Bytes)
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
		caKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
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
		caKeyDER, _ := x509.MarshalECPrivateKey(caKey)
		if err := writePEM(caKeyFile, "EC PRIVATE KEY", caKeyDER); err != nil {
			return err
		}
		caCert, _ = x509.ParseCertificate(caDER)
	}

	// Generate apiserver serving cert if absent.
	if !fileExists(srvCrtFile) {
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
			IPAddresses:  uniqueIPs("127.0.0.1", ip, advertiseIP),
			DNSNames:     []string{"localhost", "kubernetes", "kubernetes.default", "kubernetes.default.svc"},
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

	// Generate admin client cert if absent (CN=admin, O=system:masters grants full access).
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

	return nil
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
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
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

// deleteStaleKubernetesEndpoints removes the default/kubernetes Endpoints object
// from kine's MySQL table so the bootstrap controller starts fresh on each run.
func deleteStaleKubernetesEndpoints(dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	const q = `UPDATE kine SET deleted=1 WHERE name='/registry/endpoints/default/kubernetes' AND deleted=0`
	_, err = db.Exec(q)
	return err
}

// uniqueIPs returns a deduplicated list of net.IP from the given string IPs.
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

// outboundIP returns the preferred non-loopback outbound IP of this machine.
func outboundIP() string {
	// Dial a public address (no data is actually sent) to learn which local IP
	// the OS would select for outbound traffic.
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
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
