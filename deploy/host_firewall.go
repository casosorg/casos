package deploy

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/beego/beego/logs"
)

// Windows drops inbound packets that no rule allows instead of refusing them,
// so a worker node — a WSL distro reaching the host over the WSL subnet, or a
// machine on the LAN — sees the apiserver connection time out rather than fail.
// CasOS opens the one port it needs itself, because the alternative users reach
// for is turning the whole firewall off.
const (
	hostFirewallRulePrefix     = "CasOS apiserver"
	hostFirewallCommandTimeout = 20 * time.Second
)

type HostFirewallState string

const (
	HostFirewallStateNotNeeded HostFirewallState = "notNeeded"
	HostFirewallStatePresent   HostFirewallState = "present"
	HostFirewallStateAdded     HostFirewallState = "added"
	HostFirewallStateFailed    HostFirewallState = "failed"
)

type HostFirewallResult struct {
	State    HostFirewallState
	RuleName string
	Port     int
	Err      error
}

// EnsureApiserverHostFirewallRule allows inbound TCP on the apiserver port from
// the local subnets. It is a no-op off Windows, where CasOS does not manage the
// host firewall, and where a blocked port is the administrator's own doing.
func EnsureApiserverHostFirewallRule(ctx context.Context, port int) HostFirewallResult {
	if runtime.GOOS != "windows" || port <= 0 || port > 65535 {
		return HostFirewallResult{State: HostFirewallStateNotNeeded, Port: port}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	name := hostFirewallRuleName(port)
	result := HostFirewallResult{RuleName: name, Port: port}

	if hostFirewallRuleExists(ctx, name) {
		result.State = HostFirewallStatePresent
		return result
	}
	if err := addHostFirewallRule(ctx, name, port); err != nil {
		result.State = HostFirewallStateFailed
		result.Err = err
		return result
	}
	result.State = HostFirewallStateAdded
	return result
}

// Message describes the outcome for the node deployment log. An empty message
// means there is nothing worth telling the user about.
func (r HostFirewallResult) Message() string {
	switch r.State {
	case HostFirewallStatePresent:
		return fmt.Sprintf("Windows Firewall already allows inbound TCP %d for the apiserver (rule %q)", r.Port, r.RuleName)
	case HostFirewallStateAdded:
		return fmt.Sprintf("Added Windows Firewall rule %q allowing inbound TCP %d from the local subnets, so worker nodes can reach the apiserver", r.RuleName, r.Port)
	case HostFirewallStateFailed:
		return fmt.Sprintf("Could not open inbound TCP %d in Windows Firewall (%v). %s", r.Port, r.Err, hostFirewallRemediation(r.Port))
	default:
		return ""
	}
}

func (r HostFirewallResult) Level() string {
	if r.State == HostFirewallStateFailed {
		return "error"
	}
	return "info"
}

func hostFirewallRuleName(port int) string {
	return fmt.Sprintf("%s %d", hostFirewallRulePrefix, port)
}

// hostFirewallRemediation is the advice a blocked apiserver port always ends
// with, whether the rule could not be added or a security suite CasOS cannot
// configure is dropping the packets anyway.
func hostFirewallRemediation(port int) string {
	if port <= 0 {
		return "Allow inbound TCP on the apiserver port on the CasOS host and on anything filtering the path to it."
	}
	if runtime.GOOS != "windows" {
		return fmt.Sprintf("Allow inbound TCP %d on the CasOS host and on anything filtering the path to it.", port)
	}
	return fmt.Sprintf(
		"Start CasOS as an administrator so it can add the rule, or add it manually in an elevated terminal: %s. "+
			"Third-party security suites (Huorong, 360 and the like) filter separately and need the same port allowed in their own rules — allow the port rather than switching their protection off.",
		hostFirewallManualCommand(port),
	)
}

func hostFirewallManualCommand(port int) string {
	return fmt.Sprintf(`netsh advfirewall firewall add rule name="%s" dir=in action=allow protocol=TCP localport=%d`,
		hostFirewallRuleName(port), port)
}

func hostFirewallRuleExists(ctx context.Context, name string) bool {
	_, err := runHostFirewallCommand(ctx, "show", "rule", "name="+name, "dir=in")
	return err == nil
}

func addHostFirewallRule(ctx context.Context, name string, port int) error {
	// remoteip=LocalSubnet follows the interfaces the host actually has, which
	// is what keeps the rule working after WSL renumbers its subnet on restart.
	output, err := runHostFirewallCommand(ctx, "add", "rule",
		"name="+name,
		"dir=in",
		"action=allow",
		"protocol=TCP",
		fmt.Sprintf("localport=%d", port),
		"profile=any",
		"remoteip=LocalSubnet",
		"description=Added by CasOS so worker nodes can reach the Kubernetes apiserver",
	)
	if err != nil {
		// netsh answers in the system code page, so its text is not something
		// to put in front of the user, but it is what a log is for.
		logs.Warning("failed to add Windows Firewall rule %q for TCP %d: %v: %s", name, port, err, output)
		return err
	}
	return nil
}

func runHostFirewallCommand(ctx context.Context, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, hostFirewallCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "netsh", append([]string{"advfirewall", "firewall"}, args...)...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}
