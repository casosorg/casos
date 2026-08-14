package main

import (
	"context"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/beego/beego"
	"github.com/beego/beego/logs"
	logsapi "k8s.io/component-base/logs/api/v1"

	"github.com/casosorg/casos/casdoor"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/controllers"
	"github.com/casosorg/casos/deploy"
	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/proxy"
	"github.com/casosorg/casos/routers"
	"github.com/casosorg/casos/server"
)

func main() {
	// Allow multiple in-process Kubernetes components to reinitialise the global
	// logging singleton without killing the process.
	logsapi.ReapplyHandling = logsapi.ReapplyHandlingIgnoreUnchanged

	object.InitFlag()
	object.InitAdapter()
	object.CreateTables()
	if err := controllers.InitLocalSetup(); err != nil {
		panic(err)
	}
	object.InitSite()
	if err := object.SeedDefaultPolicies(); err != nil {
		logs.Warning("casbin seed: %v", err)
	}
	if err := object.ReloadAllEnforcers(); err != nil {
		logs.Warning("casbin enforcer init: %v", err)
	}
	if err := casdoor.InitCasdoorConfig(); err != nil {
		panic(err)
	}
	proxy.InitHttpClient()

	srvCfg, err := server.ConfigFromAppConf()
	if err != nil {
		panic(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	deploy.Init(ctx, deploy.ConfigFromServerConfig(srvCfg))

	readyCh, err := server.Start(ctx, srvCfg)
	if err != nil {
		panic(err)
	}
	controllers.SetServerConfig(&srvCfg)

	if err := server.StartWebhookServer(srvCfg); err != nil {
		logs.Warning("webhook server: %v", err)
	}

	go func() {
		select {
		case <-readyCh:
			adminCfg := server.AdminRestConfig(srvCfg)
			controllers.SetAdminRestConfig(adminCfg)
			deploy.SetRestConfig(adminCfg)
			logs.Info("apiserver ready — kubectl endpoint: https://127.0.0.1:%d", srvCfg.ApiserverPort)
			if err := server.Bootstrap(ctx, adminCfg, srvCfg); err != nil {
				logs.Warning("bootstrap: %v", err)
			}
			if srvCfg.ServiceLBEnabled {
				if err := server.StartServiceLB(ctx, adminCfg, srvCfg); err != nil {
					logs.Warning("start service load balancer: %v", err)
				}
			}
			if err := server.StartScheduler(ctx, srvCfg); err != nil {
				logs.Warning("start scheduler: %v", err)
			}
			if err := server.StartControllerManager(ctx, srvCfg); err != nil {
				logs.Warning("start controller-manager: %v", err)
			}
		case <-ctx.Done():
		}
	}()

	routers.InitAPI()

	apiserverOrigin := fmt.Sprintf("https://127.0.0.1:%d", srvCfg.ApiserverPort)
	beego.InsertFilter("*", beego.BeforeRouter, routers.CorsFilter)
	beego.InsertFilter("/k8s", beego.BeforeRouter, routers.K8sProxyFilter(apiserverOrigin))
	beego.InsertFilter("/k8s/*", beego.BeforeRouter, routers.K8sProxyFilter(apiserverOrigin))
	beego.InsertFilter("/", beego.BeforeRouter, routers.StaticFilter)
	beego.InsertFilter("/*", beego.BeforeRouter, routers.StaticFilter)
	beego.InsertFilter("/api/*", beego.BeforeRouter, routers.ApiFilter)

	beego.BConfig.CopyRequestBody = true
	beego.BConfig.WebConfig.Session.SessionOn = true
	beego.BConfig.WebConfig.Session.SessionName = "casos_session_id"
	beego.BConfig.WebConfig.Session.SessionProvider = "memory"
	beego.BConfig.WebConfig.Session.SessionGCMaxLifetime = int64(conf.GetConfigIntDefault("sessionLifetime", 3600*24))
	beego.BConfig.WebConfig.Session.SessionCookieSameSite = http.SameSiteLaxMode
	beego.BConfig.WebConfig.Session.SessionDisableHTTPOnly = false

	port := conf.GetConfigIntDefault("httpport", 9000)
	logs.Info("casos listening on :%d", port)
	beego.Run(fmt.Sprintf(":%v", port))
}
