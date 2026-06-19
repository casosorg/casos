package object

import (
	"context"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

type LogLine struct {
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Text      string `json:"text"`
}

type AggregatedLogs struct {
	Lines []LogLine `json:"lines"`
	Pods  []string  `json:"pods"`
}

// GetAggregatedLogs fetches logs from all pods of a deployment and merges them.
// Lines are optionally filtered by keyword (case-insensitive).
func GetAggregatedLogs(cfg *rest.Config, namespace, deployment, keyword string, tailLines int64) (*AggregatedLogs, error) {
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}

	// Resolve pod label selector from the deployment.
	dep, err := client.AppsV1().Deployments(namespace).Get(context.Background(), deployment, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	selector := dep.Spec.Selector
	labelSelector := metav1.FormatLabelSelector(selector)

	pods, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	type podResult struct {
		podName   string
		container string
		lines     []string
	}

	var (
		mu      sync.Mutex
		results []podResult
		wg      sync.WaitGroup
	)

	for _, pod := range pods.Items {
		for _, ctr := range pod.Spec.Containers {
			wg.Add(1)
			podName := pod.Name
			ctrName := ctr.Name
			go func() {
				defer wg.Done()
				text, ferr := GetPodLogs(cfg, namespace, podName, ctrName, tailLines)
				if ferr != nil {
					return
				}
				lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
				mu.Lock()
				results = append(results, podResult{podName: podName, container: ctrName, lines: lines})
				mu.Unlock()
			}()
		}
	}
	wg.Wait()

	kwLower := strings.ToLower(keyword)
	var allLines []LogLine
	podSet := map[string]struct{}{}

	for _, r := range results {
		podSet[r.podName] = struct{}{}
		for _, line := range r.lines {
			if line == "" {
				continue
			}
			if kwLower != "" && !strings.Contains(strings.ToLower(line), kwLower) {
				continue
			}
			allLines = append(allLines, LogLine{
				Pod:       r.podName,
				Container: r.container,
				Text:      line,
			})
		}
	}

	podNames := make([]string, 0, len(podSet))
	for p := range podSet {
		podNames = append(podNames, p)
	}

	if allLines == nil {
		allLines = []LogLine{}
	}
	return &AggregatedLogs{Lines: allLines, Pods: podNames}, nil
}
