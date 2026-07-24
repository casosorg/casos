package deploy

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const nodeDeployWSLProbeCommand = "[ -f /proc/sys/fs/binfmt_misc/WSLInterop ] || grep -qi microsoft /proc/version"

type NodeDeployPreflightResult struct {
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Systemd      bool   `json:"systemd"`
	PackageTool  string `json:"packageTool"`
	CanSudo      bool   `json:"canSudo"`
	ApiserverURL string `json:"apiserverUrl"`
	ApiserverOK  bool   `json:"apiserverOk"`
	WSL          bool   `json:"wsl"`
}

func RunNodeDeployPreflight(ctx context.Context, runner *NodeDeploySSHRunner, apiserverURL string) (*NodeDeployPreflightResult, error) {
	if runner == nil {
		return nil, fmt.Errorf("ssh runner is required")
	}
	result := &NodeDeployPreflightResult{}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	osName, err := runner.RunContext(ctx, "uname -s")
	if err != nil {
		return nil, fmt.Errorf("detect os: %w", err)
	}
	if strings.TrimSpace(osName) != "Linux" {
		return nil, fmt.Errorf("unsupported os %q: only Linux is supported", osName)
	}
	result.OS = "linux"

	arch, err := runner.RunContext(ctx, "arch=$(uname -m 2>/dev/null | awk 'NF {print; exit}'); if [ -z \"$arch\" ] && command -v dpkg >/dev/null 2>&1; then arch=$(dpkg --print-architecture 2>/dev/null | awk 'NF {print; exit}'); fi; if [ -z \"$arch\" ] && command -v arch >/dev/null 2>&1; then arch=$(arch 2>/dev/null | awk 'NF {print; exit}'); fi; printf %s \"$arch\"")
	if err != nil {
		return nil, fmt.Errorf("detect arch: %w", err)
	}
	if strings.TrimSpace(arch) == "" {
		return nil, fmt.Errorf("failed to detect machine architecture: uname -m returned empty output")
	}
	result.Arch = normalizeNodeDeployArch(arch)
	if result.Arch != "amd64" && result.Arch != "arm64" {
		return nil, fmt.Errorf("unsupported arch %q: only amd64 and arm64 are supported", result.Arch)
	}

	if _, err = runner.RunContext(ctx, "command -v systemctl >/dev/null"); err != nil {
		return nil, fmt.Errorf("systemctl not found: systemd is required for node services: %w", err)
	}
	if _, err = runner.RunContext(ctx, "[ \"$(ps -p 1 -o comm=)\" = systemd ]"); err != nil {
		return nil, fmt.Errorf("PID 1 is not systemd: systemd is required for node services: %w", err)
	}
	result.Systemd = true

	if _, err = runner.RunContext(ctx, "command -v apt-get >/dev/null"); err != nil {
		return nil, fmt.Errorf("unsupported package manager: apt-get is required for Ubuntu/Debian node deployment in this version: %w", err)
	}
	result.PackageTool = "apt"

	if _, err = runner.RunContext(ctx, "sudo -n true 2>/dev/null || [ \"$(id -u)\" = 0 ]"); err != nil {
		return nil, fmt.Errorf("root or passwordless sudo is required: %w", err)
	}
	result.CanSudo = true

	if isNodeDeployWSL(ctx, runner) {
		result.WSL = true
	}

	if apiserverURL != "" {
		parsed, err := url.Parse(apiserverURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, fmt.Errorf("apiserverUrl must be a valid https URL")
		}
		if _, err = runner.RunContext(ctx, "command -v curl >/dev/null"); err != nil {
			return nil, fmt.Errorf("curl is required to check apiserver reachability: %w", err)
		}
		if _, err = runner.RunContext(ctx, "command -v base64 >/dev/null"); err != nil {
			return nil, fmt.Errorf("base64 is required to check apiserver reachability: %w", err)
		}
		trimmedURL := strings.TrimRight(apiserverURL, "/")
		result.ApiserverURL = trimmedURL
		// The bootstrap kubeconfig embeds the apiserver CA, but this early
		// reachability probe runs before those files exist on the target node.
		encodedURL := base64.StdEncoding.EncodeToString([]byte(trimmedURL))
		cmd := fmt.Sprintf("apiserver_url=$(printf %%s %s | base64 -d) && curl -ksS --connect-timeout 5 --output /dev/null --write-out %%{http_code} \"$apiserver_url/readyz\"", shellSingleQuote(encodedURL))
		status, err := runner.RunContext(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("apiserver is not reachable from target: %w", err)
		}
		if !isNodeDeployApiserverProbeStatus(status) {
			return nil, fmt.Errorf("apiserver readiness probe returned HTTP status %q", strings.TrimSpace(status))
		}
		result.ApiserverOK = true
	}

	return result, nil
}

func isNodeDeployApiserverProbeStatus(status string) bool {
	code, err := strconv.Atoi(strings.TrimSpace(status))
	if err != nil {
		return false
	}
	return (code >= 200 && code < 300) || code == http.StatusUnauthorized || code == http.StatusForbidden
}

func ResolveNodeDeployApiserverURL(ctx context.Context, runner *NodeDeploySSHRunner, fallbackURL string) string {
	fallbackURL = strings.TrimRight(strings.TrimSpace(fallbackURL), "/")
	if runner == nil {
		return fallbackURL
	}
	wslCtx, wslCancel := context.WithTimeout(ctx, 3*time.Second)
	defer wslCancel()
	if !isNodeDeployWSL(wslCtx, runner) {
		return fallbackURL
	}
	gatewayCtx, gatewayCancel := context.WithTimeout(ctx, 5*time.Second)
	defer gatewayCancel()
	gateway, err := runner.RunContext(gatewayCtx, "ip route | awk '$1 == \"default\" {print $3; exit}'")
	if err != nil {
		return fallbackURL
	}
	gateway = strings.TrimSpace(gateway)
	if gateway == "" {
		return fallbackURL
	}
	parsed, err := url.Parse(fallbackURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fallbackURL
	}
	port := parsed.Port()
	if port == "" {
		if strings.Contains(gateway, ":") {
			parsed.Host = "[" + gateway + "]"
		} else {
			parsed.Host = gateway
		}
	} else {
		parsed.Host = net.JoinHostPort(gateway, port)
	}
	return strings.TrimRight(parsed.String(), "/")
}

func isNodeDeployWSL(ctx context.Context, runner *NodeDeploySSHRunner) bool {
	if runner == nil {
		return false
	}
	_, err := runner.RunContext(ctx, nodeDeployWSLProbeCommand)
	return err == nil
}

func normalizeNodeDeployArch(arch string) string {
	switch strings.TrimSpace(arch) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return strings.TrimSpace(arch)
	}
}
