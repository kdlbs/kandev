package handlers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestErrorsAreClassifiable verifies that the package's error-classification
// helpers detect typed sentinels through any depth of wrapping, so HTTP
// status mapping survives downstream wrap/unwrap changes.
func TestErrorsAreClassifiable(t *testing.T) {
	t.Run("isNotFound recognizes taskrepo.ErrTaskNotFound", func(t *testing.T) {
		wrapped := fmt.Errorf("look up failed: %w", taskrepo.ErrTaskNotFound)
		if !isNotFound(wrapped) {
			t.Errorf("expected wrapped ErrTaskNotFound to classify as not-found")
		}
	})

	t.Run("isAgentReportedError uses lifecycle.ErrAgentReported", func(t *testing.T) {
		wrapped := fmt.Errorf("waitForPromptDone: %w", lifecycle.ErrAgentReported)
		if !isAgentReportedError(wrapped) {
			t.Errorf("expected wrapped ErrAgentReported to classify")
		}
		if isAgentReportedError(errors.New("agent error: not the sentinel")) {
			t.Errorf("untyped lookalike must no longer classify")
		}
	})
}

// TestIsTimeoutError pins the UX-classification contract for the prompt
// error-message renderer. The pre-refactor substring check on "timeout"
// covered three classes of producer; the typed-sentinel rewrite must keep
// all three classified or the user-facing "Request timed out…" message
// silently downgrades to the generic "Failed to send message to agent".
func TestIsTimeoutError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("something else"), false},
		// net.Error with Timeout()==true (typed) — e.g. http.Client deadline.
		{"net error timeout", &timeoutNetErr{}, true},
		// Plain fmt.Errorf("timeout …") from waitForSessionReady, agentctl
		// health waits, agent-stream connect waits. No typed Timeout() method.
		{"session-ready timeout (substring fallback)", errors.New("timeout waiting for session to become ready after resume"), true},
		{"agent-stream timeout (substring fallback)", errors.New("timeout waiting for agent stream to connect after restart"), true},
		// context.DeadlineExceeded itself implements Timeout() so it matches
		// via the typed path; createPromptErrorMessage also calls errors.Is
		// against it directly — covered here for completeness.
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTimeoutError(tc.err); got != tc.want {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string        { return "i/o timeout" }
func (timeoutNetErr) Timeout() bool        { return true }
func (timeoutNetErr) Temporary() bool      { return false }
func (timeoutNetErr) Deadline() *time.Time { return nil }

var _ net.Error = timeoutNetErr{}
