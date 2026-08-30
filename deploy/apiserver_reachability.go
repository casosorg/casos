package deploy

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	curlExitCodeRegexp    = regexp.MustCompile(`curl: \((\d+)\)`)
	commandExitCodeRegexp = regexp.MustCompile(`exit(?:ed with)? status (\d+)`)
)

// diagnoseNodeDeployApiserverProbe turns the exit code of the reachability
// probe into the one thing the operator has to act on. How far the connection
// got is the whole diagnosis: dropped packets time out, while a host with
// nothing listening refuses them straight away.
func diagnoseNodeDeployApiserverProbe(apiserverURL string, err error) error {
	port := nodeDeployApiserverPort(apiserverURL)
	switch nodeDeployProbeExitCode(err) {
	case 28:
		return fmt.Errorf("apiserver %s is not reachable from the target node: the connection timed out, so a firewall is dropping the packets rather than the host refusing them. %s Original error: %w",
			apiserverURL, hostFirewallRemediation(port), err)
	case 7:
		return fmt.Errorf("apiserver %s is not reachable from the target node: the connection was refused, so nothing is listening on that address. Check that CasOS is running and that its apiserverBind and apiserverPort match this URL: %w",
			apiserverURL, err)
	case 6:
		return fmt.Errorf("apiserver %s is not reachable from the target node: its host name does not resolve there. Use an IP address the node can reach, or add the name to the node's /etc/hosts: %w",
			apiserverURL, err)
	case 35, 60:
		return fmt.Errorf("apiserver %s refused the TLS handshake from the target node, which usually means something other than the apiserver answers on that port: %w",
			apiserverURL, err)
	default:
		return fmt.Errorf("apiserver %s is not reachable from the target node: %w", apiserverURL, err)
	}
}

// nodeDeployProbeExitCode reads the curl exit code out of a runner error.
// curl reports it itself, and both runners repeat it as the shell exit status,
// which is what is left when curl is too quiet to say anything.
func nodeDeployProbeExitCode(err error) int {
	if err == nil {
		return 0
	}
	text := err.Error()
	for _, pattern := range []*regexp.Regexp{curlExitCodeRegexp, commandExitCodeRegexp} {
		if match := pattern.FindStringSubmatch(text); match != nil {
			code, convErr := strconv.Atoi(match[1])
			if convErr == nil {
				return code
			}
		}
	}
	return -1
}

func nodeDeployApiserverPort(apiserverURL string) int {
	parsed, err := url.Parse(strings.TrimSpace(apiserverURL))
	if err != nil {
		return 0
	}
	if parsed.Port() == "" {
		if parsed.Scheme == "https" {
			return 443
		}
		return 0
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return 0
	}
	return port
}
