package store

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	helmDiagnosticsLogTailLines  = 12
	helmDiagnosticsLogLineLen    = 240
	helmDiagnosticsLogMaxPods    = 3
	helmDiagnosticsLogReadBudget = 32 * 1024
)

// appendContainerLogDiagnostics adds the tail of the log of every container that
// died, because that is the one place the actual cause is written down. Without
// it a failed install reports CrashLoopBackOff and the operator has to go and
// find the pod themselves — and on a failed install the pod is usually rolled
// back out from under them before they can.
func appendContainerLogDiagnostics(ctx context.Context, client kubernetes.Interface, lines []string, namespace string, pods []corev1.Pod) []string {
	reported := 0
	for _, pod := range pods {
		if reported >= helmDiagnosticsLogMaxPods {
			break
		}
		for _, status := range append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...) {
			previous, ok := failedContainerLogSource(status)
			if !ok {
				continue
			}
			tail, err := containerLogTail(ctx, client, namespace, pod.Name, status.Name, previous)
			if err != nil {
				// Saying why the log could not be read is worth as much as the
				// log: a reader who is told the apiserver cannot reach the
				// kubelet knows to go and look at the node, whereas silence
				// here leaves "CrashLoopBackOff" as the whole explanation.
				lines = append(lines, fmt.Sprintf("    could not read the log of container %s in %s: %s",
					status.Name, pod.Name, oneLineDiagnosticText(err.Error(), helmDiagnosticsLogLineLen)))
				reported++
				break
			}
			if tail == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("    last log lines of container %s in %s:", status.Name, pod.Name))
			for _, line := range strings.Split(tail, "\n") {
				lines = append(lines, "      "+oneLineDiagnosticText(line, helmDiagnosticsLogLineLen))
			}
			reported++
			break
		}
	}
	return lines
}

// failedContainerLogSource reports whether a container has a log worth showing,
// and whether it lives in the previous instance rather than the running one. A
// container waiting in CrashLoopBackOff has already been replaced, so its
// evidence is in the terminated instance behind it.
func failedContainerLogSource(status corev1.ContainerStatus) (previous bool, ok bool) {
	if waiting := status.State.Waiting; waiting != nil {
		switch waiting.Reason {
		case "CrashLoopBackOff", "RunContainerError", "CreateContainerConfigError":
			return status.RestartCount > 0, true
		}
	}
	if terminated := status.State.Terminated; terminated != nil && terminated.ExitCode != 0 {
		return false, true
	}
	return false, false
}

func containerLogTail(ctx context.Context, client kubernetes.Interface, namespace, podName, containerName string, previous bool) (string, error) {
	tailLines := int64(helmDiagnosticsLogTailLines)
	limit := int64(helmDiagnosticsLogReadBudget)
	request := client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container:  containerName,
		Previous:   previous,
		TailLines:  &tailLines,
		LimitBytes: &limit,
	})
	stream, err := request.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	var collected []string
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		collected = append(collected, line)
		if len(collected) > helmDiagnosticsLogTailLines {
			collected = collected[1:]
		}
	}
	if err := scanner.Err(); err != nil && len(collected) == 0 {
		return "", err
	}
	return strings.Join(collected, "\n"), nil
}
