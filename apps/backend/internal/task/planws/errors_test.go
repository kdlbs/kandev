package planws

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// decode reads back the ErrorPayload a mapper produced, so assertions are made
// against the bytes a client actually receives rather than the Go values.
func decode(t *testing.T, msg *ws.Message, err error) ws.ErrorPayload {
	t.Helper()
	if err != nil {
		t.Fatalf("mapper returned error: %v", err)
	}
	if msg == nil {
		t.Fatal("mapper returned a nil message")
	}
	if msg.Type != ws.MessageTypeError {
		t.Fatalf("message type = %q, want %q", msg.Type, ws.MessageTypeError)
	}
	var payload ws.ErrorPayload
	if jsonErr := json.Unmarshal(msg.Payload, &payload); jsonErr != nil {
		t.Fatalf("unmarshal error payload: %v", jsonErr)
	}
	return payload
}

// Expectations repeated across the tables below, named so the package's
// occurrence count stays under goconst's threshold. They are declared here
// rather than imported from errors.go on purpose: the test must fail when the
// production message changes, which it cannot do if both read the same symbol.
const (
	wantTaskIDRequired   = "task_id is required"
	wantTaskNotFound     = "Task not found"
	wantTaskPlanNotFound = "Task plan not found"
)

func request() *ws.Message {
	return &ws.Message{ID: "req-1", Action: "task.plan.delete"}
}

// TestMappersCoverEverySentinel pins the code and message for each sentinel an
// action recognizes, plus the unrecognized-error fallback. The mappers are the
// single definition both plan surfaces use, so this table is the contract.
func TestMappersCoverEverySentinel(t *testing.T) {
	boom := errors.New("disk on fire")

	cases := []struct {
		name    string
		mapper  func(*ws.Message, error) (*ws.Message, error)
		err     error
		code    string
		message string
	}{
		{"create/task_id", CreateError, service.ErrTaskIDRequired, ws.ErrorCodeValidation, wantTaskIDRequired},
		{"create/task_not_found", CreateError, repository.ErrTaskNotFound, ws.ErrorCodeNotFound, wantTaskNotFound},
		{"create/fallback", CreateError, boom, ws.ErrorCodeInternalError, "Failed to create task plan: disk on fire"},
		// CreatePlan's ErrTaskPlanNotFound means the read-back after a
		// successful write found nothing — a server fault, not a missing plan.
		{"create/plan_not_found_is_internal", CreateError, service.ErrTaskPlanNotFound, ws.ErrorCodeInternalError, "Failed to create task plan: task plan not found"},

		{"get/task_id", GetError, service.ErrTaskIDRequired, ws.ErrorCodeValidation, wantTaskIDRequired},
		// The get fallback carries no err.Error() detail.
		{"get/fallback", GetError, boom, ws.ErrorCodeInternalError, "Failed to get task plan"},

		{"update/task_id", UpdateError, service.ErrTaskIDRequired, ws.ErrorCodeValidation, wantTaskIDRequired},
		{"update/task_not_found", UpdateError, repository.ErrTaskNotFound, ws.ErrorCodeNotFound, wantTaskNotFound},
		{"update/plan_not_found", UpdateError, service.ErrTaskPlanNotFound, ws.ErrorCodeNotFound, wantTaskPlanNotFound},
		{"update/fallback", UpdateError, boom, ws.ErrorCodeInternalError, "Failed to update task plan: disk on fire"},

		{"delete/task_id", DeleteError, service.ErrTaskIDRequired, ws.ErrorCodeValidation, wantTaskIDRequired},
		{"delete/plan_not_found", DeleteError, service.ErrTaskPlanNotFound, ws.ErrorCodeNotFound, wantTaskPlanNotFound},
		{"delete/fallback", DeleteError, boom, ws.ErrorCodeInternalError, "Failed to delete task plan: disk on fire"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := request()
			out, mapErr := tc.mapper(msg, tc.err)
			payload := decode(t, out, mapErr)
			if payload.Code != tc.code {
				t.Errorf("code = %q, want %q", payload.Code, tc.code)
			}
			if payload.Message != tc.message {
				t.Errorf("message = %q, want %q", payload.Message, tc.message)
			}
		})
	}
}

// TestErrorCoversFullVocabulary pins the mapper used by the actions that can
// surface any plan or revision failure.
func TestErrorCoversFullVocabulary(t *testing.T) {
	cases := []struct {
		err     error
		code    string
		message string
	}{
		{service.ErrTaskIDRequired, ws.ErrorCodeValidation, wantTaskIDRequired},
		{repository.ErrTaskNotFound, ws.ErrorCodeNotFound, wantTaskNotFound},
		{service.ErrSessionIDRequired, ws.ErrorCodeValidation, "session_id is required"},
		{service.ErrSessionTaskMismatch, ws.ErrorCodeValidation, "Session does not belong to task"},
		{service.ErrTaskPlanNotFound, ws.ErrorCodeNotFound, wantTaskPlanNotFound},
		{service.ErrRevisionIDRequired, ws.ErrorCodeValidation, "revision_id is required"},
		{service.ErrRevisionNotFound, ws.ErrorCodeNotFound, "Revision not found"},
		{service.ErrRevisionTaskMismatch, ws.ErrorCodeValidation, "Revision does not belong to task"},
	}

	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			out, mapErr := Error(request(), tc.err, "Failed to revert plan")
			payload := decode(t, out, mapErr)
			if payload.Code != tc.code {
				t.Errorf("code = %q, want %q", payload.Code, tc.code)
			}
			if payload.Message != tc.message {
				t.Errorf("message = %q, want %q", payload.Message, tc.message)
			}
		})
	}

	t.Run("fallback appends the cause", func(t *testing.T) {
		out, mapErr := Error(request(), errors.New("boom"), "Failed to revert plan")
		payload := decode(t, out, mapErr)
		if payload.Code != ws.ErrorCodeInternalError {
			t.Errorf("code = %q, want %q", payload.Code, ws.ErrorCodeInternalError)
		}
		if payload.Message != "Failed to revert plan: boom" {
			t.Errorf("message = %q", payload.Message)
		}
	})
}

// TestSentinelsMatchThroughWrapping guards the mappers against a service that
// starts annotating its errors: errors.Is, not equality, decides the code.
func TestSentinelsMatchThroughWrapping(t *testing.T) {
	wrapped := fmt.Errorf("delete plan: %w", service.ErrTaskPlanNotFound)
	out, mapErr := DeleteError(request(), wrapped)
	payload := decode(t, out, mapErr)
	if payload.Code != ws.ErrorCodeNotFound {
		t.Errorf("code = %q, want %q", payload.Code, ws.ErrorCodeNotFound)
	}
	if payload.Message != wantTaskPlanNotFound {
		t.Errorf("message = %q, want %q", payload.Message, wantTaskPlanNotFound)
	}
}

// TestErrorResponsePreservesEnvelope checks the mappers copy the request's ID
// and action onto the reply, which is how the caller correlates it.
func TestErrorResponsePreservesEnvelope(t *testing.T) {
	msg := &ws.Message{ID: "req-42", Action: ws.ActionMCPDeleteTaskPlan}
	out, err := DeleteError(msg, service.ErrTaskIDRequired)
	if err != nil {
		t.Fatalf("DeleteError: %v", err)
	}
	if out.ID != "req-42" {
		t.Errorf("ID = %q, want %q", out.ID, "req-42")
	}
	if out.Action != ws.ActionMCPDeleteTaskPlan {
		t.Errorf("Action = %q, want %q", out.Action, ws.ActionMCPDeleteTaskPlan)
	}
}
