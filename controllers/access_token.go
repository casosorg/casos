package controllers

import (
	"encoding/json"

	"github.com/casosorg/casos/object"
)

type accessTokenRequest struct {
	Name     string `json:"name"`
	ReadOnly bool   `json:"readOnly"`
}

type addAccessTokenResult struct {
	*object.AccessToken
	Secret string `json:"secret"`
}

// @router /api/get-access-tokens [get]
func (c *ApiController) GetAccessTokens() {
	user := c.GetSessionUser()
	if user == nil {
		c.ResponseError("please sign in first")
		return
	}
	tokens, err := object.GetAccessTokens(user.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(tokens)
}

// AddAccessToken answers with the token's secret, the only time it is ever
// readable.
// @router /api/add-access-token [post]
func (c *ApiController) AddAccessToken() {
	user := c.GetSessionUser()
	if user == nil {
		c.ResponseError("please sign in first")
		return
	}
	var req accessTokenRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	token, secret, err := object.AddAccessToken(user.Name, req.Name, req.ReadOnly)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(addAccessTokenResult{AccessToken: token, Secret: secret})
}

// @router /api/delete-access-token [post]
func (c *ApiController) DeleteAccessToken() {
	user := c.GetSessionUser()
	if user == nil {
		c.ResponseError("please sign in first")
		return
	}
	var req accessTokenRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	deleted, err := object.DeleteAccessToken(user.Name, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !deleted {
		c.ResponseError("no token named " + req.Name)
		return
	}
	c.ResponseOk()
}
