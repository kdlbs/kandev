//go:build !windows

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTaskLSPWatchTerminatesWhenCredentialRotates(t *testing.T) {
	source := newCredentialState("initial-token")
	srv := newTestServer(t)
	srv.SetCredentialSource(source)
	httpServer := httptest.NewServer(srv.Router())
	defer httpServer.Close()
	headers := http.Header{}
	headers.Set("Authorization", "Bearer initial-token")
	headers.Set(taskLSPTaskIDHeader, "task-1")
	endpoint := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/lsp/languages/kotlin/watch"
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, headers)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read initial snapshot: %v", err)
	}
	if _, _, err := source.Rotate("initial-token"); err != nil {
		t.Fatal(err)
	}
	assertConnectionClosedByServer(t, conn, 2*time.Second)
}
