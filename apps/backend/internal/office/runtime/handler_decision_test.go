package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

func mustDecisionTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse decision time: %v", err)
	}
	return parsed
}
