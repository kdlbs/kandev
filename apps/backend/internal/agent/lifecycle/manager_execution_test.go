package lifecycle

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrSessionWorkspaceNotReady_ErrorsIs(t *testing.T) {
	// The production code wraps ErrSessionWorkspaceNotReady with fmt.Errorf("%w", ...).
	// The terminal handler uses errors.Is to detect this sentinel and trigger retry logic.
	// This test ensures the wrapping chain stays detectable.

	wrapped := fmt.Errorf("%w: session test-session has no workspace path configured", ErrSessionWorkspaceNotReady)

	if !errors.Is(wrapped, ErrSessionWorkspaceNotReady) {
		t.Errorf("expected errors.Is(wrapped, ErrSessionWorkspaceNotReady) to be true")
	}

	// Double-wrapped (as done in ensurePassthroughExecutionReady timeout path)
	doubleWrapped := fmt.Errorf("%w: timed out after 30s", ErrSessionWorkspaceNotReady)
	if !errors.Is(doubleWrapped, ErrSessionWorkspaceNotReady) {
		t.Errorf("expected errors.Is(doubleWrapped, ErrSessionWorkspaceNotReady) to be true")
	}
}

func TestErrSessionWorkspaceNotReady_UnrelatedError(t *testing.T) {
	unrelated := fmt.Errorf("some other error: %w", errors.New("connection timeout"))

	if errors.Is(unrelated, ErrSessionWorkspaceNotReady) {
		t.Errorf("expected errors.Is to be false for unrelated error")
	}
}
