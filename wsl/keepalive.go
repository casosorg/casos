package wsl

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	keepAliveRetryDelay = 5 * time.Second
	keepAliveMarker     = "CASOS_KEEPALIVE=1"
)

// keepAliveScript reports that the distro is up and then idles forever. It
// sleeps in a loop because busybox does not accept "sleep infinity".
const keepAliveScript = `
echo ` + keepAliveMarker + `
while true; do sleep 3600; done
`

// KeepAlive holds one session open inside distro until ctx is done.
//
// WSL stops a distro shortly after its last session exits. Services started by
// systemd do not count, the kubelet included, so a WSL worker node shuts itself
// down about a minute after CasOS last talked to it and the cluster is left
// without a node. Keeping a session attached for as long as CasOS runs is what
// keeps the node up, and starting a new one is what brings the distro back
// after something else, such as `wsl --shutdown`, stopped it.
//
// onRestart runs once a session that had to start the distro again is up: the
// distro comes back with a new IP address, so whatever pointed at the old one
// has to be refreshed.
func KeepAlive(ctx context.Context, distro string, log func(string), onRestart func()) {
	if err := Available(); err != nil {
		log(fmt.Sprintf("not holding %s open: %v", displayDistro(distro), err))
		return
	}
	log(fmt.Sprintf("holding a session open inside %s so its worker node stays up", displayDistro(distro)))

	for session := 0; ctx.Err() == nil; session++ {
		ready := func() {}
		if session > 0 {
			ready = func() {
				log(fmt.Sprintf("%s was down and has been started again", displayDistro(distro)))
				if onRestart != nil {
					onRestart()
				}
			}
		}
		started := time.Now()
		err := holdSession(ctx, distro, ready)
		if ctx.Err() != nil {
			return
		}
		log(fmt.Sprintf("the session inside %s ended after %s: %v",
			displayDistro(distro), time.Since(started).Truncate(time.Second), err))

		select {
		case <-ctx.Done():
			return
		case <-time.After(keepAliveRetryDelay):
		}
	}
}

// holdSession runs the idle script inside distro and returns when it stops,
// which only happens when the distro goes down or ctx is canceled. ready is
// called as soon as the script announces itself.
func holdSession(ctx context.Context, distro string, ready func()) error {
	args := append(distroArgs(distro), "-u", "root", "--", "sh", "-s")
	cmd := wslCommand(ctx, args...)
	cmd.Stdin = strings.NewReader(keepAliveScript)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(decodeOutput(scanner.Bytes()), keepAliveMarker) {
				ready()
			}
		}
	}()
	// The pipe only reaches EOF once the session is over, so this waits for the
	// whole session, and it has to finish before Wait closes the pipe.
	wg.Wait()

	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("%w: %s", err, summarize(stderr.String()))
	}
	return fmt.Errorf("the idle command stopped on its own")
}
