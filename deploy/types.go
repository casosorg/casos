package deploy

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/casosorg/casos/server"
)

const (
	defaultNodeDeployKubernetesVersion = "v1.36.1"
	defaultNodeDeployCNIVersion        = "v1.5.1"
	nodeDeployClusterCIDR              = "10.244.0.0/16"
	nodeDeployClusterDNS               = "10.43.0.10"
	nodeDeployPhasePreflight           = "preflight"
	nodeDeployPhaseInstalling          = "installing"
	nodeDeployPhaseConfiguring         = "configuring"
	nodeDeployPhaseStarting            = "starting"
	nodeDeployPhaseWaiting             = "waiting"
	nodeDeployPhaseReady               = "ready"
)

type NodeDeployLogger func(level, message, phase string)

type MachineNodeDeployRequest struct {
	Owner        string `json:"owner"`
	MachineName  string `json:"machineName"`
	NodeName     string `json:"nodeName"`
	ApiserverURL string `json:"apiserverUrl"`
}

func (r *MachineNodeDeployRequest) normalize() {
	if r.Owner == "" {
		r.Owner = "admin"
	}
	r.MachineName = strings.TrimSpace(r.MachineName)
	r.NodeName = strings.TrimSpace(r.NodeName)
	r.ApiserverURL = strings.TrimSpace(r.ApiserverURL)
}

func (r *MachineNodeDeployRequest) validate() error {
	if r.MachineName == "" {
		return fmt.Errorf("machineName is required")
	}
	if strings.Contains(r.MachineName, "/") {
		return fmt.Errorf("machineName must not contain /")
	}
	if len(r.MachineName) > 100 {
		return fmt.Errorf("machineName must not exceed 100 characters")
	}
	if r.NodeName != "" {
		if err := validateNodeDeployName(r.NodeName); err != nil {
			return err
		}
	}
	if r.ApiserverURL != "" {
		parsed, err := url.Parse(r.ApiserverURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("apiserverUrl must be a valid https URL")
		}
	}
	return nil
}

type NodeDeployOptions struct {
	Machine      NodeDeployMachine
	NodeName     string
	ApiserverURL string
}

func (opts *NodeDeployOptions) validate() error {
	opts.NodeName = strings.TrimSpace(opts.NodeName)
	opts.Machine.Host = strings.TrimSpace(opts.Machine.Host)
	if opts.Machine.Host == "" {
		return fmt.Errorf("machine host is required")
	}
	if opts.NodeName == "" {
		return fmt.Errorf("nodeName is required")
	}
	if err := validateNodeDeployName(opts.NodeName); err != nil {
		return err
	}
	opts.ApiserverURL = strings.TrimRight(strings.TrimSpace(opts.ApiserverURL), "/")
	if opts.ApiserverURL != "" {
		parsed, err := url.Parse(opts.ApiserverURL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("apiserverUrl must be a valid https URL")
		}
	}
	return nil
}

type NodeDeployMachine struct {
	Host       string
	Port       int
	Username   string
	Password   string
	PrivateKey string
}

type NodeDeployResult struct {
	// ManagedPrivateKey contains a raw PEM-encoded SSH private key. Callers must
	// store it encrypted and must not log or return it to API clients.
	ManagedPrivateKey string
}

type NodeKubeconfig struct {
	Kubeconfig string
	NodeName   string
}

type KubeconfigGenerator func(nodeName, apiserverURL string) (*NodeKubeconfig, error)

type Config struct {
	AdvertiseAddress   string
	ApiserverBind      string
	ApiserverPort      int
	SandboxImage       string
	Socks5Proxy        string
	GenerateKubeconfig KubeconfigGenerator
}

func ConfigFromServerConfig(cfg server.Config) Config {
	return Config{
		AdvertiseAddress: cfg.AdvertiseAddress,
		ApiserverBind:    cfg.ApiserverBind,
		ApiserverPort:    cfg.ApiserverPort,
		SandboxImage:     cfg.SandboxImage,
		Socks5Proxy:      cfg.Socks5Proxy,
		GenerateKubeconfig: func(nodeName, apiserverURL string) (*NodeKubeconfig, error) {
			wk, err := server.GenerateWorkerKubeconfigForServer(cfg, nodeName, apiserverURL)
			if err != nil {
				return nil, err
			}
			return &NodeKubeconfig{Kubeconfig: wk.Kubeconfig, NodeName: wk.NodeName}, nil
		},
	}
}

var nodeDeployNameRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

func validateNodeDeployName(nodeName string) error {
	nodeName = strings.TrimSpace(nodeName)
	if len(nodeName) > 253 || !nodeDeployNameRegexp.MatchString(nodeName) {
		return fmt.Errorf("nodeName must be a valid RFC 1123 subdomain")
	}
	for _, label := range strings.Split(nodeName, ".") {
		if len(label) > 63 {
			return fmt.Errorf("nodeName labels must not exceed 63 characters")
		}
	}
	return nil
}
