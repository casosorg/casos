package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// Handler returns the unified gateway http.Handler.
//
// Routing rules (evaluated in order):
//
//	/k8s/* -> reverse-proxy to apiserver, /k8s prefix stripped
//	/api/* -> reverse-proxy to beego internal port
//	/*     -> static files from staticDir (React build output)
func Handler(apiserverOrigin, beegoOrigin, staticDir string) http.Handler {
	apiserverProxy := newReverseProxy(apiserverOrigin)
	beegoProxy     := newReverseProxy(beegoOrigin)
	fileServer     := http.FileServer(http.Dir(staticDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch {
		case strings.HasPrefix(r.URL.Path, "/k8s/") || r.URL.Path == "/k8s":
			// Strip /k8s so apiserver receives /api/v1/... paths unchanged.
			r2 := r.Clone(r.Context())
			r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/k8s")
			if r2.URL.Path == "" {
				r2.URL.Path = "/"
			}
			r2.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, "/k8s")
			apiserverProxy.ServeHTTP(w, r2)

		case strings.HasPrefix(r.URL.Path, "/api/"):
			beegoProxy.ServeHTTP(w, r)

		default:
			// For unknown paths without a file extension, serve index.html so
			// the React SPA can handle client-side routing.
			if r.URL.Path != "/" && filepath.Ext(r.URL.Path) == "" {
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				fileServer.ServeHTTP(w, r2)
				return
			}
			fileServer.ServeHTTP(w, r)
		}
	})
}

// Serve starts the gateway on addr (e.g. ":9000").
func Serve(addr, apiserverOrigin, beegoOrigin, staticDir string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: Handler(apiserverOrigin, beegoOrigin, staticDir),
	}
	logrus.Infof("gateway listening on %s", addr)
	return srv.ListenAndServe()
}

func newReverseProxy(origin string) *httputil.ReverseProxy {
	u, err := url.Parse(origin)
	if err != nil {
		logrus.Fatalf("proxy: invalid origin %q: %v", origin, err)
	}
	p := httputil.NewSingleHostReverseProxy(u)
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logrus.Errorf("proxy %s -> %s: %v", r.URL.Path, origin, err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}
	return p
}
