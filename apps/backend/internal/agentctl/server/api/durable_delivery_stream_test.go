package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestLoadAgentStreamReplayAddsTransportCursor(t *testing.T) {
	log := newTestLogger()
	cfg := &config.InstanceConfig{
		Port:               0,
		WorkDir:            t.TempDir(),
		SessionID:          "session-1",
		InstanceID:         "instance-1",
		DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt"),
	}
	procMgr := process.NewManager(cfg, log)
	deliveryJournal, err := procMgr.DeliveryJournal()
	if err != nil {
		t.Fatalf("open delivery journal: %v", err)
	}
	t.Cleanup(func() { _ = deliveryJournal.Close() })

	payload, err := json.Marshal(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: "replayed"})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if _, err := deliveryJournal.Append(context.Background(), journal.Event{
		SessionID:     "session-1",
		IncarnationID: "session-1",
		StreamID:      "session-1",
		Type:          adapter.EventTypeMessageChunk,
		Payload:       payload,
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}

	replay, err := NewServer(cfg, procMgr, nil, nil, log).loadAgentStreamReplay(context.Background(), 0)
	if err != nil {
		t.Fatalf("load replay: %v", err)
	}
	if len(replay) != 1 {
		t.Fatalf("replay length = %d, want 1", len(replay))
	}
	if replay[0].DeliveryStreamID != "session-1" || replay[0].DeliverySequence != 1 {
		t.Fatalf("replay cursor = %q/%d", replay[0].DeliveryStreamID, replay[0].DeliverySequence)
	}
}

func newDurableDeliveryTestServer(t *testing.T) (*Server, *process.Manager, *journal.Journal) {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{
		Port:               0,
		WorkDir:            t.TempDir(),
		SessionID:          "session-1",
		InstanceID:         "instance-1",
		DurableJournalPath: filepath.Join(t.TempDir(), "delivery.bbolt"),
	}
	procMgr := process.NewManager(cfg, log)
	deliveryJournal, err := procMgr.DeliveryJournal()
	if err != nil {
		t.Fatalf("open delivery journal: %v", err)
	}
	t.Cleanup(func() { _ = deliveryJournal.Close() })
	return NewServer(cfg, procMgr, nil, nil, log), procMgr, deliveryJournal
}

func TestDeliveryAPIRejectsForeignStreamIdentity(t *testing.T) {
	server, _, _ := newDurableDeliveryTestServer(t)

	for _, tc := range []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "status", method: http.MethodGet, target: "/api/v1/agent/delivery?stream_id=foreign-stream"},
		{name: "replay", method: http.MethodGet, target: "/api/v1/agent/delivery/stream?stream_id=foreign-stream"},
		{name: "acknowledgement", method: http.MethodPost, target: "/api/v1/agent/delivery/stream/ack", body: `{"stream_id":"foreign-stream","sequence":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()

			server.Router().ServeHTTP(resp, req)

			if resp.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusConflict, resp.Body.String())
			}
			var payload map[string]string
			if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if payload["code"] != "OWNER_MISMATCH" {
				t.Fatalf("error code = %q, want OWNER_MISMATCH", payload["code"])
			}
		})
	}
}

func TestDeliveryAPIRejectsForeignSubmissionIdentity(t *testing.T) {
	server, _, deliveryJournal := newDurableDeliveryTestServer(t)

	requestBody := `{"id":"submission-1","session_id":"other-session","incarnation_id":"other-incarnation","harness_generation":1,"hash":"hash-1","payload":"cHJvbXB0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/submissions", bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.Router().ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusConflict, resp.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != "OWNER_MISMATCH" {
		t.Fatalf("error code = %q, want OWNER_MISMATCH", payload["code"])
	}
	if _, err := deliveryJournal.GetSubmission(context.Background(), "submission-1"); err == nil {
		t.Fatal("foreign submission was admitted")
	}
}

func TestDeliveryAPIRejectsForeignStoredSubmission(t *testing.T) {
	server, _, deliveryJournal := newDurableDeliveryTestServer(t)
	if _, err := deliveryJournal.PutSubmission(context.Background(), journal.Submission{
		ID:                "foreign-submission",
		SessionID:         "other-session",
		IncarnationID:     "other-incarnation",
		HarnessGeneration: 1,
		Hash:              "hash-1",
		Payload:           []byte("prompt"),
	}); err != nil {
		t.Fatalf("store foreign submission: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/submissions/foreign-submission", nil)
	resp := httptest.NewRecorder()
	server.Router().ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d: %s", resp.Code, http.StatusConflict, resp.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["code"] != "OWNER_MISMATCH" {
		t.Fatalf("error code = %q, want OWNER_MISMATCH", payload["code"])
	}
}
