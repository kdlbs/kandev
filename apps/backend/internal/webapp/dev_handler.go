package webapp

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// DevHandler fronts a Vite dev server with the same Go boot-payload shell used
// in production. Vite remains responsible for module transforms and HMR.
type DevHandler struct {
	viteIndexURL string
	proxy        *httputil.ReverseProxy
	client       *http.Client
	payloadFor   PayloadBuilder
}

func NewDevHandler(viteBaseURL string, opts ...HandlerOption) (*DevHandler, error) {
	target, err := url.Parse(viteBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Vite dev server URL: %w", err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("parse Vite dev server URL: missing scheme or host")
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusBadGateway)
	}

	config := newHandlerDefaults(nil)
	for _, opt := range opts {
		opt(config)
	}

	indexURL := *target
	indexURL.Path = "/"
	indexURL.RawQuery = ""
	indexURL.Fragment = ""

	return &DevHandler{
		viteIndexURL: indexURL.String(),
		proxy:        proxy,
		client:       http.DefaultClient,
		payloadFor:   config.payloadFor,
	}, nil
}

func (h *DevHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		http.NotFound(w, r)
		return
	}

	if shouldProxyToVite(r) {
		h.proxy.ServeHTTP(w, r)
		return
	}

	route := ClassifyRoute(r.URL.Path)
	if route.Kind != RouteKindSPA {
		if route.Kind == RouteKindStatic {
			h.proxy.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}

	if !acceptsHTML(r) {
		h.proxy.ServeHTTP(w, r)
		return
	}

	indexHTML, err := h.fetchViteIndex(r)
	if err != nil {
		http.Error(w, "Vite dev server unavailable", http.StatusBadGateway)
		return
	}
	payload := h.payloadFor(r, route)
	html, err := RenderShellHTML(indexHTML, payload)
	if err != nil {
		http.Error(w, "web app shell unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(html)
}

func (h *DevHandler) fetchViteIndex(r *http.Request) ([]byte, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, h.viteIndexURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("vite index status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func shouldProxyToVite(r *http.Request) bool {
	if isWebSocketUpgrade(r) {
		return true
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return true
	}
	return isViteDevPath(r.URL.Path)
}

func acceptsHTML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return accept == "" || strings.Contains(accept, "text/html")
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func isViteDevPath(requestPath string) bool {
	switch {
	case strings.HasPrefix(requestPath, "/@vite/"),
		strings.HasPrefix(requestPath, "/@id/"),
		strings.HasPrefix(requestPath, "/@react-refresh"),
		strings.HasPrefix(requestPath, "/@fs/"),
		strings.HasPrefix(requestPath, "/__vite"),
		strings.HasPrefix(requestPath, "/src/"),
		strings.HasPrefix(requestPath, "/app/"),
		strings.HasPrefix(requestPath, "/components/"),
		strings.HasPrefix(requestPath, "/hooks/"),
		strings.HasPrefix(requestPath, "/lib/"),
		strings.HasPrefix(requestPath, "/generated/"),
		strings.HasPrefix(requestPath, "/node_modules/"):
		return true
	default:
		return false
	}
}
