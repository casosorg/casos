package controllers

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/casosorg/casos/object"
)

const forwardTTL = time.Hour

type openPodUIRequest struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	ContainerPort int32  `json:"containerPort"`
}

type openPodUIResponse struct {
	LocalPort int    `json:"localPort"`
	URL       string `json:"url"`
}

type forwardEntry struct {
	stop  func()
	timer *time.Timer
}

var (
	activeForwards   = map[string]*forwardEntry{}
	activeForwardsMu sync.Mutex
)

func trackForward(key string, stop func()) {
	activeForwardsMu.Lock()
	defer activeForwardsMu.Unlock()

	if prev, ok := activeForwards[key]; ok {
		prev.stop()
		if prev.timer != nil {
			prev.timer.Stop()
		}
	}

	entry := &forwardEntry{stop: stop}
	entry.timer = time.AfterFunc(forwardTTL, func() {
		activeForwardsMu.Lock()
		defer activeForwardsMu.Unlock()
		if cur, ok := activeForwards[key]; ok && cur == entry {
			cur.stop()
			delete(activeForwards, key)
		}
	})
	activeForwards[key] = entry
}

func untrackForward(key string) {
	activeForwardsMu.Lock()
	defer activeForwardsMu.Unlock()
	if cur, ok := activeForwards[key]; ok {
		cur.stop()
		if cur.timer != nil {
			cur.timer.Stop()
		}
		delete(activeForwards, key)
	}
}

func (c *ApiController) OpenPodUI() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req openPodUIRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" || req.Name == "" {
		c.ResponseError("namespace and name are required")
		return
	}

	port := req.ContainerPort
	if port <= 0 {
		pod, err := object.GetPod(cfg, req.Namespace, req.Name)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}

		exposed := readExposedPortsAnnotation(pod.Annotations)
		if len(exposed) == 0 && len(pod.Spec.Containers) > 0 {
			exposed, _ = object.LookupExposedPorts(pod.Spec.Containers[0].Image)
		}
		if len(exposed) == 0 {
			c.ResponseError("no exposed ports known for this pod; specify containerPort")
			return
		}
		port = exposed[0]
	}

	localPort, stop, err := object.OpenPodUI(cfg, req.Namespace, req.Name, port)
	if err != nil {
		c.ResponseError("start port-forward: " + err.Error())
		return
	}

	key := fmt.Sprintf("%s/%s:%d", req.Namespace, req.Name, port)
	trackForward(key, stop)

	c.ResponseOk(openPodUIResponse{
		LocalPort: localPort,
		URL:       fmt.Sprintf("http://127.0.0.1:%d", localPort),
	})
}

func (c *ApiController) ClosePodUI() {
	var req openPodUIRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" || req.Name == "" || req.ContainerPort <= 0 {
		c.ResponseError("namespace, name and containerPort are required")
		return
	}
	key := fmt.Sprintf("%s/%s:%d", req.Namespace, req.Name, req.ContainerPort)
	untrackForward(key)
	c.ResponseOk(map[string]string{"status": "closed"})
}
