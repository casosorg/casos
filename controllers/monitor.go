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
