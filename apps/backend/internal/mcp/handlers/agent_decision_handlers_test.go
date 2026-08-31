package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	officeenginedispatcher "github.com/kandev/kandev/internal/office/engine_dispatcher"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// This file covers the record_step_decision_kandev MCP transport boundary
// (handleRecordStepDecision / mapRecordStepDecisionError). Review round 1
// flagged this file at zero coverage. RecordAgentDecision's own business
// logic (role resolution, verdict/reason validation, engine re-evaluation)
// is already covered at the dashboard-service tier in
// internal/office/dashboard/agent_decisions_test.go — these tests instead
// exercise what's unique to this transport: payload validation, session
// lookup, the session/task ownership check, and error-code mapping.

// seedRecordStepDecisionSession seeds a minimal workspace/task/session chain
// so handleRecordStepDecision's h.sessionRepo.GetTaskSession lookup and the
// session.TaskID ownership check have something real to read.
func seedRecordStepDecisionSession(t *testing.T, repo *sqliterepo.Repository, workspaceID, taskID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID: workspaceID, Name: "Decision WS", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, Title: "Decision Task",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}))
}

// newRecordStepDecisionHandler builds a *Handlers whose dashboardSvc is a
// real, minimally-constructed *dashboard.DashboardService. Every test case
// here returns before RecordAgentDecision ever touches its repo/dispatcher
// dependencies, so a nil Repository is safe — dashboardSvc only needs to be
// non-nil to pass handleRecordStepDecision's first guard.
func newRecordStepDecisionHandler(t *testing.T, repo *sqliterepo.Repository, svc *dashboard.DashboardService) *Handlers {
	t.Helper()
	return &Handlers{
		sessionRepo:  repo,
		dashboardSvc: svc,
		logger:       testLogger(t).WithFields(),
	}
}

func TestHandleRecordStepDecision_NotConfigured(t *testing.T) {
	h := &Handlers{logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPRecordStepDecision, map[string]interface{}{
		"task_id": "t1", "session_id": "s1", "decision": "approved", "reason": "lgtm",
	})
	resp, err := h.handleRecordStepDecision(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleRecordStepDecision_InvalidPayload(t *testing.T) {
	svc := dashboard.NewDashboardService(nil, testLogger(t), nil, nil, nil)
	h := newRecordStepDecisionHandler(t, nil, svc)
	msg := &ws.Message{
		ID: "test-id", Type: ws.MessageTypeRequest,
		Action: ws.ActionMCPRecordStepDecision, Payload: []byte(`{"task_id":`),
	}
	resp, err := h.handleRecordStepDecision(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleRecordStepDecision_MissingFields(t *testing.T) {
	svc := dashboard.NewDashboardService(nil, testLogger(t), nil, nil, nil)
	cases := []struct {
		name    string
		payload map[string]interface{}
	}{
		{
			name:    "missing task_id",
			payload: map[string]interface{}{"session_id": "s1", "decision": "approved", "reason": "lgtm"},
		},
		{
			name:    "missing session_id",
			payload: map[string]interface{}{"task_id": "t1", "decision": "approved", "reason": "lgtm"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newRecordStepDecisionHandler(t, nil, svc)
			msg := makeWSMessage(t, ws.ActionMCPRecordStepDecision, tc.payload)
			resp, err := h.handleRecordStepDecision(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, resp, ws.ErrorCodeValidation)
		})
	}
}

func TestHandleRecordStepDecision_SessionNotFound(t *testing.T) {
	svc := dashboard.NewDashboardService(nil, testLogger(t), nil, nil, nil)
	_, repo := newTestTaskService(t)
	h := newRecordStepDecisionHandler(t, repo, svc)

	msg := makeWSMessage(t, ws.ActionMCPRecordStepDecision, map[string]interface{}{
		"task_id": "missing-task", "session_id": "missing-session", "decision": "approved", "reason": "lgtm",
	})
	resp, err := h.handleRecordStepDecision(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
}

func TestHandleRecordStepDecision_SessionDoesNotBelongToTask(t *testing.T) {
	svc := dashboard.NewDashboardService(nil, testLogger(t), nil, nil, nil)
	_, repo := newTestTaskService(t)
	seedRecordStepDecisionSession(t, repo, "ws-rd1", "task-rd1", "sess-rd1")
	h := newRecordStepDecisionHandler(t, repo, svc)

	msg := makeWSMessage(t, ws.ActionMCPRecordStepDecision, map[string]interface{}{
		"task_id": "some-other-task", "session_id": "sess-rd1", "decision": "approved", "reason": "lgtm",
	})
	resp, err := h.handleRecordStepDecision(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

// fakeAgentDecisionRepo satisfies dashboard.Repository. It embeds the
// (nil) interface so every method it doesn't override panics if called —
// only GetTaskWorkflowStepID is exercised by RecordAgentDecision's
// AC-55(1) precondition check.
type fakeAgentDecisionRepo struct {
	dashboard.Repository
	workflowStepID string
}

func (r *fakeAgentDecisionRepo) GetTaskWorkflowStepID(_ context.Context, _ string) (string, error) {
	return r.workflowStepID, nil
}

// fakeAgentDecisionStore satisfies dashboard.DecisionStore. RecordAgentDecision
// only checks it for non-nil; the actual decision write happens inside the
// engine dispatcher's RecordDecision, not through this store.
type fakeAgentDecisionStore struct {
	dashboard.DecisionStore
}

// sessionCapturingDispatcher satisfies shared.WorkflowEngineDispatcher plus
// the additive capabilities RecordAgentDecision reaches via type assertion
// (decisionRecordingDispatcher, roleResolvingDispatcher). It captures the
// SessionID RecordDecision was actually called with, so the test can assert
// on the transport boundary this file exists to cover: that
// handleRecordStepDecision's req.SessionID reaches
// RecordAgentDecisionInput.SessionID and, from there,
// officeenginedispatcher.RecordDecisionInput.SessionID.
type sessionCapturingDispatcher struct {
	gotSessionID string
}

func (d *sessionCapturingDispatcher) HandleTrigger(
	context.Context, string, engine.Trigger, any, string,
) error {
	return nil
}

func (d *sessionCapturingDispatcher) ResolveParticipantRole(
	_ context.Context, _, _, _ string,
) (string, string, error) {
	return "approver", "participant-1", nil
}

func (d *sessionCapturingDispatcher) RecordDecision(
	_ context.Context, in officeenginedispatcher.RecordDecisionInput,
) (officeenginedispatcher.RecordDecisionResult, error) {
	d.gotSessionID = in.SessionID
	return officeenginedispatcher.RecordDecisionResult{
		StepID:     in.StepID,
		DecisionID: "dec-rd3",
		DecidedAt:  time.Now(),
	}, nil
}

// TestHandleRecordStepDecision_PassesSessionIDToDispatcher is the F1 fix
// from review round 1: handleRecordStepDecision is the only production
// caller that supplies a non-blank SessionID (the human dashboard path
// deliberately leaves it blank), so nothing else in the suite proves the
// transport actually threads req.SessionID through
// RecordAgentDecisionInput.SessionID into the dispatcher's
// RecordDecisionInput.SessionID — the exact binding
// dispatcher_decisions_test.go and agent_decisions_test.go prove happens
// correctly *once it arrives*. Confirmed this test fails for the right
// reason: reverting agent_decision_handlers.go's `SessionID: req.SessionID`
// line (review round 1's measured mutation) makes gotSessionID come back
// "" instead of "sess-rd3", and this assertion catches that.
func TestHandleRecordStepDecision_PassesSessionIDToDispatcher(t *testing.T) {
	_, repo := newTestTaskService(t)
	seedRecordStepDecisionSession(t, repo, "ws-rd3", "task-rd3", "sess-rd3")

	svc := dashboard.NewDashboardService(
		&fakeAgentDecisionRepo{workflowStepID: "step-rd3"}, testLogger(t), nil, nil, nil,
	)
	svc.SetDecisionStore(&fakeAgentDecisionStore{})
	dispatcher := &sessionCapturingDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	h := newRecordStepDecisionHandler(t, repo, svc)
	msg := makeWSMessage(t, ws.ActionMCPRecordStepDecision, map[string]interface{}{
		"task_id": "task-rd3", "session_id": "sess-rd3", "decision": "approved", "reason": "lgtm",
	})
	resp, err := h.handleRecordStepDecision(context.Background(), msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "unexpected error response: %s", resp.Payload)

	require.Equal(t, "sess-rd3", dispatcher.gotSessionID)
}

func TestMapRecordStepDecisionError_ForbiddenMapsTo403(t *testing.T) {
	msg := makeWSMessage(t, ws.ActionMCPRecordStepDecision, map[string]interface{}{})
	resp, err := mapRecordStepDecisionError(msg, shared.ErrForbidden)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
}

func TestMapRecordStepDecisionError_UnexpectedFailureMapsToInternal(t *testing.T) {
	msg := makeWSMessage(t, ws.ActionMCPRecordStepDecision, map[string]interface{}{})
	resp, err := mapRecordStepDecisionError(msg, errors.New("task has no workflow step bound"))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestMapRecordStepDecisionError_ValidationMapsToValidation(t *testing.T) {
	msg := makeWSMessage(t, ws.ActionMCPRecordStepDecision, map[string]interface{}{})
	validationErr := &dashboard.AgentDecisionValidationError{Err: errors.New("reason is required")}
	resp, err := mapRecordStepDecisionError(msg, validationErr)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}
