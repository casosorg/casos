package routers

import (
	"github.com/beego/beego"
	"github.com/beego/beego/context"
)

func ApiFilter(ctx *context.Context) {
	if beego.AppConfig.DefaultBool("isDemoMode", false) && !isAllowedInDemoMode(ctx.Request.Method, ctx.Request.URL.Path) {
		denyRequest(ctx)
		return
	}

	if !isPublicAPI(ctx.Request.Method, ctx.Request.URL.Path) && !isSignedIn(ctx) {
		responseError(ctx, "please sign in first")
	}
}

func isPublicAPI(method, urlPath string) bool {
	if method == "POST" {
		return urlPath == "/api/signin" || urlPath == "/api/signout"
	}
	if method == "GET" {
		return urlPath == "/api/get-built-in-site"
	}
	return false
}

func isSignedIn(ctx *context.Context) bool {
	if ctx.Input.CruSession == nil {
		return false
	}
	return ctx.Input.CruSession.Get("user") != nil
}

func isAllowedInDemoMode(method, urlPath string) bool {
	if method == "POST" {
		return urlPath == "/api/signin" || urlPath == "/api/signout"
	}
	return true
}
