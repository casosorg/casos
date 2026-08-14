package routers

import (
	"net/http"

	"github.com/beego/beego/context"

	"github.com/casosorg/casos/authn"
	"github.com/casosorg/casos/conf"
)

func ApiFilter(ctx *context.Context) {
	if !isPublicAPI(ctx.Request.URL.Path) && !hasValidSession(ctx) {
		denyUnauthenticatedRequest(ctx)
		return
	}
	if conf.IsDemoMode() && !isAllowedInDemoMode(ctx.Request.Method, ctx.Request.URL.Path) {
		denyRequest(ctx)
	}
}

func isPublicAPI(urlPath string) bool {
	switch urlPath {
	case "/api/signin", "/api/signout", "/api/get-account", "/api/get-signin-options", "/api/setup", "/api/e2e/signin", "/api/get-built-in-site":
		return true
	default:
		return false
	}
}

func hasValidSession(ctx *context.Context) bool {
	claims, ok := authn.SessionClaims(ctx.Input.Session("user"))
	if ok && authn.ValidateSessionClaims(claims) {
		return true
	}
	if ctx.Input.CruSession != nil {
		_ = ctx.Input.CruSession.Delete("user")
	}
	return false
}

func denyUnauthenticatedRequest(ctx *context.Context) {
	ctx.Output.SetStatus(http.StatusUnauthorized)
	responseError(ctx, "please sign in first")
}

func isAllowedInDemoMode(method, urlPath string) bool {
	if method == "POST" {
		return urlPath == "/api/signin" || urlPath == "/api/signout"
	}
	return true
}
