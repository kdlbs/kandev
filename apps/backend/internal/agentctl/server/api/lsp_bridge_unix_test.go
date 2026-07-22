//go:build !windows

package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

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
