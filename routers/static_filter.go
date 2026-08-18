package routers

import (
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/beego/beego"
	"github.com/beego/beego/context"
	webassets "github.com/casosorg/casos/web"
	web2assets "github.com/casosorg/casos/web2"
)

const indexFile = "index.html"

// staticAssets is the compiled frontend: embedded in the binary for standalone
// builds, read from disk for every other build.
var staticAssets = frontendAssets()

// frontendAssets picks between the two frontends that currently live in the
// repository. web2 is the shadcn rewrite of web and is the one that ships:
// standalone `-tags embed` builds carry it, and every other build serves it
// from web2/build as soon as `yarn build` has run there. web is the fallback
// for a checkout that has only ever built the old UI, and deleting web2/build
// is all it takes to go back to it — no rebuild of the backend, no config.
func frontendAssets() fs.FS {
	if web2assets.Available() {
		return web2assets.Files()
	}
	return webassets.Files()
}

func init() {
	// Windows resolves MIME types through the registry, where .js is routinely
	// registered as text/plain — which browsers refuse to execute as a script.
	// Pin the types the frontend depends on so the UI loads on every platform.
	for extension, contentType := range map[string]string{
		".js":   "text/javascript; charset=utf-8",
		".css":  "text/css; charset=utf-8",
		".json": "application/json",
		".svg":  "image/svg+xml",
	} {
		_ = mime.AddExtensionType(extension, contentType)
	}
}

func StaticFilter(ctx *context.Context) {
	urlPath := ctx.Request.URL.Path
	if strings.HasPrefix(urlPath, "/api/") ||
		strings.HasPrefix(urlPath, "/k8s/") ||
		strings.HasPrefix(urlPath, "/.well-known/") ||
		urlPath == "/k8s" {
		return
	}

	name := strings.TrimPrefix(path.Clean(urlPath), "/")
	if name == "" || name == "." {
		name = indexFile
	}

	file, err := openAsset(name)
	if err != nil {
		// An unknown path without an extension is a React router route that
		// index.html resolves client side. A missing path with an extension is
		// a genuine 404 and is left to beego.
		if path.Ext(name) != "" {
			return
		}
		name = indexFile
		if file, err = openAsset(name); err != nil {
			return
		}
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return
	}
	setCacheControl(ctx.ResponseWriter, name)

	// ServeContent picks the content type from the name, and streams while
	// honouring range and conditional requests whenever the asset can seek —
	// which both the embedded and the on-disk file systems can.
	if seeker, ok := file.(io.ReadSeeker); ok {
		serveAssetContent(ctx, name, info.ModTime(), seeker)
		return
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return
	}
	serveAssetContent(ctx, name, info.ModTime(), bytes.NewReader(content))
}

// serveAssetContent enables Beego's gzip setting for frontend text assets while
// preserving ServeContent's conditional request handling for every other case.
// Range and HEAD requests stay uncompressed because their response semantics
// depend on the original byte representation and length.
func serveAssetContent(ctx *context.Context, name string, modTime time.Time, content io.ReadSeeker) {
	if shouldGzipAsset(ctx.Request, name) {
		ctx.ResponseWriter.Header().Set("Content-Encoding", "gzip")
		ctx.ResponseWriter.Header().Add("Vary", "Accept-Encoding")
		ctx.ResponseWriter.Header().Del("Content-Length")
		gz := gzip.NewWriter(ctx.ResponseWriter)
		http.ServeContent(&gzipResponseWriter{Writer: gz, ResponseWriter: ctx.ResponseWriter}, ctx.Request, name, modTime, content)
		// ServeContent writes no body for conditional responses such as 304;
		// closing gzip in that case would create an invalid response body.
		if ctx.ResponseWriter.Status == http.StatusOK {
			_ = gz.Close()
		}
		return
	}
	http.ServeContent(ctx.ResponseWriter, ctx.Request, name, modTime, content)
}

func shouldGzipAsset(req *http.Request, name string) bool {
	if !beego.BConfig.EnableGzip || req == nil || req.Method != http.MethodGet || req.Header.Get("Range") != "" {
		return false
	}
	extension := strings.ToLower(path.Ext(name))
	if extension != ".js" && extension != ".css" && extension != ".html" {
		return false
	}
	return context.ParseEncoding(req) == "gzip"
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func openAsset(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	return staticAssets.Open(name)
}

// hashedAssetDirs are the directories whose file names carry a content hash, so
// their contents never change under a given name. Create React App emits web/
// into static/ and Vite emits web2/ into assets/; both frontends are served by
// this filter, so both spellings have to be recognised or the new UI ships
// without any caching at all.
var hashedAssetDirs = []string{"static/", "assets/"}

// setCacheControl applies the policy the frontend build implies: content-hashed
// files can be cached forever, while index.html and the remaining top-level
// assets keep their names across releases and must be revalidated or an upgrade
// serves a stale app.
func setCacheControl(writer http.ResponseWriter, name string) {
	for _, dir := range hashedAssetDirs {
		if strings.HasPrefix(name, dir) {
			writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			return
		}
	}
	writer.Header().Set("Cache-Control", "no-cache")
}
