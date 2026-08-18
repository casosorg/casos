package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"

	"github.com/casosorg/casos/conf"
)

const defaultHelmInstallTimeout = 5 * time.Minute

func helmInstallOperationDeadline(installTimeout time.Duration) time.Duration {
	return helmChartLoadTimeout + helmCompatibilityTimeout + installTimeout + helmDiagnosticsTimeout
}

func configuredHelmInstallTimeout() (time.Duration, error) {
	return parseHelmInstallTimeout(conf.GetConfigString("helmInstallTimeout"))
}

func parseHelmInstallTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultHelmInstallTimeout, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid helmInstallTimeout %q: %w", raw, err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("helmInstallTimeout must be greater than zero")
	}
	return timeout, nil
}

func configureHelmInstall(install *action.Install, releaseName, namespace string, timeout time.Duration) {
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.CreateNamespace = true
	install.Wait = true
	install.WaitForJobs = true
	install.Timeout = timeout
	install.PostRenderer = configuredLocalImagePullPolicyPostRenderer()
}

func finishFailedHelmInstall(installErr error, diagnose func(error) error, cleanup func() error) error {
	if diagnose != nil {
		installErr = diagnose(installErr)
	}
	if cleanupErr := cleanup(); cleanupErr != nil {
		return errors.Join(installErr, fmt.Errorf("clean up failed Helm release: %w", cleanupErr))
	}
	return installErr
}

func cleanupFailedHelmRelease(actionConfig *action.Configuration, install *action.Install, failedRelease *release.Release) error {
	if failedRelease == nil || failedRelease.Info == nil || failedRelease.Info.Status != release.StatusFailed {
		return nil
	}
	currentRelease, err := actionConfig.Releases.Last(install.ReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("verify failed Helm release before cleanup: %w", err)
	}
	if !shouldCleanupFailedHelmRelease(failedRelease, currentRelease) {
		return nil
	}

	uninstall := action.NewUninstall(actionConfig)
	uninstall.DisableHooks = install.DisableHooks
	uninstall.KeepHistory = false
	uninstall.Timeout = install.Timeout
	_, err = uninstall.Run(install.ReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil
	}
	return err
}

func shouldCleanupFailedHelmRelease(failedRelease, currentRelease *release.Release) bool {
	if failedRelease == nil || failedRelease.Info == nil || failedRelease.Info.Status != release.StatusFailed ||
		currentRelease == nil || currentRelease.Info == nil ||
		failedRelease.Name != currentRelease.Name || failedRelease.Version != currentRelease.Version {
		return false
	}
	return currentRelease.Info.Status == release.StatusFailed || currentRelease.Info.Status == release.StatusPendingInstall
}
