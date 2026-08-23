package deploy

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/wsl"
)

// Zero configuration worker node.
//
// A cluster with no node cannot run anything, so CasOS never leaves itself
// without one: at startup it turns the machine it is already running on into a
// worker node. On Linux that is the host itself, reached through a local shell
// that needs neither sshd nor a credential. On Windows the host cannot run a
// kubelet, so its WSL distribution stands in for it, and CasOS installs WSL
// when the host has none.
//
// Everything runs in the background and reports through the server log. The
// deployment itself is an ordinary node deployment task, so the Machines page
// shows its progress and its logs.

const (
	localNodeBootstrapAttempts   = 3
	localNodeBootstrapRetryDelay = time.Minute
	localNodeDeployPollPeriod    = 5 * time.Second
)

// One run at a time: two would fight over wsl --shutdown and over the machine
// record they both write.
var localNodeBootstrapMutex sync.Mutex

// The keepalive outlives every bootstrap attempt, so it is started once.
var wslKeepAliveOnce sync.Once

// StartLocalNodeBootstrap enrolls the CasOS host as a worker node in the
// background. It is a no-op where that cannot work, so main can call it
// unconditionally.
func StartLocalNodeBootstrap() {
	if !conf.GetConfigBoolDefault("autoEnrollLocalNode", true) {
		logs.Info("automatic node setup is disabled by autoEnrollLocalNode")
		return
	}
	if err := localNodePlatformSupported(); err != nil {
		// A permanent property of the host, so there is nothing to retry.
		logs.Warning("automatic node setup: %v", err)
		return
	}
	go func() {
		defer func() {
			if v := recover(); v != nil {
				logs.Error("automatic node setup panic: %v", v)
			}
		}()
		runLocalNodeBootstrap(defaultService.contextSnapshot())
	}()
}

func localNodePlatformSupported() error {
	switch runtime.GOOS {
	case "windows", "linux":
		return nil
	case "darwin":
		return fmt.Errorf("macOS cannot host a Kubernetes worker node, because the kubelet needs a Linux kernel: add a Linux machine on the Machines page, or run CasOS inside a Linux VM")
	default:
		return fmt.Errorf("%s cannot host a Kubernetes worker node: add a Linux machine on the Machines page", runtime.GOOS)
	}
}

// runLocalNodeBootstrap retries, because a cluster left without a node is the
// one outcome worth spending a few more minutes to avoid: the control plane
// may still have been settling, or WSL may still have been booting.
func runLocalNodeBootstrap(ctx context.Context) {
	for attempt := 1; attempt <= localNodeBootstrapAttempts; attempt++ {
		err := bootstrapLocalNode(ctx)
		if err == nil {
			logs.Info("automatic node setup: this machine is a Ready worker node")
			return
		}
		logs.Warning("automatic node setup (attempt %d/%d): %v", attempt, localNodeBootstrapAttempts, err)
		if ctx.Err() != nil || attempt == localNodeBootstrapAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(localNodeBootstrapRetryDelay):
		}
	}
	logs.Error("automatic node setup did not succeed, the cluster has no worker node until one is added on the Machines page")
}

func bootstrapLocalNode(ctx context.Context) error {
	localNodeBootstrapMutex.Lock()
	defer localNodeBootstrapMutex.Unlock()

	machine, err := localNodeMachine(ctx)
	if err != nil {
		return err
	}
	task, err := defaultService.DeployMachineNode(MachineNodeDeployRequest{
		Owner:       machine.Owner,
		MachineName: machine.Name,
		NodeName:    machine.Name,
	})
	if err != nil {
		return fmt.Errorf("deploy node %s: %w", machine.Name, err)
	}
	logs.Info("automatic node setup: deploying node %s as task %d", machine.Name, task.Id)
	if err = waitForNodeDeployTask(ctx, task.Id); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err = ensureWindowsWSLClusterRoutes(ctx, machine.Ip); err != nil {
			return err
		}
	}
	return nil
}

// ensureWindowsWSLClusterRoutes lets the Windows-hosted control plane reach
// Services and Pods behind the WSL worker. WSL's NAT address changes whenever
// it restarts, so these active-store routes must be reconciled on every CasOS
// startup rather than installed once.
func ensureWindowsWSLClusterRoutes(ctx context.Context, gateway string) error {
	ip := net.ParseIP(strings.TrimSpace(gateway))
	if ip == nil || ip.To4() == nil || ip.IsLoopback() {
		return fmt.Errorf("configure WSL cluster routes: invalid WSL gateway %q", gateway)
	}
	gateway = ip.String()
	for _, route := range []struct {
		network string
		mask    string
	}{
		{network: "10.43.0.0", mask: "255.255.0.0"},
		{network: "10.244.0.0", mask: "255.255.0.0"},
	} {
		args := []string{"CHANGE", route.network, "MASK", route.mask, gateway, "METRIC", "5"}
		output, err := exec.CommandContext(ctx, "route.exe", args...).CombinedOutput()
		if err != nil {
			args[0] = "ADD"
			output, err = exec.CommandContext(ctx, "route.exe", args...).CombinedOutput()
		}
		if err != nil {
			return fmt.Errorf("configure Windows route %s/16 through WSL %s: %w: %s", route.network, gateway, err, strings.TrimSpace(string(output)))
		}
	}
	logs.Info("automatic node setup: Windows routes to Service and Pod networks now use WSL gateway %s", gateway)
	return nil
}

// localNodeMachine registers whatever stands in for this machine and returns
// the machine record to deploy onto.
func localNodeMachine(ctx context.Context) (*object.Machine, error) {
	if runtime.GOOS == "windows" {
		return localWSLNodeMachine(ctx)
	}
	return AddLocalHostMachine(ctx, "admin")
}

func localWSLNodeMachine(ctx context.Context) (*object.Machine, error) {
	distro, err := localWSLNodeDistro(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = wsl.EnsureSystemd(ctx, distro, func(line string) { logs.Info("wsl setup: %s", line) }); err != nil {
		return nil, err
	}
	startWSLKeepAlive(ctx, distro)
	result, err := AddLocalWSLMachine(ctx, "admin", distro)
	if err != nil {
		return nil, fmt.Errorf("enroll %s: %w", distro, err)
	}
	return result.Machine, nil
}

// startWSLKeepAlive keeps the distro that hosts the worker node running for as
// long as CasOS does. WSL stops a distro once nothing is attached to it, which
// takes the kubelet down with it, so without this the node goes NotReady a
// minute after the deployment finishes.
func startWSLKeepAlive(ctx context.Context, distro string) {
	wslKeepAliveOnce.Do(func() {
		go wsl.KeepAlive(ctx, distro,
			func(line string) { logs.Info("wsl keepalive: %s", line) },
			func() { reenrollWSLNode(ctx, distro) })
	})
}

// reenrollWSLNode refreshes what a restart of the distro invalidates. The node
// itself needs nothing, systemd starts the kubelet again, but WSL comes back on
// a new IP address, so the machine record and the host routes into the cluster
// both still point at the old one.
func reenrollWSLNode(ctx context.Context, distro string) {
	localNodeBootstrapMutex.Lock()
	defer localNodeBootstrapMutex.Unlock()

	result, err := AddLocalWSLMachine(ctx, "admin", distro)
	if err != nil {
		logs.Warning("automatic node setup: re-enrolling %s after its restart failed: %v", distro, err)
		return
	}
	if err = ensureWindowsWSLClusterRoutes(ctx, result.Machine.Ip); err != nil {
		logs.Warning("automatic node setup: %v", err)
	}
}

// localWSLNodeDistro returns the distro to enroll, installing WSL first when
// the host has nothing that can host a node.
func localWSLNodeDistro(ctx context.Context) (string, error) {
	status, err := wsl.Detect(ctx)
	if err != nil {
		return "", err
	}
	if selected := status.NodeDistro(); selected != nil {
		logs.Info("automatic node setup: using WSL distribution %s", selected.Name)
		return selected.Name, nil
	}

	logs.Info("automatic node setup: no usable WSL distribution (%s), installing one", status.Detail)
	installed, err := wsl.Install(ctx, wsl.DefaultInstallDistro, func(line string) { logs.Info("wsl install: %s", line) })
	if err != nil {
		return "", err
	}
	selected := installed.NodeDistro()
	if selected == nil {
		return "", fmt.Errorf("WSL was installed but no usable distribution is registered yet, restart Windows to finish enabling WSL")
	}
	logs.Info("automatic node setup: installed WSL distribution %s", selected.Name)
	return selected.Name, nil
}

func waitForNodeDeployTask(ctx context.Context, taskId int64) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(localNodeDeployPollPeriod):
		}
		task, err := object.GetMachineNodeDeployTask(taskId)
		if err != nil || task == nil {
			continue
		}
		switch task.Status {
		case object.MachineNodeDeployStatusSucceeded:
			return nil
		case object.MachineNodeDeployStatusFailed:
			if task.ErrorMsg != "" {
				return fmt.Errorf("node deployment failed: %s", task.ErrorMsg)
			}
			return fmt.Errorf("node deployment failed")
		}
	}
}
