// Package recoveryoperation owns the wire-safe phases and state classifications
// for durable workspace recovery progress.
package recoveryoperation

import "errors"

const (
	KindManagedCloneRelocation = "managed_clone_relocation"

	StateRunning     = "running"
	StateCompleted   = "completed"
	StateFailed      = "failed"
	StateInterrupted = "interrupted"

	PhaseChecking             = "checking"
	PhaseSnapshotting         = "snapshotting"
	PhaseVerifyingSnapshot    = "verifying_snapshot"
	PhaseRestoring            = "restoring"
	PhaseVerifyingReplacement = "verifying_replacement"
	PhasePublishing           = "publishing"
	PhaseResuming             = "resuming"
)

var (
	ErrStaleWriter = errors.New("workspace recovery operation writer is stale")
	ErrInProgress  = errors.New("workspace recovery operation is already in progress")
	ErrIdentity    = errors.New("workspace recovery operation identity mismatch")
)

func IsTerminal(state string) bool {
	switch state {
	case StateCompleted, StateFailed, StateInterrupted:
		return true
	default:
		return false
	}
}

func IsKnownPhase(phase string) bool {
	switch phase {
	case PhaseChecking, PhaseSnapshotting, PhaseVerifyingSnapshot, PhaseRestoring,
		PhaseVerifyingReplacement, PhasePublishing, PhaseResuming:
		return true
	default:
		return false
	}
}
