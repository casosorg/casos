package controllers

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/casosorg/casos/object"
)

const (
	protocolHTTP     = "http"
	protocolVNC      = "vnc"
	protocolTerminal = "terminal"
	forwardTTL       = time.Hour
)

type openPodRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type openPodResponse struct {
	LocalPort int    `json:"localPort"`
	URL       string `json:"url"`
	UIMode    string `json:"uiMode"`
}

type portEntry struct {
	LocalPort int
	Protocol  string
	Stop      func()
	Timer     *time.Timer
}

var (
	activeForwards   = map[string]*portEntry{}
	activeForwardsMu sync.Mutex
)

// portEntryFor returns the active port-forward entry for a pod, if any.
// Exposed to pod_proxy.go for reverse-proxy dispatch.
func portEntryFor(ns, name string) (portEntry, bool) {
	activeForwardsMu.Lock()
	defer activeForwardsMu.Unlock()
	entry, ok := activeForwards[ns+"/"+name]
	if !ok {
		return portEntry{}, false
	}
	return *entry, true
}

func trackForward(key string, entry *portEntry) {
	activeForwardsMu.Lock()
	defer activeForwardsMu.Unlock()

	if prev, ok := activeForwards[key]; ok {
		prev.Stop()
		if prev.Timer != nil {
			prev.Timer.Stop()
		}
	}
	entry.Timer = time.AfterFunc(forwardTTL, func() {
		activeForwardsMu.Lock()
		defer activeForwardsMu.Unlock()
		if cur, ok := activeForwards[key]; ok && cur == entry {
			cur.Stop()
			delete(activeForwards, key)
		}
	})
	activeForwards[key] = entry
}

func untrackForward(key string) {
	activeForwardsMu.Lock()
	defer activeForwardsMu.Unlock()
	if cur, ok := activeForwards[key]; ok {
		cur.Stop()
		if cur.Timer != nil {
			cur.Timer.Stop()
		}
		delete(activeForwards, key)
	}
}

// pickPortAndProtocol decides which container port to forward to and what
// protocol the reverse proxy should treat it as, given the pod's UIMode
// and declared ports.
func pickPortAndProtocol(uiMode string, ports []int32) (int32, string, error) {
	switch uiMode {
	case "terminal":
		return ttydSidecarPort, protocolTerminal, nil
	case "vnc":
		for _, p := range ports {
			if p == 5800 || p == 5900 {
				return p, protocolVNC, nil
			}
		}
		// Fall back to the first port; treat as VNC.
		if len(ports) > 0 {
			return ports[0], protocolVNC, nil
		}
	case "web":
		if len(ports) > 0 {
			return ports[0], protocolHTTP, nil
		}
	}
	return 0, "", fmt.Errorf("no port available for uiMode %q", uiMode)
}

func (c *ApiController) OpenPod() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req openPodRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" || req.Name == "" {
		c.ResponseError("namespace and name are required")
		return
	}

	pod, err := object.GetPod(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	summary := toPodSummary(*pod)
	port, protocol, err := pickPortAndProtocol(summary.UIMode, summary.ContainerPorts)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	localPort, stop, err := object.OpenPodUI(cfg, req.Namespace, req.Name, port)
	if err != nil {
		c.ResponseError("start port-forward: " + err.Error())
		return
	}

	key := req.Namespace + "/" + req.Name
	trackForward(key, &portEntry{LocalPort: localPort, Protocol: protocol, Stop: stop})

	c.ResponseOk(openPodResponse{
		LocalPort: localPort,
		URL:       fmt.Sprintf("/p/%s/%s/", req.Namespace, req.Name),
		UIMode:    summary.UIMode,
	})
}

func (c *ApiController) ClosePod() {
	var req openPodRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" || req.Name == "" {
		c.ResponseError("namespace and name are required")
		return
	}
	key := req.Namespace + "/" + req.Name
	untrackForward(key)
	c.ResponseOk(map[string]string{"status": "closed"})
}
