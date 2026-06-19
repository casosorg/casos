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

	if !isPublicAPI(ctx.Request.URL.Path) && !isSignedIn(ctx) {
		responseError(ctx, "please sign in first")
		return
	}
}

func isPublicAPI(urlPath string) bool {
	return urlPath == "/api/signin" ||
		urlPath == "/api/signout" ||
		urlPath == "/api/get-built-in-site" ||
		urlPath == "/api/get-account"
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
