package main

import (
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// pprofAddrEnv names the loopback address for the operator profiling listener.
const pprofAddrEnv = "KANDEV_AGENTCTL_PPROF_ADDR"

// startPprofListenerIfEnabled serves the standard net/http/pprof handlers on a
// separate address when KANDEV_AGENTCTL_PPROF_ADDR is set (for example
// 127.0.0.1:6061). The instance routers keep /debug/pprof behind their
// per-session token, which only the backend holds, so an operator cannot
// profile a running agentctl that hosts many sessions. The listener is opt-in
// and refuses any address that is not loopback.
//
// The returned stop function owns the listener it starts: it closes it and
// waits for the serving goroutine to exit. It is a no-op when the listener was
// not started, and it is safe to call more than once.
func startPprofListenerIfEnabled(log *logger.Logger) func() {
	addr := strings.TrimSpace(os.Getenv(pprofAddrEnv))
	if addr == "" {
		return func() {}
	}
	if !isLoopbackAddr(addr) {
		log.Warn("pprof listener disabled: address is not loopback", zap.String("address", addr))
		return func() {}
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Warn("pprof listener failed to bind", zap.String("address", addr), zap.Error(err))
		return func() {}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	done := make(chan struct{})
	go func() {
		defer close(done)
		log.Info("pprof listener starting", zap.String("address", ln.Addr().String()))
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Warn("pprof listener stopped", zap.String("address", addr), zap.Error(err))
		}
	}()
	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			_ = srv.Close()
			<-done
		})
	}
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
