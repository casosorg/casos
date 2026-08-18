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

func init() {
	// CasOS runs kube-scheduler and kube-controller-manager in-process, and both
	// are cobra commands (see server.StartScheduler and
	// server.StartControllerManager). On Windows, cobra assumes a cobra command
	// launched from Explorer is a CLI the user double-clicked by mistake, and
	// answers by printing a notice and calling os.Exit(1) — which takes the whole
	// of CasOS down with it, seconds after an apparently successful startup, and
	// only ever when started by double-click.
	//
	// CasOS is a server that merely embeds those commands, so that guard is never
	// wanted here. Clearing the text disables it, and clearing it unconditionally
	// means no second launch detector has to agree with cobra's for CasOS to
	// survive.
	cobra.MousetrapHelpText = ""
}

func main() {
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
	object.InitUsers()
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
			// Last, because it deploys a worker node and therefore needs the
			// scheduler, the controller-manager and the cluster networking that
			// Bootstrap installs to be running already.
			deploy.StartLocalNodeBootstrap()
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
	beego.BConfig.EnableGzip = true
	beego.BConfig.WebConfig.Session.SessionOn = true
	beego.BConfig.WebConfig.Session.SessionProvider = "file"
	beego.BConfig.WebConfig.Session.SessionProviderConfig = "./tmp"
	beego.BConfig.WebConfig.Session.SessionGCMaxLifetime = 3600 * 24 * 365

	port := conf.GetConfigIntDefault("httpport", 9000)
	logs.Info("casos listening on :%d", port)
	if util.StartedByDoubleClick() {
		go openWhenReady(port)
	}
	beego.Run(fmt.Sprintf(":%v", port))
}

// How long openWhenReady waits before telling the user CasOS did not come up.
// The web server binds within milliseconds of this timer starting; the
// allowance is for a machine still busy initialising the control plane, not for
// the listener itself.
const startupTimeout = 60 * time.Second

// openWhenReady opens the CasOS web UI once the server answers, and reports the
// failure in a dialog if it never does.
//
// It runs only for a CasOS started by double-clicking casos.exe. That user has
// no terminal to read a URL out of and no reason to expect one, and the console
// window they do get is buried under Kubernetes logs within seconds — so both
// the address and any startup failure have to reach them some other way.
func openWhenReady(port int) {
	webURL := fmt.Sprintf("http://localhost:%d/", port)

	if err := util.WaitForServer(webURL, startupTimeout); err != nil {
		logs.Warning("startup: %v", err)
		util.ReportStartupFailure(fmt.Sprintf(
			"CasOS did not finish starting within %s.\n\n"+
				"The CasOS console window shows what went wrong. The most common "+
				"cause is port %d already being in use by another program — change "+
				"httpport in conf/app.conf to use a different one.",
			startupTimeout, port))
		return
	}

	logs.Info("casos ready — opening %s", webURL)
	if err := util.OpenBrowser(webURL); err != nil {
		logs.Warning("open browser: %v", err)
	}
}
