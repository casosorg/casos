package server

import (
	"context"
	"os"
	"time"

	"github.com/beego/beego"
	"github.com/beego/beego/logs"
)

// How long CasOS spends winding down after a shutdown signal before it exits
// regardless. Long enough for in-flight web requests to finish, short enough
// that a Ctrl+C never leaves the user waiting on a subsystem that is stuck.
const shutdownTimeout = 10 * time.Second

// ShutdownOnSignal ends the process once the first SIGINT or SIGTERM cancels
// ctx.
//
// signal.NotifyContext takes those signals away from the Go runtime: with a
// handler registered, Ctrl+C no longer terminates CasOS by default, it only
// cancels ctx. beego.Run blocks forever and never watches ctx, so without this
// goroutine the web server keeps serving after Ctrl+C and only killing the
// process stops it.
func ShutdownOnSignal(ctx context.Context, stop context.CancelFunc) {
	<-ctx.Done()

	// Hand the signals back to the runtime first, so a second Ctrl+C kills
	// CasOS outright even if the wind-down below hangs.
	stop()
	logs.Info("shutdown signal received — stopping casos")

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Stops the web server accepting new connections and waits for the
		// in-flight ones. Harmless if beego.Run has not started it yet.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := beego.BeeApp.Server.Shutdown(shutdownCtx); err != nil {
			logs.Warning("web server shutdown: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		logs.Warning("shutdown did not finish within %s — exiting anyway", shutdownTimeout)
	}
	os.Exit(0)
}
