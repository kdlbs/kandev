package orchestrator

import (
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// TestErrorsAreClassifiable verifies that the orchestrator's error
// classification helpers rely exclusively on errors.Is + wrapped sentinels,
// not substring matches on the formatted message. A library upgrade that
// changes wording must not silently break these checks.
func TestErrorsAreClassifiable(t *testing.T) {
	t.Run("isAgentPromptInProgressError requires the sentinel", func(t *testing.T) {
		if !isAgentPromptInProgressError(ErrAgentPromptInProgress) {
			t.Errorf("exact sentinel must classify")
		}
		if !isAgentPromptInProgressError(fmt.Errorf("wrap: %w", ErrAgentPromptInProgress)) {
			t.Errorf("wrapped sentinel must classify")
		}
		if isAgentPromptInProgressError(errors.New("agent is currently processing a prompt")) {
			t.Errorf("untyped lookalike must no longer classify")
		}
	})

	t.Run("isSessionResetInProgressError requires the sentinel", func(t *testing.T) {
		if !isSessionResetInProgressError(ErrSessionResetInProgress) {
			t.Errorf("exact sentinel must classify")
		}
		if !isSessionResetInProgressError(fmt.Errorf("wrap: %w", ErrSessionResetInProgress)) {
			t.Errorf("wrapped sentinel must classify")
		}
		if isSessionResetInProgressError(errors.New("session reset in progress")) {
			t.Errorf("untyped lookalike must no longer classify")
		}
	})

	t.Run("isAgentAlreadyRunningError uses lifecycle.ErrAgentAlreadyRunning", func(t *testing.T) {
		if !isAgentAlreadyRunningError(lifecycle.ErrAgentAlreadyRunning) {
			t.Errorf("exact sentinel must classify")
		}
		if !isAgentAlreadyRunningError(fmt.Errorf("LaunchAgent: %w", lifecycle.ErrAgentAlreadyRunning)) {
			t.Errorf("wrapped sentinel must classify")
		}
		if isAgentAlreadyRunningError(errors.New("session already has an agent running somewhere")) {
			t.Errorf("untyped lookalike must no longer classify")
		}
	})
}
