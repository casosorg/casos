package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// quoteChar is the double quote wrapping each field of tasklist CSV output.
const quoteChar = `"`

// How long ReclaimPort waits for the operating system to release a port after
// the process holding it was killed, and how often it re-checks meanwhile.
const (
	portReleaseTimeout = 5 * time.Second
	portReleasePoll    = 100 * time.Millisecond
)

// listeningPIDs returns the processes holding a TCP listener on port. An empty
// result means nothing listens there, or the platform lookup failed, which
// callers must read the same way: nothing here can be stopped.
func listeningPIDs(port int) []int {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "netstat -ano -p tcp | findstr :"+strconv.Itoa(port))
	case "darwin", "linux":
		cmd = exec.Command("lsof", "-t", "-sTCP:LISTEN", "-i", ":"+strconv.Itoa(port))
	default:
		return nil
	}

	// Both commands exit non-zero when nothing matches, so a failure here is
	// not distinguishable from — and means the same as — an empty result.
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseListeningPIDs(string(output), runtime.GOOS, port)
}

// parseListeningPIDs pulls the PIDs out of netstat or lsof output.
func parseListeningPIDs(output, goos string, port int) []int {
	suffix := ":" + strconv.Itoa(port)
	seen := map[int]bool{}
	pids := []int{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		var raw string
		if goos == "windows" {
			// TCP  0.0.0.0:20080  0.0.0.0:0  LISTENING  4242
			//
			// The local address is what has to match: a line for an outbound
			// connection *to* the port names a process that does not hold it,
			// and killing that one would leave the port right where it was.
			if len(fields) < 5 || !strings.HasSuffix(fields[1], suffix) || !strings.EqualFold(fields[3], "LISTENING") {
				continue
			}
			raw = fields[len(fields)-1]
		} else {
			raw = fields[0]
		}

		pid, err := strconv.Atoi(raw)
		if err != nil || pid == 0 || seen[pid] {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
	}
	return pids
}

// processImageName returns the executable name of a running process, without
// its directory. An empty name is returned when the process is gone or the
// platform lookup fails, which callers must read as "not known to be ours".
func processImageName(pid int) string {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH")
	case "darwin", "linux":
		cmd = exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	default:
		return ""
	}

	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseProcessImageName(string(output), runtime.GOOS)
}

// parseProcessImageName pulls the executable name out of the tasklist or ps
// output for a single process.
func parseProcessImageName(output, goos string) string {
	line := strings.TrimSpace(output)
	if line == "" {
		return ""
	}
	line = strings.TrimSpace(strings.SplitN(line, "\n", 2)[0])
	if goos == "windows" {
		// tasklist prints one CSV record per match, image name first. A PID
		// with no match prints an INFO line carrying no quoted field at all.
		fields := strings.Split(line, quoteChar)
		if len(fields) < 2 {
			return ""
		}
		return fields[1]
	}
	return filepath.Base(line)
}

// isOwnExecutable reports whether pid belongs to another instance of the
// program running right now.
func isOwnExecutable(pid int) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	name := processImageName(pid)
	if name == "" {
		return false
	}
	return strings.EqualFold(name, filepath.Base(self))
}

// isSystemProcess reports whether pid belongs to the operating system rather
// than to a program: PID 0 and Windows' System process on PID 4 both appear as
// the holder of ports reserved by the kernel, and neither can be killed.
func isSystemProcess(pid int) bool {
	if pid <= 1 {
		return true
	}
	return runtime.GOOS == "windows" && pid == 4
}

func describeProcess(pid int) string {
	name := processImageName(pid)
	if name == "" {
		name = "an unidentified program"
	}
	return fmt.Sprintf("%s (pid %d)", name, pid)
}

// StopOldInstance kills a leftover CasOS instance still holding port, which is
// what a restart runs into while the previous process is on its way out.
//
// Only a process running the same executable is killed. The port may just as
// well belong to an unrelated program -- a real etcd on 2379, say -- and
// taking that down to claim the port would destroy data CasOS does not own.
// Callers move aside to another port instead, so a port still occupied when
// this returns is not an error.
func StopOldInstance(port int) error {
	for _, pid := range listeningPIDs(port) {
		if pid == os.Getpid() || isSystemProcess(pid) || !isOwnExecutable(pid) {
			continue
		}

		process, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		if err = process.Kill(); err != nil {
			return err
		}
		fmt.Printf("The old instance with pid: %d has been stopped\n", pid)
	}
	return nil
}

// ReclaimPort frees bind:port by killing whatever listens on it, and returns a
// description of what it stopped -- empty when the port was free already.
//
// It is for the ports CasOS cannot move aside from: the apiserver, the webhook
// server and the web UI each publish their number in a kubeconfig, a webhook
// registration or a bookmark, so a port that is taken used to leave the
// component with nothing to do but fail. These ports sit in the block CasOS
// reserves for itself, which nothing else has business holding -- in practice
// the holder is a previous CasOS that has not finished exiting -- so taking the
// port back is the more useful answer than refusing to start.
//
// An empty bind means every interface.
func ReclaimPort(bind string, port int) (string, error) {
	if PortAvailable(bind, port) {
		return "", nil
	}

	pids := listeningPIDs(port)
	if len(pids) == 0 {
		return "", fmt.Errorf("port %d is in use by a program that could not be identified", port)
	}

	stopped := []string{}
	failed := []string{}
	for _, pid := range pids {
		if pid == os.Getpid() || isSystemProcess(pid) {
			failed = append(failed, describeProcess(pid)+": not a process CasOS may stop")
			continue
		}

		description := describeProcess(pid)
		process, err := os.FindProcess(pid)
		if err == nil {
			err = process.Kill()
		}
		if err != nil {
			// Not fatal on its own: the process may have exited between the
			// lookup and the kill, in which case the port check below passes.
			failed = append(failed, description+": "+err.Error())
			continue
		}
		stopped = append(stopped, description)
	}

	// The listener outlives the kill by a moment, so the port has to be waited
	// for rather than assumed free.
	deadline := time.Now().Add(portReleaseTimeout)
	for {
		if PortAvailable(bind, port) {
			return strings.Join(stopped, ", "), nil
		}
		if time.Now().After(deadline) {
			if len(failed) > 0 {
				return "", fmt.Errorf("port %d is still in use: %s", port, strings.Join(failed, "; "))
			}
			return "", fmt.Errorf("port %d is still in use %s after stopping %s",
				port, portReleaseTimeout, strings.Join(stopped, ", "))
		}
		time.Sleep(portReleasePoll)
	}
}
