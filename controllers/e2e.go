package controllers

import (
	"github.com/beego/beego/logs"
	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"

	"github.com/casosorg/casos/conf"
)

const e2eTokenHeader = "X-Casos-E2E-Token"

func (c *ApiController) E2ESignin() {
	if !conf.GetConfigBool("e2eTestMode") {
		c.ResponseError("E2E test mode is disabled")
		return
	}

	token := conf.GetConfigString("e2eTestToken")
	if token == "" {
		c.ResponseError("E2E test token is not configured")
		return
	}
	if c.Ctx.Input.Header(e2eTokenHeader) != token {
		c.ResponseError("invalid E2E token")
		return
	}

	claims := &casdoorsdk.Claims{
		User: casdoorsdk.User{
			Owner:       "built-in",
			Name:        "ci-user",
			DisplayName: "CI User",
			IsAdmin:     true,
		},
	}
	c.SetSessionClaims(claims)
	logs.Info("E2E test sign-in used for user %s", claims.Name)

	c.ResponseOk(claims.User)
}
