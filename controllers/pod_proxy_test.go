package controllers

import "testing"

func TestExtractNSAndName(t *testing.T) {
	cases := []struct {
		path     string
		wantNS   string
		wantName string
		wantRest string
		wantOK   bool
	}{
		{"/p/default/my-pod/", "default", "my-pod", "/", true},
		{"/p/default/my-pod", "default", "my-pod", "", true},
		{"/p/default/my-pod/vnc.html", "default", "my-pod", "/vnc.html", true},
		{"/p/default/my-pod/websockify", "default", "my-pod", "/websockify", true},
		{"/p/kube-system/coredns-abc-xyz/some/deep/path", "kube-system", "coredns-abc-xyz", "/some/deep/path", true},
		{"/p/default/", "", "", "", false},
		{"/p/default", "", "", "", false},
		{"/p/", "", "", "", false},
		{"/p", "", "", "", false},
		{"/api/open-pod-ui", "", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			ns, name, rest, ok := extractNSAndName(c.path)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if ns != c.wantNS {
				t.Errorf("ns = %q, want %q", ns, c.wantNS)
			}
			if name != c.wantName {
				t.Errorf("name = %q, want %q", name, c.wantName)
			}
			if rest != c.wantRest {
				t.Errorf("rest = %q, want %q", rest, c.wantRest)
			}
		})
	}
}
