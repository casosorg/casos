package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/beego/beego"
	"github.com/sirupsen/logrus"

	"github.com/casosorg/casos/controllers"
	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/proxy"
	"github.com/casosorg/casos/routers"
	"github.com/casosorg/casos/server"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Load beego config from conf/app.conf.
	if err := beego.LoadAppConfig("ini", "conf/app.conf"); err != nil {
		logrus.Fatalf("load app.conf: %v", err)
	}

	// Initialize database connection.
	if err := object.InitDB(); err != nil {
		logrus.Fatalf("db init: %v", err)
	}

	// Start control plane (kine + apiserver) in-process.
	srvCfg, err := server.ConfigFromAppConf()
	if err != nil {
		logrus.Fatalf("server config: %v", err)
	}
	readyCh, err := server.Start(ctx, srvCfg)
	if err != nil {
		logrus.Fatalf("control plane start: %v", err)
	}

	// Register beego routes.
	routers.InitAPI()
	beego.BConfig.CopyRequestBody = true
	beego.BConfig.Listen.HTTPPort = mustInt("httpport", 9090)

	// Start beego on its internal port (not the public-facing gateway).
	go func() {
		logrus.Infof("beego listening on :%d", beego.BConfig.Listen.HTTPPort)
		beego.Run()
	}()

	// Start the unified gateway on gatewayPort (default 9000).
	gatewayPort := mustInt("gatewayPort", 9000)
	beegoOrigin := fmt.Sprintf("http://127.0.0.1:%d", beego.BConfig.Listen.HTTPPort)
	apiserverOrigin := fmt.Sprintf("https://127.0.0.1:%d", srvCfg.ApiserverPort)
	go func() {
		addr := fmt.Sprintf(":%d", gatewayPort)
		if err := proxy.Serve(addr, apiserverOrigin, beegoOrigin, "web/build"); err != nil {
			logrus.Fatalf("gateway: %v", err)
		}
	}()

	// Inject admin rest config and start scheduler once apiserver is ready.
	go func() {
		select {
		case <-readyCh:
			controllers.SetAdminRestConfig(server.AdminRestConfig(srvCfg))
			logrus.Infof("apiserver ready — kubectl endpoint: https://127.0.0.1:%d", srvCfg.ApiserverPort)
			logrus.Infof("UI available at http://localhost:%d", gatewayPort)
			if err := server.StartScheduler(ctx, srvCfg); err != nil {
				logrus.Errorf("start scheduler: %v", err)
			}
			if err := server.StartControllerManager(ctx, srvCfg); err != nil {
				logrus.Errorf("start controller-manager: %v", err)
			}
		case <-ctx.Done():
		}
	}()

	<-ctx.Done()
	logrus.Info("shutting down")
}

func mustInt(key string, def int) int {
	v, err := beego.AppConfig.Int(key)
	if err != nil || v == 0 {
		return def
	}
	return v
}

func init() {
	if _, err := os.Stat("conf/app.conf"); os.IsNotExist(err) {
		logrus.Warn("conf/app.conf not found; defaults will be used")
	}
}
