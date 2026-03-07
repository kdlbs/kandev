package websocket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
)

// TunnelInfo describes an active tunnel binding.
type TunnelInfo struct {
	Port       int `json:"port"`
	TunnelPort int `json:"tunnel_port"`
}

// tunnelEntry tracks a running tunnel HTTP server.
type tunnelEntry struct {
	port       int
	tunnelPort int
	server     *http.Server
	ln         net.Listener
	cancel     context.CancelFunc
}

// TunnelManager manages per-session port tunnels that bind dedicated host
// ports and reverse-proxy traffic to agentctl's port-proxy endpoint.
// Unlike the path-based PortProxyHandler, tunnels serve apps at the root
// path so web apps that expect "/" work correctly.
type TunnelManager struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger

	mu      sync.Mutex
	tunnels map[string]*tunnelEntry // key: "sessionId:port"
}

// NewTunnelManager creates a new TunnelManager.
func NewTunnelManager(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *TunnelManager {
	return &TunnelManager{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "port-tunnel")),
		tunnels:      make(map[string]*tunnelEntry),
	}
}

// StartTunnel creates a tunnel for the given session and port.
// If tunnelPort > 0, it binds to that specific port; if 0, the OS picks one.
// Returns the actual tunnel port.
func (m *TunnelManager) StartTunnel(sessionID string, port int, tunnelPort int) (int, error) {
	cacheKey := sessionID + ":" + strconv.Itoa(port)

	m.mu.Lock()
	if entry, ok := m.tunnels[cacheKey]; ok {
		m.mu.Unlock()
		return entry.tunnelPort, nil
	}
	m.mu.Unlock()

	execution, ok := m.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok {
		return 0, fmt.Errorf("session not found or no active execution")
	}

	agentctlClient := execution.GetAgentCtlClient()
	if agentctlClient == nil {
		return 0, fmt.Errorf("agentctl client not available")
	}

	target, err := url.Parse(agentctlClient.BaseURL())
	if err != nil {
		return 0, fmt.Errorf("failed to parse agentctl URL: %w", err)
	}

	bindAddr := fmt.Sprintf(":%d", tunnelPort)
	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		if isAddrInUse(err) {
			return 0, fmt.Errorf("port %d is already in use, choose a different port", tunnelPort)
		}
		return 0, fmt.Errorf("failed to bind tunnel port %d: %w", tunnelPort, err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port

	proxy := m.createTunnelProxy(cacheKey, target, port)
	ctx, cancel := context.WithCancel(context.Background())
	srv := &http.Server{Handler: proxy}

	entry := &tunnelEntry{
		port:       port,
		tunnelPort: actualPort,
		server:     srv,
		ln:         ln,
		cancel:     cancel,
	}

	m.mu.Lock()
	m.tunnels[cacheKey] = entry
	m.mu.Unlock()

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			m.logger.Error("tunnel server error",
				zap.String("session_id", sessionID),
				zap.Int("port", port),
				zap.Int("tunnel_port", actualPort),
				zap.Error(serveErr))
			m.removeTunnel(cacheKey)
		}
	}()

	m.logger.Info("tunnel started",
		zap.String("session_id", sessionID),
		zap.Int("port", port),
		zap.Int("tunnel_port", actualPort),
		zap.String("target", agentctlClient.BaseURL()))

	return actualPort, nil
}

// StopTunnel stops a tunnel for the given session and port.
func (m *TunnelManager) StopTunnel(sessionID string, port int) error {
	cacheKey := sessionID + ":" + strconv.Itoa(port)

	m.mu.Lock()
	entry, ok := m.tunnels[cacheKey]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("no tunnel found for session %s port %d", sessionID, port)
	}
	delete(m.tunnels, cacheKey)
	m.mu.Unlock()

	entry.cancel()
	_ = entry.ln.Close()

	m.logger.Info("tunnel stopped",
		zap.String("session_id", sessionID),
		zap.Int("port", port),
		zap.Int("tunnel_port", entry.tunnelPort))

	return nil
}

// ListTunnels returns all active tunnels for a session.
// Returns []TunnelInfo (satisfies TunnelController interface).
func (m *TunnelManager) ListTunnels(sessionID string) any {
	m.mu.Lock()
	defer m.mu.Unlock()

	prefix := sessionID + ":"
	tunnels := make([]TunnelInfo, 0)
	for key, entry := range m.tunnels {
		if strings.HasPrefix(key, prefix) {
			tunnels = append(tunnels, TunnelInfo{
				Port:       entry.port,
				TunnelPort: entry.tunnelPort,
			})
		}
	}
	return tunnels
}

// InvalidateSession stops all tunnels for a session.
func (m *TunnelManager) InvalidateSession(sessionID string) {
	m.mu.Lock()
	prefix := sessionID + ":"
	var toStop []*tunnelEntry
	for key, entry := range m.tunnels {
		if strings.HasPrefix(key, prefix) {
			toStop = append(toStop, entry)
			delete(m.tunnels, key)
		}
	}
	m.mu.Unlock()

	for _, entry := range toStop {
		entry.cancel()
		_ = entry.ln.Close()
	}

	if len(toStop) > 0 {
		m.logger.Info("invalidated session tunnels",
			zap.String("session_id", sessionID),
			zap.Int("count", len(toStop)))
	}
}

// Shutdown stops all active tunnels.
func (m *TunnelManager) Shutdown() {
	m.mu.Lock()
	entries := make([]*tunnelEntry, 0, len(m.tunnels))
	for _, entry := range m.tunnels {
		entries = append(entries, entry)
	}
	m.tunnels = make(map[string]*tunnelEntry)
	m.mu.Unlock()

	for _, entry := range entries {
		entry.cancel()
		_ = entry.ln.Close()
	}

	if len(entries) > 0 {
		m.logger.Info("shutdown all tunnels", zap.Int("count", len(entries)))
	}
}

func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) {
			return errors.Is(sysErr.Err, syscall.EADDRINUSE)
		}
	}
	return false
}

func (m *TunnelManager) removeTunnel(cacheKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tunnels, cacheKey)
}

func (m *TunnelManager) createTunnelProxy(cacheKey string, target *url.URL, port int) *httputil.ReverseProxy {
	portStr := strconv.Itoa(port)

	proxy := &httputil.ReverseProxy{}
	proxy.Rewrite = func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		// Rewrite: /{path} → /api/v1/port-proxy/{port}/{path}
		incoming := r.In.URL.Path
		if incoming == "" {
			incoming = "/"
		}
		r.Out.URL.Path = "/api/v1/port-proxy/" + portStr + incoming
		r.Out.URL.RawPath = ""
		// Preserve original Host header for CORS/Origin validation.
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

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		m.logger.Error("tunnel proxy error",
			zap.String("cache_key", cacheKey),
			zap.String("request_path", r.URL.Path),
			zap.Error(err))
		http.Error(w, "tunnel proxy error", http.StatusBadGateway)
	}

	// Flush immediately for SSE/streaming responses.
	proxy.FlushInterval = -1

	return proxy
}
