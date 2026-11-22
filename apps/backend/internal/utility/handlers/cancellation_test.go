package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/utility/dto"
	"github.com/kandev/kandev/internal/utility/models"
)

// executePromptWithCanceledContext posts through the production router with an
// already-cancelled request context, the state a client hang-up leaves behind.
func executePromptWithCanceledContext(t *testing.T, f *utilityFixture, body string) (int, dto.ExecutePromptResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/utility/execute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	var got dto.ExecutePromptResponse
	decodeInto(t, rec.Body.Bytes(), &got)
	return rec.Code, got
}

// TestExecutePromptClientCancellationSettlesTheCall pins that a cancelled
// request leaves a terminal row. The cancelled context cannot carry the write,
// so without a detached settlement the row would stay pending forever.
func TestExecutePromptClientCancellationSettlesTheCall(t *testing.T) {
	f := newUtilityFixture(t, utilityFixtureOptions{
		executor: &stubInferenceExecutor{err: context.Canceled},
	})
	seedExecutableAgent(f)

	status, got := executePromptWithCanceledContext(t, f, `{"utility_agent_id":"custom-1","session_id":"session-1"}`)
	if status != statusClientClosedRequest {
		t.Fatalf("status = %d, want %d; body = %#v", status, statusClientClosedRequest, got)
	}

	record := f.repo.call(got.CallID)
	if record == nil {
		t.Fatal("no call record stored")
	}
	if record.Status != models.CallStatusCancelled {
		t.Fatalf("status = %q, want %q", record.Status, models.CallStatusCancelled)
	}
	if record.CompletedAt == nil {
		t.Fatal("cancelled call record has no completion timestamp")
	}
}

func TestExecutePromptClientCancellationSettlesSuccessfulCalls(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "session-bound", body: `{"utility_agent_id":"custom-1","session_id":"session-1"}`},
		{name: "sessionless", body: `{"utility_agent_id":"custom-1"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newUtilityFixture(t, utilityFixtureOptions{
				executor: &stubInferenceExecutor{resp: &agentctlutil.PromptResponse{
					Success: true, Response: "completed", Model: "gpt-5",
				}},
			})
			seedExecutableAgent(f)

			status, got := executePromptWithCanceledContext(t, f, tt.body)
			if status != http.StatusOK || !got.Success {
				t.Fatalf("status = %d, response = %#v; want a successful result", status, got)
			}

			record := f.repo.call(got.CallID)
			if record == nil || record.Status != "completed" {
				t.Fatalf("call record = %#v, want completed", record)
			}
			if record.CompletedAt == nil {
				t.Fatal("completed call record has no completion timestamp")
			}
		})
	}
}

func TestCreateCallWithExecutionProfileSurvivesCancellationAfterInsert(t *testing.T) {
	f := newUtilityFixture(t, utilityFixtureOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	f.repo.cancelAfterNextCallCreate(cancel)

	call, err := f.service.CreateCallWithExecutionProfile(ctx, "utility-1", "session-1", "prompt", "gpt-5", "logical", "concrete")
	if err != nil {
		t.Fatalf("CreateCallWithExecutionProfile: %v", err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatalf("context error = %v, want cancellation after insert", ctx.Err())
	}
	if call.AgentProfileID != "logical" || call.ExecutionProfileID != "concrete" {
		t.Fatalf("profile attribution = (%q, %q), want (logical, concrete)", call.AgentProfileID, call.ExecutionProfileID)
	}
	if got := f.repo.call(call.ID); got == nil || got.AgentProfileID != "logical" || got.ExecutionProfileID != "concrete" {
		t.Fatalf("stored call = %#v, want both profile IDs", got)
	}
	creates, updates := f.repo.callWriteCounts()
	if creates != 1 || updates != 0 {
		t.Fatalf("call writes = create %d, update %d; want one insert and no follow-up update", creates, updates)
	}
}
