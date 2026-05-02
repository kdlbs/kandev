package websocket

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

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

// portProxyEntry caches a reverse proxy and its target for a session:port pair.
type portProxyEntry struct {
	proxy  *httputil.ReverseProxy
	target string
}

// PortProxyHandler reverse-proxies HTTP and WebSocket traffic to arbitrary
// localhost ports inside a remote executor via agentctl.
type PortProxyHandler struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger

	mu      sync.Mutex
	proxies map[string]*portProxyEntry // key: "sessionId:port"
}

// NewPortProxyHandler creates a new port proxy handler.
func NewPortProxyHandler(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *PortProxyHandler {
	return &PortProxyHandler{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "port-proxy")),
		proxies:      make(map[string]*portProxyEntry),
	}
}

// HandlePortProxy handles all HTTP/WS requests to /port-proxy/:sessionId/:port/*path.
func (h *PortProxyHandler) HandlePortProxy(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	portStr := c.Param("port")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1024 || port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid port: must be 1024-65535"})
		return
	}

	proxy, err := h.resolveProxy(c, sessionID, port)
	if err != nil {
		return // error already written to response
	}

	// Rewrite path: strip /port-proxy/:sessionId/:port prefix,
	// forward as /api/v1/port-proxy/:port/{remainingPath} to agentctl.
	originalPath := c.Request.URL.Path
	prefix := "/port-proxy/" + sessionID + "/" + portStr
	remaining := strings.TrimPrefix(originalPath, prefix)
	if remaining == "" {
		remaining = "/"
	}
	c.Request.URL.Path = "/api/v1/port-proxy/" + portStr + remaining
	c.Request.URL.RawPath = ""

	h.logger.Debug("port proxy forwarding",
		zap.String("session_id", sessionID),
		zap.Int("port", port),
		zap.String("original_path", originalPath),
		zap.String("rewritten_path", c.Request.URL.Path))

	defer func() {
		if r := recover(); r != nil {
			if r == http.ErrAbortHandler {
				h.logger.Debug("port proxy: client disconnected",
					zap.String("session_id", sessionID),
					zap.Int("port", port))
				return
			}
			panic(r)
		}
	}()

	proxy.ServeHTTP(c.Writer, c.Request)
}

func (h *PortProxyHandler) resolveProxy(c *gin.Context, sessionID string, port int) (*httputil.ReverseProxy, error) {
	cacheKey := sessionID + ":" + strconv.Itoa(port)

	h.mu.Lock()
	defer h.mu.Unlock()

	if entry, ok := h.proxies[cacheKey]; ok {
		return entry.proxy, nil
	}

	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found or no active execution"})
		return nil, fmt.Errorf("session not found")
	}

	agentctlClient := execution.GetAgentCtlClient()
	if agentctlClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agentctl client not available"})
		return nil, fmt.Errorf("agentctl client not available")
	}

	baseURL := agentctlClient.BaseURL()
	target, err := url.Parse(baseURL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to resolve agentctl target"})
		return nil, err
	}

	authToken := agentctlClient.AuthToken()
	proxy := h.createProxy(cacheKey, target, authToken)
	h.proxies[cacheKey] = &portProxyEntry{proxy: proxy, target: baseURL}

	h.logger.Info("created port proxy",
		zap.String("session_id", sessionID),
		zap.Int("port", port),
		zap.String("execution_id", execution.ID),
		zap.String("target", baseURL))
	return proxy, nil
}

func (h *PortProxyHandler) createProxy(cacheKey string, target *url.URL, authToken string) *httputil.ReverseProxy {
	proxy := &httputil.ReverseProxy{}
	proxy.Rewrite = func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.Out.URL.Path = r.In.URL.Path
		r.Out.URL.RawPath = ""
		// Inject agentctl auth token
		if authToken != "" {
			r.Out.Header.Set("Authorization", "Bearer "+authToken)
		}
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

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.logger.Error("port proxy error",
			zap.String("cache_key", cacheKey),
			zap.String("request_path", r.URL.Path),
			zap.Error(err))
		h.invalidateProxy(cacheKey)
		http.Error(w, "port proxy error", http.StatusBadGateway)
	}

	// Flush immediately for SSE/streaming responses.
	proxy.FlushInterval = -1

	return proxy
}

func (h *PortProxyHandler) invalidateProxy(cacheKey string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.proxies, cacheKey)
}

// InvalidateSession removes all cached proxies for a session.
func (h *PortProxyHandler) InvalidateSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	prefix := sessionID + ":"
	for key := range h.proxies {
		if strings.HasPrefix(key, prefix) {
			delete(h.proxies, key)
		}
	}
}
