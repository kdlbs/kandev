package backendapp

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// serverBindRetryInterval is how often the background retry loop re-attempts a
// bind for an address that failed the initial bind (e.g. a tailnet IP that only
// exists once tailscaled comes up). It is a var so tests can shorten it.
var serverBindRetryInterval = 5 * time.Second

// serverListeners owns the set of net.Listeners that serve a single shared
// http.Server, plus a background retry loop that keeps trying hosts whose
// initial bind failed. The single-listener case is just the one-element form.
//
// All active listeners are closed by the shared http.Server on Shutdown; the
// retry loop is stopped explicitly via Stop() before that Shutdown so no new
// listener can appear after shutdown begins.
type serverListeners struct {
	server *http.Server
	port   int
	log    *logger.Logger

	mu    sync.Mutex
	bound []string // ln.Addr().String() for each active listener

	retryCancel context.CancelFunc
	retryDone   chan struct{}
	stopOnce    sync.Once
}

// startHTTPServers binds one listener per host and serves the shared handler on
// each. Bind-failure policy: if EVERY bind fails it returns false (fatal); if
// SOME fail it logs a prominent warning naming the failed hosts, serves the
// rest, and launches a background retry loop that binds the failed hosts once
// they become available. Returns the manager so shutdown can stop the retry
// loop and the readiness probe can pick a reachable address.
func startHTTPServers(server *http.Server, hosts []string, port int, log *logger.Logger) (*serverListeners, bool) {
	sl := &serverListeners{server: server, port: port, log: log}

	var failed []string
	for _, h := range hosts {
		if err := sl.bind(h); err != nil {
			failed = append(failed, h)
		}
	}

	if len(sl.boundAddrs()) == 0 {
		log.Error("Server failed to bind any configured address", zap.Strings("hosts", hosts))
		return nil, false
	}

	if len(failed) > 0 {
		log.Warn("Server bound only some addresses; retrying the rest in the background",
			zap.Strings("failed", failed),
			zap.Strings("bound", sl.boundAddrs()))
		sl.startRetry(failed)
	}

	return sl, true
}

// bind listens on host:port and, on success, serves the shared handler on the
// new listener. A failure is logged and returned so the caller can decide the
// all-fail vs. partial-fail policy.
func (sl *serverListeners) bind(host string) error {
	addr := serverListenAddr(host, sl.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		sl.log.Warn("Server listen error", zap.String("addr", addr), zap.Error(err))
		return err
	}

	sl.mu.Lock()
	sl.bound = append(sl.bound, ln.Addr().String())
	sl.mu.Unlock()

	sl.serve(ln)
	return nil
}

// serve runs the shared http.Server on ln in its own goroutine. http.Server
// supports Serve being called concurrently on multiple listeners and closes
// all of them on Shutdown. The defensive ln.Close() covers the narrow race
// where a retry bind completes after Shutdown has already started (Serve then
// returns ErrServerClosed without closing the listener).
func (sl *serverListeners) serve(ln net.Listener) {
	go func() {
		defer func() { _ = ln.Close() }()
		sl.log.Info("WebSocket server listening",
			zap.String("addr", ln.Addr().String()), zap.Int("port", sl.port))
		if err := sl.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			sl.log.Error("Server serve error",
				zap.String("addr", ln.Addr().String()), zap.Error(err))
		}
	}()
}

func (sl *serverListeners) boundAddrs() []string {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return append([]string(nil), sl.bound...)
}

// startRetry launches the background retry loop for the given failed hosts.
func (sl *serverListeners) startRetry(hosts []string) {
	ctx, cancel := context.WithCancel(context.Background())
	sl.retryCancel = cancel
	sl.retryDone = make(chan struct{})
	go sl.retryLoop(ctx, hosts)
}

// retryLoop periodically re-attempts binding the still-failed hosts until all
// succeed or the context is cancelled (graceful shutdown). Each successful bind
// starts serving immediately, so a late-arriving tailnet IP self-heals.
func (sl *serverListeners) retryLoop(ctx context.Context, hosts []string) {
	defer close(sl.retryDone)

	ticker := time.NewTicker(serverBindRetryInterval)
	defer ticker.Stop()

	pending := append([]string(nil), hosts...)
	for len(pending) > 0 {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		var still []string
		for _, h := range pending {
			if err := sl.bind(h); err != nil {
				still = append(still, h)
				continue
			}
			sl.log.Info("Server bound previously-failed address", zap.String("host", h))
		}
		pending = still
	}
}

// Stop cancels the background retry loop and waits for it to drain. Safe to
// call concurrently and when no retry loop was started; the cancel+drain runs
// at most once via stopOnce. Must run before server.Shutdown so no new listener
// appears after shutdown begins.
func (sl *serverListeners) Stop() {
	if sl == nil {
		return
	}
	sl.stopOnce.Do(func() {
		if sl.retryCancel != nil {
			sl.retryCancel()
			<-sl.retryDone
		}
	})
}

// probeAddr returns an address the readiness probe can dial. It prefers a bound
// address that resolves to loopback (always reachable from the box without
// depending on a routable interface), falling back to the first bound address.
func (sl *serverListeners) probeAddr() string {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if len(sl.bound) == 0 {
		return ""
	}
	for _, a := range sl.bound {
		probe := serverProbeAddr(a)
		if host, _, err := net.SplitHostPort(probe); err == nil && config.IsLoopbackHost(host) {
			return probe
		}
	}
	return serverProbeAddr(sl.bound[0])
}
