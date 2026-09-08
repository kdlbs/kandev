package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.5
func TestAgentStreamWriterReleasesMCPRequestAfterWriteFailure(t *testing.T) {
	connectionCh := make(chan *websocket.Conn, 1)
	releaseServer := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connectionCh <- conn
		<-releaseServer
	}))
	t.Cleanup(func() {
		close(releaseServer)
		httpServer.Close()
	})

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial WebSocket: %v", err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })
	serverConn := <-connectionCh

	log := newTestLogger()
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	server := &Server{mcpBackendClient: backend, logger: log}
	var wg sync.WaitGroup
	wg.Add(1)
	go server.runAgentStreamWriter(
		context.Background(), serverConn, "stream-write-failure", nil,
		backend.GetRequestChannel(), func([]byte) error { return errors.New("socket write failed") }, &wg,
	)

	errCh := make(chan error, 1)
	go func() {
		errCh <- backend.RequestPayload(context.Background(), "mcp.tools.call", nil, nil)
	}()

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "failed to write MCP request") {
			t.Fatalf("request error = %v, want write failure", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("MCP request remained blocked after stream write failure")
	}
	wg.Wait()
}

// @covers AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.5
func TestAgentStreamWSDisconnectReleasesDeliveredMCPRequest(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir()}
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	server := httptest.NewServer(NewServer(cfg, process.NewManager(cfg, log), nil, backend, log).router)
	t.Cleanup(server.Close)
	conn := dialTestWS(t, server)

	errCh := make(chan error, 1)
	go func() {
		errCh <- backend.RequestPayload(context.Background(), "mcp.tools.call", nil, nil)
	}()

	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read MCP request: %v", err)
	}
	var request ws.Message
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("decode MCP request: %v", err)
	}
	if request.Type != ws.MessageTypeRequest {
		t.Fatalf("message type = %q, want request", request.Type)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close stream: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil || !strings.Contains(err.Error(), "agent stream disconnected") {
			t.Fatalf("request error = %v, want agent stream disconnected", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("delivered MCP request remained blocked after stream disconnect")
	}
}
