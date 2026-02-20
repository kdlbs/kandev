package websocket

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

// proxyEntry caches both the reverse proxy and its target URL so we can
// detect when the upstream has changed (e.g. VS Code restarted on a new port).
type proxyEntry struct {
	proxy  *httputil.ReverseProxy
	target string // "host:port"
}

// VscodeProxyHandler reverse-proxies HTTP and WebSocket traffic to code-server
// running inside an agentctl instance.
type VscodeProxyHandler struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger

	// Cache proxies per session to avoid re-creation
	mu      sync.Mutex
	proxies map[string]*proxyEntry
}

// NewVscodeProxyHandler creates a new VS Code proxy handler.
func NewVscodeProxyHandler(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *VscodeProxyHandler {
	return &VscodeProxyHandler{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "vscode-proxy")),
		proxies:      make(map[string]*proxyEntry),
	}
}

// HandleVscodeProxy handles all HTTP/WS requests to /vscode/:sessionId/*path.
// It resolves the session to find the agentctl host and VS Code port,
// then reverse-proxies the request to code-server.
func (h *VscodeProxyHandler) HandleVscodeProxy(c *gin.Context) {
	sessionID := c.Param("sessionId")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}

	proxy, err := h.resolveProxy(c, sessionID)
	if err != nil {
		return // error already written to response
	}

	// Strip the /vscode/:sessionId prefix from the request path
	originalPath := c.Request.URL.Path
	prefix := "/vscode/" + sessionID
	newPath := strings.TrimPrefix(originalPath, prefix)
	if newPath == "" {
		newPath = "/"
	}
	c.Request.URL.Path = newPath
	c.Request.URL.RawPath = ""

	// ReverseProxy panics with http.ErrAbortHandler when the client disconnects
	// mid-stream (e.g. closing the VS Code panel). Recover silently to avoid
	// scary stack traces in the logs from Gin's recovery middleware.
	defer func() {
		if r := recover(); r != nil {
			if r == http.ErrAbortHandler {
				h.logger.Debug("vscode proxy: client disconnected", zap.String("session_id", sessionID))
				return
			}
			panic(r) // re-panic for unexpected errors
		}
	}()

	proxy.ServeHTTP(c.Writer, c.Request)
}

// resolveProxy returns a cached proxy if the target hasn't changed, or creates
// a new one. Only performs the upstream status check when no cache exists or
// when the cached target is stale.
func (h *VscodeProxyHandler) resolveProxy(c *gin.Context, sessionID string) (*httputil.ReverseProxy, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Fast path: return cached proxy if available
	if entry, ok := h.proxies[sessionID]; ok {
		return entry.proxy, nil
	}

	// Slow path: resolve upstream target
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

	vscodeStatus, err := agentctlClient.VscodeStatus(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get vscode status", zap.Error(err), zap.String("session_id", sessionID))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get vscode status"})
		return nil, err
	}

	if vscodeStatus.Status != "running" || vscodeStatus.Port == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "code-server is not running"})
		return nil, fmt.Errorf("code-server not running")
	}

	targetHost := agentctlClient.Host()
	targetAddr := fmt.Sprintf("%s:%d", targetHost, vscodeStatus.Port)
	target := &url.URL{
		Scheme: "http",
		Host:   targetAddr,
	}

	proxy := h.createProxy(sessionID, target)
	h.proxies[sessionID] = &proxyEntry{proxy: proxy, target: targetAddr}
	return proxy, nil
}

// createProxy builds a reverse proxy for the given target URL.
func (h *VscodeProxyHandler) createProxy(sessionID string, target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Allow WebSocket upgrades by preserving hop-by-hop headers
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Preserve WebSocket headers that SingleHostReverseProxy strips
		if req.Header.Get("Upgrade") != "" {
			req.Header.Set("Connection", "Upgrade")
		}
	}

	// Handle WebSocket connections
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Remove hop-by-hop restriction for WebSocket upgrades
		if resp.StatusCode == http.StatusSwitchingProtocols {
			resp.Header.Set("Connection", "Upgrade")
		}
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		h.logger.Error("vscode proxy error",
			zap.String("session_id", sessionID),
			zap.Error(err))
		// Remove cached proxy on error so it gets recreated
		h.InvalidateProxy(sessionID)
		http.Error(w, "VS Code proxy error", http.StatusBadGateway)
	}

	return proxy
}

// InvalidateProxy removes a cached proxy for a session (e.g., when VS Code stops).
func (h *VscodeProxyHandler) InvalidateProxy(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.proxies, sessionID)
}
