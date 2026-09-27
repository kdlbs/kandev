package api

import (
	"bytes"
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
		if !errors.Is(err, mcpserver.ErrKandevCallOutcomeUnknown) {
			t.Fatalf("request error = %v, want %v", err, mcpserver.ErrKandevCallOutcomeUnknown)
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
		if !errors.Is(err, mcpserver.ErrKandevCallOutcomeUnknown) {
			t.Fatalf("request error = %v, want %v", err, mcpserver.ErrKandevCallOutcomeUnknown)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("delivered MCP request remained blocked after stream disconnect")
	}
}

// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.1
func TestAgentStreamSupersedeReleasesDeliveredMCPRequestAsUnknownOutcome(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir()}
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	server := httptest.NewServer(NewServer(cfg, process.NewManager(cfg, log), nil, backend, log).router)
	t.Cleanup(server.Close)
	first := dialTestWS(t, server)
	t.Cleanup(func() { _ = first.Close() })

	errCh := make(chan error, 1)
	go func() {
		errCh <- backend.RequestPayload(context.Background(), "mcp.tools.call", nil, nil)
	}()

	_ = first.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := first.ReadMessage(); err != nil {
		t.Fatalf("read MCP request on first stream: %v", err)
	}

	second := dialTestWS(t, server)
	t.Cleanup(func() { _ = second.Close() })

	select {
	case err := <-errCh:
		if !errors.Is(err, mcpserver.ErrKandevCallOutcomeUnknown) {
			t.Fatalf("request error = %v, want %v", err, mcpserver.ErrKandevCallOutcomeUnknown)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("delivered MCP request remained blocked after stream supersede")
	}
}

// TestAgentStreamWriterGatesRequestChUntilConfirmed covers system design part
// 2 "Sent and not sent" step 1: the writer reads requestCh only while its
// stream is current and confirmed, so an unconfirmed reconnect-coordinator
// stream (opened with attach_id) never carries a Kandev call until the
// backend confirms it.
// @covers AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.1
func TestAgentStreamWriterGatesRequestChUntilConfirmed(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir()}
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := NewServer(cfg, process.NewManager(cfg, log), nil, backend, log)
	server := httptest.NewServer(s.router)
	t.Cleanup(server.Close)

	conn := dialAgentStreamWithAttachID(t, server, "attach-1")
	t.Cleanup(func() { _ = conn.Close() })
	if !waitForAttachmentCurrent(t, s, "attach-1") {
		t.Fatal("stream never became current with attach_id attach-1")
	}

	// A gorilla/websocket read error is permanent (every later call on the
	// same Conn returns it again), so a single long-lived reader goroutine
	// feeds a channel instead of the test using a short read deadline that
	// would otherwise poison the connection for the post-confirm read below.
	msgCh := make(chan []byte, 1)
	go func() {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		msgCh <- data
	}()

	requestErrCh := make(chan error, 1)
	go func() {
		requestErrCh <- backend.RequestPayload(context.Background(), "mcp.tools.call", nil, nil)
	}()

	select {
	case <-msgCh:
		t.Fatal("unconfirmed stream delivered an MCP request before confirm")
	case err := <-requestErrCh:
		t.Fatalf("RequestPayload settled before confirm: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	confirmBody, err := json.Marshal(AgentStreamConfirmRequest{AttachID: "attach-1"})
	if err != nil {
		t.Fatalf("marshal confirm request: %v", err)
	}
	resp, err := http.Post(server.URL+"/api/v1/agent/stream/confirm", "application/json", bytes.NewReader(confirmBody))
	if err != nil {
		t.Fatalf("POST confirm: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("confirm status = %d, want 204", resp.StatusCode)
	}

	var data []byte
	select {
	case data = <-msgCh:
	case err := <-requestErrCh:
		t.Fatalf("RequestPayload settled before delivering the request: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("MCP request was not delivered after confirm")
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
	case err := <-requestErrCh:
		if !errors.Is(err, mcpserver.ErrKandevCallOutcomeUnknown) {
			t.Fatalf("request error = %v, want %v", err, mcpserver.ErrKandevCallOutcomeUnknown)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delivered MCP request remained blocked after stream close")
	}
}
