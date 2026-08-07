package websocket

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
	"github.com/kandev/kandev/internal/lsp/installer"
	"github.com/kandev/kandev/internal/lsp/protocol"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type recordingLSPMessageWriter struct {
	deadline        time.Time
	deadlineOnWrite time.Time
	messageType     int
	payload         []byte
}

type readDeadlineListener struct {
	net.Listener
	deadlines chan time.Time
}

func (l *readDeadlineListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &readDeadlineConn{Conn: conn, deadlines: l.deadlines}, nil
}

type readDeadlineConn struct {
	net.Conn
	deadlines chan time.Time
}

func (c *readDeadlineConn) SetReadDeadline(deadline time.Time) error {
	if deadline.After(time.Now()) {
		select {
		case c.deadlines <- deadline:
		default:
		}
	}
	return c.Conn.SetReadDeadline(deadline)
}

func (w *recordingLSPMessageWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func (w *recordingLSPMessageWriter) WriteMessage(messageType int, payload []byte) error {
	w.deadlineOnWrite = w.deadline
	w.messageType = messageType
	w.payload = payload
	return nil
}

func TestLSPTaskAttachmentRouteResolvesTaskLanguageAndOnlyDetaches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upgrader := gorillaws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upstream upgrade: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		messageType, payload, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(messageType, payload)
		}
	}))
	defer upstream.Close()

	host := &gatewayFakeTaskHost{attachURL: "ws" + strings.TrimPrefix(upstream.URL, "http")}
	resolver := &gatewayFakeAttachmentResolver{target: &sharedlsp.AttachmentTarget{
		Host: host, Language: "kotlin", Generation: 7,
	}}
	handler := NewLSPHandler(resolver, testLogger())
	router := gin.New()
	router.GET("/lsp/tasks/:taskId/:language/attach", handler.HandleLSPConnection)
	server := httptest.NewServer(router)
	defer server.Close()

	conn, _, err := gorillaws.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/lsp/tasks/task-1/kotlin/attach", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(gorillaws.TextMessage, []byte(`{"jsonrpc":"2.0","id":1}`)); err != nil {
		t.Fatal(err)
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if string(payload) != `{"jsonrpc":"2.0","id":1}` {
		t.Fatalf("payload = %s", payload)
	}
	if resolver.taskID != "task-1" || resolver.language != "kotlin" {
		t.Fatalf("resolved task=%q language=%q", resolver.taskID, resolver.language)
	}
	if host.dialLanguage != "kotlin" || host.dialGeneration != 7 || host.stopCalls != 0 {
		t.Fatalf("host dial=%q/%d stop=%d", host.dialLanguage, host.dialGeneration, host.stopCalls)
	}
}

func TestLSPProxySetsReadDeadlineForStaleConnectionCleanup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upgrader := gorillaws.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _, _ = conn.ReadMessage()
	}))
	defer upstream.Close()
	host := &gatewayFakeTaskHost{attachURL: "ws" + strings.TrimPrefix(upstream.URL, "http")}
	resolver := &gatewayFakeAttachmentResolver{target: &sharedlsp.AttachmentTarget{
		Host: host, Language: "kotlin", Generation: 1,
	}}
	handler := NewLSPHandler(resolver, testLogger())
	router := gin.New()
	router.GET("/lsp/tasks/:taskId/:language/attach", handler.HandleLSPConnection)
	server := httptest.NewUnstartedServer(router)
	deadlines := make(chan time.Time, 4)
	server.Listener = &readDeadlineListener{Listener: server.Listener, deadlines: deadlines}
	server.Start()
	defer server.Close()

	conn, _, err := gorillaws.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/lsp/tasks/task-1/kotlin/attach", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	select {
	case deadline := <-deadlines:
		if time.Until(deadline) <= 0 {
			t.Fatalf("read deadline = %v", deadline)
		}
	case <-time.After(time.Second):
		t.Fatal("proxy did not configure a read deadline")
	}
}

func TestLSPTaskAttachmentAuthorizationFailureHappensBeforeUpgradeOrDial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &gatewayFakeAttachmentResolver{err: repoerrors.ErrTaskNotFound}
	handler := NewLSPHandler(resolver, testLogger())
	router := gin.New()
	router.GET("/lsp/tasks/:taskId/:language/attach", handler.HandleLSPConnection)
	server := httptest.NewServer(router)
	defer server.Close()

	_, response, err := gorillaws.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/lsp/tasks/hidden/go/attach", nil,
	)
	if err == nil {
		t.Fatal("hidden task attachment unexpectedly upgraded")
	}
	if response == nil || response.StatusCode != http.StatusNotFound {
		t.Fatalf("response = %#v error=%v", response, err)
	}
	if resolver.taskID != "hidden" || resolver.language != "go" {
		t.Fatalf("resolver calls task=%q language=%q", resolver.taskID, resolver.language)
	}
}

func TestLSPTaskAttachmentUnrelatedNotFoundErrorStaysInternal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &gatewayFakeAttachmentResolver{err: errors.New("language-server binary not found")}
	handler := NewLSPHandler(resolver, testLogger())
	router := gin.New()
	router.GET("/lsp/tasks/:taskId/:language/attach", handler.HandleLSPConnection)
	server := httptest.NewServer(router)
	defer server.Close()

	_, response, err := gorillaws.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http")+"/lsp/tasks/task-1/go/attach", nil,
	)
	if err == nil {
		t.Fatal("attachment unexpectedly upgraded")
	}
	if response == nil || response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("response = %#v error=%v", response, err)
	}
}

func TestReadLSPMessageValidAndBoundedFraming(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	message, err := protocol.ReadMessage(bufio.NewReader(strings.NewReader(raw)))
	if err != nil || string(message) != body {
		t.Fatalf("message=%q error=%v", message, err)
	}
	for name, raw := range map[string]string{
		"missing": "Content-Type: application/json\r\n\r\n{}",
		"invalid": "Content-Length: abc\r\n\r\n{}",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := protocol.ReadMessage(bufio.NewReader(strings.NewReader(raw))); err == nil {
				t.Fatal("malformed frame accepted")
			}
		})
	}
	if _, err := protocol.ReadMessage(bufio.NewReader(bytes.NewReader(nil))); err != io.EOF {
		t.Fatalf("empty frame error = %v", err)
	}
}

func TestLspCommandViaRegistry(t *testing.T) {
	tests := map[string]string{
		"typescript": "typescript-language-server",
		"go":         "gopls",
		"rust":       "rust-analyzer",
		"python":     "pyright-langserver",
		"kotlin":     "kotlin-lsp",
		"unknown":    "",
	}
	for language, want := range tests {
		binary, _ := installer.LspCommand(language)
		if binary != want {
			t.Fatalf("LspCommand(%q)=%q want=%q", language, binary, want)
		}
	}
}

func TestWriteLSPProxyMessageSetsDeadlineBeforeWriting(t *testing.T) {
	writer := &recordingLSPMessageWriter{}
	startedAt := time.Now()
	if err := writeLSPProxyMessage(writer, gorillaws.TextMessage, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	if writer.deadlineOnWrite.Before(startedAt.Add(lspProxyWriteTimeout)) {
		t.Fatalf("write deadline = %v", writer.deadlineOnWrite)
	}
}

func TestForwardLSPClosePreservesTaskHostApplicationCode(t *testing.T) {
	writer := &recordingLSPMessageWriter{}
	handler := &LSPHandler{}
	handler.forwardLSPClose(writer, &gorillaws.CloseError{Code: 4007, Text: "task-host failure"})
	closeCode := int(writer.payload[0])<<8 | int(writer.payload[1])
	if closeCode != 4007 {
		t.Fatalf("close code = %d", closeCode)
	}
}

type gatewayFakeAttachmentResolver struct {
	taskID   string
	language string
	target   *sharedlsp.AttachmentTarget
	err      error
}

func (f *gatewayFakeAttachmentResolver) ResolveAttachment(
	_ context.Context,
	taskID, language string,
) (*sharedlsp.AttachmentTarget, error) {
	f.taskID, f.language = taskID, language
	return f.target, f.err
}

type gatewayFakeTaskHost struct {
	attachURL      string
	dialLanguage   string
	dialGeneration uint64
	stopCalls      int
}

func (f *gatewayFakeTaskHost) DiscoverLSP(context.Context) (*sharedlsp.DiscoveryResult, error) {
	return &sharedlsp.DiscoveryResult{State: sharedlsp.DetectionComplete}, nil
}

func (f *gatewayFakeTaskHost) RefreshTaskLSPWorkspace(context.Context) (*sharedlsp.WorkspaceUpdateResult, error) {
	return &sharedlsp.WorkspaceUpdateResult{}, nil
}

func (f *gatewayFakeTaskHost) StartTaskLSP(context.Context, sharedlsp.TaskHostStartRequest) (*sharedlsp.RuntimeSnapshot, error) {
	return nil, errors.New("unexpected start")
}

func (f *gatewayFakeTaskHost) RestartTaskLSP(context.Context, sharedlsp.TaskHostStartRequest) (*sharedlsp.RuntimeSnapshot, error) {
	return nil, errors.New("unexpected restart")
}

func (f *gatewayFakeTaskHost) UpdateTaskLSPConfiguration(
	context.Context,
	sharedlsp.TaskHostConfigurationRequest,
) (*sharedlsp.RuntimeSnapshot, error) {
	return nil, errors.New("unexpected configuration update")
}

func (f *gatewayFakeTaskHost) StopTaskLSP(context.Context, sharedlsp.TaskHostStopRequest) (*sharedlsp.RuntimeSnapshot, error) {
	f.stopCalls++
	return nil, errors.New("unexpected stop")
}

func (f *gatewayFakeTaskHost) TaskLSPSnapshot(context.Context, string) (*sharedlsp.RuntimeSnapshot, error) {
	return nil, errors.New("unexpected snapshot")
}

func (f *gatewayFakeTaskHost) WatchTaskLSP(
	context.Context,
	string,
	func(sharedlsp.RuntimeSnapshot) error,
) error {
	return errors.New("unexpected watch")
}

func (f *gatewayFakeTaskHost) DialTaskLSPAttach(
	ctx context.Context,
	language string,
	generation uint64,
) (*gorillaws.Conn, *http.Response, error) {
	f.dialLanguage, f.dialGeneration = language, generation
	return gorillaws.DefaultDialer.DialContext(ctx, f.attachURL, nil)
}
