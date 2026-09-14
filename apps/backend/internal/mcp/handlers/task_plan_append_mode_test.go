package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestMCPUpdateTaskPlanModeValidation pins AC-TASKS-PLAN-APPEND-001.1/001.3:
// an absent mode or the literal "replace"/"append" are accepted; any other
// value, including one differing only in letter case, is rejected with a
// message naming both accepted values, before anything else runs.
func TestMCPUpdateTaskPlanModeValidation(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: "base", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	invalid := []string{"Append", "APPEND", "REPLACE", "merge", "appended"}
	for _, mode := range invalid {
		t.Run("rejects "+mode, func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{
				"task_id": mcpPlanTaskID, "content": "fragment", "mode": mode,
			})
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			out, handleErr := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan, string(payload)))
			if handleErr != nil {
				t.Fatalf("handler returned error: %v", handleErr)
			}
			if out.Type != ws.MessageTypeError {
				t.Fatalf("type = %q, want %q (payload %s)", out.Type, ws.MessageTypeError, out.Payload)
			}
			var errPayload ws.ErrorPayload
			if jsonErr := json.Unmarshal(out.Payload, &errPayload); jsonErr != nil {
				t.Fatalf("unmarshal error payload: %v", jsonErr)
			}
			if errPayload.Code != ws.ErrorCodeValidation {
				t.Errorf("code = %q, want %q", errPayload.Code, ws.ErrorCodeValidation)
			}
			if !strings.Contains(errPayload.Message, "replace") || !strings.Contains(errPayload.Message, "append") {
				t.Errorf("message %q does not name both accepted values", errPayload.Message)
			}

			plan, getErr := h.planService.GetPlan(ctx, mcpPlanTaskID)
			if getErr != nil {
				t.Fatalf("GetPlan: %v", getErr)
			}
			if plan.Content != "base" {
				t.Fatalf("stored content changed after a rejected mode: %q", plan.Content)
			}
		})
	}

	t.Run("empty mode defaults to replace", func(t *testing.T) {
		out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
			`{"task_id":"`+mcpPlanTaskID+`","content":"whole new document"}`))
		if err != nil {
			t.Fatalf("handleUpdateTaskPlan: %v", err)
		}
		plan := decodeMCPPlanPayload(t, out)
		if plan["content"] != "whole new document" {
			t.Errorf("content = %v, want the literal submitted content (replace)", plan["content"])
		}
	})
}

// TestMCPUpdateTaskPlanAppendComposesFragment pins the MCP surface's success
// path for mode="append": the response payload reflects the composed
// content, not the submitted fragment alone.
func TestMCPUpdateTaskPlanAppendComposesFragment(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: "# Plan\n\nstep one", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		`{"task_id":"`+mcpPlanTaskID+`","content":"## Step two","mode":"append"}`))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	plan := decodeMCPPlanPayload(t, out)
	want := "# Plan\n\nstep one\n\n## Step two"
	if plan["content"] != want {
		t.Errorf("content = %v, want %q", plan["content"], want)
	}
}

// TestMCPUpdateTaskPlanAppendRejectsEmptyContent pins that append's empty
// content check runs through PlanService (not the handler's replace-only
// pre-check) but still surfaces the same "content is required" message.
func TestMCPUpdateTaskPlanAppendRejectsEmptyContent(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: "base", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		`{"task_id":"`+mcpPlanTaskID+`","content":"","mode":"append"}`))
	assertMCPPlanError(t, out, err, ws.ErrorCodeValidation, "content is required")
}

// TestMCPUpdateTaskPlanAppendRejectsWhitespaceOnlyFragment pins
// AC-TASKS-PLAN-APPEND-001.5 at the MCP surface.
func TestMCPUpdateTaskPlanAppendRejectsWhitespaceOnlyFragment(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: "base", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}

	payload, err := json.Marshal(map[string]string{
		"task_id": mcpPlanTaskID, "content": "  \n\t  ", "mode": "append",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	out, handleErr := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan, string(payload)))
	assertMCPPlanError(t, out, handleErr, ws.ErrorCodeValidation,
		"append fragment must contain a non-whitespace character")
}

// TestMCPUpdateTaskPlanAppendAgainstMissingPlanReportsNotFound pins
// AC-TASKS-PLAN-APPEND-001.6 at the MCP surface: append against a task with
// no plan reports the same not-found response as replace.
func TestMCPUpdateTaskPlanAppendAgainstMissingPlanReportsNotFound(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	out, err := h.handleUpdateTaskPlan(context.Background(), mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		`{"task_id":"`+mcpPlanlessID+`","content":"fragment","mode":"append"}`))
	assertMCPPlanError(t, out, err, ws.ErrorCodeNotFound, "Task plan not found")
}

// readFailingPlanRepo makes GetTaskPlan fail unconditionally, simulating a
// transient storage read failure at the MCP surface (AC-TASKS-PLAN-APPEND-003.5).
type readFailingPlanRepo struct {
	*sqliterepo.Repository
}

func (r *readFailingPlanRepo) GetTaskPlan(context.Context, string) (*models.TaskPlan, error) {
	return nil, errors.New("simulated read failure")
}

// TestMCPUpdateTaskPlanAppendReadFailureIsDistinctFromNotFound pins
// AC-TASKS-PLAN-APPEND-003.5's error-mapping contract: a failed read is
// reported with a different code and message than "Task plan not found", so
// an agent cannot mistake it for an invitation to call
// create_task_plan_kandev.
func TestMCPUpdateTaskPlanAppendReadFailureIsDistinctFromNotFound(t *testing.T) {
	h, repo := newMCPPlanTestHandlersWithRepo(t)
	ctx := context.Background()
	if _, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: "base", CreatedBy: "agent",
	}); err != nil {
		t.Fatalf("seed CreatePlan: %v", err)
	}
	h.planService = service.NewPlanService(&readFailingPlanRepo{repo}, nil, h.logger)

	out, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		`{"task_id":"`+mcpPlanTaskID+`","content":"fragment","mode":"append"}`))
	assertMCPPlanError(t, out, err, ws.ErrorCodeInternalError,
		"Could not read the current plan content; the append was not applied")
}

// TestMCPCreateTaskPlanIgnoresModeField pins that create_task_plan_kandev's
// WS handler has no Mode field at all: a "mode" key present in the payload
// is silently dropped by JSON unmarshaling and the content is stored
// verbatim, never composed against anything.
func TestMCPCreateTaskPlanIgnoresModeField(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	out, err := h.handleCreateTaskPlan(context.Background(), mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		`{"task_id":"`+mcpPlanTaskID+`","content":"fragment","mode":"append"}`))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	plan := decodeMCPPlanPayload(t, out)
	if plan["content"] != "fragment" {
		t.Errorf("content = %v, want the literal submitted content, uncomposed", plan["content"])
	}
}
