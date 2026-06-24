package controllers

import (
	"encoding/json"
	"strconv"

	"github.com/casosorg/casos/deploy"
)

// PreflightMachineNode checks whether a machine can be used as a worker node.
// @router /api/preflight-machine-node [post]
func (c *ApiController) PreflightMachineNode() {
	if c.RequireAdmin() {
		return
	}
	var req deploy.MachineNodeDeployRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	result, err := deploy.DefaultService().PreflightMachineNode(req)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(result)
}

// DeployMachineNode starts an async worker node deployment task.
// @router /api/deploy-machine-node [post]
func (c *ApiController) DeployMachineNode() {
	if c.RequireAdmin() {
		return
	}
	var req deploy.MachineNodeDeployRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	task, err := deploy.DefaultService().DeployMachineNode(req)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(task)
}

// RepairMachineNode reruns the same idempotent worker node deployment flow.
// @router /api/repair-machine-node [post]
func (c *ApiController) RepairMachineNode() {
	if c.RequireAdmin() {
		return
	}
	var req deploy.MachineNodeDeployRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	task, err := deploy.DefaultService().RepairMachineNode(req)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(task)
}

// GetMachineNodeTasks returns node deployment tasks for a machine.
// @router /api/get-machine-node-tasks [get]
func (c *ApiController) GetMachineNodeTasks() {
	if c.RequireAdmin() {
		return
	}
	tasks, err := deploy.DefaultService().GetMachineNodeTasks(c.GetString("owner"), c.GetString("machineName"))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(tasks)
}

// GetMachineNodeLogs returns deployment logs for one task.
// @router /api/get-machine-node-logs [get]
func (c *ApiController) GetMachineNodeLogs() {
	if c.RequireAdmin() {
		return
	}
	taskId, err := strconv.ParseInt(c.GetString("taskId"), 10, 64)
	if err != nil || taskId <= 0 {
		c.ResponseError("invalid taskId")
		return
	}
	logs, err := deploy.DefaultService().GetMachineNodeLogs(taskId)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(logs)
}
