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
// what lets a test assert on lease admission after the handler returns.
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

func TestNewLSPHandlerUsesTypedCapacity(t *testing.T) {
	handler := newLSPHandler(nil, nil, testLogger(), 21, nil, nil)
	if handler.leases.max != 21 {
		t.Fatalf("LSP capacity = %d, want 21", handler.leases.max)
	}
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
	handler := newLSPHandler(nil, nil, testLogger(), 1, nil, nil)

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
		})
	}
}

func TestHandleLSPConnectionClosesWithSessionNotFoundCode(t *testing.T) {
	manager := &recordingLSPLifecycleManager{resolveErr: errors.New("no such session")}
	handler := newLSPHandler(manager, nil, testLogger(), 1, nil, nil)

	conn, served := dialLSPHandler(t, handler, "/lsp/ghost?language=go")
	expectLSPCloseCode(t, conn, lspCloseSessionNotFound, "session not found")
	joinWithin(t, served, "HandleLSPConnection")
}

func TestHandleLSPConnectionClosesWithUnsupportedExecutorCode(t *testing.T) {
	manager := &recordingLSPLifecycleManager{runtimeName: agentruntime.RuntimeSSH}
	handler := newLSPHandler(manager, nil, testLogger(), 1, nil, nil)

	conn, served := dialLSPHandler(t, handler, "/lsp/remote?language=go")
	expectLSPCloseCode(t, conn, lspCloseUnsupportedExecutor, lspCloseUnsupportedCloseText)
	joinWithin(t, served, "HandleLSPConnection")

	if manager.ensureCalls != 0 {
		t.Fatalf("GetOrEnsureExecution calls = %d, want 0 for an unsupported runtime", manager.ensureCalls)
	}
}

func TestHandleLSPConnectionDoesNotKeepLeaseWhenAgentctlIsMissing(t *testing.T) {
	manager := &recordingLSPLifecycleManager{
		runtimeName: agentruntime.RuntimeStandalone,
		execution:   &lifecycle.AgentExecution{ID: "execution", SessionID: "no-agentctl", RuntimeName: agentruntime.RuntimeStandalone},
	}
	handler := newLSPHandler(manager, nil, testLogger(), 1, nil, nil)

	conn, served := dialLSPHandler(t, handler, "/lsp/no-agentctl?language=go")
	expectLSPCloseCode(t, conn, lspCloseSessionNotFound, "failed to connect to task host LSP stream")
	joinWithin(t, served, "HandleLSPConnection")

	if handler.HasActiveLSPLease("no-agentctl") {
		t.Fatal("LSP lease retained for an execution without an agentctl client")
	}
}
