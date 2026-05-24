package handlers

import (
	"errors"
	"fmt"
	"testing"

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
