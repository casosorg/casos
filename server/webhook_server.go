package server

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/beego/beego/logs"

	"github.com/casosorg/casos/util"
)

// StartWebhookServer generates the webhook TLS cert (if absent) and launches
// the shared HTTPS server that hosts both the admission and authorization
// webhook endpoints.
func StartWebhookServer(cfg Config) error {
	certDir := filepath.Join(cfg.DataDir, "tls")
	if err := EnsureWebhookCert(certDir); err != nil {
		return fmt.Errorf("webhook cert: %w", err)
	}

	mux := http.NewServeMux()
	RegisterAdmissionHandler(mux)
	RegisterAuthorizationHandler(mux)

	return startHTTPSServer(certDir, cfg.WebhookPort, mux)
}

func startHTTPSServer(certDir string, port int, handler http.Handler) error {
	cert, err := tls.LoadX509KeyPair(
		filepath.Join(certDir, "webhook.crt"),
		filepath.Join(certDir, "webhook.key"),
	)
	if err != nil {
		return fmt.Errorf("load webhook cert: %w", err)
	}

	if stopped, err := util.ReclaimPort("", port); err != nil {
		logs.Warning("webhook port: %v", err)
	} else if stopped != "" {
		logs.Warning("port %d was held by %s, which has been stopped so the webhook server can use it", port, stopped)
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		},
	}

	go func() {
		logs.Info("webhook server listening on :%d", port)
		if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			logs.Error("webhook server error: %v", err)
		}
	}()
	return nil
}
