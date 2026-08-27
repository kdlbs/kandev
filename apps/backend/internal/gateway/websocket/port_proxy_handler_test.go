package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// recordedRequest captures what a fake agentctl saw, so proxy tests can assert
// on the rewritten path and injected headers rather than only on status codes.
type recordedRequest struct {
	path   string
	auth   string
	host   string
	method string
}

// fakeAgentctl is an httptest server standing in for agentctl inside an executor.
type fakeAgentctl struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recordedRequest
}

func (f *fakeAgentctl) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, recordedRequest{
		path:   r.URL.Path,
		auth:   r.Header.Get("Authorization"),
		host:   r.Host,
		method: r.Method,
	})
}

func (f *fakeAgentctl) seen() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedRequest(nil), f.requests...)
}

func newFakeAgentctl(t *testing.T, handler http.HandlerFunc) *fakeAgentctl {
	t.Helper()
	fake := &fakeAgentctl{}
	fake.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.record(r)
		handler(w, r)
	}))
	// Keep-alive-free so the reverse proxy's pooled connections do not outlive
	// the test and trip this package's goroutine-leak check.
	fake.server.Config.SetKeepAlivesEnabled(false)
	fake.server.Start()
	t.Cleanup(fake.server.Close)
	return fake
}

// gatewayResponse is the outcome of a request made against the gateway under test.
type gatewayResponse struct {
	status int
	body   string
	header http.Header
}

// serveGateway runs handler behind a real HTTP server. httptest.ResponseRecorder
// is not a http.CloseNotifier, which httputil.ReverseProxy requires, so proxy
// paths have to be driven over a real connection.
func serveGateway(t *testing.T, handler http.Handler) func(method, path string) gatewayResponse {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.SetKeepAlivesEnabled(false)
	srv.Start()
	t.Cleanup(srv.Close)

	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	return func(method, path string) gatewayResponse {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+path, nil)
		if err != nil {
			t.Fatalf("build request %s: %v", path, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %s: %v", path, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body for %s: %v", path, err)
		}
		return gatewayResponse{status: resp.StatusCode, body: string(body), header: resp.Header}
	}
}

func newProxyTestManager(t *testing.T, log *logger.Logger) *lifecycle.Manager {
	t.Helper()
	return lifecycle.NewManager(
		nil,
		bus.NewMemoryEventBus(log),
		nil,
		nil,
		nil,
		nil,
		lifecycle.ExecutorFallbackDeny,
		t.TempDir(),
		log,
	)
}

// addExecutionForURL registers a synthetic execution whose agentctl client
// points at serverURL and authenticates with authToken.
func addExecutionForURL(
	t *testing.T,
	manager *lifecycle.Manager,
	sessionID, serverURL, authToken string,
	log *logger.Logger,
) *lifecycle.AgentExecution {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	host, portString, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	execution := &lifecycle.AgentExecution{ID: "exec-" + sessionID, SessionID: sessionID}
	execution.SetAgentCtlClientForTesting(
		agentctlclient.NewClient(host, port, log, agentctlclient.WithAuthToken(authToken)),
	)
	if err := manager.ExecutionStoreForTesting().Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	return execution
}

func newPortProxyEngine(handler *PortProxyHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/port-proxy/:sessionId/:port/*path", handler.HandlePortProxy)
	return engine
}

func decodeErrorBody(t *testing.T, body []byte) string {
	t.Helper()
	var parsed map[string]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	return parsed["error"]
}

func TestHandlePortProxyRewritesPathAndInjectsAgentctlToken(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("console.log(1)"))
	})

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-1", upstream.server.URL, "port-token", log)
	gateway := serveGateway(t, newPortProxyEngine(NewPortProxyHandler(manager, log)))

	got := gateway(http.MethodGet, "/port-proxy/sess-1/3000/assets/app.js?v=2")
	if got.status != http.StatusTeapot {
		t.Fatalf("status = %d, want %d (upstream status must pass through)", got.status, http.StatusTeapot)
	}
	if got.body != "console.log(1)" {
		t.Fatalf("body = %q, want the upstream body", got.body)
	}

	seen := upstream.seen()
	if len(seen) != 1 {
		t.Fatalf("upstream requests = %d, want 1", len(seen))
	}
	if seen[0].path != "/api/v1/port-proxy/3000/assets/app.js" {
		t.Fatalf("upstream path = %q, want %q", seen[0].path, "/api/v1/port-proxy/3000/assets/app.js")
	}
	if seen[0].auth != "Bearer port-token" {
		t.Fatalf("upstream Authorization = %q, want %q", seen[0].auth, "Bearer port-token")
	}
}

// Root-absolute URLs in proxied HTML must be re-anchored on the proxy prefix,
// or every asset request escapes the proxy chain and hits the gateway origin.
func TestHandlePortProxyRewritesRootAbsoluteURLsInHTML(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<link href="/static/app.css"><script src="/static/app.js"></script>`))
	})

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-html", upstream.server.URL, "", log)
	gateway := serveGateway(t, newPortProxyEngine(NewPortProxyHandler(manager, log)))

	got := gateway(http.MethodGet, "/port-proxy/sess-html/8080/index.html")
	for _, want := range []string{
		`href="/port-proxy/sess-html/8080/static/app.css"`,
		`src="/port-proxy/sess-html/8080/static/app.js"`,
	} {
		if !strings.Contains(got.body, want) {
			t.Fatalf("rewritten body %q missing %q", got.body, want)
		}
	}
	if seen := upstream.seen(); len(seen) != 1 || seen[0].path != "/api/v1/port-proxy/8080/index.html" {
		t.Fatalf("upstream requests = %+v, want a single /api/v1/port-proxy/8080/index.html", seen)
	}
}

func TestHandlePortProxyRejectsPortsOutsideTheAllowedRange(t *testing.T) {
	tests := []struct {
		name string
		port string
	}{
		{name: "privileged", port: "80"},
		{name: "boundary below", port: "1023"},
		{name: "above range", port: "70000"},
		{name: "not a number", port: "http"},
	}
	log, _ := observedTerminalLogger(t)
	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-1", "http://127.0.0.1:1", "", log)
	gateway := serveGateway(t, newPortProxyEngine(NewPortProxyHandler(manager, log)))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gateway(http.MethodGet, "/port-proxy/sess-1/"+tt.port+"/x")
			if got.status != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", got.status, http.StatusBadRequest)
			}
			if msg := decodeErrorBody(t, []byte(got.body)); msg != "invalid port: must be 1024-65535" {
				t.Fatalf("error = %q", msg)
			}
		})
	}
}

func TestHandlePortProxyRequiresSessionID(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler := NewPortProxyHandler(newProxyTestManager(t, log), log)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/port-proxy//3000/", nil)
	c.Params = gin.Params{{Key: "sessionId", Value: ""}, {Key: "port", Value: "3000"}}
	handler.HandlePortProxy(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if got := decodeErrorBody(t, recorder.Body.Bytes()); got != "sessionId is required" {
		t.Fatalf("error = %q", got)
	}
}

// The proxy prefix ends exactly at the port, so a request for the app root must
// still forward a "/" path to agentctl rather than an empty one.
func TestHandlePortProxyForwardsRootPathForPrefixOnlyRequests(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-root", upstream.server.URL, "", log)
	handler := NewPortProxyHandler(manager, log)

	// The gin wildcard route would redirect the prefix-only form, so the handler
	// is driven directly with the params it reads.
	gateway := serveGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := gin.CreateTestContext(w)
		c.Request = r
		c.Params = gin.Params{{Key: "sessionId", Value: "sess-root"}, {Key: "port", Value: "3000"}}
		handler.HandlePortProxy(c)
	}))
	gateway(http.MethodGet, "/port-proxy/sess-root/3000")

	seen := upstream.seen()
	if len(seen) != 1 || seen[0].path != "/api/v1/port-proxy/3000/" {
		t.Fatalf("upstream requests = %+v, want a single /api/v1/port-proxy/3000/", seen)
	}
}

// A bare execution lookup skips the service-layer chokepoint, so the proxy must
// authorize session ownership itself before touching the per-session cache.
func TestHandlePortProxyDeniesForeignSessionBeforeResolvingProxy(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-other", upstream.server.URL, "", log)
	manager.SetSessionAccessChecker(func(context.Context, string) error {
		return errors.New("not your session")
	})
	handler := NewPortProxyHandler(manager, log)
	gateway := serveGateway(t, newPortProxyEngine(handler))

	got := gateway(http.MethodGet, "/port-proxy/sess-other/3000/secret")
	if got.status != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", got.status, http.StatusNotFound)
	}
	if msg := decodeErrorBody(t, []byte(got.body)); msg != "session not found" {
		t.Fatalf("error = %q", msg)
	}
	if n := len(upstream.seen()); n != 0 {
		t.Fatalf("upstream requests = %d, want 0 for a denied session", n)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.proxies) != 0 {
		t.Fatalf("cached proxies = %d, want 0 for a denied session", len(handler.proxies))
	}
}

func TestHandlePortProxyReportsMissingExecutionAndClient(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	manager := newProxyTestManager(t, log)
	clientless := &lifecycle.AgentExecution{ID: "exec-noclient", SessionID: "sess-noclient"}
	if err := manager.ExecutionStoreForTesting().Add(clientless); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	gateway := serveGateway(t, newPortProxyEngine(NewPortProxyHandler(manager, log)))

	tests := []struct {
		name       string
		sessionID  string
		wantStatus int
		wantError  string
	}{
		{
			name:       "no execution",
			sessionID:  "sess-missing",
			wantStatus: http.StatusNotFound,
			wantError:  "session not found or no active execution",
		},
		{
			name:       "no agentctl client",
			sessionID:  "sess-noclient",
			wantStatus: http.StatusServiceUnavailable,
			wantError:  "agentctl client not available",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gateway(http.MethodGet, "/port-proxy/"+tt.sessionID+"/3000/")
			if got.status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", got.status, tt.wantStatus)
			}
			if msg := decodeErrorBody(t, []byte(got.body)); msg != tt.wantError {
				t.Fatalf("error = %q, want %q", msg, tt.wantError)
			}
		})
	}
}

func TestHandlePortProxyReusesCachedProxyPerSessionAndPort(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	upstream := newFakeAgentctl(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-cache", upstream.server.URL, "", log)
	handler := NewPortProxyHandler(manager, log)
	gateway := serveGateway(t, newPortProxyEngine(handler))

	for _, path := range []string{
		"/port-proxy/sess-cache/3000/a",
		"/port-proxy/sess-cache/3000/b",
		"/port-proxy/sess-cache/4000/a",
	} {
		if got := gateway(http.MethodGet, path); got.status != http.StatusOK {
			t.Fatalf("status for %s = %d, want %d", path, got.status, http.StatusOK)
		}
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.proxies) != 2 {
		t.Fatalf("cached proxies = %d, want 2 (one per session:port)", len(handler.proxies))
	}
	for _, key := range []string{"sess-cache:3000", "sess-cache:4000"} {
		if _, ok := handler.proxies[key]; !ok {
			t.Fatalf("cache key %q missing from %v", key, handler.proxies)
		}
	}
}

// A dead upstream must surface as 502 and drop the cached proxy, so the next
// request re-resolves the (possibly moved) agentctl target.
func TestHandlePortProxyInvalidatesCacheOnUpstreamFailure(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()

	manager := newProxyTestManager(t, log)
	addExecutionForURL(t, manager, "sess-dead", deadURL, "", log)
	handler := NewPortProxyHandler(manager, log)
	gateway := serveGateway(t, newPortProxyEngine(handler))

	got := gateway(http.MethodGet, "/port-proxy/sess-dead/3000/")
	if got.status != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", got.status, http.StatusBadGateway)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.proxies) != 0 {
		t.Fatalf("cached proxies = %d, want 0 after an upstream failure", len(handler.proxies))
	}
}

func TestPortProxyInvalidateSessionDropsOnlyThatSession(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler := NewPortProxyHandler(nil, log)

	handler.mu.Lock()
	handler.proxies["sess-a:3000"] = &portProxyEntry{target: "a-3000"}
	handler.proxies["sess-a:4000"] = &portProxyEntry{target: "a-4000"}
	handler.proxies["sess-b:3000"] = &portProxyEntry{target: "b-3000"}
	handler.mu.Unlock()

	handler.InvalidateSession("sess-a")

	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.proxies) != 1 {
		t.Fatalf("cached proxies = %d, want 1", len(handler.proxies))
	}
	if _, ok := handler.proxies["sess-b:3000"]; !ok {
		t.Fatalf("sess-b entry was dropped: %v", handler.proxies)
	}
}
