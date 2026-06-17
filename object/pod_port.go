package object

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

func OpenPodUI(cfg *rest.Config, namespace, podName string, containerPort int32) (localPort int, stop func(), err error) {
	if cfg == nil {
		return 0, nil, errors.New("nil rest config")
	}
	if namespace == "" || podName == "" || containerPort <= 0 {
		return 0, nil, errors.New("namespace, pod name and containerPort are required")
	}

	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	if err != nil {
		return 0, nil, fmt.Errorf("spdy roundtripper: %w", err)
	}

	serverURL, err := url.Parse(cfg.Host)
	if err != nil {
		return 0, nil, fmt.Errorf("parse apiserver host %q: %w", cfg.Host, err)
	}
	serverURL.Path = fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName)

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", serverURL)

	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	fw, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"},
		[]string{fmt.Sprintf("0:%d", containerPort)},
		stopChan, readyChan, nil, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("new portforward: %w", err)
	}

	runErr := make(chan error, 1)
	go func() { runErr <- fw.ForwardPorts() }()

	select {
	case <-readyChan:
	case <-runErr:
		return 0, nil, errors.New("portforward exited before ready")
	}

	ports, err := fw.GetPorts()
	if err != nil || len(ports) == 0 {
		close(stopChan)
		<-runErr
		return 0, nil, fmt.Errorf("get ports: %w", err)
	}
	localPort = int(ports[0].Local)

	stop = sync.OnceFunc(func() {

		close(stopChan)
		<-runErr
	})
	return localPort, stop, nil
}

func ContainerPortsFromPod(pod *corev1.Pod) []int32 {
	if pod == nil {
		return nil
	}
	seen := map[int32]bool{}
	var out []int32
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.ContainerPort > 0 && !seen[p.ContainerPort] {
				seen[p.ContainerPort] = true
				out = append(out, p.ContainerPort)
			}
		}
	}

	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
