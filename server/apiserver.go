package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
	serviceClusterIPRange      = "10.43.0.0/16"
	kubernetesServiceIP        = "10.43.0.1"
	kubernetesEndpointsEtcdKey = "/registry/services/endpoints/default/kubernetes"
)

// Start launches kine and the apiserver in-process.
// The returned channel is closed once the apiserver /readyz endpoint responds 200.
func Start(ctx context.Context, cfg Config) (<-chan struct{}, error) {
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

	if err := util.StopOldInstance(2379); err != nil {
		logrus.Warnf("failed to stop old instance on port 2379: %v", err)
	}
	if err := ensureKineDataDirectory(cfg.DatastoreEndpoint); err != nil {
		return nil, err
	}
	etcdCfg, err := endpoint.Listen(ctx, kineEndpointConfig(cfg.DatastoreEndpoint))
	if err != nil {
		return nil, fmt.Errorf("kine listen: %w", err)
	}
	logrus.Infof("kine started, etcd endpoint: %v", etcdCfg.Endpoints)

	cleanupCtx, cancelCleanup := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCleanup()
	if err := deleteStaleKubernetesEndpoints(cleanupCtx, etcdCfg.Endpoints[0]); err != nil {
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

	if err := fs.Parse(buildApiserverArgs(cfg, certDir, etcdCfg.Endpoints[0], authzKubeconfig)); err != nil {
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

func kineEndpointConfig(datastoreEndpoint string) endpoint.Config {
	return endpoint.Config{
		Endpoint:            datastoreEndpoint,
		Listener:            "tcp://127.0.0.1:2379",
		EmulatedETCDVersion: "3.6.11",
		CompactBatchSize:    100,
		NotifyInterval:      time.Second,
	}
}

func ensureKineDataDirectory(datastoreEndpoint string) error {
	if !strings.HasPrefix(datastoreEndpoint, "sqlite://") {
		return nil
	}
	databasePath := strings.SplitN(strings.TrimPrefix(datastoreEndpoint, "sqlite://"), "?", 2)[0]
	if databasePath == "" {
		return fmt.Errorf("SQLite kine endpoint has no database path")
	}
	if runtime.GOOS == "windows" && len(databasePath) >= 3 && databasePath[0] == '/' && databasePath[2] == ':' {
		databasePath = databasePath[1:]
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
	_, err = client.Txn(ctx).Then(
		clientv3.OpGet(kubernetesEndpointsEtcdKey),
		clientv3.OpDelete(kubernetesEndpointsEtcdKey),
	).Commit()
	return err
}
