package handlers

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type cleanupHandlerSessions struct {
	SessionRepository
	sessions     map[string]*models.TaskSession
	receipt      *models.TaskSessionCleanupReceipt
	lookupErrors map[string]error
}

func (r *cleanupHandlerSessions) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	if err := r.lookupErrors[id]; err != nil {
		return nil, err
	}
	if session := r.sessions[id]; session != nil {
		return session, nil
	}
	return nil, models.ErrTaskSessionNotFound
}

func (r *cleanupHandlerSessions) GetTaskSessionCleanupReceipt(_ context.Context, taskID, sessionID string) (*models.TaskSessionCleanupReceipt, error) {
	if r.receipt != nil && r.receipt.TaskID == taskID && r.receipt.SessionID == sessionID {
		return r.receipt, nil
	}
	return nil, errors.New("receipt not found")
}

type cleanupHandlerCloser struct {
	SessionLauncher
	repo      *cleanupHandlerSessions
	called    bool
	deleteErr error
}

func (c *cleanupHandlerCloser) DeleteSession(_ context.Context, sessionID string) error {
	c.called = true
	if c.deleteErr != nil {
		return c.deleteErr
	}
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
	if !closer.called || payload.Receipt.SessionID != "source" || payload.Receipt.TaskID != "task" ||
		payload.Receipt.WorkspaceID != "workspace" || !payload.Receipt.EvidenceRetained || payload.Receipt.Disposition != "archived" {
		t.Fatalf("cleanup result = called:%t receipt:%+v", closer.called, payload.Receipt)
	}
}

func TestCloseTaskSessionRejectsActiveTargetWithoutCallingCleanup(t *testing.T) {
	repo := &cleanupHandlerSessions{sessions: map[string]*models.TaskSession{
		"primary": {ID: "primary", TaskID: "task", IsPrimary: true},
		"source":  {ID: "source", TaskID: "task", State: models.TaskSessionStateRunning},
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
	assertWSError(t, response, ws.ErrorCodeValidation)
	if closer.called {
		t.Fatal("cleanup called for active target")
	}
}

func TestCloseTaskSessionSanitizesRepositoryAndCleanupFailures(t *testing.T) {
	req := makeWSMessage(t, ws.ActionMCPCloseTaskSession, map[string]interface{}{
		"task_id": "task", "caller_session_id": "primary", "target_session_id": "source",
	})
	repo := &cleanupHandlerSessions{sessions: map[string]*models.TaskSession{
		"primary": {ID: "primary", TaskID: "task", IsPrimary: true},
		"source":  {ID: "source", TaskID: "task", State: models.TaskSessionStateCompleted},
	}, lookupErrors: map[string]error{"primary": errors.New("private database address")}}
	closer := &cleanupHandlerCloser{repo: repo}
	h := &Handlers{sessionRepo: repo, sessionLauncher: closer}
	response, err := h.handleCloseTaskSession(queuePrincipal("workspace", "task", "primary"), req)
	if err != nil {
		t.Fatal(err)
	}
	assertWSError(t, response, ws.ErrorCodeInternalError)
	if closer.called || bytes.Contains(response.Payload, []byte("private database address")) {
		t.Fatal("lookup failure reached closer or leaked details")
	}

	delete(repo.lookupErrors, "primary")
	repo.lookupErrors["source"] = errors.New("private target database address")
	response, err = h.handleCloseTaskSession(queuePrincipal("workspace", "task", "primary"), req)
	if err != nil {
		t.Fatal(err)
	}
	assertWSError(t, response, ws.ErrorCodeInternalError)
	if closer.called || bytes.Contains(response.Payload, []byte("private target database address")) {
		t.Fatal("target lookup failure reached closer or leaked details")
	}

	delete(repo.lookupErrors, "source")
	closer.deleteErr = errors.New("private cleanup SQL statement")
	response, err = h.handleCloseTaskSession(queuePrincipal("workspace", "task", "primary"), req)
	if err != nil {
		t.Fatal(err)
	}
	assertWSError(t, response, ws.ErrorCodeInternalError)
	if bytes.Contains(response.Payload, []byte("private cleanup SQL statement")) {
		t.Fatal("cleanup failure leaked details")
	}
}

func TestTerminalSessionScopeParseErrorDoesNotExposeDecoderDetails(t *testing.T) {
	msg := &ws.Message{ID: "request", Action: ws.ActionMCPCloseTaskSession}
	response, err := terminalSessionScopeError(msg, errors.New("private payload detail"), "", "session cleanup")
	if err != nil {
		t.Fatal(err)
	}
	assertWSError(t, response, ws.ErrorCodeBadRequest)
	if bytes.Contains(response.Payload, []byte("private payload detail")) {
		t.Fatal("decoder details leaked")
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
