package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/casosorg/casos/util"
	"github.com/k3s-io/kine/pkg/endpoint"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	clientv3 "go.etcd.io/etcd/client/v3"

	globalflag "k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/logs"
	apiserverapp "k8s.io/kubernetes/cmd/kube-apiserver/app"
	"k8s.io/kubernetes/cmd/kube-apiserver/app/options"
)

const (
	// kineBindAddress and kineDefaultPort are where kine offers its etcd API.
	// Only the in-process apiserver dials it, and it is handed the address
	// kine reports back, so the port is free to move when something else on
	// the machine already holds the default.
	kineBindAddress = "127.0.0.1"
	kineDefaultPort = 20379

	serviceClusterIPRange      = "10.43.0.0/16"
	kubernetesServiceIP        = "10.43.0.1"
	kubernetesEndpointsEtcdKey = "/registry/services/endpoints/default/kubernetes"
)

// Start launches kine and the apiserver in-process.
// The returned channel is closed once the apiserver /readyz endpoint responds 200.
func Start(ctx context.Context, cfg Config) (<-chan struct{}, error) {
	// Before anything else that binds a port: the usual holder of the apiserver
	// port is a previous CasOS still running, and stopping it here frees the
	// rest of the ports this process is about to want as well.
	if stopped, err := util.ReclaimPort("", cfg.ApiserverPort); err != nil {
		logrus.Warnf("apiserver port: %v", err)
	} else if stopped != "" {
		logrus.Warnf("port %d was held by %s, which has been stopped so the apiserver can use it", cfg.ApiserverPort, stopped)
	}

	certDir := filepath.Join(cfg.DataDir, "tls")
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir tls: %w", err)
	}
	if err := ensureCerts(certDir, cfg.ApiserverBind, cfg.AdvertiseAddress); err != nil {
		return nil, fmt.Errorf("certs: %w", err)
	}
	if err := ensureServiceAccountKey(certDir); err != nil {
		return nil, fmt.Errorf("service account key: %w", err)
	}

	if err := util.StopOldInstance(kineDefaultPort); err != nil {
		logrus.Warnf("failed to stop old instance on port %d: %v", kineDefaultPort, err)
	}
	if err := ensureKineDataDirectory(cfg.DatastoreEndpoint); err != nil {
		return nil, err
	}
	kinePort, err := util.FreePortFrom(kineBindAddress, kineDefaultPort)
	if err != nil {
		return nil, fmt.Errorf("kine port: %w", err)
	}
	if kinePort != kineDefaultPort {
		logrus.Warnf("port %d is taken by another program, kine is using %d instead", kineDefaultPort, kinePort)
	}
	etcdCfg, err := endpoint.Listen(ctx, kineEndpointConfig(cfg.DatastoreEndpoint, kinePort))
	if err != nil {
		return nil, fmt.Errorf("kine listen: %w", err)
	}
	if len(etcdCfg.Endpoints) == 0 {
		return nil, fmt.Errorf("kine started without an etcd endpoint")
	}
	etcdEndpoint := etcdCfg.Endpoints[0]
	logrus.Infof("kine started, etcd endpoint: %v", etcdCfg.Endpoints)

	cleanupCtx, cancelCleanup := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCleanup()
	if err := deleteStaleKubernetesEndpoints(cleanupCtx, etcdEndpoint); err != nil {
		logrus.Warnf("failed to delete stale kubernetes endpoints: %v", err)
	}

	s := options.NewServerRunOptions()
	namedFlagSets := s.Flags()
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), "kube-apiserver", logs.SkipLoggingConfigurationFlags())
	fs := pflag.NewFlagSet("kube-apiserver", pflag.ContinueOnError)
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}
	authzKubeconfig, err := EnsureAuthzWebhookConfig(certDir, cfg.WebhookPort)
	if err != nil {
		logrus.Warnf("authz webhook kubeconfig: %v — authorization webhook disabled", err)
		authzKubeconfig = ""
	}

	if err := fs.Parse(buildApiserverArgs(cfg, certDir, etcdEndpoint, authzKubeconfig)); err != nil {
		return nil, fmt.Errorf("apiserver flag parse: %w", err)
	}

	completedOpts, err := s.Complete(ctx)
	if err != nil {
		return nil, fmt.Errorf("apiserver complete: %w", err)
	}
	if errs := completedOpts.Validate(); len(errs) != 0 {
		return nil, fmt.Errorf("apiserver validate: %v", errs)
	}

	stopCh := make(chan struct{})
	go func() {
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

func kineEndpointConfig(datastoreEndpoint string, port int) endpoint.Config {
	return endpoint.Config{
		Endpoint:            datastoreEndpoint,
		Listener:            fmt.Sprintf("tcp://%s:%d", kineBindAddress, port),
		EmulatedETCDVersion: "3.6.11",
		CompactBatchSize:    100,
		NotifyInterval:      time.Second,
	}
}

// ensureKineDataDirectory creates the parent directory of a SQLite kine
// datastore. Kine only creates the directory for its own default DSN, so a
// configured path has to exist before endpoint.Listen opens the database.
func ensureKineDataDirectory(datastoreEndpoint string) error {
	if !strings.HasPrefix(datastoreEndpoint, "sqlite://") {
		return nil
	}
	dataSourceName := strings.TrimPrefix(datastoreEndpoint, "sqlite://")
	databasePath, err := util.SQLiteDatabasePath(dataSourceName)
	if err != nil {
		return err
	}
	if databasePath == "" {
		return fmt.Errorf("SQLite kine endpoint has no database path")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return fmt.Errorf("mkdir kine data directory: %w", err)
	}
	return nil
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

func authzMode(kubeconfig string) string {
	if kubeconfig != "" {
		return "Node,RBAC,Webhook"
	}
	return "Node,RBAC"
}

func buildApiserverArgs(cfg Config, certDir, etcdEndpoint, authzKubeconfig string) []string {
	saKey := filepath.Join(certDir, "sa.key")
	saPub := filepath.Join(certDir, "sa.pub")
	args := []string{
		"--advertise-address=" + cfg.AdvertiseAddress,
		"--bind-address=0.0.0.0",
		fmt.Sprintf("--secure-port=%d", cfg.ApiserverPort),
		"--etcd-servers=" + etcdEndpoint,
		"--service-cluster-ip-range=" + serviceClusterIPRange,
		"--allow-privileged=true",
		"--authorization-mode=" + authzMode(authzKubeconfig),
		"--enable-admission-plugins=NodeRestriction,ValidatingAdmissionWebhook",
		"--tls-cert-file=" + filepath.Join(certDir, "apiserver.crt"),
		"--tls-private-key-file=" + filepath.Join(certDir, "apiserver.key"),
		"--client-ca-file=" + filepath.Join(certDir, "ca.crt"),
		"--service-account-key-file=" + saPub,
		"--service-account-signing-key-file=" + saKey,
		"--service-account-issuer=https://kubernetes.default.svc",
		"--cert-dir=" + certDir,
		"--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
		"--kubelet-client-certificate=" + filepath.Join(certDir, "apiserver-kubelet-client.crt"),
		"--kubelet-client-key=" + filepath.Join(certDir, "apiserver-kubelet-client.key"),
	}
	if authzKubeconfig != "" {
		args = append(args,
			"--authorization-webhook-config-file="+authzKubeconfig,
			"--authorization-webhook-version=v1",
			"--authorization-webhook-cache-authorized-ttl=30s",
			"--authorization-webhook-cache-unauthorized-ttl=10s",
		)
	}
	return args
}

// deleteStaleKubernetesEndpoints removes the default/kubernetes Endpoints object
// through kine so the bootstrap controller starts fresh on each run.
func deleteStaleKubernetesEndpoints(ctx context.Context, etcdEndpoint string) error {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{etcdEndpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return err
	}
	defer client.Close()
	// Kine rejects a bare DeleteRange RPC and recognises a delete only by the
	// transaction shape "range followed by delete", so the range operation is
	// required even though its result is unused.
	_, err = client.Txn(ctx).Then(
		clientv3.OpGet(kubernetesEndpointsEtcdKey),
		clientv3.OpDelete(kubernetesEndpointsEtcdKey),
	).Commit()
	return err
}
