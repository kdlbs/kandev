package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/github"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type recordingFreshCIRunService struct {
	input github.RequestFreshCIRunInput
	err   error
}

func (s *recordingFreshCIRunService) RequestFreshCIRun(
	_ context.Context, input github.RequestFreshCIRunInput,
) (*github.CIRunReceipt, error) {
	s.input = input
	if s.err != nil {
		return nil, s.err
	}
	return &github.CIRunReceipt{
		RequestID: "request-1", TaskID: input.TargetTaskID, RunID: input.SourceRunID,
		WorkflowID: 77, HeadSHA: input.ExpectedHeadSHA, Attempt: input.ExpectedSourceAttempt + 1,
		Operation: github.CIRunOperationRerunFailedJobs, EvidenceKind: input.EvidenceKind,
		Status: github.CIRunRequestSucceeded,
	}, nil
}

func TestHandleRequestFreshCIRunUsesTrustedWireIdentity(t *testing.T) {
	recorder := &recordingFreshCIRunService{}
	handler := &Handlers{freshCIRuns: recorder}
	payload := github.RequestFreshCIRunInput{
		ActorTaskID: "coordinator-1", ActorSessionID: "session-1", TargetTaskID: "target-1",
		RepositoryID: "repository-1", PRNumber: 42,
		ExpectedHeadSHA:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ExpectedWorkflowStepID: "ci-fixup", SourceRunID: 100, ExpectedSourceAttempt: 1,
		EvidenceKind: github.CIRunEvidencePRHead, IdempotencyKey: "key-1",
	}
	raw, _ := json.Marshal(payload)
	response, err := handler.handleRequestFreshCIRun(context.Background(), &ws.Message{
		ID: "message-1", Action: ws.ActionMCPRequestFreshCIRun, Payload: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Type == ws.MessageTypeError {
		t.Fatalf("response error = %s", response.Payload)
	}
	if recorder.input.ActorTaskID != "coordinator-1" || recorder.input.ActorSessionID != "session-1" {
		t.Fatalf("input = %+v", recorder.input)
	}
}

func TestHandleRequestFreshCIRunReturnsStableFailureClass(t *testing.T) {
	recorder := &recordingFreshCIRunService{err: &github.CIRunRequestError{Class: github.CIRunFailureHeadDrift}}
	handler := &Handlers{freshCIRuns: recorder}
	raw := []byte(`{
		"actor_task_id":"coordinator-1","actor_session_id":"session-1",
		"task_id":"target-1","repository_id":"repository-1","pr_number":42,
		"expected_head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"expected_workflow_step_id":"ci-fixup","source_run_id":100,
		"expected_source_attempt":1,"evidence_kind":"pr_head","idempotency_key":"key-1"
	}`)
	response, err := handler.handleRequestFreshCIRun(context.Background(), &ws.Message{
		ID: "message-1", Action: ws.ActionMCPRequestFreshCIRun, Payload: raw,
	})
	if err != nil || response.Type != ws.MessageTypeError {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	var payload ws.ErrorPayload
	if decodeErr := json.Unmarshal(response.Payload, &payload); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if payload.Code != ws.ErrorCodeConflict || payload.Details["failure_class"] != "head_drift" {
		t.Fatalf("error payload = %+v", payload)
	}
}
