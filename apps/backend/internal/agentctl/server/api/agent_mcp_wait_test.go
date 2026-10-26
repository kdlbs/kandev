package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func newWaitingBackend(t *testing.T, procMgr *process.Manager) *mcpserver.ChannelBackendClient {
	t.Helper()
	backend := mcpserver.NewChannelBackendClient(newTestLogger())
	backend.SetAttachmentSnapshotter(func() mcpserver.AttachmentSnapshot {
		snap := procMgr.AttachmentSnapshot()
		return mcpserver.AttachmentSnapshot{Attached: snap.Attached, Ready: snap.Ready, Epoch: snap.Epoch, ChangeCh: snap.ChangeCh}
	})
	t.Cleanup(backend.Close)
	return backend
}

func TestMCPCallWaitsForStreamAndIsAnsweredAfterAttach(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir()}
	procMgr := process.NewManager(cfg, log)
	backend := newWaitingBackend(t, procMgr)
	server := httptest.NewServer(NewServer(cfg, procMgr, nil, backend, log).router)
	t.Cleanup(server.Close)

	errCh := make(chan error, 1)
	go func() { errCh <- backend.RequestPayload(context.Background(), "mcp.tools.call", nil, nil) }()
	select {
	case err := <-errCh:
		t.Fatalf("call returned while detached: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	conn := dialTestWS(t, server)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read MCP request after attach: %v", err)
	}
	var request ws.Message
	if err := json.Unmarshal(data, &request); err != nil {
		t.Fatalf("decode: %v", err)
	}
	response, err := ws.NewResponse(request.ID, request.Action, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(response)
	if err := conn.WriteMessage(1, payload); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("call error = %v, want success", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call not answered after stream attached")
	}
}

func TestMCPCallBoundToEndedStreamDoesNotResendAndReportsUnknownOutcome(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{Port: 0, WorkDir: t.TempDir()}
	procMgr := process.NewManager(cfg, log)
	backend := newWaitingBackend(t, procMgr)
	server := httptest.NewServer(NewServer(cfg, procMgr, nil, backend, log).router)
	t.Cleanup(server.Close)
	conn := dialTestWS(t, server)
	waitForReady(t, procMgr)

	errCh := make(chan error, 1)
	go func() { errCh <- backend.RequestPayload(context.Background(), "mcp.tools.call", nil, nil) }()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read MCP request: %v", err)
	}
	_ = conn.Close()
	select {
	case err := <-errCh:
		if !errors.Is(err, mcpserver.ErrKandevCallOutcomeUnknown) {
			t.Fatalf("error = %v, want outcome unknown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bound call not failed after stream end")
	}
}

func waitForReady(t *testing.T, procMgr *process.Manager) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !procMgr.AttachmentSnapshot().Ready {
		if time.Now().After(deadline) {
			t.Fatal("stream writer never became ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
