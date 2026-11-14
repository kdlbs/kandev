package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

type testSessionAuthenticationError struct {
	failure agentctltypes.SessionOperationFailure
	cause   error
}

func (e *testSessionAuthenticationError) Error() string {
	return "session operation requires authentication"
}
func (e *testSessionAuthenticationError) Unwrap() error { return e.cause }
func (e *testSessionAuthenticationError) SessionOperationFailure() agentctltypes.SessionOperationFailure {
	return e.failure
}

func TestBootstrapAuthenticationCause(t *testing.T) {
	wrapped := fmt.Errorf("bootstrap wrapper: %w", &testSessionAuthenticationError{
		failure: agentctltypes.SessionOperationFailure{
			Code:        agentctltypes.SessionOperationFailureCodeAuthenticationRequired,
			AuthMethods: []streams.AuthMethodInfo{{ID: "cursor_login", Name: "Cursor CLI login"}},
		},
		cause: errors.New("provider diagnostic token=private-token"),
	})
	failure := bootstrapFailureFor(&AgentExecution{}, wrapped)
	if failure == nil {
		t.Fatal("bootstrapFailureFor returned nil")
	}
	if failure.SafeCode() != models.AgentErrorCauseCodeAuthenticationRequired {
		t.Fatalf("safe code = %q, want %q", failure.SafeCode(), models.AgentErrorCauseCodeAuthenticationRequired)
	}
	if failure.SafeDetail() != "Agent authentication is required." {
		t.Fatalf("safe detail = %q, want fixed authentication detail", failure.SafeDetail())
	}
	if failure.SessionFailure == nil || len(failure.SessionFailure.AuthMethods) != 1 || failure.SessionFailure.AuthMethods[0].ID != "cursor_login" {
		t.Fatalf("transient session failure = %#v, want safe Cursor method snapshot", failure.SessionFailure)
	}
	if !errors.Is(failure, wrapped) && failure.Unwrap() != wrapped {
		t.Fatal("bootstrap failure did not retain the wrapped protocol cause")
	}

	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "forged code text", err: errors.New("ACP error code -32000 authentication required")},
		{name: "unsupported typed cause", err: &testSessionAuthenticationError{failure: agentctltypes.SessionOperationFailure{Code: "internal_error"}, cause: errors.New("authentication required")}},
		{name: "cancellation", err: context.Canceled},
		{name: "timeout", err: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bootstrapFailureFor(&AgentExecution{}, tc.err)
			if got == nil {
				t.Fatal("bootstrapFailureFor returned nil")
			}
			if tc.name == "timeout" {
				if got.SafeCode() != models.AgentErrorCauseCodeTimeout {
					t.Fatalf("safe code = %q, want timeout", got.SafeCode())
				}
				return
			}
			if tc.name == "cancellation" {
				if !errors.Is(got, context.Canceled) {
					t.Fatalf("failure = %v, want cancellation", got)
				}
				return
			}
			if got.SafeCode() == models.AgentErrorCauseCodeAuthenticationRequired {
				t.Fatalf("untrusted/non-auth error acquired authentication cause: %+v", got)
			}
		})
	}
}
