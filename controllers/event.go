package controllers

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/casosorg/casos/object"
)

type clusterEventSummary struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Type       string `json:"type"`
	Reason     string `json:"reason"`
	Message    string `json:"message"`
	Kind       string `json:"kind"`
	ObjectName string `json:"objectName"`
	Count      int32  `json:"count"`
	LastSeen   string `json:"lastSeen"`
}

func toClusterEventSummary(event corev1.Event) clusterEventSummary {
	return clusterEventSummary{
		Name:       event.Name,
		Namespace:  event.Namespace,
		Type:       event.Type,
		Reason:     event.Reason,
		Message:    event.Message,
		Kind:       event.InvolvedObject.Kind,
		ObjectName: event.InvolvedObject.Name,
		Count:      event.Count,
		LastSeen:   object.EventTimestamp(event).UTC().Format("2006-01-02 15:04:05"),
	}
}

// GetEvents
// @router /api/get-events [get]
func (c *ApiController) GetEvents() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	namespace := c.GetString("namespace")
	if namespace == "all" {
		namespace = ""
	}
	limit, _ := c.GetInt("limit", 200)

	events, err := object.GetEvents(cfg, namespace, c.GetString("type"), limit)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	result := make([]clusterEventSummary, 0, len(events))
	for _, event := range events {
		result = append(result, toClusterEventSummary(event))
	}
	c.ResponseOk(result)
}
