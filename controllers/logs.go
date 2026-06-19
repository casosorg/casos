package controllers

import (
	"strconv"

	"github.com/casosorg/casos/object"
)

// GetAggregatedLogs fetches merged logs from all pods of a deployment.
// Query: namespace, deployment, keyword, tailLines
// @router /api/get-aggregated-logs [get]
func (c *ApiController) GetAggregatedLogs() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	namespace := c.GetString("namespace")
	deployment := c.GetString("deployment")
	keyword := c.GetString("keyword")

	if namespace == "" {
		namespace = "default"
	}
	if deployment == "" {
		c.ResponseError("deployment is required")
		return
	}

	var tailLines int64 = 200
	if v := c.GetString("tailLines"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			tailLines = n
		}
	}

	result, err := object.GetAggregatedLogs(cfg, namespace, deployment, keyword, tailLines)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(result)
}
