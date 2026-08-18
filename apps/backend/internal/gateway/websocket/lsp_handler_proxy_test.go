package websocket

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
)

// dialLSPHandler serves handler.HandleLSPConnection over a real HTTP server and
// dials it. The returned channel closes once the handler has returned, which is
// what lets a test assert on post-handler state (capacity release) without
// polling.
func dialLSPHandler(t *testing.T, handler *LSPHandler, target string) (*gorillaws.Conn, <-chan struct{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	served := make(chan struct{})
	router := gin.New()
	router.GET("/lsp/:sessionId", func(c *gin.Context) {
		defer close(served)
		handler.HandleLSPConnection(c)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	conn, resp, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+target, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, served
}

// expectLSPCloseCode reads from conn and asserts the peer closed with wantCode.
func expectLSPCloseCode(t *testing.T, conn *gorillaws.Conn, wantCode int, wantText string) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, _, err := conn.ReadMessage()
	var closeErr *gorillaws.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("ReadMessage() error = %v, want a websocket close error", err)
	}
	if closeErr.Code != wantCode {
		t.Fatalf("close code = %d, want %d", closeErr.Code, wantCode)
	}
	if wantText != "" && closeErr.Text != wantText {
		t.Fatalf("close text = %q, want %q", closeErr.Text, wantText)
	}
}

func TestNewLSPHandlerInitializesCapacityAndLogger(t *testing.T) {
	handler := NewLSPHandler(nil, nil, testLogger())
	if handler.capacity == nil {
		t.Fatal("capacity limiter was not initialized")
	}
	if handler.logger == nil {
		t.Fatal("logger was not initialized")
	}
	if !handler.capacity.TryAcquire() {
		t.Fatal("a freshly built handler must have capacity available")
	}
	handler.capacity.Release()
}

func TestHandleLSPConnectionRejectsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name      string
		sessionID string
		query     string
		wantError string
	}{
		{name: "missing session", sessionID: "", query: "?language=go", wantError: "sessionId is required"},
		{name: "missing language", sessionID: "s-1", query: "", wantError: "language query parameter is required"},
		{
			name:      "unsupported language",
			sessionID: "s-1",
			query:     "?language=cobol",
			wantError: "unsupported language: cobol",
		},
	}
	handler := &LSPHandler{logger: testLogger(), capacity: newLSPCapacityLimiter(1)}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/lsp/"+tt.sessionID+tt.query, nil)
			c.Params = gin.Params{{Key: "sessionId", Value: tt.sessionID}}
			handler.HandleLSPConnection(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			if got := decodeErrorBody(t, recorder.Body.Bytes()); got != tt.wantError {
				t.Fatalf("error = %q, want %q", got, tt.wantError)
			}
			if !handler.capacity.TryAcquire() {
				t.Fatal("a rejected request must not consume a capacity slot")
			}
			handler.capacity.Release()
		})
	}
}

func TestHandleLSPConnectionClosesWithSessionNotFoundCode(t *testing.T) {
	manager := &recordingLSPLifecycleManager{resolveErr: errors.New("no such session")}
	handler := &LSPHandler{
		lifecycleMgr: manager,
		capacity:     newLSPCapacityLimiter(1),
		logger:       testLogger(),
	}

	conn, served := dialLSPHandler(t, handler, "/lsp/ghost?language=go")
	expectLSPCloseCode(t, conn, lspCloseSessionNotFound, "session not found")
	joinWithin(t, served, "HandleLSPConnection")

	if !handler.capacity.TryAcquire() {
		t.Fatal("capacity was not released after a failed resolve")
	}
	handler.capacity.Release()
}

func TestHandleLSPConnectionClosesWithUnsupportedExecutorCode(t *testing.T) {
	manager := &recordingLSPLifecycleManager{runtimeName: agentruntime.RuntimeSSH}
	handler := &LSPHandler{
		lifecycleMgr: manager,
		capacity:     newLSPCapacityLimiter(1),
		logger:       testLogger(),
	}

	conn, served := dialLSPHandler(t, handler, "/lsp/remote?language=go")
	expectLSPCloseCode(t, conn, lspCloseUnsupportedExecutor, lspCloseUnsupportedCloseText)
	joinWithin(t, served, "HandleLSPConnection")

	if manager.ensureCalls != 0 {
		t.Fatalf("GetOrEnsureExecution calls = %d, want 0 for an unsupported runtime", manager.ensureCalls)
	}
}

// The capacity slot is acquired inside resolveLSPExecution; a later failure
// (here: an execution with no agentctl client) must still give it back.
func TestHandleLSPConnectionReleasesCapacityWhenAgentctlIsMissing(t *testing.T) {
	manager := &recordingLSPLifecycleManager{
		runtimeName: agentruntime.RuntimeStandalone,
		execution:   &lifecycle.AgentExecution{RuntimeName: agentruntime.RuntimeStandalone},
	}
	handler := &LSPHandler{
		lifecycleMgr: manager,
		capacity:     newLSPCapacityLimiter(1),
		logger:       testLogger(),
	}

	conn, served := dialLSPHandler(t, handler, "/lsp/no-agentctl?language=go")
	expectLSPCloseCode(t, conn, lspCloseSessionNotFound, "agentctl unavailable")
	joinWithin(t, served, "HandleLSPConnection")

	if !handler.capacity.TryAcquire() {
		t.Fatal("capacity was not released after the agentctl client was found missing")
	}
	handler.capacity.Release()
}

func TestProxyLSPConnectionsForwardsMessagesInBothDirections(t *testing.T) {
	handler := &LSPHandler{logger: testLogger(), capacity: newLSPCapacityLimiter(1)}

	browser, handlerBrowserSide := newTerminalWSPair(t)
	handlerUpstreamSide, upstream := newTerminalWSPair(t)

	done := spawnJoinable(t, "proxyLSPConnections", func() { _ = browser.Close() }, func() {
		handler.proxyLSPConnections(handlerBrowserSide, handlerUpstreamSide, "sess-lsp", "go")
	})

	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if err := browser.WriteMessage(gorillaws.TextMessage, request); err != nil {
		t.Fatalf("browser write: %v", err)
	}
	if err := upstream.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	msgType, got, err := upstream.ReadMessage()
	if err != nil {
		t.Fatalf("upstream read: %v", err)
	}
	if msgType != gorillaws.TextMessage || string(got) != string(request) {
		t.Fatalf("upstream received (%d, %q), want (%d, %q)",
			msgType, got, gorillaws.TextMessage, request)
	}

	response := []byte(`{"jsonrpc":"2.0","id":1,"result":{}}`)
	if err := upstream.WriteMessage(gorillaws.TextMessage, response); err != nil {
		t.Fatalf("upstream write: %v", err)
	}
	if err := browser.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, got, err = browser.ReadMessage()
	if err != nil {
		t.Fatalf("browser read: %v", err)
	}
	if string(got) != string(response) {
		t.Fatalf("browser received %q, want %q", got, response)
	}

	if err := browser.Close(); err != nil {
		t.Fatalf("close browser: %v", err)
	}
	joinWithin(t, done, "proxyLSPConnections")

	if err := upstream.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, _, err := upstream.ReadMessage(); err == nil {
		t.Fatal("upstream connection still readable after the browser disconnected")
	}
}

// A task-host LSP crash carries an application close code; the browser needs
// that exact code to decide whether to retry or surface an install prompt.
func TestProxyLSPConnectionsPropagatesUpstreamCloseCode(t *testing.T) {
	handler := &LSPHandler{logger: testLogger(), capacity: newLSPCapacityLimiter(1)}

	browser, handlerBrowserSide := newTerminalWSPair(t)
	handlerUpstreamSide, upstream := newTerminalWSPair(t)

	done := spawnJoinable(t, "proxyLSPConnections", func() { _ = browser.Close() }, func() {
		handler.proxyLSPConnections(handlerBrowserSide, handlerUpstreamSide, "sess-lsp", "go")
	})

	closeFrame := gorillaws.FormatCloseMessage(lspCloseInstallFailed, "install failed")
	if err := upstream.WriteControl(
		gorillaws.CloseMessage, closeFrame, time.Now().Add(wsTestTimeout),
	); err != nil {
		t.Fatalf("upstream close: %v", err)
	}

	expectLSPCloseCode(t, browser, lspCloseInstallFailed, "install failed")
	joinWithin(t, done, "proxyLSPConnections")
}
