package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	terminalInputFrame  byte = 0
	terminalResizeFrame byte = 1
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type termResizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// wsPipeWriter forwards bytes to the WebSocket client.
type wsPipeWriter struct {
	conn *websocket.Conn
	mu   *sync.Mutex
}

func (w *wsPipeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Binary frame protocol from browser to backend:
//
//	0 — stdin data (rest of frame is the raw bytes)
//	1 — terminal resize (rest of frame is JSON {"cols":N,"rows":N})
func pumpTerminalInput(
	ctx context.Context,
	conn *websocket.Conn,
	stdin *io.PipeWriter,
	resizeCh chan<- termResizeMsg,
	cancel context.CancelFunc,
) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// Closing the WebSocket unblocks ReadMessage when the exec context ends.
			_ = conn.Close()
		case <-done:
		}
	}()

	defer cancel()
	defer close(done)
	defer close(resizeCh)
	defer stdin.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				_ = stdin.CloseWithError(ctx.Err())
			} else {
				_ = stdin.CloseWithError(err)
			}
			return
		}
		if len(msg) == 0 {
			continue
		}

		switch msg[0] {
		case terminalInputFrame:
			if _, err := stdin.Write(msg[1:]); err != nil {
				return
			}
		case terminalResizeFrame:
			var sz termResizeMsg
			if err := json.Unmarshal(msg[1:], &sz); err != nil {
				continue
			}
			select {
			case resizeCh <- sz:
			default:
			}
		}
	}
}

type termSizeQueue struct{ ch <-chan termResizeMsg }

func (q *termSizeQueue) Next() *remotecommand.TerminalSize {
	sz, ok := <-q.ch
	if !ok {
		return nil
	}
	return &remotecommand.TerminalSize{Width: sz.Cols, Height: sz.Rows}
}

func writeTerminalMessage(conn *websocket.Conn, mu *sync.Mutex, msg string) {
	mu.Lock()
	defer mu.Unlock()

	_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n%s\r\n", msg)))
}

// PodTerminal opens an interactive WebSocket terminal into a pod container.
// @router /api/pod-terminal [get]
func (c *ApiController) PodTerminal() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	namespace := c.GetString("namespace")
	name := c.GetString("name")
	container := c.GetString("container")
	if namespace == "" {
		namespace = "default"
	}
	if name == "" {
		c.ResponseError("name is required")
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Ctx.ResponseWriter, c.Ctx.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	writeMu := &sync.Mutex{}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		writeTerminalMessage(conn, writeMu, "k8s client error: "+err.Error())
		return
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"sh", "-c", "if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    false, // TTY mode combines stderr into stdout, so use one output stream.
			TTY:       true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		writeTerminalMessage(conn, writeMu, "exec error: "+err.Error())
		return
	}

	ctx, cancel := context.WithCancel(c.Ctx.Request.Context())
	defer cancel()

	stdinReader, stdinWriter := io.Pipe()
	defer stdinReader.Close()

	resizeCh := make(chan termResizeMsg, 4)
	go pumpTerminalInput(ctx, conn, stdinWriter, resizeCh, cancel)

	writer := &wsPipeWriter{conn: conn, mu: writeMu}
	if err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             stdinReader,
		Stdout:            writer,
		Tty:               true,
		TerminalSizeQueue: &termSizeQueue{ch: resizeCh},
	}); err != nil && ctx.Err() == nil {
		writeTerminalMessage(conn, writeMu, "terminal error: "+err.Error())
	}
}
