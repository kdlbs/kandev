package client

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestDialLSPUsesTaskHostStreamWithAuthHeaders(t *testing.T) {
	var gotPath string
	var gotLanguage string
	var gotAutoInstall string
	var gotAuth string
	var gotInstanceID string

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotLanguage = r.URL.Query().Get("language")
		gotAutoInstall = r.URL.Query().Get("autoInstall")
		gotAuth = r.Header.Get("Authorization")
		gotInstanceID = r.Header.Get("X-Instance-ID")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(server.Close)

	hostPort := strings.TrimPrefix(server.URL, "http://")
	host, portString, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("parse test server host: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}

	client := NewClient(host, port, newTestLogger(),
		WithAuthToken("secret-token"),
		WithExecutionID("exec-1"),
	)
	conn, _, err := client.DialLSP(context.Background(), "python", true)
	if err != nil {
		t.Fatalf("DialLSP: %v", err)
	}
	_ = conn.Close()

	if gotPath != "/api/v1/lsp/stream" {
		t.Fatalf("path = %q, want /api/v1/lsp/stream", gotPath)
	}
	if gotLanguage != "python" || gotAutoInstall != "true" {
		t.Fatalf("query language=%q autoInstall=%q", gotLanguage, gotAutoInstall)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotInstanceID != "exec-1" {
		t.Fatalf("X-Instance-ID = %q", gotInstanceID)
	}
}
