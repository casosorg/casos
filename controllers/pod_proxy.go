package controllers

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

func extractNSAndName(path string) (ns, name, rest string, ok bool) {
	rest = strings.TrimPrefix(path, "/p/")
	if rest == path {
		return "", "", "", false
	}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", false
	}
	ns = parts[0]
	name = parts[1]
	if len(parts) == 2 {
		rest = ""
	} else {
		rest = "/" + parts[2]
	}
	return ns, name, rest, true
}

func (c *ApiController) PodProxy() {
	ns, name, rest, ok := extractNSAndName(c.Ctx.Request.URL.Path)
	if !ok {
		http.Error(c.Ctx.ResponseWriter, "bad proxy path: "+c.Ctx.Request.URL.Path, http.StatusNotFound)
		return
	}

	entry, found := portEntryFor(ns, name)
	if !found {
		http.Error(c.Ctx.ResponseWriter, "no active session for "+ns+"/"+name+" — click Open first", http.StatusNotFound)
		return
	}

	if rest == "" {
		rest = "/"
	}

	target := &url.URL{
		Scheme: "http",
		Host:   "127.0.0.1:" + strconv.Itoa(entry.LocalPort),
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.URL.Path = rest
		if req.URL.RawPath != "" {
			req.URL.RawPath = rest
		}
		// WebSocket passthrough: httputil.ReverseProxy strips hop-by-hop
		// headers (including Connection), so we re-assert Connection: Upgrade
		// whenever the client asked for an upgrade. noVNC's /websockify and
		// ttyd's /ws both rely on this.
		if strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
			req.Header.Set("Connection", "Upgrade")
		}
	}
	proxy.ServeHTTP(c.Ctx.ResponseWriter, c.Ctx.Request)
}
