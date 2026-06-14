package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/casosorg/casos/util"
	_ "github.com/go-sql-driver/mysql"
	"github.com/k3s-io/kine/pkg/endpoint"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"

	globalflag "k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/logs"
	apiserverapp "k8s.io/kubernetes/cmd/kube-apiserver/app"
	"k8s.io/kubernetes/cmd/kube-apiserver/app/options"
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

	if err := deleteStaleKubernetesEndpoints(cfg.DSN); err != nil {
		logrus.Warnf("failed to delete stale kubernetes endpoints: %v", err)
	}

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
		"--kubelet-preferred-address-types=InternalIP,ExternalIP,Hostname",
		"--kubelet-client-certificate=" + filepath.Join(certDir, "apiserver-kubelet-client.crt"),
		"--kubelet-client-key=" + filepath.Join(certDir, "apiserver-kubelet-client.key"),
	}
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
