package executor

import "errors"

// recoveryGuardRefusal is implemented by launch errors that report a refusal
// by the startup recovery guard (see orchestrator.SessionRecoveryGuardError).
type recoveryGuardRefusal interface {
	error
	RecoveryGuardRefusal() bool
	RetryableRecoveryGuardRefusal() bool
}

// IsRecoveryGuardRefusal reports whether err is any refusal by the startup
// recovery guard. The refusal launched nothing, so work addressed to the
// session (a peer message) must be kept rather than rolled back.
func IsRecoveryGuardRefusal(err error) bool {
	var refusal recoveryGuardRefusal
	return errors.As(err, &refusal) && refusal.RecoveryGuardRefusal()
}

// IsRetryableRecoveryGuardRefusal reports whether err is a retryable refusal.
// Only a retryable refusal leaves session state alone: the unstoppable-agent
// refusal needs another restart, so it stays a visible FAILED session.
func IsRetryableRecoveryGuardRefusal(err error) bool {
	var refusal recoveryGuardRefusal
	return errors.As(err, &refusal) && refusal.RetryableRecoveryGuardRefusal()
}
