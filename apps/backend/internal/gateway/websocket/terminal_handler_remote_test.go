package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
)

// shellStartRequest mirrors the JSON body agentctl receives for shell starts.
type shellStartRequest struct {
	TerminalID string `json:"terminal_id"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
}

// fakeShellAgentctl serves the per-terminal shell endpoints an executor's
// agentctl exposes: a start call plus a binary WebSocket stream.
type fakeShellAgentctl struct {
	server *httptest.Server
	shells chan *gorillaws.Conn

	mu     sync.Mutex
	starts []shellStartRequest
}

func newFakeShellAgentctl(t *testing.T, startStatus, streamStatus int) *fakeShellAgentctl {
	t.Helper()
	fake := &fakeShellAgentctl{shells: make(chan *gorillaws.Conn, 1)}
	fake.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/shell/terminal/start":
			var req shellStartRequest
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &req)
			fake.mu.Lock()
			fake.starts = append(fake.starts, req)
			fake.mu.Unlock()
			w.WriteHeader(startStatus)
		case strings.HasSuffix(r.URL.Path, "/stream"):
			if streamStatus != http.StatusOK {
				w.WriteHeader(streamStatus)
				return
			}
			conn, err := testWSUpgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			fake.shells <- conn
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	fake.server.Config.SetKeepAlivesEnabled(false)
	fake.server.Start()
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeShellAgentctl) startedShells() []shellStartRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]shellStartRequest(nil), f.starts...)
}

// serveOnce runs handle for a single request and returns the server plus a
// channel that closes when the handler has returned.
func serveOnce(t *testing.T, handle func(*gin.Context)) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	served := make(chan struct{})
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(served)
		c, _ := gin.CreateTestContext(w)
		c.Request = r
		handle(c)
	}))
	srv.Config.SetKeepAlivesEnabled(false)
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, served
}

// dialTerminal opens a terminal WebSocket against a one-shot handler server.
func dialTerminal(t *testing.T, srv *httptest.Server, path string) *gorillaws.Conn {
	t.Helper()
	conn, resp, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+path, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial terminal %s: %v", path, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// getTerminal performs a plain HTTP GET (no upgrade) against a one-shot handler.
func getTerminal(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: wsTestTimeout}
	resp, err := client.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}

func remoteExecutionFor(
	t *testing.T,
	manager *lifecycle.Manager,
	sessionID, serverURL string,
	log *logger.Logger,
) *lifecycle.AgentExecution {
	t.Helper()
	execution := addExecutionForURL(t, manager, sessionID, serverURL, "shell-token", log)
	execution.TaskEnvironmentID = "env-1"
	return execution
}

func TestHandleRemoteUserShellWSRequiresAgentctlClient(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	execution := &lifecycle.AgentExecution{ID: "exec-1", SessionID: "session-1", TaskEnvironmentID: "env-1"}

	srv, served := serveOnce(t, func(c *gin.Context) {
		handler.handleRemoteUserShellWS(c, execution, "env-1", "term-1")
	})
	status, body := getTerminal(t, srv, "/terminal/environment/env-1?terminalId=term-1")
	joinWithin(t, served, "handleRemoteUserShellWS")

	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", status, http.StatusNotFound)
	}
	if got := decodeErrorBody(t, []byte(body)); got != "remote execution not available" {
		t.Fatalf("error = %q", got)
	}
}

func TestHandleRemoteUserShellWSRejectsInvalidScriptBeforeStartingAShell(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, manager := newBridgeTestHandler(t, log)
	handler.scriptService = &stubScriptService{err: errors.New("no such script")}
	agentctl := newFakeShellAgentctl(t, http.StatusOK, http.StatusOK)
	execution := remoteExecutionFor(t, manager, "session-1", agentctl.server.URL, log)

	srv, served := serveOnce(t, func(c *gin.Context) {
		handler.handleRemoteUserShellWS(c, execution, "env-1", "term-1")
	})
	status, body := getTerminal(t, srv, "/terminal/environment/env-1?terminalId=term-1&scriptId=missing")
	joinWithin(t, served, "handleRemoteUserShellWS")

	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if got := decodeErrorBody(t, []byte(body)); got != "invalid script ID" {
		t.Fatalf("error = %q", got)
	}
	if starts := agentctl.startedShells(); len(starts) != 0 {
		t.Fatalf("agentctl shell starts = %v, want none for an invalid script", starts)
	}
}

func TestHandleRemoteUserShellWSReportsShellStartFailure(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, manager := newBridgeTestHandler(t, log)
	agentctl := newFakeShellAgentctl(t, http.StatusInternalServerError, http.StatusOK)
	execution := remoteExecutionFor(t, manager, "session-1", agentctl.server.URL, log)

	srv, served := serveOnce(t, func(c *gin.Context) {
		handler.handleRemoteUserShellWS(c, execution, "env-1", "term-1")
	})
	status, body := getTerminal(t, srv, "/terminal/environment/env-1?terminalId=term-1")
	joinWithin(t, served, "handleRemoteUserShellWS")

	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if got := decodeErrorBody(t, []byte(body)); !strings.Contains(got, "start shell terminal failed") {
		t.Fatalf("error = %q, want the agentctl start failure", got)
	}
}

func TestHandleRemoteUserShellWSReportsStreamFailure(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, manager := newBridgeTestHandler(t, log)
	agentctl := newFakeShellAgentctl(t, http.StatusOK, http.StatusNotFound)
	execution := remoteExecutionFor(t, manager, "session-1", agentctl.server.URL, log)

	srv, served := serveOnce(t, func(c *gin.Context) {
		handler.handleRemoteUserShellWS(c, execution, "env-1", "term-1")
	})
	status, body := getTerminal(t, srv, "/terminal/environment/env-1?terminalId=term-1")
	joinWithin(t, served, "handleRemoteUserShellWS")

	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if got := decodeErrorBody(t, []byte(body)); !strings.Contains(got, "shell terminal stream") {
		t.Fatalf("error = %q, want the stream connect failure", got)
	}
	if starts := agentctl.startedShells(); len(starts) != 1 {
		t.Fatalf("agentctl shell starts = %v, want exactly 1", starts)
	}
}

// The end-to-end remote shell path: start the terminal on agentctl, send the
// script's initial command, then bridge keystrokes and output in both directions.
func TestHandleRemoteUserShellWSBridgesAgentctlShell(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, manager := newBridgeTestHandler(t, log)
	handler.scriptService = &stubScriptService{
		script: &scripts.RepositoryScript{Name: "Dev server", Command: "npm run dev"},
	}
	agentctl := newFakeShellAgentctl(t, http.StatusOK, http.StatusOK)
	execution := remoteExecutionFor(t, manager, "session-1", agentctl.server.URL, log)

	srv, served := serveOnce(t, func(c *gin.Context) {
		handler.handleRemoteUserShellWS(c, execution, "env-1", "term-1")
	})
	browser := dialTerminal(t, srv, "/terminal/environment/env-1?terminalId=term-1&scriptId=script-1")

	shellTimer := time.NewTimer(wsTestTimeout)
	defer shellTimer.Stop()
	var shell *gorillaws.Conn
	select {
	case shell = <-agentctl.shells:
	case <-shellTimer.C:
		t.Fatal("agentctl never received a shell stream connection")
	}
	t.Cleanup(func() { _ = shell.Close() })

	starts := agentctl.startedShells()
	if len(starts) != 1 {
		t.Fatalf("agentctl shell starts = %v, want exactly 1", starts)
	}
	if starts[0].TerminalID != "term-1" || starts[0].Cols != 80 || starts[0].Rows != 24 {
		t.Fatalf("shell start = %+v, want term-1 at 80x24", starts[0])
	}

	if got := readBinary(t, shell); string(got) != "npm run dev\n" {
		t.Fatalf("initial command sent upstream = %q, want %q", got, "npm run dev\n")
	}

	if err := browser.WriteMessage(gorillaws.BinaryMessage, []byte("ls\r")); err != nil {
		t.Fatalf("browser write: %v", err)
	}
	if got := readBinary(t, shell); string(got) != "ls\r" {
		t.Fatalf("keystrokes forwarded upstream = %q, want %q", got, "ls\r")
	}

	if err := shell.WriteMessage(gorillaws.BinaryMessage, []byte("file.txt\r\n")); err != nil {
		t.Fatalf("shell write: %v", err)
	}
	if got := readBinary(t, browser); string(got) != "file.txt\r\n" {
		t.Fatalf("shell output forwarded to browser = %q, want %q", got, "file.txt\r\n")
	}

	if err := browser.Close(); err != nil {
		t.Fatalf("close browser: %v", err)
	}
	joinWithin(t, served, "handleRemoteUserShellWS")
}

// The local (host-executor) shell path: start a shell process for the terminal,
// upgrade, and bridge until the browser disconnects.
func TestHandleUserShellWSStartsShellAndBridgesUntilDisconnect(t *testing.T) {
	log, _ := observedTerminalLogger(t)
	handler, _ := newBridgeTestHandler(t, log)
	runner := newBridgeTestRunner(t, log)
	t.Cleanup(func() { _ = runner.StopUserShell(context.Background(), "env-1", "term-1") })

	execution := &lifecycle.AgentExecution{
		ID:                "exec-1",
		SessionID:         "session-1",
		TaskEnvironmentID: "env-1",
		WorkspacePath:     t.TempDir(),
	}
	srv, served := serveOnce(t, func(c *gin.Context) {
		handler.handleUserShellWS(c, execution, "env-1", "term-1", runner)
	})
	browser := dialTerminal(t, srv, "/terminal/environment/env-1?terminalId=term-1&label=Logs")

	shells := runner.ListUserShells("env-1")
	if len(shells) != 1 {
		t.Fatalf("user shells = %v, want exactly 1", shells)
	}
	if shells[0].Label != "Logs" {
		t.Fatalf("shell label = %q, want %q", shells[0].Label, "Logs")
	}

	if err := browser.Close(); err != nil {
		t.Fatalf("close browser: %v", err)
	}
	joinWithin(t, served, "handleUserShellWS")
}
