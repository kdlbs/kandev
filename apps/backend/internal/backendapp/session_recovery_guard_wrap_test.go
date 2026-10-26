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

func TestWrapSessionRecoveryGuardError_CarriesNomadRecoveryCode(t *testing.T) {
	for _, code := range []string{lifecycle.NomadRecoveryCodeStarting, lifecycle.NomadRecoveryCodeOperatorAction} {
		t.Run(code, func(t *testing.T) {
			cause := &lifecycle.NomadRecoveryGuardError{Code: code}

			err := wrapSessionRecoveryGuardError(cause, "session-4")

			var guardErr *orchestrator.SessionRecoveryGuardError
			require.ErrorAs(t, err, &guardErr)
			require.Equal(t, code, guardErr.Code)
			require.True(t, guardErr.Retryable, "operator_action must not surface as the unstoppable-agent shape")
			require.Equal(t, "session-4", guardErr.SessionID)
			require.ErrorIs(t, err, lifecycle.ErrSessionRecoveryGuarded)
		})
	}
}

func TestWrapSessionRecoveryGuardError_PlainSentinelHasNoCode(t *testing.T) {
	var guardErr *orchestrator.SessionRecoveryGuardError
	require.ErrorAs(t, wrapSessionRecoveryGuardError(lifecycle.ErrSessionRecoveryGuarded, "s"), &guardErr)
	require.Empty(t, guardErr.Code)
}
