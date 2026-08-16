package main

import (
	"context"
	"flag"
	"fmt"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/beego/beego"
	"github.com/beego/beego/logs"
	"github.com/spf13/cobra"
	logsapi "k8s.io/component-base/logs/api/v1"

	"github.com/casosorg/casos/casdoor"
	"github.com/casosorg/casos/conf"
	"github.com/casosorg/casos/controllers"
	"github.com/casosorg/casos/deploy"
	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/proxy"
	"github.com/casosorg/casos/routers"
	"github.com/casosorg/casos/server"
	"github.com/casosorg/casos/util"
)

// Build metadata, set through -ldflags "-X main.version=..." when GoReleaser
// builds a release. A binary built any other way reports the fallbacks, which
// is how a development build tells itself apart from a released one.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func versionString() string {
	return fmt.Sprintf("casos %s (commit %s, built %s, %s %s/%s)",
		version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func main() {
	doubleClicked := util.IsDoubleClicked()
	if doubleClicked {
		// CasOS embeds Cobra-based Kubernetes commands. Cobra's Windows
		// mousetrap would otherwise terminate the whole process after startup.
		cobra.MousetrapHelpText = ""
	}

	// Allow multiple in-process Kubernetes components to reinitialise the global
	// logging singleton without killing the process.
	logsapi.ReapplyHandling = logsapi.ReapplyHandlingIgnoreUnchanged

	// Registered before InitFlag, which is what parses the command line.
	showVersion := flag.Bool("version", false, "print the CasOS version and exit")

	object.InitFlag()

	// Answered before any subsystem starts, so the flag works on a machine with
	// no database, no config file and no writable data directory.
	if *showVersion {
		fmt.Println(versionString())
		return
	}

	object.InitAdapter()
	object.CreateTables()
	object.InitSite()
	if err := object.SeedDefaultPolicies(); err != nil {
		logs.Warning("casbin seed: %v", err)
	}
	if err := object.ReloadAllEnforcers(); err != nil {
		logs.Warning("casbin enforcer init: %v", err)
	}
	casdoor.InitCasdoorConfig()
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
	beego.BConfig.WebConfig.Session.SessionProvider = "file"
	beego.BConfig.WebConfig.Session.SessionProviderConfig = "./tmp"
	beego.BConfig.WebConfig.Session.SessionGCMaxLifetime = 3600 * 24 * 365

	port := conf.GetConfigIntDefault("httpport", 9000)
	url := fmt.Sprintf("http://localhost:%v/", port)
	logs.Info("casos listening on :%d", port)
	if doubleClicked {
		go func() {
			if err := util.WaitForCasOS(url, 30*time.Second); err != nil {
				logs.Warning("CasOS startup: %v", err)
				return
			}
			logs.Info("CasOS started successfully. Open %s in your browser.", url)
			if err := util.ShowStartupNotice(url); err != nil {
				logs.Warning("show startup notice: %v", err)
			}
		}()
	}
	beego.Run(fmt.Sprintf(":%v", port))
}
