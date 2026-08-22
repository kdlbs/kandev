package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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
