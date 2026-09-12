//go:build !windows

package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestLSPStreamTerminatesWhenCredentialRotates completes the fencing coverage
// for the long-lived upgrades: the language-server bridge pipes client frames
// straight into a subprocess with the workspace as its root, so a superseded
// holder keeping one open still reads and edits the workspace through it.
func TestLSPStreamTerminatesWhenCredentialRotates(t *testing.T) {
	binDir := t.TempDir()
	serverPath := filepath.Join(binDir, "kotlin-lsp")
	if err := os.WriteFile(serverPath, []byte("#!/bin/sh\nexec /bin/cat\n"), 0o755); err != nil {
		t.Fatalf("write fake language server: %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	// Prepend rather than replace: the package's later tests resolve git
	// through a lookup cached the first time any test triggers it, so a test
	// that empties PATH decides that answer for every test after it.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	source := newCredentialState("initial-token")
	srv := newTestServer(t)
	srv.SetCredentialSource(source)

	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()

	conn := dialWithAuth(t, httpServer, "/api/v1/lsp/stream?language=kotlin", "initial-token")
	defer func() { _ = conn.Close() }()

	// Read the ready frame and echo one request through the bridge, so the
	// post-rotation closure cannot be mistaken for a bridge that never came up.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read ready frame: %v", err)
	}
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("pre-rotation write: %v", err)
	}
	_, echoed, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("pre-rotation read: %v", err)
	}
	if string(echoed) != string(payload) {
		t.Fatalf("echoed payload = %s, want %s", echoed, payload)
	}

	if _, _, err := source.Rotate("initial-token"); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	assertConnectionClosedByServer(t, conn, 2*time.Second)
}
