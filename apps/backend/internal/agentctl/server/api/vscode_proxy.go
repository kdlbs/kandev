package api

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// handleVscodeProxy reverse-proxies requests to the local code-server process.
// It assumes VS Code is running according to procMgr status and routes all paths
// under /api/v1/vscode/proxy/*path to code-server root.
func (s *Server) handleVscodeProxy(c *gin.Context) {
	info := s.procMgr.VscodeInfo()
	incomingPath := c.Request.URL.Path
	s.logger.Debug("vscode proxy request received",
		zap.String("method", c.Request.Method),
		zap.String("incoming_path", incomingPath),
		zap.String("status", string(info.Status)),
		zap.Int("port", info.Port))
	if info.Status != "running" || info.Port == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "code-server is not running"})
		return
	}

	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", info.Port))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid vscode target"})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Rewrite = func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		// Strip /api/v1/vscode/proxy prefix and forward remaining path.
		path := strings.TrimPrefix(r.Out.URL.Path, "/api/v1/vscode/proxy")
		if path == "" {
			path = "/"
		}
		s.logger.Debug("rewrote vscode proxy path",
			zap.String("incoming_path", r.In.URL.Path),
			zap.String("forwarded_path", path))
		r.Out.URL.Path = path
		r.Out.URL.RawPath = ""
		if r.Out.Header.Get("Upgrade") != "" {
			r.Out.Header.Set("Connection", "Upgrade")
		}
	}

	proxy.ModifyResponse = func(resp *http.Response) error {
		s.logger.Debug("vscode upstream response",
			zap.Int("status", resp.StatusCode),
			zap.String("request_path", resp.Request.URL.Path))
		if resp.StatusCode == http.StatusSwitchingProtocols {
			resp.Header.Set("Connection", "Upgrade")
		}
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, proxyErr error) {
		s.logger.Warn("vscode proxy upstream error",
			zap.Int("port", info.Port),
			zap.Error(proxyErr))
		http.Error(w, "code-server proxy error", http.StatusBadGateway)
	}

	defer func() {
		if r := recover(); r != nil {
			if r == http.ErrAbortHandler {
				s.logger.Debug("vscode proxy: client disconnected")
				return
			}
			panic(r)
		}
	}()

	proxy.ServeHTTP(c.Writer, c.Request)
}
