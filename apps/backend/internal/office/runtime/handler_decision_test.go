package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/shared"
)

type recordingDecisionRecorder struct {
	inputs []RecordAgentDecisionInput
	result RecordAgentDecisionResult
	err    error
}

func (r *recordingDecisionRecorder) RecordAgentDecision(
	_ context.Context,
	input RecordAgentDecisionInput,
) (RecordAgentDecisionResult, error) {
	r.inputs = append(r.inputs, input)
	return r.result, r.err
}

func TestRuntimeHandler_RecordAgentDecisionRejectsCallerIdentityFields(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{})

	resp := h.request(t, http.MethodPost, "/runtime/task/decision", map[string]string{
		"decision": "approved",
		"reason":   "looks good",
		"task_id":  "forged-task",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
}

func TestRuntimeHandler_RecordAgentDecisionUsesSignedRunIdentity(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{})
	h.decisions.result = RecordAgentDecisionResult{
		Decision:          "approved",
		Role:              "reviewer",
		StepID:            "step-1",
		DecisionID:        "decision-1",
		DecidedAt:         mustDecisionTime(t, "2026-09-07T20:00:00Z"),
		TransitionApplied: true,
		Guards:            []DecisionGuard{},
	}

	resp := h.request(t, http.MethodPost, "/runtime/task/decision", map[string]string{
		"decision": "approved",
		"reason":   "looks good",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if len(h.decisions.inputs) != 1 {
		t.Fatalf("decision inputs = %d, want 1", len(h.decisions.inputs))
	}
	input := h.decisions.inputs[0]
	if input.TaskID != "task-1" || input.AgentProfileID != "agent-1" || input.SessionID != "sess-1" ||
		input.Decision != "approved" || input.Reason != "looks good" {
		t.Fatalf("decision input = %+v", input)
	}

	var body RecordAgentDecisionResult
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode decision response: %v", err)
	}
	if body.Decision != "approved" || body.Role != "reviewer" || body.StepID != "step-1" ||
		body.DecisionID != "decision-1" || !body.TransitionApplied || body.Guards == nil {
		t.Fatalf("decision response = %+v", body)
	}
	assertActionRunEvent(t, h.runEvents, "record_agent_decision", "task", "task-1")
}

func TestRuntimeHandler_RecordAgentDecisionRejectsTasklessRun(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{})
	token, err := h.agentSvc.MintRuntimeJWT("agent-1", "", "ws-1", "run-1", "sess-1", "")
	if err != nil {
		t.Fatalf("mint taskless runtime token: %v", err)
	}
	h.token = token

	resp := h.request(t, http.MethodPost, "/runtime/task/decision", map[string]string{
		"decision": "approved",
		"reason":   "looks good",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "decision requires a task-bound runtime") {
		t.Fatalf("body = %q, want task-bound runtime error", resp.Body.String())
	}
	if len(h.decisions.inputs) != 0 {
		t.Fatalf("decision inputs = %d, want 0", len(h.decisions.inputs))
	}
}

func TestRuntimeHandler_RecordAgentDecisionMapsValidationAndPermissionErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantEvent  string
	}{
		{name: "validation", err: NewDecisionValidationError(errors.New("invalid decision")), wantStatus: http.StatusBadRequest, wantEvent: "runtime.denied"},
		{name: "permission", err: shared.ErrForbidden, wantStatus: http.StatusForbidden, wantEvent: "runtime.denied"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRuntimeHandlerHarness(t, Capabilities{})
			h.decisions.err = tc.err
			resp := h.request(t, http.MethodPost, "/runtime/task/decision", map[string]string{
				"decision": "approved",
				"reason":   "looks good",
			})
			if resp.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", resp.Code, tc.wantStatus, resp.Body.String())
			}
			if len(h.runEvents.events) != 1 || h.runEvents.events[0].eventType != tc.wantEvent {
				t.Fatalf("run events = %#v, want one %s event", h.runEvents.events, tc.wantEvent)
			}
		})
	}
}

func TestRuntimeHandler_RecordAgentDecisionHidesUnexpectedErrors(t *testing.T) {
	h := newRuntimeHandlerHarness(t, Capabilities{})
	h.decisions.err = errors.New("database password leaked")

	resp := h.request(t, http.MethodPost, "/runtime/task/decision", map[string]string{
		"decision": "approved",
		"reason":   "looks good",
	})
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusInternalServerError, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "database password leaked") {
		t.Fatalf("body leaked internal error: %q", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "internal runtime error") {
		t.Fatalf("body = %q, want opaque runtime error", resp.Body.String())
	}
}

func mustDecisionTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse decision time: %v", err)
	}
	return parsed
}
