package backendapp

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/stretchr/testify/require"
)

func TestWrapSessionRecoveryGuardErrorTranslatesRetryableSentinel(t *testing.T) {
	err := wrapSessionRecoveryGuardError(lifecycle.ErrSessionRecoveryGuarded, "session-1")

	var guardErr *orchestrator.SessionRecoveryGuardError
	require.ErrorAs(t, err, &guardErr)
	require.True(t, guardErr.Retryable)
	require.Equal(t, "session-1", guardErr.SessionID)
	require.ErrorIs(t, err, lifecycle.ErrSessionRecoveryGuarded)
}

func TestWrapSessionRecoveryGuardErrorTranslatesNonRetryableSentinel(t *testing.T) {
	err := wrapSessionRecoveryGuardError(lifecycle.ErrSessionUnstoppableAgent, "session-2")

	var guardErr *orchestrator.SessionRecoveryGuardError
	require.ErrorAs(t, err, &guardErr)
	require.False(t, guardErr.Retryable)
	require.Equal(t, "session-2", guardErr.SessionID)
	require.ErrorIs(t, err, lifecycle.ErrSessionUnstoppableAgent)
}

func TestWrapSessionRecoveryGuardErrorPassesThroughUnrelatedError(t *testing.T) {
	cause := errors.New("boom")

	err := wrapSessionRecoveryGuardError(cause, "session-3")

	require.Same(t, cause, err)
}
