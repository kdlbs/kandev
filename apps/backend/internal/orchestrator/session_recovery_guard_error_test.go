package orchestrator

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionRecoveryGuardErrorRetryablePreservesTypedCauseAndDetails(t *testing.T) {
	cause := errors.New("session is guarded pending recovery")
	err := &SessionRecoveryGuardError{Cause: cause, SessionID: "session-1", Retryable: true}

	require.ErrorIs(t, error(err), cause)
	require.Equal(t, map[string]interface{}{
		"kind":       "session_recovery_in_progress",
		"retryable":  true,
		"session_id": "session-1",
	}, err.Details())
	require.Contains(t, err.Error(), "try again shortly")
}

func TestSessionRecoveryGuardErrorNonRetryablePreservesTypedCauseAndDetails(t *testing.T) {
	cause := errors.New("session has an unstoppable agent from a prior launch")
	err := &SessionRecoveryGuardError{Cause: cause, SessionID: "session-2", Retryable: false}

	require.ErrorIs(t, error(err), cause)
	require.Equal(t, map[string]interface{}{
		"kind":       "session_recovery_unstoppable",
		"retryable":  false,
		"session_id": "session-2",
	}, err.Details())
	require.Contains(t, err.Error(), "requires another restart")
}

func TestSessionRecoveryGuardErrorNilReceiverIsSafe(t *testing.T) {
	var err *SessionRecoveryGuardError

	require.Nil(t, err.Unwrap())
	require.Nil(t, err.Details())
	require.Contains(t, err.Error(), "blocked by the recovery guard")
}

func TestSessionRecoveryGuardErrorDetailsCarryCode(t *testing.T) {
	withCode := (&SessionRecoveryGuardError{SessionID: "s", Retryable: true, Code: "nomad_recovery_operator_action"}).Details()
	if withCode["code"] != "nomad_recovery_operator_action" || withCode["retryable"] != true {
		t.Fatalf("details = %v", withCode)
	}
	if _, ok := (&SessionRecoveryGuardError{SessionID: "s", Retryable: true}).Details()["code"]; ok {
		t.Fatal("a guard without a code must not expose one")
	}
}
