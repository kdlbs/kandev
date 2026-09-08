package backendapp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestServerListenAddr(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "blank host keeps port-only address", host: "", port: 38429, want: ":38429"},
		{name: "wildcard host", host: "0.0.0.0", port: 38429, want: "0.0.0.0:38429"},
		{name: "loopback host", host: "127.0.0.1", port: 38429, want: "127.0.0.1:38429"},
		{name: "ipv6 host", host: "::1", port: 38429, want: "[::1]:38429"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverListenAddr(tt.host, tt.port); got != tt.want {
				t.Fatalf("serverListenAddr(%q, %d) = %q, want %q", tt.host, tt.port, got, tt.want)
			}
		})
	}
}

func TestDesktopHealthTokenTrimsEnv(t *testing.T) {
	t.Setenv(desktopHealthTokenEnv, "  token-value  ")

	if got := desktopHealthToken(); got != "token-value" {
		t.Fatalf("desktopHealthToken() = %q, want token-value", got)
	}
}

// TestHealthHandlerAlwaysOkRegardlessOfReadiness is the regression test for
// R1-1: /health is a liveness probe and must answer 200 (with the desktop
// health token echoed, when configured) whether or not the `ready` flag has
// flipped. Gating it on readiness would bring back the crash loop docs/specs/
// startup-listener-before-recovery/spec.md exists to fix.
func TestHealthHandlerAlwaysOkRegardlessOfReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(desktopHealthTokenEnv, "route-health-token")

	for _, readyState := range []bool{false, true} {
		ready.Store(readyState)
		t.Cleanup(func() { ready.Store(false) })

		router := gin.New()
		router.GET("/health", healthHandler(routeParams{}))
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		router.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("ready=%v: health status = %d, want %d", readyState, response.Code, http.StatusOK)
		}
		if got := response.Header().Get(desktopHealthTokenHeader); got != "route-health-token" {
			t.Fatalf("ready=%v: health token header = %q, want route-health-token", readyState, got)
		}
	}
}

// TestReadyHandlerGatesOnReadyFlag is the regression test for the task-02
// requirement that readiness (unlike liveness) reports not-ready during a
// blocked startup and ready once startup completes.
func TestReadyHandlerGatesOnReadyFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ready.Store(false)
	t.Cleanup(func() { ready.Store(false) })

	router := gin.New()
	router.GET("/ready", readyHandler(routeParams{}))

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("not-ready status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}

	ready.Store(true)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want %d", response.Code, http.StatusOK)
	}
}

func listenOnFreePort(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free port: %v", err)
	}
	return ln
}

func listenerPort(t *testing.T, ln net.Listener) int {
	t.Helper()
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected TCP listener, got %T", ln.Addr())
	}
	return tcpAddr.Port
}
