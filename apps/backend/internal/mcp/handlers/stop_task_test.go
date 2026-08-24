package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type principalAuthorityStore struct {
	principal *models.WorkspaceAgentPrincipal
	grants    []*models.CoordinatorGrant
	audits    []*models.CoordinatorAuditEvent
}

func (s *principalAuthorityStore) GetActiveWorkspaceAgentPrincipalForTask(_ context.Context, _, _ string) (*models.WorkspaceAgentPrincipal, error) {
	return s.principal, nil
}
func (s *principalAuthorityStore) ListActiveWorkspaceAgentPrincipalGrants(_ context.Context, _, _ string) ([]*models.CoordinatorGrant, error) {
	return s.grants, nil
}
func (s *principalAuthorityStore) CreateCoordinatorAuditEvent(_ context.Context, event *models.CoordinatorAuditEvent) error {
	s.audits = append(s.audits, event)
	return nil
}
func (*principalAuthorityStore) FinishCoordinatorAuditEvent(context.Context, string, string, string) error {
	return nil
}

type recordingTaskStopper struct {
	result orchestrator.CoordinatorTaskStopResult
	err    error
	calls  []string
}

func (s *recordingTaskStopper) StopTaskForCoordinator(
	_ context.Context,
	taskID string,
) (orchestrator.CoordinatorTaskStopResult, error) {
	s.calls = append(s.calls, taskID)
	return s.result, s.err
}

func stopTaskTestHandler(
	t *testing.T,
	tasks map[string]*models.Task,
	errorsByID map[string]error,
	stopper TaskStopper,
) *Handlers {
	return &Handlers{
		taskStopper: stopper,
		stopTaskGetter: func(_ context.Context, taskID string) (*models.Task, error) {
			if err := errorsByID[taskID]; err != nil {
				return nil, err
			}
			task, ok := tasks[taskID]
			if !ok {
				return nil, taskrepo.ErrTaskNotFound
			}
			return task, nil
		},
		logger: testLogger(t),
	}
}

func TestHandleStopTask_AuthorizesOnlyDirectParentInWorkspace(t *testing.T) {
	tasks := map[string]*models.Task{
		"grandparent": {ID: "grandparent", WorkspaceID: "ws-1"},
		"parent":      {ID: "parent", WorkspaceID: "ws-1", ParentID: "grandparent"},
		"child":       {ID: "child", WorkspaceID: "ws-1", ParentID: "parent"},
		"sibling":     {ID: "sibling", WorkspaceID: "ws-1", ParentID: "grandparent"},
		"unrelated":   {ID: "unrelated", WorkspaceID: "ws-1"},
		"cross-child": {ID: "cross-child", WorkspaceID: "ws-2", ParentID: "parent"},
	}
	tests := []struct {
		name       string
		senderID   string
		targetID   string
		wantStatus orchestrator.CoordinatorTaskStopStatus
		wantCode   string
	}{
		{name: "direct parent", senderID: "parent", targetID: "child", wantStatus: orchestrator.CoordinatorTaskStopStatusStopped},
		{name: "self", senderID: "parent", targetID: "parent", wantCode: ws.ErrorCodeForbidden},
		{name: "sibling", senderID: "parent", targetID: "sibling", wantCode: ws.ErrorCodeForbidden},
		{name: "child", senderID: "child", targetID: "parent", wantCode: ws.ErrorCodeForbidden},
		{name: "grandparent", senderID: "grandparent", targetID: "child", wantCode: ws.ErrorCodeForbidden},
		{name: "unrelated", senderID: "unrelated", targetID: "child", wantCode: ws.ErrorCodeForbidden},
		{name: "cross workspace", senderID: "parent", targetID: "cross-child", wantCode: ws.ErrorCodeForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stopper := &recordingTaskStopper{result: orchestrator.CoordinatorTaskStopResult{Status: tt.wantStatus}}
			h := stopTaskTestHandler(t, tasks, nil, stopper)
			msg := makeWSMessage(t, ws.ActionMCPStopTask, map[string]interface{}{
				"task_id":        tt.targetID,
				"sender_task_id": tt.senderID,
				"reason":         "forged destructive cleanup reason",
				"force":          true,
			})

			resp, err := h.handleStopTask(context.Background(), msg)
			if err != nil {
				t.Fatalf("handleStopTask: %v", err)
			}
			if tt.wantCode != "" {
				assertWSError(t, resp, tt.wantCode)
				if len(stopper.calls) != 0 {
					t.Fatalf("forbidden request invoked stopper: %v", stopper.calls)
				}
				return
			}
			var payload struct {
				TaskID string                                 `json:"task_id"`
				Status orchestrator.CoordinatorTaskStopStatus `json:"status"`
			}
			if err := resp.ParsePayload(&payload); err != nil {
				t.Fatalf("parse response: %v", err)
			}
			if payload.TaskID != tt.targetID || payload.Status != tt.wantStatus {
				t.Fatalf("response = %#v", payload)
			}
			if len(stopper.calls) != 1 || stopper.calls[0] != tt.targetID {
				t.Fatalf("stopper calls = %v", stopper.calls)
			}
		})
	}
}

func TestHandleStopTask_RequiresCurrentPrincipalSessionAndAuditsGrant(t *testing.T) {
	tasks := map[string]*models.Task{
		"agent":  {ID: "agent", WorkspaceID: "ws-1"},
		"target": {ID: "target", WorkspaceID: "ws-1"},
	}
	store := &principalAuthorityStore{principal: &models.WorkspaceAgentPrincipal{
		ID: "principal", WorkspaceID: "ws-1", PluginInstallationID: "install", LogicalKey: "agent", BackingTaskID: "agent", BackingSessionID: "current",
	}, grants: []*models.CoordinatorGrant{{ID: "grant", PrincipalID: "principal", WorkspaceID: "ws-1", ScopeKind: coordinator.ScopeWorkspace, ScopeID: "ws-1", Capabilities: string(coordinator.CapabilityOrchestrate)}}}
	h := stopTaskTestHandler(t, tasks, nil, &recordingTaskStopper{result: orchestrator.CoordinatorTaskStopResult{Status: orchestrator.CoordinatorTaskStopStatusStopped}})
	h.SetCoordinatorAuthority(coordinator.New(store, func() bool { return true }))

	stale := makeWSMessage(t, ws.ActionMCPStopTask, map[string]interface{}{"task_id": "target", "sender_task_id": "agent", "sender_session_id": "stale"})
	resp, err := h.handleStopTask(context.Background(), stale)
	if err != nil {
		t.Fatalf("stale handleStopTask: %v", err)
	}
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	if len(store.audits) != 0 {
		t.Fatalf("stale session audits = %#v", store.audits)
	}

	current := makeWSMessage(t, ws.ActionMCPStopTask, map[string]interface{}{"task_id": "target", "sender_task_id": "agent", "sender_session_id": "current"})
	resp, err = h.handleStopTask(context.Background(), current)
	if err != nil {
		t.Fatalf("current handleStopTask: %v", err)
	}
	if resp.Type != ws.MessageTypeResponse || len(store.audits) != 1 {
		t.Fatalf("response/audits = %#v / %#v", resp, store.audits)
	}
	audit := store.audits[0]
	if audit.PrincipalID != "principal" || audit.ActorTaskID != "agent" || audit.ActorSessionID != "current" {
		t.Fatalf("audit provenance = %#v", audit)
	}
}

func TestHandleStopTask_ValidatesPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		code    string
	}{
		{name: "malformed", payload: []byte(`{"task_id":`), code: ws.ErrorCodeBadRequest},
		{name: "missing task", payload: []byte(`{"sender_task_id":"parent"}`), code: ws.ErrorCodeValidation},
		{name: "missing sender", payload: []byte(`{"task_id":"child"}`), code: ws.ErrorCodeValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := stopTaskTestHandler(t, nil, nil, &recordingTaskStopper{})
			msg := &ws.Message{ID: "request-1", Action: ws.ActionMCPStopTask, Payload: tt.payload}
			resp, err := h.handleStopTask(context.Background(), msg)
			if err != nil {
				t.Fatalf("handleStopTask: %v", err)
			}
			assertWSError(t, resp, tt.code)
		})
	}
}

func TestHandleStopTask_MapsLookupFailures(t *testing.T) {
	infraFailure := errors.New("database unavailable")
	tests := []struct {
		name       string
		errorsByID map[string]error
		wantCode   string
	}{
		{name: "sender missing", errorsByID: map[string]error{"parent": taskrepo.ErrTaskNotFound}, wantCode: ws.ErrorCodeNotFound},
		{name: "sender lookup failure", errorsByID: map[string]error{"parent": infraFailure}, wantCode: ws.ErrorCodeInternalError},
		{name: "target missing", errorsByID: map[string]error{"child": taskrepo.ErrTaskNotFound}, wantCode: ws.ErrorCodeNotFound},
		{name: "target lookup failure", errorsByID: map[string]error{"child": infraFailure}, wantCode: ws.ErrorCodeInternalError},
	}
	tasks := map[string]*models.Task{
		"parent": {ID: "parent", WorkspaceID: "ws-1"},
		"child":  {ID: "child", WorkspaceID: "ws-1", ParentID: "parent"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stopper := &recordingTaskStopper{}
			h := stopTaskTestHandler(t, tasks, tt.errorsByID, stopper)
			msg := makeWSMessage(t, ws.ActionMCPStopTask, map[string]interface{}{
				"task_id": "child", "sender_task_id": "parent",
			})

			resp, err := h.handleStopTask(context.Background(), msg)
			if err != nil {
				t.Fatalf("handleStopTask: %v", err)
			}
			assertWSError(t, resp, tt.wantCode)
			if len(stopper.calls) != 0 {
				t.Fatalf("lookup failure invoked stopper: %v", stopper.calls)
			}
		})
	}
}

func TestHandleStopTask_MapsStopperFailure(t *testing.T) {
	tasks := map[string]*models.Task{
		"parent": {ID: "parent", WorkspaceID: "ws-1"},
		"child":  {ID: "child", WorkspaceID: "ws-1", ParentID: "parent"},
	}
	stopper := &recordingTaskStopper{err: errors.New("stop failed")}
	h := stopTaskTestHandler(t, tasks, nil, stopper)
	msg := makeWSMessage(t, ws.ActionMCPStopTask, map[string]interface{}{
		"task_id": "child", "sender_task_id": "parent",
	})

	resp, err := h.handleStopTask(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleStopTask: %v", err)
	}
	assertWSError(t, resp, ws.ErrorCodeInternalError)
}

func TestHandleStopTask_ReturnsNotRunning(t *testing.T) {
	tasks := map[string]*models.Task{
		"parent": {ID: "parent", WorkspaceID: "ws-1"},
		"child":  {ID: "child", WorkspaceID: "ws-1", ParentID: "parent"},
	}
	stopper := &recordingTaskStopper{result: orchestrator.CoordinatorTaskStopResult{
		Status: orchestrator.CoordinatorTaskStopStatusNotRunning,
	}}
	h := stopTaskTestHandler(t, tasks, nil, stopper)
	msg := makeWSMessage(t, ws.ActionMCPStopTask, map[string]interface{}{
		"task_id": "child", "sender_task_id": "parent",
	})

	resp, err := h.handleStopTask(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleStopTask: %v", err)
	}
	var payload struct {
		Status orchestrator.CoordinatorTaskStopStatus `json:"status"`
	}
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if payload.Status != orchestrator.CoordinatorTaskStopStatusNotRunning {
		t.Fatalf("status = %q, want not_running", payload.Status)
	}
}

func TestHandleStopTask_RejectsMissingStopperAndInvalidStatus(t *testing.T) {
	tasks := map[string]*models.Task{
		"parent": {ID: "parent", WorkspaceID: "ws-1"},
		"child":  {ID: "child", WorkspaceID: "ws-1", ParentID: "parent"},
	}
	tests := []struct {
		name    string
		stopper TaskStopper
	}{
		{name: "missing stopper"},
		{name: "invalid status", stopper: &recordingTaskStopper{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := stopTaskTestHandler(t, tasks, nil, tt.stopper)
			msg := makeWSMessage(t, ws.ActionMCPStopTask, map[string]interface{}{
				"task_id": "child", "sender_task_id": "parent",
			})
			resp, err := h.handleStopTask(context.Background(), msg)
			if err != nil {
				t.Fatalf("handleStopTask: %v", err)
			}
			assertWSError(t, resp, ws.ErrorCodeInternalError)
		})
	}
}

func TestRegisterHandlers_RegistersStopTask(t *testing.T) {
	tasks := map[string]*models.Task{
		"parent": {ID: "parent", WorkspaceID: "ws-1"},
		"child":  {ID: "child", WorkspaceID: "ws-1", ParentID: "parent"},
	}
	stopper := &recordingTaskStopper{result: orchestrator.CoordinatorTaskStopResult{
		Status: orchestrator.CoordinatorTaskStopStatusStopped,
	}}
	h := stopTaskTestHandler(t, tasks, nil, stopper)
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	msg := makeWSMessage(t, ws.ActionMCPStopTask, map[string]interface{}{
		"task_id": "child", "sender_task_id": "parent",
	})

	resp, err := dispatcher.Dispatch(context.Background(), msg)
	if err != nil {
		t.Fatalf("dispatch stop task: %v", err)
	}
	if resp == nil || resp.Type != ws.MessageTypeResponse {
		t.Fatalf("dispatch response = %#v", resp)
	}
	if len(stopper.calls) != 1 || stopper.calls[0] != "child" {
		t.Fatalf("stopper calls = %v", stopper.calls)
	}
}
