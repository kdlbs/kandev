package handlers

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestSessionRecoveryFailureResponseExposesSafeProcessCause(t *testing.T) {
	msg, err := ws.NewRequest("req-recover", ws.ActionSessionRecover, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	exitCode := 1
	failure := &orchestrator.SessionRecoveryFailure{
		Err:      errors.New("session failed: generic readiness wrapper"),
		Identity: orchestrator.SessionRecoveryIdentity{AttemptID: "resume-1", ExecutionID: "exec-1", ErrorStamp: "stamp-1"},
		Causes: []models.AgentErrorCause{{
			Operation: models.AgentErrorCauseOperationResume,
			Code:      models.AgentErrorCauseCodeProcessExited,
			Reason:    models.AgentErrorCauseReasonMiseConfigUntrusted,
			ExitCode:  &exitCode,
		}},
	}
	resp, responseErr := sessionRecoveryFailureResponse(msg, failure, "s1")
	if responseErr != nil || resp == nil {
		t.Fatalf("response = %#v, error = %v", resp, responseErr)
	}
	var payload ws.ErrorPayload
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if payload.Details["kind"] != "session_recovery_failed" || payload.Details["error_stamp"] != "stamp-1" {
		t.Fatalf("details = %#v", payload.Details)
	}
	encoded, _ := json.Marshal(payload.Details["causes"])
	var causes []models.AgentErrorCause
	if err := json.Unmarshal(encoded, &causes); err != nil {
		t.Fatalf("decode causes: %v", err)
	}
	if len(causes) != 1 || causes[0].Code != models.AgentErrorCauseCodeProcessExited ||
		causes[0].ExitCode == nil || *causes[0].ExitCode != 1 ||
		causes[0].Detail != models.AgentErrorCauseProcessExitedDetail {
		t.Fatalf("causes = %#v", causes)
	}
}

func TestSessionRecoveryFailureResponseIgnoresUncorrelatedError(t *testing.T) {
	msg, _ := ws.NewRequest("req-recover", ws.ActionSessionRecover, nil)
	resp, err := sessionRecoveryFailureResponse(msg, errors.New("generic recovery failure"), "s1")
	if resp != nil || err != nil {
		t.Fatalf("response = %#v, error = %v, want no typed details", resp, err)
	}
}
