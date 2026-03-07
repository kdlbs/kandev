package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// portProxyCache caches reverse proxies per port to avoid re-creation.
type portProxyCache struct {
	mu      sync.Mutex
	proxies map[int]*httputil.ReverseProxy
}

func newPortProxyCache() *portProxyCache {
	return &portProxyCache{proxies: make(map[int]*httputil.ReverseProxy)}
}

func (c *portProxyCache) get(port int) (*httputil.ReverseProxy, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.proxies[port]
	return p, ok
}

func (c *portProxyCache) set(port int, p *httputil.ReverseProxy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.proxies[port] = p
}

// handlePortProxy reverse-proxies HTTP/WebSocket requests to a local port
// inside the executor. Routes /api/v1/port-proxy/:port/*path to
// http://127.0.0.1:{port}/{path}.
func (s *Server) handlePortProxy(c *gin.Context) {
	portStr := c.Param("port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1024 || port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port: must be 1024-65535"})
		return
	}

	proxy, ok := s.portProxies.get(port)
	if !ok {
		proxy = s.createPortProxy(port)
		s.portProxies.set(port, proxy)
	}

	// Strip /api/v1/port-proxy/:port prefix, forward remaining path.
	incomingPath := c.Request.URL.Path
	prefix := "/api/v1/port-proxy/" + portStr
	path := strings.TrimPrefix(incomingPath, prefix)
	if path == "" {
		path = "/"
	}
	c.Request.URL.Path = path
	c.Request.URL.RawPath = ""

	s.logger.Debug("port proxy request",
		zap.Int("port", port),
		zap.String("incoming_path", incomingPath),
		zap.String("forwarded_path", path))

	defer func() {
		if r := recover(); r != nil {
			if r == http.ErrAbortHandler {
				s.logger.Debug("port proxy: client disconnected", zap.Int("port", port))
				return
			}
			panic(r)
		}
	}()

	proxy.ServeHTTP(c.Writer, c.Request)
}

func (s *Server) createPortProxy(port int) *httputil.ReverseProxy {
	target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))

	proxy := &httputil.ReverseProxy{}
	proxy.Rewrite = func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.Out.URL.Path = r.In.URL.Path
		r.Out.URL.RawPath = ""
		// Preserve original Host header for CORS/Origin validation in tunneled apps.
		r.Out.Host = r.In.Host
		if r.Out.Header.Get("Upgrade") != "" {
			r.Out.Header.Set("Connection", "Upgrade")
		}
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.StatusCode == http.StatusSwitchingProtocols {
			resp.Header.Set("Connection", "Upgrade")
		}
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, proxyErr error) {
		s.logger.Warn("port proxy upstream error",
			zap.Int("port", port),
			zap.Error(proxyErr))
		http.Error(w, "port proxy error", http.StatusBadGateway)
	}

	// Flush immediately for SSE/streaming responses
	proxy.FlushInterval = -1

	return proxy
}
