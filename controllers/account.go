package controllers

import (
	"os"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	"github.com/casosorg/casos/conf"
)

func (c *ApiController) Signin() {
	if conf.IsLocalAuthMode() {
		c.signinWithPassword()
		return
	}

	code := c.Input().Get("code")
	state := c.Input().Get("state")
	if code == "" {
		c.ResponseError("authorization code is required")
		return
	}
	if !c.validateOAuthState(state) {
		c.ResponseError("invalid OAuth state")
		return
	}

	token, err := casdoorsdk.GetOAuthToken(code, state)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	claims, err := casdoorsdk.ParseJwtToken(token.AccessToken)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	claims.AccessToken = token.AccessToken
	if err = c.SessionRegenerateID(); err != nil {
		c.ResponseError("failed to create session")
		return
	}
	c.SetSessionClaims(claims)

	c.ResponseOk(claims)
}

func (c *ApiController) Signout() {
	c.DestroySession()

	c.ResponseOk()
}

func (c *ApiController) GetAccount() {
	if c.RequireSignedIn() {
		return
	}

	claims := c.GetSessionClaims()
	hostname, _ := os.Hostname()

	c.ResponseOk(claims, hostname)
}
