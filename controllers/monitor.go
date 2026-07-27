package controllers

import (
	"strconv"

	"github.com/casosorg/casos/object"
)

// GetMonitorOverview returns summary and checks from one cluster snapshot.
// @router /api/get-monitor-overview [get]
func (c *ApiController) GetMonitorOverview() {
	c.ResponseOk(object.GetMonitorOverview(getAdminRestConfig()))
}

// GetMonitorMetrics returns Prometheus-backed instant or range metric data.
// @router /api/get-monitor-metrics [get]
func (c *ApiController) GetMonitorMetrics() {
	query, err := object.ParseMonitorMetricQuery(object.MonitorMetricQueryParams{
		Scope:     c.GetString("scope"),
		Metric:    c.GetString("metric"),
		Namespace: c.GetString("namespace"),
		Name:      c.GetString("name"),
		Start:     c.GetString("start"),
		End:       c.GetString("end"),
		Step:      c.GetString("step"),
	})
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	metrics, err := object.GetMonitorMetrics(c.Ctx.Request.Context(), query)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(metrics)
}

// GetNodeMonitorOverview returns Node metadata, resource trends, and Pods scheduled on the Node.
// @router /api/get-node-monitor-overview [get]
func (c *ApiController) GetNodeMonitorOverview() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	overview, err := object.GetNodeMonitorOverview(c.Ctx.Request.Context(), cfg, c.resourceMonitorQueryParams("node"))
	if err != nil {
		c.ResponseError(err.Error(), object.MonitorStructuredError{Code: object.MonitorErrorInvalidParams, Message: err.Error()})
		return
	}
	c.ResponseOk(overview)
}

// GetPodMonitorOverview returns Pod metadata and Pod/container resource trends.
// @router /api/get-pod-monitor-overview [get]
func (c *ApiController) GetPodMonitorOverview() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	overview, err := object.GetPodMonitorOverview(c.Ctx.Request.Context(), cfg, c.resourceMonitorQueryParams("pod"))
	if err != nil {
		c.ResponseError(err.Error(), object.MonitorStructuredError{Code: object.MonitorErrorInvalidParams, Message: err.Error()})
		return
	}
	c.ResponseOk(overview)
}

// GetWorkloadMonitorOverview returns workload metadata and current-Pods resource trends.
// @router /api/get-workload-monitor-overview [get]
func (c *ApiController) GetWorkloadMonitorOverview() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	params := c.resourceMonitorQueryParams(c.GetString("kind"))
	overview, err := object.GetWorkloadMonitorOverview(c.Ctx.Request.Context(), cfg, params)
	if err != nil {
		c.ResponseError(err.Error(), object.MonitorStructuredError{Code: object.MonitorErrorInvalidParams, Message: err.Error()})
		return
	}
	c.ResponseOk(overview)
}

// GetPVCMonitorOverview returns PVC metadata and kubelet volume stats trends.
// @router /api/get-pvc-monitor-overview [get]
func (c *ApiController) GetPVCMonitorOverview() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	overview, err := object.GetPVCMonitorOverview(c.Ctx.Request.Context(), cfg, c.resourceMonitorQueryParams("pvc"))
	if err != nil {
		c.ResponseError(err.Error(), object.MonitorStructuredError{Code: object.MonitorErrorInvalidParams, Message: err.Error()})
		return
	}
	c.ResponseOk(overview)
}

// GetMonitorTop returns cluster Top N rankings for nodes, pods, or workloads.
// @router /api/get-monitor-top [get]
func (c *ApiController) GetMonitorTop() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	top, err := object.GetMonitorTop(c.Ctx.Request.Context(), cfg, object.MonitorTopQueryParams{
		Resource:  c.GetString("resource"),
		Metric:    c.GetString("metric"),
		Namespace: c.GetString("namespace"),
		Limit:     c.GetString("limit"),
	})
	if err != nil {
		c.ResponseError(err.Error(), object.MonitorStructuredError{Code: object.MonitorErrorInvalidParams, Message: err.Error()})
		return
	}
	c.ResponseOk(top)
}

// GetMonitorResourceInventory returns Kubernetes resources with current Prometheus metrics.
// @router /api/get-monitor-resource-inventory [get]
func (c *ApiController) GetMonitorResourceInventory() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	inventory, err := object.GetMonitorResourceInventory(c.Ctx.Request.Context(), cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(inventory)
}

// GetMonitorResourceEvents returns Kubernetes Events for a single resource.
// @router /api/get-monitor-resource-events [get]
func (c *ApiController) GetMonitorResourceEvents() {
	limit := 100
	if raw := c.GetString("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	events, err := object.GetMonitorResourceEvents(getAdminRestConfig(), c.GetString("kind"), c.GetString("namespace"), c.GetString("name"), limit)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(events)
}

// GetMonitorSummary returns a lightweight observability overview.
// @router /api/get-monitor-summary [get]
func (c *ApiController) GetMonitorSummary() {
	c.ResponseOk(object.GetMonitorSummary(getAdminRestConfig()))
}

// GetMonitorChecks returns cluster health check results.
// @router /api/get-monitor-checks [get]
func (c *ApiController) GetMonitorChecks() {
	c.ResponseOk(object.GetMonitorChecks(getAdminRestConfig()))
}

// GetMonitorEvents returns recent Kubernetes events.
// @router /api/get-monitor-events [get]
func (c *ApiController) GetMonitorEvents() {
	limit := 100
	if raw := c.GetString("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	events, err := object.GetMonitorEvents(getAdminRestConfig(), c.GetString("namespace"), limit)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(events)
}

// GetMonitorIssues returns actionable monitor issues built from the cluster snapshot.
// @router /api/get-monitor-issues [get]
func (c *ApiController) GetMonitorIssues() {
	issues, err := object.GetMonitorIssues(getAdminRestConfig())
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(issues)
}

// GetMonitorDiagnosis returns events, log preview, and AI-ready context for one object.
// @router /api/get-monitor-diagnosis [get]
func (c *ApiController) GetMonitorDiagnosis() {
	tailLines := int64(100)
	if raw := c.GetString("tailLines"); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			tailLines = parsed
		}
	}
	includePrevious := true
	if raw := c.GetString("previous"); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			includePrevious = parsed
		}
	}

	diagnosis, err := object.GetMonitorDiagnosis(
		getAdminRestConfig(),
		c.GetString("kind"),
		c.GetString("namespace"),
		c.GetString("name"),
		c.GetString("container"),
		tailLines,
		includePrevious,
	)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(diagnosis)
}

func (c *ApiController) resourceMonitorQueryParams(kind string) object.ResourceMonitorQueryParams {
	return object.ResourceMonitorQueryParams{
		ResourceKind: kind,
		Namespace:    c.GetString("namespace"),
		Name:         c.GetString("name"),
		Mode:         c.GetString("mode"),
		Start:        c.GetString("start"),
		End:          c.GetString("end"),
		Step:         c.GetString("step"),
		PodLimit:     c.GetString("podLimit"),
		SelectedPods: c.GetString("selectedPods"),
	}
}
