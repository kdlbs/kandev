package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
