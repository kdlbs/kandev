//go:build !windows

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

var errForcedWebSocketWrite = errors.New("forced WebSocket write failure")

type writeFailingConn struct {
	net.Conn
	failWrites atomic.Bool
}

func (c *writeFailingConn) Write(data []byte) (int, error) {
	if c.failWrites.Load() {
		return 0, errForcedWebSocketWrite
	}
	return c.Conn.Write(data)
}

type writeFailingListener struct {
	net.Listener
	accepted chan *writeFailingConn
}

func (l *writeFailingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	wrapped := &writeFailingConn{Conn: conn}
	l.accepted <- wrapped
	return wrapped, nil
}

func TestHandleLSPStreamBridgesFramesAndStopsOwnedProcess(t *testing.T) {
	binDir := t.TempDir()
	workDir := t.TempDir()
	home := t.TempDir()
	serverPath := filepath.Join(binDir, "kotlin-lsp")
	if err := os.WriteFile(serverPath, []byte("#!/bin/sh\nexec /bin/cat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)

	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: workDir, SessionID: "session-1"}
	procMgr := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = procMgr.StopForTeardown(ctx)
	})
	s := NewServer(cfg, procMgr, nil, nil, log)
	ts := httptest.NewServer(s.router)
	t.Cleanup(ts.Close)

	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(ts.URL, "http")+"/api/v1/lsp/stream?language=kotlin",
		nil,
	)
	if err != nil {
		t.Fatalf("dial lsp stream: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, ready, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ready: %v", err)
	}
	var status struct {
		Status       string   `json:"status"`
		Workspace    string   `json:"workspacePath"`
		WorkspaceURI string   `json:"workspaceUri"`
		RepoSubpaths []string `json:"repoSubpaths"`
	}
	if err := json.Unmarshal(ready, &status); err != nil {
		t.Fatalf("decode ready: %v", err)
	}
	if status.Status != "ready" || status.Workspace != workDir || status.WorkspaceURI != workspaceFileURI(workDir) || status.RepoSubpaths == nil {
		t.Fatalf("ready status = %v", status)
	}

	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write LSP payload: %v", err)
	}
	_, echoed, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read LSP payload: %v", err)
	}
	if string(echoed) != string(payload) {
		t.Fatalf("echoed payload = %s, want %s", echoed, payload)
	}

	_ = conn.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(procMgr.ListProcesses("session-1")) == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("LSP process remains tracked after WebSocket close: %v", procMgr.ListProcesses("session-1"))
}

func TestHandleLSPStreamStopsProcessWhenForwardingToWebSocketFails(t *testing.T) {
	binDir := t.TempDir()
	serverPath := filepath.Join(binDir, "kotlin-lsp")
	script := "#!/bin/sh\nIFS= read -r _\nprintf 'Content-Length: 2\\r\\n\\r\\n{}'\nexec /bin/cat >/dev/null\n"
	if err := os.WriteFile(serverPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", binDir)

	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), SessionID: "session-write-failure"}
	procMgr := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = procMgr.StopForTeardown(ctx)
	})
	server := NewServer(cfg, procMgr, nil, nil, log)
	handlerReturned := make(chan struct{})
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		server.router.ServeHTTP(writer, request)
		close(handlerReturned)
	})
	httpServer := httptest.NewUnstartedServer(handler)
	listener := &writeFailingListener{
		Listener: httpServer.Listener,
		accepted: make(chan *writeFailingConn, 1),
	}
	httpServer.Listener = listener
	httpServer.Start()
	t.Cleanup(httpServer.Close)

	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(httpServer.URL, "http")+"/api/v1/lsp/stream?language=kotlin",
		nil,
	)
	if err != nil {
		t.Fatalf("dial lsp stream: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	serverConn := <-listener.accepted

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read ready: %v", err)
	}
	serverConn.failWrites.Store(true)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{}`)); err != nil {
		t.Fatalf("write trigger payload: %v", err)
	}

	select {
	case <-handlerReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("LSP handler remained blocked after the stdout forwarder exited")
	}
	if processes := procMgr.ListProcesses(cfg.SessionID); len(processes) != 0 {
		t.Fatalf("LSP process remains tracked after forwarder exit: %v", processes)
	}
}
