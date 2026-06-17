package controllers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodUIModeTerminal(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"casos.io/ttyd-injected": "true"},
		},
	}
	if got := podUIMode(pod, []int32{7681}); got != "terminal" {
		t.Errorf("want terminal, got %q", got)
	}
}

func TestPodUIModeVNC(t *testing.T) {
	pod := &corev1.Pod{}
	if got := podUIMode(pod, []int32{7681, 5800}); got != "vnc" {
		t.Errorf("want vnc (5800 present), got %q", got)
	}
	if got := podUIMode(pod, []int32{5900}); got != "vnc" {
		t.Errorf("want vnc (5900 present), got %q", got)
	}
}

func TestPodUIModeWeb(t *testing.T) {
	pod := &corev1.Pod{}
	if got := podUIMode(pod, []int32{80}); got != "web" {
		t.Errorf("want web, got %q", got)
	}
}

func TestPodUIModeUnknown(t *testing.T) {
	pod := &corev1.Pod{}
	if got := podUIMode(pod, nil); got != "unknown" {
		t.Errorf("want unknown, got %q", got)
	}
}

func TestPickPortAndProtocol(t *testing.T) {
	cases := []struct {
		name       string
		uiMode     string
		ports      []int32
		wantPort   int32
		wantProto  string
		wantErr    bool
	}{
		{"terminal returns sidecar port", "terminal", nil, ttydSidecarPort, protocolTerminal, false},
		{"vnc prefers 5800", "vnc", []int32{7681, 5800, 5900}, 5800, protocolVNC, false},
		{"vnc falls back to 5900", "vnc", []int32{5900}, 5900, protocolVNC, false},
		{"vnc empty ports errors", "vnc", nil, 0, "", true},
		{"web returns first port", "web", []int32{80, 443}, 80, protocolHTTP, false},
		{"web empty errors", "web", nil, 0, "", true},
		{"unknown errors", "unknown", []int32{80}, 0, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			port, proto, err := pickPortAndProtocol(c.uiMode, c.ports)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, c.wantErr)
			}
			if err != nil {
				return
			}
			if port != c.wantPort {
				t.Errorf("port = %d, want %d", port, c.wantPort)
			}
			if proto != c.wantProto {
				t.Errorf("proto = %q, want %q", proto, c.wantProto)
			}
		})
	}
}

func TestPortEntryForMissing(t *testing.T) {
	if _, ok := portEntryFor("__nope__", "__nope__"); ok {
		t.Fatal("expected miss for unknown pod")
	}
}

func TestTTYDContainerDefaults(t *testing.T) {
	c := ttydSidecarContainer()
	if c.Name != "casos-ttyd" {
		t.Errorf("name = %q", c.Name)
	}
	if c.Image != ttydSidecarImage {
		t.Errorf("image = %q", c.Image)
	}
	if len(c.Ports) != 1 || c.Ports[0].ContainerPort != ttydSidecarPort {
		t.Errorf("ports = %+v, want one entry with %d", c.Ports, ttydSidecarPort)
	}
	if len(c.Args) == 0 {
		t.Error("ttyd args empty")
	}
}
