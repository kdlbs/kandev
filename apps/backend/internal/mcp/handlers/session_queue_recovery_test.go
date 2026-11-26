package handlers

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type recoveryHandlerSessions struct {
	SessionRepository
	sessions     map[string]*models.TaskSession
	received     *messagequeue.QueueRecoveryScope
	lookupErrors map[string]error
}

func (r *recoveryHandlerSessions) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if err := r.lookupErrors[id]; err != nil {
		return nil, err
	}
	if session := r.sessions[id]; session != nil {
		return session, nil
	}
	return nil, models.ErrTaskSessionNotFound
}

func (r *recoveryHandlerSessions) RecoverTaskSessionQueue(
	_ context.Context, scope messagequeue.QueueRecoveryScope,
) (*messagequeue.QueueRecoveryResult, error) {
	r.received = &scope
	return &messagequeue.QueueRecoveryResult{
		Receipt: messagequeue.QueueRecoveryReceipt{ID: "receipt", TaskID: "task", WorkspaceID: "workspace", SourceSessionID: "source", DestinationSessionID: "primary", EntryCount: 1, SnapshotSHA256: "snapshot-hash"},
		Entries: []messagequeue.QueuedMessage{{
			ID: "entry", SessionID: "source", TaskID: "task", Content: "exact body",
			Metadata: map[string]interface{}{"origin": "peer", "credential": "must stay private"},
		}},
	}, nil
}

func TestRecoverSessionQueueReturnsScopedSnapshotWithoutPrivateMetadata(t *testing.T) {
	repo := &recoveryHandlerSessions{sessions: map[string]*models.TaskSession{
		"primary": {ID: "primary", TaskID: "task", IsPrimary: true},
		"source":  {ID: "source", TaskID: "task", State: models.TaskSessionStateCompleted},
	}}
	h := &Handlers{sessionRepo: repo}
	resp, err := h.handleRecoverSessionQueue(queuePrincipal("workspace", "task", "primary"),
		makeWSMessage(t, ws.ActionMCPRecoverSessionQueue, map[string]interface{}{
			"task_id": "task", "caller_session_id": "primary", "target_session_id": "source",
		}))
	if err != nil {
		t.Fatalf("recover queue: %v", err)
	}
	var payload struct {
		Entries              []messagequeue.QueueRecoveryEntry `json:"entries"`
		TaskID               string                            `json:"task_id"`
		SourceSessionID      string                            `json:"source_session_id"`
		DestinationSessionID string                            `json:"destination_session_id"`
		ReadbackCount        int                               `json:"readback_count"`
		Receipt              messagequeue.QueueRecoveryReceipt `json:"receipt"`
	}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse recovery response: %v", err)
	}
	if repo.received == nil || repo.received.TaskID != "task" || repo.received.WorkspaceID != "workspace" ||
		repo.received.SourceSessionID != "source" || repo.received.DestinationSessionID != "primary" {
		t.Fatalf("recovery scope = %#v", repo.received)
	}
	if payload.TaskID != "task" || payload.SourceSessionID != "source" || payload.DestinationSessionID != "primary" || payload.ReadbackCount != 1 ||
		payload.Receipt.ID != "receipt" || payload.Receipt.TaskID != "task" || payload.Receipt.WorkspaceID != "workspace" ||
		payload.Receipt.SourceSessionID != "source" || payload.Receipt.DestinationSessionID != "primary" || payload.Receipt.SnapshotSHA256 != "snapshot-hash" {
		t.Fatalf("recovery identity = %+v", payload)
	}
	if len(payload.Entries) != 1 || payload.Entries[0].Content != "exact body" || payload.Entries[0].Origin != "peer" {
		t.Fatalf("recovery entries = %#v", payload.Entries)
	}
	if payload.Entries[0].ContentSHA256 == "" || string(resp.Payload) == "" {
		t.Fatalf("missing recovery hash or response: %#v", payload.Entries)
	}
	if strings.Contains(string(resp.Payload), "must stay private") {
		t.Fatal("private metadata leaked in recovery response")
	}
}

func TestRecoverSessionQueueRejectsCrossTaskTarget(t *testing.T) {
	repo := &recoveryHandlerSessions{sessions: map[string]*models.TaskSession{
		"primary": {ID: "primary", TaskID: "task", IsPrimary: true},
		"source":  {ID: "source", TaskID: "other", State: models.TaskSessionStateCompleted},
	}}
	h := &Handlers{sessionRepo: repo}
	resp, err := h.handleRecoverSessionQueue(queuePrincipal("workspace", "task", "primary"),
		makeWSMessage(t, ws.ActionMCPRecoverSessionQueue, map[string]interface{}{
			"task_id": "task", "caller_session_id": "primary", "target_session_id": "source",
		}))
	if err != nil {
		t.Fatalf("recover queue: %v", err)
	}
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	if repo.received != nil {
		t.Fatalf("cross-task recovery reached repository: %#v", repo.received)
	}
}

func TestRecoverSessionQueueSanitizesLookupFailures(t *testing.T) {
	req := makeWSMessage(t, ws.ActionMCPRecoverSessionQueue, map[string]interface{}{
		"task_id": "task", "caller_session_id": "primary", "target_session_id": "source",
	})
	repo := &recoveryHandlerSessions{sessions: map[string]*models.TaskSession{
		"primary": {ID: "primary", TaskID: "task", IsPrimary: true},
		"source":  {ID: "source", TaskID: "task", State: models.TaskSessionStateCompleted},
	}, lookupErrors: map[string]error{"primary": errors.New("private database address")}}
	h := &Handlers{sessionRepo: repo}
	for _, id := range []string{"primary", "source"} {
		if id == "source" {
			delete(repo.lookupErrors, "primary")
			repo.lookupErrors["source"] = errors.New("private database address")
		}
		response, err := h.handleRecoverSessionQueue(queuePrincipal("workspace", "task", "primary"), req)
		if err != nil {
			t.Fatal(err)
		}
		assertWSError(t, response, ws.ErrorCodeInternalError)
		if repo.received != nil || bytes.Contains(response.Payload, []byte("private database address")) {
			t.Fatalf("%s lookup reached recovery or leaked details", id)
		}
	}
}

func TestQueueRecoveryConflictDoesNotExposeClaimDetails(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{ID: "request", Action: ws.ActionMCPRecoverSessionQueue}
	response, err := h.queueRecoveryError(msg, "source", errors.Join(messagequeue.ErrQueueRecoveryConflict, errors.New("private claim table")))
	if err != nil {
		t.Fatal(err)
	}
	assertWSError(t, response, ws.ErrorCodeValidation)
	if bytes.Contains(response.Payload, []byte("private claim table")) {
		t.Fatal("claim details leaked")
	}
}
