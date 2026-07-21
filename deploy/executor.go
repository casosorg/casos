package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	"golang.org/x/crypto/ssh"
)

// NodeDeploySSHConfig holds connection settings for a deployment target.
type NodeDeploySSHConfig struct {
	Host           string
	Port           int
	Username       string
	Password       string
	PrivateKey     string
	Timeout        time.Duration
	CommandTimeout time.Duration
}

func (c NodeDeploySSHConfig) String() string {
	return fmt.Sprintf("NodeDeploySSHConfig{Host:%s Port:%d Username:%s Password:[redacted] PrivateKey:[redacted] Timeout:%s CommandTimeout:%s}",
		c.Host, c.Port, c.Username, c.Timeout, c.CommandTimeout)
}

// NodeDeploySSHRunner executes remote shell commands over SSH.
type NodeDeploySSHRunner struct {
	client         *ssh.Client
	commandTimeout time.Duration
}

const defaultCommandTimeout = 20 * time.Minute

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func NewNodeDeploySSHRunner(cfg NodeDeploySSHConfig) (*NodeDeploySSHRunner, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Username = strings.TrimSpace(cfg.Username)
	if cfg.Host == "" {
		return nil, fmt.Errorf("ssh host is required")
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("ssh username is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 20 * time.Second
	}
	if cfg.CommandTimeout == 0 {
		cfg.CommandTimeout = defaultCommandTimeout
	}
	auth, err := nodeDeployAuthMethods(cfg)
	if err != nil {
		return nil, err
	}
	clientCfg := &ssh.ClientConfig{
		User: cfg.Username,
		Auth: auth,
		// First-time machine enrollment only has basic SSH information today.
		// Add known_hosts or fingerprint support before using this on untrusted networks.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.Timeout,
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, err
	}
	return &NodeDeploySSHRunner{client: client, commandTimeout: cfg.CommandTimeout}, nil
}

func (r *NodeDeploySSHRunner) effectiveCommandTimeout() time.Duration {
	if r.commandTimeout <= 0 {
		return defaultCommandTimeout
	}
	return r.commandTimeout
}

func (r *NodeDeploySSHRunner) contextCommandTimeout(ctx context.Context) time.Duration {
	timeout := r.effectiveCommandTimeout()
	if ctx == nil {
		return timeout
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return timeout
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Nanosecond
	}
	if remaining < timeout {
		return remaining
	}
	return timeout
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func nodeDeployAuthMethods(cfg NodeDeploySSHConfig) ([]ssh.AuthMethod, error) {
	methods := []ssh.AuthMethod{}
	if strings.TrimSpace(cfg.Password) != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}
	if strings.TrimSpace(cfg.PrivateKey) != "" {
		signer, err := ssh.ParsePrivateKey([]byte(cfg.PrivateKey))
		if err != nil {
			if strings.Contains(err.Error(), "encrypted") || strings.Contains(err.Error(), "passphrase") {
				return nil, fmt.Errorf("parse private key: passphrase-protected private keys are not supported")
			}
			logs.Warning("failed to parse SSH private key: %v", err)
			return nil, fmt.Errorf("invalid private key: the key format is not supported or the key is corrupted")
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("missing ssh credential")
	}
	return methods, nil
}

func (r *NodeDeploySSHRunner) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}

// Run executes a shell command on the remote host. Callers must shell-escape
// user-controlled values before concatenating them into command.
func (r *NodeDeploySSHRunner) Run(command string) (string, error) {
	return r.RunContext(context.Background(), command)
}

func (r *NodeDeploySSHRunner) RunContext(ctx context.Context, command string) (string, error) {
	// command is passed directly to the remote shell. Callers must not
	// concatenate unescaped user-controlled values into this string.
	if err := contextErr(ctx); err != nil {
		return "", err
	}
	session, err := r.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var out lockedBuffer
	session.Stdout = &out
	session.Stderr = &out
	err = r.runSessionWithContext(ctx, session, command)
	text := strings.TrimSpace(out.String())
	if err != nil {
		if text == "" {
			return "", err
		}
		return text, fmt.Errorf("%w: %s", err, text)
	}
	return text, nil
}

func (r *NodeDeploySSHRunner) RunQuiet(command string) error {
	return r.RunQuietContext(context.Background(), command)
}

func (r *NodeDeploySSHRunner) RunQuietContext(ctx context.Context, command string) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	session, err := r.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var out lockedBuffer
	session.Stdout = &out
	session.Stderr = &out
	if err = r.runSessionWithContext(ctx, session, command); err != nil {
		return fmt.Errorf("%w: %s", err, summarizeSSHOutput(out.String()))
	}
	return nil
}

func (r *NodeDeploySSHRunner) RunRoot(command string) (string, error) {
	return r.RunRootContext(context.Background(), command)
}

func (r *NodeDeploySSHRunner) RunRootContext(ctx context.Context, command string) (string, error) {
	return r.RunContext(ctx, fmt.Sprintf("if [ \"$(id -u)\" = 0 ]; then sh -c %s; else sudo sh -c %s; fi",
		shellSingleQuote(command), shellSingleQuote(command)))
}

func (r *NodeDeploySSHRunner) WriteFile(path, content string, mode string) error {
	return r.WriteFileContext(context.Background(), path, content, mode)
}

func (r *NodeDeploySSHRunner) WriteFileContext(ctx context.Context, path, content string, mode string) error {
	if !isAllowedNodeDeployPath(path) {
		return fmt.Errorf("unsupported node deployment path: %s", path)
	}
	if len(mode) != 4 || mode[0] != '0' {
		return fmt.Errorf("invalid file mode: %q", mode)
	}
	if _, err := strconv.ParseUint(mode, 8, 16); err != nil {
		return fmt.Errorf("invalid file mode: %q", mode)
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	session, err := r.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	pipe, err := session.StdinPipe()
	if err != nil {
		return err
	}
	var out lockedBuffer
	session.Stdout = &out
	session.Stderr = &out

	cmd := fmt.Sprintf("if [ \"$(id -u)\" = 0 ]; then install -D -m %s /dev/stdin %s; else sudo install -D -m %s /dev/stdin %s; fi",
		shellSingleQuote(mode), shellSingleQuote(path), shellSingleQuote(mode), shellSingleQuote(path))
	waitCh, err := r.startSessionWithContext(ctx, session, cmd)
	if err != nil {
		_ = pipe.Close()
		return err
	}
	_, copyErr := io.WriteString(pipe, content)
	closeErr := pipe.Close()
	waitErr := <-waitCh
	if copyErr != nil {
		return fmt.Errorf("write content to pipe: %w (remote error: %v)", copyErr, waitErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close pipe: %w (remote error: %v)", closeErr, waitErr)
	}
	if waitErr != nil {
		return fmt.Errorf("%w: %s", waitErr, summarizeSSHOutput(out.String()))
	}
	return nil
}

func (r *NodeDeploySSHRunner) AppendAuthorizedKey(publicKey string) error {
	return r.AppendAuthorizedKeyContext(context.Background(), publicKey)
}

func (r *NodeDeploySSHRunner) AppendAuthorizedKeyContext(ctx context.Context, publicKey string) error {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" || strings.ContainsAny(publicKey, "\r\n") {
		return fmt.Errorf("authorized key must be a single non-empty line")
	}
	cmd := fmt.Sprintf("set -e; mkdir -p ~/.ssh; chmod 700 ~/.ssh; touch ~/.ssh/authorized_keys; chmod 600 ~/.ssh/authorized_keys; if ! grep -qxF %s ~/.ssh/authorized_keys; then printf '%%s\\n' %s >> ~/.ssh/authorized_keys; fi",
		shellSingleQuote(publicKey), shellSingleQuote(publicKey))
	_, err := r.RunContext(ctx, cmd)
	return err
}

func (r *NodeDeploySSHRunner) runSessionWithContext(ctx context.Context, session *ssh.Session, command string) error {
	waitCh, err := r.startSessionWithContext(ctx, session, command)
	if err != nil {
		return err
	}
	return <-waitCh
}

func (r *NodeDeploySSHRunner) startSessionWithContext(ctx context.Context, session *ssh.Session, command string) (<-chan error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}
	if err := session.Start(command); err != nil {
		return nil, err
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- session.Wait()
	}()

	timeout := r.contextCommandTimeout(ctx)
	resultCh := make(chan error, 1)
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case err := <-waitCh:
			resultCh <- err
		case <-ctx.Done():
			_ = session.Signal(ssh.SIGKILL)
			_ = session.Close()
			resultCh <- ctx.Err()
		case <-timer.C:
			_ = session.Signal(ssh.SIGKILL)
			_ = session.Close()
			resultCh <- fmt.Errorf("remote command timed out after %s", timeout)
		}
	}()
	return resultCh, nil
}

func isAllowedNodeDeployPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\x00") || strings.Contains(path, "..") {
		return false
	}
	allowedExactPaths := []string{
		"/etc/containerd/config.toml",
		"/etc/containerd/certs.d/docker.io/hosts.toml",
		"/etc/containerd/certs.d/registry.k8s.io/hosts.toml",
		"/etc/kubernetes/worker.kubeconfig",
		"/etc/kubernetes/ca.crt",
		"/etc/systemd/system/kubelet.service",
		"/etc/systemd/system/kube-proxy.service",
		"/var/lib/kubelet/config.yaml",
		"/var/lib/kube-proxy/config.yaml",
	}
	for _, allowedPath := range allowedExactPaths {
		if path == allowedPath {
			return true
		}
	}
	return false
}

// shellSingleQuote returns value quoted for one POSIX shell argument.
// Values containing newlines must be validated or encoded by callers.
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func summarizeSSHOutput(output string) string {
	text := strings.TrimSpace(output)
	if text == "" {
		return "remote command failed"
	}
	text = strings.ReplaceAll(text, "\n", " ")
	if len(text) > 200 {
		return text[:200] + "..."
	}
	return text
}
