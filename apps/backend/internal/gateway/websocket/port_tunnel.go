package websocket

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/netutil"
)

// TunnelInfo describes an active tunnel binding.
type TunnelInfo struct {
	Port       int `json:"port"`
	TunnelPort int `json:"tunnel_port"`
}

// tunnelEntry tracks a running tunnel HTTP server. Entries reach m.tunnels
// only once they are complete, so every reader can dereference them.
type tunnelEntry struct {
	port       int
	tunnelPort int
	server     *http.Server
	ln         net.Listener
	cancel     context.CancelFunc
}

// pendingTunnel is a StartTunnel that is still resolving and binding. It is
// kept out of m.tunnels so that a concurrent caller waits for the in-flight
// bind and shares its outcome instead of observing a half-built entry, and so
// that StopTunnel, ListTunnels, InvalidateSession and Shutdown never see one.
// tunnelPort and err are written before done is closed and read only after.
type pendingTunnel struct {
	done       chan struct{}
	tunnelPort int
	err        error
}

// errStartCanceled reports a start that was abandoned mid-bind because
// InvalidateSession or Shutdown dropped its reservation.
var errStartCanceled = errors.New("tunnel start canceled by session invalidation or shutdown")

// errManagerClosed reports a start refused because Shutdown already ran.
// Shutdown is terminal: nothing may bind a listener it will never close.
var errManagerClosed = errors.New("tunnel manager is shut down")

// TunnelManager manages per-session port tunnels that bind dedicated host
// ports and reverse-proxy traffic to agentctl's port-proxy endpoint.
// Unlike the path-based PortProxyHandler, tunnels serve apps at the root
// path so web apps that expect "/" work correctly.
type TunnelManager struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger

	mu      sync.Mutex
	closed  bool                      // set by Shutdown; refuses every later start
	tunnels map[string]*tunnelEntry   // key: "sessionId:port"; complete entries only
	pending map[string]*pendingTunnel // key: "sessionId:port"; starts still in flight
}

// NewTunnelManager creates a new TunnelManager.
func NewTunnelManager(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *TunnelManager {
	return &TunnelManager{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "port-tunnel")),
		tunnels:      make(map[string]*tunnelEntry),
		pending:      make(map[string]*pendingTunnel),
	}
}

// StartTunnel creates a tunnel for the given session and port.
// If tunnelPort > 0, it binds to that specific port; if 0, the OS picks one.
// Returns the actual tunnel port.
func (m *TunnelManager) StartTunnel(sessionID string, port int, tunnelPort int) (int, error) {
	cacheKey := sessionID + ":" + strconv.Itoa(port)

	started, existingPort, err := m.claimStart(cacheKey)
	if started == nil {
		// Another caller already owns this session:port, either as a running
		// tunnel or as an in-flight start we just waited for.
		return existingPort, err
	}

	target, authToken, ln, err := m.resolveAndBind(sessionID, tunnelPort)
	if err != nil {
		return m.finishStart(cacheKey, started, 0, err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port

	proxy := m.createTunnelProxy(cacheKey, target, port, authToken)
	ctx, cancel := context.WithCancel(context.Background())
	srv := &http.Server{Handler: proxy}

	entry := &tunnelEntry{
		port:       port,
		tunnelPort: actualPort,
		server:     srv,
		ln:         ln,
		cancel:     cancel,
	}

	if !m.publish(cacheKey, started, entry) {
		cancel()
		_ = ln.Close()
		m.logger.Info("tunnel start canceled while binding",
			zap.String("session_id", sessionID),
			zap.Int("port", port))
		return m.finishStart(cacheKey, started, 0, errStartCanceled)
	}

	m.serveTunnel(ctx, srv, ln, cacheKey, sessionID, port, actualPort)

	m.logger.Info("tunnel started",
		zap.String("session_id", sessionID),
		zap.Int("port", port),
		zap.Int("tunnel_port", actualPort))

	return m.finishStart(cacheKey, started, actualPort, nil)
}

// claimStart reserves cacheKey for the calling StartTunnel. It returns a
// non-nil pendingTunnel when the caller owns the start; otherwise it returns
// the result of the tunnel that already exists or was being built
// concurrently, having waited for the latter to settle.
func (m *TunnelManager) claimStart(cacheKey string) (*pendingTunnel, int, error) {
	m.mu.Lock()
	closed := m.closed
	entry, running := m.tunnels[cacheKey]
	inFlight, starting := m.pending[cacheKey]
	var started *pendingTunnel
	if !closed && !running && !starting {
		started = &pendingTunnel{done: make(chan struct{})}
		m.pending[cacheKey] = started
	}
	m.mu.Unlock()

	switch {
	case closed:
		return nil, 0, errManagerClosed
	case running:
		return nil, entry.tunnelPort, nil
	case starting:
		<-inFlight.done // Never while holding m.mu: the owner needs it to finish.
		return nil, inFlight.tunnelPort, inFlight.err
	default:
		return started, 0, nil
	}
}

// publish installs a finished tunnel under cacheKey, before the reservation is
// released, so a caller arriving in between finds the tunnel rather than
// starting a second one. It reports false when the reservation is no longer
// registered, which means InvalidateSession or Shutdown tore this session down
// while the bind was in flight: the caller must then abandon its listener
// instead of installing a tunnel nobody will ever close.
func (m *TunnelManager) publish(cacheKey string, started *pendingTunnel, entry *tunnelEntry) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.pending[cacheKey] != started {
		return false
	}
	m.tunnels[cacheKey] = entry
	return true
}

// finishStart releases the reservation taken by claimStart and hands its
// outcome to every caller waiting on the same session:port. The identity check
// matters: once a teardown has canceled this reservation another start may own
// the key, and that newcomer's reservation must survive.
func (m *TunnelManager) finishStart(cacheKey string, started *pendingTunnel, tunnelPort int, err error) (int, error) {
	started.tunnelPort = tunnelPort
	started.err = err

	m.mu.Lock()
	if m.pending[cacheKey] == started {
		delete(m.pending, cacheKey)
	}
	m.mu.Unlock()

	close(started.done)
	return tunnelPort, err
}

// resolveAndBind resolves the agentctl target URL for the session and binds a
// local TCP listener for the tunnel.
func (m *TunnelManager) resolveAndBind(sessionID string, tunnelPort int) (*url.URL, string, net.Listener, error) {
	execution, ok := m.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok {
		return nil, "", nil, fmt.Errorf("session not found or no active execution")
	}

	agentctlClient, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if agentctlClient == nil {
		return nil, "", nil, fmt.Errorf("agentctl client not available")
	}

	target, err := url.Parse(agentctlClient.BaseURL())
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to parse agentctl URL: %w", err)
	}
	authToken := agentctlClient.AuthToken()

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", tunnelPort))
	if err != nil {
		if netutil.IsAddrInUse(err) {
			return nil, "", nil, fmt.Errorf("port %d is already in use, choose a different port", tunnelPort)
		}
		return nil, "", nil, fmt.Errorf("failed to bind tunnel port %d: %w", tunnelPort, err)
	}

	return target, authToken, ln, nil
}

// serveTunnel starts the tunnel HTTP server and its shutdown goroutine.
func (m *TunnelManager) serveTunnel(ctx context.Context, srv *http.Server, ln net.Listener, cacheKey, sessionID string, port, tunnelPort int) {
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			m.logger.Error("tunnel server error",
				zap.String("session_id", sessionID),
				zap.Int("port", port),
				zap.Int("tunnel_port", tunnelPort),
				zap.Error(serveErr))
			m.removeTunnel(cacheKey)
		}
	}()
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
	canceled := m.cancelPendingStarts(prefix)
	m.mu.Unlock()

	for _, entry := range toStop {
		entry.cancel()
		_ = entry.ln.Close()
	}

	if len(toStop) > 0 || canceled > 0 {
		m.logger.Info("invalidated session tunnels",
			zap.String("session_id", sessionID),
			zap.Int("count", len(toStop)),
			zap.Int("canceled_starts", canceled))
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
	canceled := m.cancelPendingStarts("")
	m.closed = true
	m.mu.Unlock()

	for _, entry := range entries {
		entry.cancel()
		_ = entry.ln.Close()
	}

	if len(entries) > 0 || canceled > 0 {
		m.logger.Info("shutdown all tunnels",
			zap.Int("count", len(entries)),
			zap.Int("canceled_starts", canceled))
	}
}

// cancelPendingStarts drops every reservation whose key has the given prefix
// ("" for all of them) and reports how many were dropped. A start whose
// reservation is gone abandons its listener in publish rather than installing a
// tunnel into a session, or a manager, that has already been torn down.
// The caller must hold m.mu.
func (m *TunnelManager) cancelPendingStarts(prefix string) int {
	canceled := 0
	for key := range m.pending {
		if strings.HasPrefix(key, prefix) {
			delete(m.pending, key)
			canceled++
		}
	}
	return canceled
}

func (m *TunnelManager) removeTunnel(cacheKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tunnels, cacheKey)
}

func (m *TunnelManager) createTunnelProxy(cacheKey string, target *url.URL, port int, authToken string) *httputil.ReverseProxy {
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
		// Inject agentctl auth token
		if authToken != "" {
			r.Out.Header.Set("Authorization", "Bearer "+authToken)
		}
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
