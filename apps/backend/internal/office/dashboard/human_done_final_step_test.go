package dashboard_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
)

type fakeFinalStepMover struct {
	result *dashboard.FinalStepMoveResult
	err    error
	calls  []string
}

func (m *fakeFinalStepMover) MoveTaskToFinalStep(_ context.Context, taskID string) (*dashboard.FinalStepMoveResult, error) {
	m.calls = append(m.calls, taskID)
	return m.result, m.err
}

func completedAtDone() *dashboard.FinalStepMoveResult {
	return &dashboard.FinalStepMoveResult{State: "COMPLETED", StepID: "step-done", StepName: "Done"}
}

// @covers AC-OFFICE-HUMAN-DONE-001.1
func TestUpdateTaskStatusAsHuman_DoneAtWork_MovesToFinalStep(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "hd1", "ws-hd", "HD", "in_progress", "Work")
	mover := &fakeFinalStepMover{result: completedAtDone()}
	rt := &recordingReactivity{result: &dashboard.TaskReactivityResult{}}
	deps.svc.SetTaskFinalStepMover(mover)
	deps.svc.SetReactivityApplier(rt)

	result, err := deps.svc.UpdateTaskStatusAsHuman(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID: "hd1", NewStatus: "done",
	})
	if err != nil {
		t.Fatalf("UpdateTaskStatusAsHuman: %v", err)
	}
	if len(mover.calls) != 1 || mover.calls[0] != "hd1" {
		t.Fatalf("mover calls = %v, want [hd1]", mover.calls)
	}
	if result == nil || result.Status != "done" || result.StepName != "Done" {
		t.Fatalf("result = %+v, want done at Done", result)
	}
	if state := readTaskState(t, deps, "hd1"); state != "in_progress" {
		t.Errorf("state = %q, want the move to own the write (unchanged here)", state)
	}
	if len(rt.calls) != 0 {
		t.Errorf("reactivity calls = %d, want 0", len(rt.calls))
	}
}

// @covers AC-OFFICE-HUMAN-DONE-001.2
func TestUpdateTaskStatusAsHuman_FinalStepDoesNotComplete_ReturnsMovedStatus(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "hd2", "ws-hd", "HD", "in_progress", "Work")
	deps.svc.SetTaskFinalStepMover(&fakeFinalStepMover{result: &dashboard.FinalStepMoveResult{
		State: "IN_PROGRESS", StepID: "step-done", StepName: "Done",
	}})

	result, err := deps.svc.UpdateTaskStatusAsHuman(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID: "hd2", NewStatus: "done",
	})
	if err != nil {
		t.Fatalf("UpdateTaskStatusAsHuman: %v", err)
	}
	if result == nil || result.Status != "in_progress" {
		t.Fatalf("result = %+v, want in_progress", result)
	}
}

// @covers AC-OFFICE-HUMAN-DONE-001.3
func TestUpdateTaskStatusAsHuman_DoneAtWork_MoveRefused_NoStateChangeNoWake(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "hd3", "ws-hd", "HD", "in_progress", "Work")
	refusal := &dashboard.FinalStepMoveRefusedError{
		Reason:  dashboard.ApprovalGateReasonApprovals,
		Pending: []dashboard.PendingApproverDTO{{AgentProfileID: "agent-ceo", Name: "CEO"}},
		Err:     errors.New("approvals pending"),
	}
	rt := &recordingReactivity{result: &dashboard.TaskReactivityResult{}}
	deps.svc.SetTaskFinalStepMover(&fakeFinalStepMover{err: refusal})
	deps.svc.SetReactivityApplier(rt)

	_, err := deps.svc.UpdateTaskStatusAsHuman(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID: "hd3", NewStatus: "done",
	})
	var got *dashboard.FinalStepMoveRefusedError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v, want FinalStepMoveRefusedError", err)
	}
	if state := readTaskState(t, deps, "hd3"); state != "in_progress" {
		t.Errorf("state = %q, want in_progress", state)
	}
	if len(rt.calls) != 0 {
		t.Errorf("reactivity calls = %d, want 0", len(rt.calls))
	}
}

// @covers AC-OFFICE-HUMAN-DONE-001.4
func TestUpdateTaskStatusAsHuman_TerminalStep_KeepsDirectGate(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "hd4", "ws-hd", "HD", "in_progress", 2)
	mover := &fakeFinalStepMover{result: completedAtDone()}
	deps.svc.SetTaskFinalStepMover(mover)

	result, err := deps.svc.UpdateTaskStatusAsHuman(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID: "hd4", NewStatus: "done",
	})
	if err != nil {
		t.Fatalf("UpdateTaskStatusAsHuman: %v", err)
	}
	if len(mover.calls) != 0 {
		t.Fatalf("mover calls = %v, want none", mover.calls)
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil for the direct path", result)
	}
	if state := readTaskState(t, deps, "hd4"); state != "COMPLETED" {
		t.Errorf("state = %q, want COMPLETED", state)
	}
}

func TestUpdateTaskStatusAsHuman_NonDoneStatus_SkipsMover(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "hd5", "ws-hd", "HD", "todo", "Work")
	mover := &fakeFinalStepMover{result: completedAtDone()}
	deps.svc.SetTaskFinalStepMover(mover)

	if _, err := deps.svc.UpdateTaskStatusAsHuman(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID: "hd5", NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("UpdateTaskStatusAsHuman: %v", err)
	}
	if len(mover.calls) != 0 {
		t.Fatalf("mover calls = %v, want none", mover.calls)
	}
	if state := readTaskState(t, deps, "hd5"); state != "IN_PROGRESS" {
		t.Errorf("state = %q, want IN_PROGRESS", state)
	}
}

func patchStatusDone(t *testing.T, deps *testDeps, taskID string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/office/tasks/"+taskID, strings.NewReader(`{"status":"done"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return w, body
}

// @covers AC-OFFICE-HUMAN-DONE-001.1
func TestUpdateTaskEndpoint_HumanDoneAtWork_MovesToFinalStep(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "ep-hd1", "ws-hd", "HD", "in_progress", "Work")
	deps.svc.SetTaskFinalStepMover(&fakeFinalStepMover{result: completedAtDone()})

	w, body := patchStatusDone(t, deps, "ep-hd1")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %v", w.Code, body)
	}
	if body["status"] != "done" {
		t.Errorf("response status = %v, want done", body["status"])
	}
	step, _ := body["workflow_step"].(map[string]interface{})
	if step["id"] != "step-done" || step["name"] != "Done" {
		t.Errorf("workflow_step = %v, want step-done/Done", body["workflow_step"])
	}
}

// @covers AC-OFFICE-HUMAN-DONE-001.3
func TestUpdateTaskEndpoint_HumanDoneRefused_409WithoutStatus(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "ep-hd2", "ws-hd", "HD", "in_progress", "Work")
	deps.svc.SetTaskFinalStepMover(&fakeFinalStepMover{err: &dashboard.FinalStepMoveRefusedError{
		Reason:  dashboard.ApprovalGateReasonApprovals,
		Pending: []dashboard.PendingApproverDTO{{AgentProfileID: "agent-ceo", Name: "CEO"}},
		Err:     errors.New("approvals pending"),
	}})

	w, body := patchStatusDone(t, deps, "ep-hd2")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %v", w.Code, body)
	}
	if _, has := body["status"]; has {
		t.Errorf("response has status %v, want none (nothing changed)", body["status"])
	}
	if body["reason"] != dashboard.ApprovalGateReasonApprovals {
		t.Errorf("reason = %v, want approvals", body["reason"])
	}
	pending, _ := body["pending_approvers"].([]interface{})
	if len(pending) != 1 {
		t.Fatalf("pending_approvers = %v, want 1 entry", body["pending_approvers"])
	}
}

func TestUpdateTaskEndpoint_HumanDoneRefusedMoveConflict_CarriesCode(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "ep-hd3", "ws-hd", "HD", "in_progress", "Work")
	deps.svc.SetTaskFinalStepMover(&fakeFinalStepMover{err: &dashboard.FinalStepMoveRefusedError{
		Reason: dashboard.FinalStepMoveReasonWorkflowMove,
		Code:   "task_move_active_session",
		Err:    errors.New("task has an active session"),
	}})

	w, body := patchStatusDone(t, deps, "ep-hd3")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %v", w.Code, body)
	}
	if body["reason"] != dashboard.FinalStepMoveReasonWorkflowMove || body["code"] != "task_move_active_session" {
		t.Errorf("body = %v, want workflow_move/task_move_active_session", body)
	}
	if _, has := body["status"]; has {
		t.Errorf("response has status %v, want none", body["status"])
	}
}

func TestUpdateTaskEndpoint_HumanDoneQueued_ReportsQueued(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskAtNonTerminalStep(t, deps.db, "ep-hd4", "ws-hd", "HD", "in_progress", "Work")
	deps.svc.SetTaskFinalStepMover(&fakeFinalStepMover{result: &dashboard.FinalStepMoveResult{
		State: "IN_PROGRESS", StepID: "step-done", StepName: "Done", Queued: true,
	}})

	w, body := patchStatusDone(t, deps, "ep-hd4")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %v", w.Code, body)
	}
	if body["status"] != "in_progress" || body["queued"] != true {
		t.Errorf("body = %v, want in_progress and queued", body)
	}
}
