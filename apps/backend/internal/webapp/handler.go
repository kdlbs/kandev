package webapp

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

const IndexHTML = "index.html"

type PayloadBuilder func(*http.Request, RouteClassification) BootPayload

type Handler struct {
	assets       fs.FS
	payloadFor   PayloadBuilder
	runtime      RuntimeConfig
	indexPath    string
	cacheControl string
}

type HandlerOption func(*Handler)

func NewHandler(assets fs.FS, opts ...HandlerOption) *Handler {
	h := &Handler{
		assets:       assets,
		runtime:      RuntimeConfig{APIPrefix: "/api/v1", WebSocketPath: "/ws"},
		indexPath:    IndexHTML,
		cacheControl: "public, max-age=31536000, immutable",
	}
	h.payloadFor = func(_ *http.Request, route RouteClassification) BootPayload {
		return NewBootPayload(route, h.runtime, nil)
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func WithRuntimeConfig(runtime RuntimeConfig) HandlerOption {
	return func(h *Handler) {
		h.runtime = runtime
	}
}

func WithPayloadBuilder(builder PayloadBuilder) HandlerOption {
	return func(h *Handler) {
		if builder != nil {
			h.payloadFor = builder
		}
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.assets == nil {
		http.NotFound(w, r)
		return
	}

	if h.serveAsset(w, r) {
		return
	}

	route := ClassifyRoute(r.URL.Path)
	if route.Kind != RouteKindSPA {
		http.NotFound(w, r)
		return
	}

	payload := h.payloadFor(r, route)
	html, err := RenderShell(h.assets, h.indexPath, payload)
	if err != nil {
		http.Error(w, "web app shell unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

func (h *Handler) serveAsset(w http.ResponseWriter, r *http.Request) bool {
	name := assetName(r.URL.Path)
	if name == "" || name == h.indexPath {
		return false
	}

	file, err := h.assets.Open(name)
	if err != nil {
		return false
	}
	defer func() {
		_ = file.Close()
	}()

	stat, err := file.Stat()
	if err != nil || stat.IsDir() {
		return false
	}

	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", h.cacheControl)
	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		data, readErr := io.ReadAll(file)
		if readErr != nil {
			http.Error(w, "read web asset", http.StatusInternalServerError)
			return true
		}
		_, _ = w.Write(data)
		return true
	}
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), seeker)
	return true
}

func assetName(requestPath string) string {
	cleanPath := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	if cleanPath == "/" {
		return ""
	}
	return strings.TrimPrefix(cleanPath, "/")
}
