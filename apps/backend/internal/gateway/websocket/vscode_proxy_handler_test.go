package websocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

const vscodeStatusPath = "/api/v1/vscode/status"

// newFakeCodeServer serves the agentctl VS Code status endpoint plus a stub
// code-server behind the proxy path.
func newFakeCodeServer(t *testing.T, status string) *fakeAgentctl {
	t.Helper()
	return newFakeAgentctl(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == vscodeStatusPath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"` + status + `","port":8443,"url":"http://127.0.0.1:8443"}`))
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("code-server:" + r.URL.Path))
	})
}

func newVscodeEngine(handler *VscodeProxyHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/vscode/:sessionId/*path", handler.HandleVscodeProxy)
	return engine
}

// countStatusCalls reports how many times the upstream VS Code status endpoint
// was polled, which is how the per-session proxy cache is observed.
func countStatusCalls(seen []recordedRequest) int {
	n := 0
	for _, req := range seen {
		if req.path == vscodeStatusPath {
			n++
		}
	}
	return n
}

func TestHandleVscodeProxyRewritesPathAndInjectsAgentctlToken(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeCodeServer(t, "running")

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-vs", upstream.server.URL, "vscode-token", log)
	gateway := serveGateway(t, newVscodeEngine(NewVscodeProxyHandler(manager, log)))

	got := gateway(http.MethodGet, "/vscode/sess-vs/static/out/main.js")
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", got.status, http.StatusOK)
	}
	if got.body != "code-server:/api/v1/vscode/proxy/static/out/main.js" {
		t.Fatalf("body = %q, want the proxied code-server body", got.body)
	}

	var proxied *recordedRequest
	for i, req := range upstream.seen() {
		if strings.HasPrefix(req.path, "/api/v1/vscode/proxy") {
			proxied = &upstream.seen()[i]
		}
	}
	if proxied == nil {
		t.Fatalf("no proxied request reached the upstream: %+v", upstream.seen())
	}
	if proxied.path != "/api/v1/vscode/proxy/static/out/main.js" {
		t.Fatalf("upstream path = %q", proxied.path)
	}
	if proxied.auth != "Bearer vscode-token" {
		t.Fatalf("upstream Authorization = %q, want %q", proxied.auth, "Bearer vscode-token")
	}
}

// Older agentctl instances only register /api/v1/vscode/proxy/*path, so both
// root forms — with and without a trailing slash — must reach
// /api/v1/vscode/proxy/ upstream. Only the no-slash form exercises the
// empty-remainder branch; "/vscode/:id/" would survive dropping it.
func TestHandleVscodeProxyKeepsTrailingSlashForRootRequests(t *testing.T) {
	for _, requestPath := range []string{"/vscode/sess-root/", "/vscode/sess-root"} {
		t.Run(requestPath, func(t *testing.T) {
			log, _ := observedTerminalLogger(t)
			upstream := newFakeCodeServer(t, "running")

			manager := newProxyTestManager(t, log)
			addExecutionForURL(t, manager, "sess-root", upstream.server.URL, "", log)
			handler := NewVscodeProxyHandler(manager, log)

			// The gin wildcard route would redirect the no-slash form, so both
			// variants are driven through the handler with the params it reads.
			gateway := serveGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, _ := gin.CreateTestContext(w)
				c.Request = r
				c.Params = gin.Params{{Key: "sessionId", Value: "sess-root"}}
				handler.HandleVscodeProxy(c)
			}))

			got := gateway(http.MethodGet, requestPath)
			if got.status != http.StatusOK {
				t.Fatalf("status = %d, want %d", got.status, http.StatusOK)
			}
			if got.body != "code-server:/api/v1/vscode/proxy/" {
				t.Fatalf("body = %q, want the root code-server path", got.body)
			}
		})
	}
}

func TestHandleVscodeProxyRejectsStoppedCodeServer(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeCodeServer(t, "stopped")

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-stopped", upstream.server.URL, "", log)
	handler := NewVscodeProxyHandler(manager, log)
	gateway := serveGateway(t, newVscodeEngine(handler))

	got := gateway(http.MethodGet, "/vscode/sess-stopped/")
	if got.status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", got.status, http.StatusServiceUnavailable)
	}
	if msg := decodeErrorBody(t, []byte(got.body)); msg != "code-server is not running" {
		t.Fatalf("error = %q", msg)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.proxies) != 0 {
		t.Fatalf("cached proxies = %d, want 0 while code-server is stopped", len(handler.proxies))
	}
}

func TestHandleVscodeProxyReportsUnreachableStatusEndpoint(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-broken", upstream.server.URL, "", log)
	gateway := serveGateway(t, newVscodeEngine(NewVscodeProxyHandler(manager, log)))

	got := gateway(http.MethodGet, "/vscode/sess-broken/")
	if got.status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", got.status, http.StatusServiceUnavailable)
	}
	if msg := decodeErrorBody(t, []byte(got.body)); msg != "failed to get vscode status" {
		t.Fatalf("error = %q", msg)
	}
}

func TestHandleVscodeProxyRequiresSessionID(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler := NewVscodeProxyHandler(newProxyTestManager(t, log), log)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/vscode//", nil)
	c.Params = gin.Params{{Key: "sessionId", Value: ""}}
	handler.HandleVscodeProxy(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := decodeErrorBody(t, recorder.Body.Bytes()); got != "sessionId is required" {
		t.Fatalf("error = %q", got)
	}
}

func TestHandleVscodeProxyDeniesForeignSessionBeforeStatusLookup(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeCodeServer(t, "running")

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-foreign", upstream.server.URL, "", log)
	manager.SetSessionAccessChecker(func(context.Context, string) error {
		return errors.New("not your session")
	})
	gateway := serveGateway(t, newVscodeEngine(NewVscodeProxyHandler(manager, log)))

	got := gateway(http.MethodGet, "/vscode/sess-foreign/")
	if got.status != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", got.status, http.StatusNotFound)
	}
	if msg := decodeErrorBody(t, []byte(got.body)); msg != "session not found" {
		t.Fatalf("error = %q", msg)
	}
	if n := len(upstream.seen()); n != 0 {
		t.Fatalf("upstream requests = %d, want 0 for a denied session", n)
	}
}

func TestHandleVscodeProxyReportsMissingExecutionAndClient(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	manager := newProxyTestManager(t, log)
	clientless := &lifecycle.AgentExecution{ID: "exec-vs-noclient", SessionID: "sess-vs-noclient"}
	if err := manager.ExecutionStoreForTesting().Add(clientless); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	gateway := serveGateway(t, newVscodeEngine(NewVscodeProxyHandler(manager, log)))

	tests := []struct {
		name       string
		sessionID  string
		wantStatus int
		wantError  string
	}{
		{
			name:       "no execution",
			sessionID:  "sess-vs-missing",
			wantStatus: http.StatusNotFound,
			wantError:  "session not found or no active execution",
		},
		{
			name:       "no agentctl client",
			sessionID:  "sess-vs-noclient",
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "agentctl client not available",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gateway(http.MethodGet, "/vscode/"+tt.sessionID+"/")
			if got.status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", got.status, tt.wantStatus)
			}
			if msg := decodeErrorBody(t, []byte(got.body)); msg != tt.wantError {
				t.Fatalf("error = %q, want %q", msg, tt.wantError)
			}
		})
	}
}

// The cached proxy is the whole point of the fast path: a second request must
// not re-poll the upstream VS Code status endpoint.
func TestHandleVscodeProxyChecksUpstreamStatusOnlyOnce(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeCodeServer(t, "running")

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-cached", upstream.server.URL, "", log)
	handler := NewVscodeProxyHandler(manager, log)
	gateway := serveGateway(t, newVscodeEngine(handler))

	for _, path := range []string{"/vscode/sess-cached/", "/vscode/sess-cached/manifest.json"} {
		if got := gateway(http.MethodGet, path); got.status != http.StatusOK {
			t.Fatalf("status for %s = %d, want %d", path, got.status, http.StatusOK)
		}
	}
	if n := countStatusCalls(upstream.seen()); n != 1 {
		t.Fatalf("vscode status polls = %d, want 1 (second request must reuse the cache)", n)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.proxies) != 1 {
		t.Fatalf("cached proxies = %d, want 1", len(handler.proxies))
	}
}

func TestHandleVscodeProxyInvalidatesCacheOnUpstreamFailure(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeCodeServer(t, "running")

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-flaky", upstream.server.URL, "", log)
	handler := NewVscodeProxyHandler(manager, log)
	gateway := serveGateway(t, newVscodeEngine(handler))

	if got := gateway(http.MethodGet, "/vscode/sess-flaky/"); got.status != http.StatusOK {
		t.Fatalf("priming status = %d, want %d", got.status, http.StatusOK)
	}
	handler.mu.Lock()
	cached := len(handler.proxies)
	handler.mu.Unlock()
	if cached != 1 {
		t.Fatalf("cached proxies = %d after priming, want 1", cached)
	}

	upstream.server.Close()

	got := gateway(http.MethodGet, "/vscode/sess-flaky/")
	if got.status != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d after the upstream died", got.status, http.StatusBadGateway)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.proxies) != 0 {
		t.Fatalf("cached proxies = %d, want 0 after an upstream failure", len(handler.proxies))
	}
}
