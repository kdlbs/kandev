package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type cleanupHandlerSessions struct {
	SessionRepository
	sessions map[string]*models.TaskSession
	receipt  *models.TaskSessionCleanupReceipt
}

func (r *cleanupHandlerSessions) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if session := r.sessions[id]; session != nil {
		return session, nil
	}
	return nil, errors.New("session not found")
}

func (r *cleanupHandlerSessions) GetTaskSessionCleanupReceipt(_ context.Context, taskID, sessionID string) (*models.TaskSessionCleanupReceipt, error) {
	if r.receipt != nil && r.receipt.TaskID == taskID && r.receipt.SessionID == sessionID {
		return r.receipt, nil
	}
	return nil, errors.New("receipt not found")
}

type cleanupHandlerCloser struct {
	SessionLauncher
	repo   *cleanupHandlerSessions
	called bool
}

func (c *cleanupHandlerCloser) DeleteSession(_ context.Context, sessionID string) error {
	c.called = true
	c.repo.receipt = &models.TaskSessionCleanupReceipt{
		TaskID: "task", WorkspaceID: "workspace", SessionID: sessionID, Disposition: "archived", EvidenceRetained: true,
	}
	return nil
}

func TestCloseTaskSessionReturnsScopedCleanupReceipt(t *testing.T) {
	repo := &cleanupHandlerSessions{sessions: map[string]*models.TaskSession{
		"primary": {ID: "primary", TaskID: "task", IsPrimary: true},
		"source":  {ID: "source", TaskID: "task", State: models.TaskSessionStateCompleted},
	}}
	closer := &cleanupHandlerCloser{repo: repo}
	h := &Handlers{sessionRepo: repo, sessionLauncher: closer}
	response, err := h.handleCloseTaskSession(queuePrincipal("workspace", "task", "primary"),
		makeWSMessage(t, ws.ActionMCPCloseTaskSession, map[string]interface{}{
			"task_id": "task", "caller_session_id": "primary", "target_session_id": "source",
		}))
	if err != nil {
		t.Fatalf("close task session: %v", err)
	}
	var payload struct {
		Receipt models.TaskSessionCleanupReceipt `json:"receipt"`
	}
	if err := response.ParsePayload(&payload); err != nil {
		t.Fatalf("parse cleanup receipt: %v", err)
	}
	if !closer.called || payload.Receipt.SessionID != "source" || payload.Receipt.Disposition != "archived" {
		t.Fatalf("cleanup result = called:%t receipt:%+v", closer.called, payload.Receipt)
	}
}

func TestCloseTaskSessionRejectsCrossTaskTarget(t *testing.T) {
	repo := &cleanupHandlerSessions{sessions: map[string]*models.TaskSession{
		"primary": {ID: "primary", TaskID: "task", IsPrimary: true},
		"foreign": {ID: "foreign", TaskID: "other", State: models.TaskSessionStateCompleted},
	}}
	closer := &cleanupHandlerCloser{repo: repo}
	h := &Handlers{sessionRepo: repo, sessionLauncher: closer}
	response, err := h.handleCloseTaskSession(queuePrincipal("workspace", "task", "primary"),
		makeWSMessage(t, ws.ActionMCPCloseTaskSession, map[string]interface{}{
			"task_id": "task", "caller_session_id": "primary", "target_session_id": "foreign",
		}))
	if err != nil {
		t.Fatalf("close task session: %v", err)
	}
	if closer.called {
		t.Fatal("cross-task cleanup called session closer")
	}
	assertWSError(t, response, ws.ErrorCodeForbidden)
}
