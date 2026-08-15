package websocket

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

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

func TestNewLSPHandlerInitializesTaskControllerAndLogger(t *testing.T) {
	resolver := &gatewayFakeAttachmentResolver{}
	handler := NewLSPHandler(resolver, testLogger())
	if handler.controller != resolver {
		t.Fatal("task attachment resolver was not retained")
	}
	if handler.logger == nil {
		t.Fatal("logger was not initialized")
	}
}

func TestHandleLSPConnectionMapsControllerErrorsBeforeUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantError  string
	}{
		{
			name: "unsupported language", err: sharedlsp.ErrUnsupportedLanguage,
			wantStatus: http.StatusBadRequest, wantError: sharedlsp.ErrUnsupportedLanguage.Error(),
		},
		{
			name: "server not ready", err: sharedlsp.ErrAttachmentNotReady,
			wantStatus: http.StatusConflict, wantError: sharedlsp.ErrAttachmentNotReady.Error(),
		},
		{
			name: "task not ready", err: sharedlsp.ErrTaskNotReady,
			wantStatus: http.StatusUnprocessableEntity, wantError: sharedlsp.ErrTaskNotReady.Error(),
		},
		{
			name: "unsupported executor", err: sharedlsp.ErrExecutorUnsupported,
			wantStatus: http.StatusUnprocessableEntity, wantError: sharedlsp.ErrExecutorUnsupported.Error(),
		},
		{
			name: "hidden task", err: repoerrors.ErrTaskNotFound,
			wantStatus: http.StatusNotFound, wantError: lspUnavailableText,
		},
		{
			name: "internal", err: errors.New("private runtime failure"),
			wantStatus: http.StatusInternalServerError,
			wantError:  lspUnavailableText,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &gatewayFakeAttachmentResolver{err: tt.err}
			handler := NewLSPHandler(resolver, testLogger())
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(
				http.MethodGet, "/lsp/tasks/task-1/kotlin/attach", nil,
			)
			c.Params = gin.Params{
				{Key: "taskId", Value: "task-1"},
				{Key: "language", Value: "kotlin"},
			}
			handler.HandleLSPConnection(c)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if got := decodeErrorBody(t, recorder.Body.Bytes()); got != tt.wantError {
				t.Fatalf("error = %q, want %q", got, tt.wantError)
			}
			if resolver.taskID != "task-1" || resolver.language != "kotlin" {
				t.Fatalf("resolver called with task=%q language=%q", resolver.taskID, resolver.language)
			}
		})
	}
}

func TestHandleLSPConnectionReturnsSafeErrorWhenTaskHostAttachFails(t *testing.T) {
	for _, test := range []struct {
		name     string
		response *http.Response
		status   int
	}{
		{name: "transport failure", status: http.StatusBadGateway},
		{
			name: "task-host status", response: &http.Response{StatusCode: http.StatusConflict},
			status: http.StatusConflict,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gatewayHost := &gatewayFakeTaskHost{
				dialErr: errors.New("task host unavailable"), dialResponse: test.response,
			}
			resolver := &gatewayFakeAttachmentResolver{target: &sharedlsp.AttachmentTarget{
				Host: gatewayHost, Language: "go", Generation: 3,
			}}
			handler := NewLSPHandler(resolver, testLogger())
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/lsp/tasks/task-1/go/attach", nil)
			c.Params = gin.Params{
				{Key: "taskId", Value: "task-1"},
				{Key: "language", Value: "go"},
			}

			handler.HandleLSPConnection(c)

			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d", recorder.Code, test.status)
			}
			if got := decodeErrorBody(t, recorder.Body.Bytes()); got != lspHostUnavailableText {
				t.Fatalf("error = %q", got)
			}
		})
	}
}

func TestProxyLSPConnectionsForwardsMessagesInBothDirections(t *testing.T) {
	handler := &LSPHandler{logger: testLogger()}

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
	handler := &LSPHandler{logger: testLogger()}

	browser, handlerBrowserSide := newTerminalWSPair(t)
	handlerUpstreamSide, upstream := newTerminalWSPair(t)

	done := spawnJoinable(t, "proxyLSPConnections", func() { _ = browser.Close() }, func() {
		handler.proxyLSPConnections(handlerBrowserSide, handlerUpstreamSide, "sess-lsp", "go")
	})

	const taskHostCloseCode = 4007
	closeFrame := gorillaws.FormatCloseMessage(taskHostCloseCode, "task-host failure")
	if err := upstream.WriteControl(
		gorillaws.CloseMessage, closeFrame, time.Now().Add(wsTestTimeout),
	); err != nil {
		t.Fatalf("upstream close: %v", err)
	}

	expectLSPCloseCode(t, browser, taskHostCloseCode, "task-host failure")
	joinWithin(t, done, "proxyLSPConnections")
}
