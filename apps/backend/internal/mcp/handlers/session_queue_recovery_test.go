package handlers

import (
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
	sessions map[string]*models.TaskSession
	received *messagequeue.QueueRecoveryScope
}

func (r *recoveryHandlerSessions) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if session := r.sessions[id]; session != nil {
		return session, nil
	}
	return nil, errors.New("session not found")
}

func (r *recoveryHandlerSessions) RecoverTaskSessionQueue(
	_ context.Context, scope messagequeue.QueueRecoveryScope,
) (*messagequeue.QueueRecoveryResult, error) {
	r.received = &scope
	return &messagequeue.QueueRecoveryResult{
		Receipt: messagequeue.QueueRecoveryReceipt{ID: "receipt", EntryCount: 1},
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
		Entries []messagequeue.QueueRecoveryEntry `json:"entries"`
	}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse recovery response: %v", err)
	}
	if repo.received == nil || repo.received.TaskID != "task" || repo.received.WorkspaceID != "workspace" ||
		repo.received.SourceSessionID != "source" || repo.received.DestinationSessionID != "primary" {
		t.Fatalf("recovery scope = %#v", repo.received)
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
