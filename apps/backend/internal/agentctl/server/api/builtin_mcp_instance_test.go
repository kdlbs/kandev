package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestBuiltinMCPInstanceAuthExemptionsUseEffectiveRoute(t *testing.T) {
	cfg := &config.InstanceConfig{InstanceID: "exec-1", ScopedMCPRoutes: true}
	paths := builtinMCPExemptPaths(cfg)
	for _, path := range []string{
		"/health",
		"/i/exec-1/mcp",
		"/i/exec-1/sse",
		"/i/exec-1/message",
		"/i/exec-1/mcp/packages/rich-output",
	} {
		if !containsRoute(paths, path) {
			t.Errorf("route exemptions %v do not include %q", paths, path)
		}
	}
	for _, path := range []string{"/mcp", "/sse", "/message", "/i/other/mcp"} {
		if containsRoute(paths, path) {
			t.Errorf("route exemptions %v unexpectedly include %q", paths, path)
		}
	}
}

func TestBuiltinMCPInstanceRoutesRejectStaleTargetsBeforeDispatch(t *testing.T) {
	var dispatched atomic.Int32
	srv := newBuiltinMCPRouteServer(t, "exec-1", true)
	srv.router.Any("/mcp", countMCPDispatch(&dispatched))
	srv.router.Any("/mcp/packages/unknown", countMCPDispatch(&dispatched))
	srv.router.Any("/i/other/mcp", countMCPDispatch(&dispatched))
	srv.router.Any("/i/other/mcp/packages/unknown", countMCPDispatch(&dispatched))
	srv.router.Any("/i/exec-1/mcp", countMCPDispatch(&dispatched))

	for _, path := range []string{"/mcp", "/mcp/packages/unknown", "/i/other/mcp", "/i/other/mcp/packages/unknown"} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			for _, token := range []string{"", "Bearer control-token"} {
				request := httptest.NewRequest(method, path, nil)
				if token != "" {
					request.Header.Set("Authorization", token)
				}
				response := httptest.NewRecorder()
				srv.Router().ServeHTTP(response, request)
				if response.Code != http.StatusNotFound {
					t.Fatalf("%s %s with auth %q returned %d, want 404: %s", method, path, token, response.Code, response.Body.String())
				}
				if dispatched.Load() != 0 {
					t.Fatalf("stale route dispatched %d requests", dispatched.Load())
				}
			}
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/i/exec-1/mcp", nil)
	response := httptest.NewRecorder()
	srv.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK || dispatched.Load() != 1 {
		t.Fatalf("own route returned %d with %d dispatches, want 200 and 1", response.Code, dispatched.Load())
	}
	wrongOwner := httptest.NewRequest(http.MethodPost, "/i/exec-1/mcp", nil)
	wrongOwner.Header.Set("X-Instance-ID", "other")
	response = httptest.NewRecorder()
	srv.Router().ServeHTTP(response, wrongOwner)
	if response.Code != http.StatusNotFound || dispatched.Load() != 1 {
		t.Fatalf("wrong owner returned %d with %d dispatches, want 404 and unchanged dispatch count", response.Code, dispatched.Load())
	}
	request = httptest.NewRequest(http.MethodGet, "/health", nil)
	response = httptest.NewRecorder()
	srv.Router().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health returned %d, want 200", response.Code)
	}
}

func TestBuiltinMCPInstanceLegacyRouteRemainsBare(t *testing.T) {
	var dispatched atomic.Int32
	srv := newBuiltinMCPRouteServer(t, "exec-1", false)
	srv.router.Any("/mcp", countMCPDispatch(&dispatched))
	srv.router.Any("/i/exec-1/mcp", countMCPDispatch(&dispatched))

	for _, test := range []struct {
		path string
		want int
	}{{path: "/mcp", want: http.StatusOK}, {path: "/i/exec-1/mcp", want: http.StatusNotFound}} {
		response := httptest.NewRecorder()
		srv.Router().ServeHTTP(response, httptest.NewRequest(http.MethodPost, test.path, nil))
		if response.Code != test.want {
			t.Fatalf("POST %s returned %d, want %d", test.path, response.Code, test.want)
		}
	}
	if dispatched.Load() != 1 {
		t.Fatalf("dispatch count = %d, want 1", dispatched.Load())
	}
}

func newBuiltinMCPRouteServer(t *testing.T, instanceID string, scoped bool) *Server {
	t.Helper()
	return newBuiltinMCPRouteServerWithLogger(t, instanceID, scoped, newTestLogger())
}

func newBuiltinMCPRouteServerWithLogger(t *testing.T, instanceID string, scoped bool, log *logger.Logger) *Server {
	t.Helper()
	cfg := &config.InstanceConfig{Port: 43210, InstanceID: instanceID, ScopedMCPRoutes: scoped, AuthToken: "control-token", NativeMCPRoutePrefix: ""}
	if scoped {
		cfg.NativeMCPRoutePrefix = "/i/" + instanceID
	}
	procMgr := process.NewManager(cfg, log)
	return NewServer(cfg, procMgr, nil, nil, log)
}

func TestBuiltinMCPInstancePortReuseRejectsPriorPrefix(t *testing.T) {
	var dispatched atomic.Int32
	srv := newBuiltinMCPRouteServer(t, "replacement-2", true)
	srv.router.Any("/i/replacement-1/mcp", countMCPDispatch(&dispatched))
	srv.router.Any("/i/replacement-2/mcp", countMCPDispatch(&dispatched))
	for _, test := range []struct {
		path string
		want int
	}{{path: "/i/replacement-1/mcp", want: http.StatusNotFound}, {path: "/i/replacement-2/mcp", want: http.StatusOK}} {
		response := httptest.NewRecorder()
		srv.Router().ServeHTTP(response, httptest.NewRequest(http.MethodPost, test.path, nil))
		if response.Code != test.want {
			t.Fatalf("POST %s returned %d, want %d", test.path, response.Code, test.want)
		}
	}
	if dispatched.Load() != 1 {
		t.Fatalf("dispatch count = %d, want 1", dispatched.Load())
	}
}

func TestBuiltinMCPInstanceRejectWarningsAreRateLimited(t *testing.T) {
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	var dispatched atomic.Int32
	const requests = 32
	for _, instanceID := range []string{"exec-1", "exec-2"} {
		srv := newBuiltinMCPRouteServerWithLogger(t, instanceID, true, log)
		srv.router.Any("/i/other/mcp", countMCPDispatch(&dispatched))
		for range requests {
			response := httptest.NewRecorder()
			srv.Router().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/i/other/mcp", nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("stale route returned %d, want 404", response.Code)
			}
		}
	}
	if got := observed.FilterMessage("rejected stale built-in MCP route").Len(); got != 10 {
		t.Fatalf("warning count = %d, want capped burst of 5 from each of two servers", got)
	}
	if got := dispatched.Load(); got != 0 {
		t.Fatalf("stale route dispatched %d requests, want 0", got)
	}
}

func countMCPDispatch(counter *atomic.Int32) gin.HandlerFunc {
	return func(c *gin.Context) {
		counter.Add(1)
		c.Status(http.StatusOK)
	}
}

func TestBuiltinMCPInstanceLegacyAuthExemptionsStayBare(t *testing.T) {
	paths := builtinMCPExemptPaths(&config.InstanceConfig{InstanceID: "exec-1"})
	for _, path := range []string{"/health", "/mcp", "/sse", "/message", "/mcp/packages/rich-output"} {
		if !containsRoute(paths, path) {
			t.Errorf("legacy exemptions %v do not include %q", paths, path)
		}
	}
	if containsRoute(paths, "/i/exec-1/mcp") {
		t.Fatalf("legacy exemptions %v unexpectedly include scoped route", paths)
	}
}

func containsRoute(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
