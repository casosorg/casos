package controllers

import (
	"strconv"

	"github.com/casosorg/casos/object"
)

// GetMonitorSummary returns a lightweight observability overview.
// @router /api/monitor/summary [get]
func (c *ApiController) GetMonitorSummary() {
	c.ResponseOk(object.GetMonitorSummary(getAdminRestConfig()))
}

// GetMonitorChecks returns cluster health check results.
// @router /api/monitor/checks [get]
func (c *ApiController) GetMonitorChecks() {
	c.ResponseOk(object.GetMonitorChecks(getAdminRestConfig()))
}

// GetMonitorEvents returns recent Kubernetes events.
// @router /api/monitor/events [get]
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
