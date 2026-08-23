package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// errServerBindFailed signals that the HTTP listener could not be bound.
// startGatewayAndServe checks for it with errors.Is so it can skip the
// "Failed to start orchestrator" log for what is actually a bind failure —
// bindBootstrapListeners (via startHTTPServers) already logs the specific
// cause at the appropriate level.
var errServerBindFailed = errors.New("server bind failed")

// handlerSwitch is an http.Handler whose effective implementation can be
// swapped at runtime without closing or rebinding the underlying listener.
// Startup constructs it holding the bootstrap handler and later Store()s the
// fully wired Gin router once buildHTTPServer returns, so exactly one
// *http.Server/*serverListeners pair flows from bind through shutdown.
type handlerSwitch struct {
	current atomic.Pointer[http.Handler]
}

// newHandlerSwitch returns a handlerSwitch already serving initial.
func newHandlerSwitch(initial http.Handler) *handlerSwitch {
	hs := &handlerSwitch{}
	hs.Store(initial)
	return hs
}

// Store replaces the handler that serves subsequent requests.
func (hs *handlerSwitch) Store(h http.Handler) {
	hs.current.Store(&h)
}

func (hs *handlerSwitch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	(*hs.current.Load()).ServeHTTP(w, r)
}

// newBootstrapHandler answers every request while the real router is still
// being built. GET /health always returns 200 (plus the desktop health
// token header, if configured) so the launcher's liveness probe
// (internal/launcher/health.go) succeeds regardless of startup progress —
// making liveness depend on readiness here would bring back the crash loop
// this handler exists to fix. Every other path returns a deterministic 503
// with a parseable "starting" body instead of hanging, resetting, or 404ing.
// GET /health's body is identical to healthHandler's in helpers.go (status
// "ok", service, mode, version) since /health is a pure liveness probe and
// its shape must not depend on startup progress.
func newBootstrapHandler(version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health" {
			body := map[string]any{
				statusKey:       startingStatus,
				serviceFieldKey: kandevName,
				versionFieldKey: version,
			}
			writeBootstrapJSON(w, http.StatusServiceUnavailable, body)
			return
		}
		if token := desktopHealthToken(); token != "" {
			w.Header().Set(desktopHealthTokenHeader, token)
		}
		writeBootstrapJSON(w, http.StatusOK, map[string]any{
			statusKey:       "ok",
			serviceFieldKey: kandevName,
			"mode":          "websocket+http",
			versionFieldKey: version,
		})
	})
}

func writeBootstrapJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// bindBootstrapListeners binds the configured HTTP addresses immediately,
// before any startup work runs, and serves them with the bootstrap handler.
// The returned handlerSwitch is later Store()d with the real router; the
// returned *http.Server and *serverListeners are the same instances that
// must flow unmodified into awaitShutdown, so shutdown needs no changes.
func bindBootstrapListeners(cfg *config.Config, log *logger.Logger, version string) (*handlerSwitch, *http.Server, *serverListeners, error) {
	handler := newHandlerSwitch(newBootstrapHandler(version))
	server := &http.Server{
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeoutDuration(),
		WriteTimeout: cfg.Server.WriteTimeoutDuration(),
	}

	port := resolvedHTTPPort(cfg)
	hosts, err := cfg.Server.ResolvedBinds()
	if err != nil {
		log.Error("Invalid server bind configuration", zap.Error(err))
		return nil, nil, nil, errServerBindFailed
	}
	listeners, ok := startHTTPServers(server, hosts, port, log)
	if !ok {
		return nil, nil, nil, errServerBindFailed
	}

	log.Info("HTTP listener bound; serving liveness probe while startup continues",
		zap.Int("port", port), zap.Strings("hosts", hosts))

	return handler, server, listeners, nil
}

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
