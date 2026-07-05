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
