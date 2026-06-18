package controllers

import (
	"encoding/json"
	"strconv"

	"github.com/casosorg/casos/object"
)

// GetCasbinRules godoc
// @router /api/get-casbin-rules [get]
func (c *ApiController) GetCasbinRules() {
	scope := c.GetString("scope")
	if scope == "" {
		c.ResponseError("scope is required (admission or authorization)")
		return
	}
	rules, err := object.GetCasbinRules(scope)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(rules)
}

// AddCasbinRule godoc
// @router /api/add-casbin-rule [post]
func (c *ApiController) AddCasbinRule() {
	var rule object.CasbinRule
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &rule); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if rule.Scope != object.ScopeAdmission && rule.Scope != object.ScopeAuthorization {
		c.ResponseError("scope must be admission or authorization")
		return
	}
	if rule.PType == "" || rule.V0 == "" {
		c.ResponseError("pType and v0 are required")
		return
	}
	if err := object.AddCasbinRule(&rule); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := object.ReloadEnforcer(rule.Scope); err != nil {
		c.ResponseError("rule saved but enforcer reload failed: " + err.Error())
		return
	}
	c.ResponseOk()
}

// DeleteCasbinRule godoc
// @router /api/delete-casbin-rule [post]
func (c *ApiController) DeleteCasbinRule() {
	var body struct {
		Id    string `json:"id"`
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &body); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	id, err := strconv.ParseInt(body.Id, 10, 64)
	if err != nil {
		c.ResponseError("invalid id")
		return
	}
	if err := object.DeleteCasbinRule(id); err != nil {
		c.ResponseError(err.Error())
		return
	}
	if body.Scope != "" {
		if err := object.ReloadEnforcer(body.Scope); err != nil {
			c.ResponseError("rule deleted but enforcer reload failed: " + err.Error())
			return
		}
	}
	c.ResponseOk()
}

// ReloadCasbinEnforcer godoc
// @router /api/reload-casbin-enforcer [post]
func (c *ApiController) ReloadCasbinEnforcer() {
	var body struct {
		Scope string `json:"scope"`
	}
	_ = json.Unmarshal(c.Ctx.Input.RequestBody, &body)
	var err error
	if body.Scope == "" {
		err = object.ReloadAllEnforcers()
	} else {
		err = object.ReloadEnforcer(body.Scope)
	}
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}
