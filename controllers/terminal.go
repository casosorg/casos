package controllers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type termResizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// wsPipeWriter forwards bytes to the WebSocket client.
type wsPipeWriter struct{ conn *websocket.Conn }

func (w *wsPipeWriter) Write(p []byte) (int, error) {
	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// wsPipeReader reads stdin from the WebSocket client.
// Binary frame protocol (first byte = message type):
//
//	0 — stdin data (rest of frame is the raw bytes)
//	1 — terminal resize (rest of frame is JSON {"cols":N,"rows":N})
type wsPipeReader struct {
	conn     *websocket.Conn
	buf      []byte
	resizeCh chan termResizeMsg
}

func (r *wsPipeReader) Read(p []byte) (int, error) {
	for {
		if len(r.buf) > 0 {
			n := copy(p, r.buf)
			r.buf = r.buf[n:]
			return n, nil
		}
		_, msg, err := r.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		if len(msg) == 0 {
			continue
		}
		switch msg[0] {
		case 0: // stdin
			data := msg[1:]
			n := copy(p, data)
			if n < len(data) {
				r.buf = data[n:]
			}
			return n, nil
		case 1: // resize
			var sz termResizeMsg
			if json.Unmarshal(msg[1:], &sz) == nil {
				select {
				case r.resizeCh <- sz:
				default:
				}
			}
		}
	}
}

type termSizeQueue struct{ ch chan termResizeMsg }

func (q *termSizeQueue) Next() *remotecommand.TerminalSize {
	sz, ok := <-q.ch
	if !ok {
		return nil
	}
	return &remotecommand.TerminalSize{Width: sz.Cols, Height: sz.Rows}
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

	conn, err := wsUpgrader.Upgrade(c.Ctx.ResponseWriter, c.Ctx.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("k8s client error: "+err.Error()))
		return
	}

	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"sh", "-c", "bash 2>/dev/null || sh"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	resizeCh := make(chan termResizeMsg, 4)
	reader := &wsPipeReader{conn: conn, resizeCh: resizeCh}
	writer := &wsPipeWriter{conn: conn}

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("exec error: "+err.Error()))
		return
	}

	_ = exec.StreamWithContext(c.Ctx.Request.Context(), remotecommand.StreamOptions{
		Stdin:             reader,
		Stdout:            writer,
		Stderr:            writer,
		Tty:               true,
		TerminalSizeQueue: &termSizeQueue{ch: resizeCh},
	})
}
