package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// TestIdentityAdvertisesDetachedContinuity pins that this build's advertised
// capability set includes detached-continuity.v1 (system design part 01
// "Capability compatibility"), so a backend that requires it can adopt.
func TestIdentityAdvertisesDetachedContinuity(t *testing.T) {
	if !slices.Contains(SurvivalCapabilities, "detached-continuity.v1") {
		t.Fatalf("SurvivalCapabilities = %v, want it to contain detached-continuity.v1", SurvivalCapabilities)
	}
}

// dialAgentStreamWithAttachID dials the agent stream WebSocket with the
// given attach_id query parameter (empty means the launch-stream form with
// no attach_id at all, confirmed at once).
func dialAgentStreamWithAttachID(t *testing.T, server *httptest.Server, attachID string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/agent/stream"
	if attachID != "" {
		wsURL += "?attach_id=" + attachID
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial WebSocket: %v", err)
	}
	return conn
}

// TestAgentStreamConfirmEndpoint covers the three Confirm branches over the
// real HTTP/WS surface: first confirmation (204), idempotent retry (204),
// and a mismatched attach_id (409 ATTACH_NOT_CURRENT, no change).
func TestAgentStreamConfirmEndpoint(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.router)
	defer server.Close()

	conn := dialAgentStreamWithAttachID(t, server, "attach-1")
	defer func() { _ = conn.Close() }()

	postConfirm := func(attachID string) int {
		body, err := json.Marshal(AgentStreamConfirmRequest{AttachID: attachID})
		if err != nil {
			t.Fatalf("marshal confirm request: %v", err)
		}
		resp, err := http.Post(server.URL+"/api/v1/agent/stream/confirm", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST confirm: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}

	if !waitForAttachmentCurrent(t, s, "attach-1") {
		t.Fatal("stream never became current with attach_id attach-1")
	}

	if got := postConfirm("attach-2"); got != http.StatusConflict {
		t.Fatalf("confirm with wrong attach_id = %d, want 409", got)
	}
	if s.procMgr.AttachmentStatus().Confirmed {
		t.Fatal("a mismatched confirm must not change anything")
	}

	if got := postConfirm("attach-1"); got != http.StatusNoContent {
		t.Fatalf("first confirm = %d, want 204", got)
	}
	if !s.procMgr.AttachmentStatus().Confirmed {
		t.Fatal("expected the stream confirmed after a matching confirm")
	}

	if got := postConfirm("attach-1"); got != http.StatusNoContent {
		t.Fatalf("retried confirm = %d, want 204 (idempotent)", got)
	}
}

// TestAgentStreamSupersedes pins that a second connection supersedes the
// first: the first is closed with code 4001 reason superseded, and the
// second becomes current.
func TestAgentStreamSupersedes(t *testing.T) {
	s := newTestServer(t)
	server := httptest.NewServer(s.router)
	defer server.Close()

	first := dialAgentStreamWithAttachID(t, server, "")
	defer func() { _ = first.Close() }()

	closeCode := make(chan int, 1)
	first.SetCloseHandler(func(code int, _ string) error {
		closeCode <- code
		return nil
	})
	go func() {
		for {
			if _, _, err := first.ReadMessage(); err != nil {
				return
			}
		}
	}()

	second := dialAgentStreamWithAttachID(t, server, "")
	defer func() { _ = second.Close() }()

	select {
	case code := <-closeCode:
		if code != process.CloseCodeSuperseded {
			t.Fatalf("first stream close code = %d, want %d", code, process.CloseCodeSuperseded)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first stream was never closed as superseded")
	}
}

// waitForAttachmentCurrent polls AttachmentStatus until a current stream with
// the given attach_id is observed, or fails the test after a short deadline.
func waitForAttachmentCurrent(t *testing.T, s *Server, attachID string) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status := s.procMgr.AttachmentStatus()
		if status.Current && status.AttachID == attachID {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}
