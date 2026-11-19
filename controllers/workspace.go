package controllers

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/beego/beego/logs"
	"github.com/casosorg/casos/object"
)

func (c *ApiController) workspaceOwner() string {
	user := c.GetSessionUser()
	if user != nil && user.Owner != "" {
		subject := user.Id
		if subject == "" {
			subject = user.Name
		}
		if subject != "" {
			identity, _ := json.Marshal([2]string{user.Owner, subject})
			digest := sha256.Sum256(identity)
			return fmt.Sprintf("user:%x", digest)
		}
	}
	c.Ctx.Output.SetStatus(http.StatusUnauthorized)
	c.ResponseError("please sign in first")
	return ""
}

func (c *ApiController) workspaceError(err error) {
	switch {
	case errors.Is(err, object.ErrWorkspaceOwnerRequired):
		c.Ctx.Output.SetStatus(http.StatusUnauthorized)
		c.ResponseError("please sign in first")
	case errors.Is(err, object.ErrWorkspaceNotFound):
		c.Ctx.Output.SetStatus(http.StatusNotFound)
		c.ResponseError("workspace not found")
	case errors.Is(err, object.ErrWorkspaceInvalidInput):
		c.Ctx.Output.SetStatus(http.StatusBadRequest)
		c.ResponseError(err.Error())
	default:
		logs.Error("workspace metadata: %v", err)
		c.Ctx.Output.SetStatus(http.StatusInternalServerError)
		c.ResponseError("failed to load workspace metadata")
	}
}

func (c *ApiController) workspacePage() (int, int, bool) {
	offset, offsetErr := c.GetInt("offset", 0)
	limit, limitErr := c.GetInt("limit", 20)
	if offsetErr != nil || limitErr != nil || offset < 0 || limit < 1 || limit > 100 {
		c.Ctx.Output.SetStatus(http.StatusBadRequest)
		c.ResponseError("offset must be nonnegative and limit between 1 and 100")
		return 0, 0, false
	}
	return offset, limit, true
}

func (c *ApiController) GetAIWorkspaces() {
	owner := c.workspaceOwner()
	if owner == "" {
		return
	}
	offset, limit, ok := c.workspacePage()
	if !ok {
		return
	}
	workspaces, err := object.GetWorkspaces(owner, offset, limit)
	if err != nil {
		c.workspaceError(err)
		return
	}
	c.ResponseOk(workspaces)
}

func (c *ApiController) GetAIWorkspace() {
	owner := c.workspaceOwner()
	if owner == "" {
		return
	}
	workspace, err := object.GetWorkspace(owner, c.GetString("id"))
	if err != nil {
		c.workspaceError(err)
		return
	}
	c.ResponseOk(workspace)
}

func (c *ApiController) GetAIWorkspaceSessions() {
	owner := c.workspaceOwner()
	if owner == "" {
		return
	}
	offset, limit, ok := c.workspacePage()
	if !ok {
		return
	}
	sessions, err := object.GetWorkspaceSessions(owner, c.GetString("workspaceId"), offset, limit)
	if err != nil {
		c.workspaceError(err)
		return
	}
	c.ResponseOk(sessions)
}
