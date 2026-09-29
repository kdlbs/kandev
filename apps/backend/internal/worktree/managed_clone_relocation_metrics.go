package worktree

import (
	"expvar"
)

const (
	managedCloneRelocationOutcomeCompleted          = "completed"
	managedCloneRelocationOutcomeDirtyRefused       = "dirty_refused"
	managedCloneRelocationOutcomeAuthorizationStale = "authorization_stale"
	managedCloneRelocationOutcomeReconciled         = "reconciled"
	managedCloneRelocationOutcomeFailed             = "failed"
)

var managedCloneRelocationTotal = expvar.NewMap("managed_clone_relocation_total")

func incManagedCloneRelocationOutcome(reason string) string {
	switch reason {
	case managedCloneRelocationOutcomeCompleted,
		managedCloneRelocationOutcomeDirtyRefused,
		managedCloneRelocationOutcomeAuthorizationStale,
		managedCloneRelocationOutcomeReconciled,
		managedCloneRelocationOutcomeFailed:
	default:
		reason = managedCloneRelocationOutcomeFailed
	}
	managedCloneRelocationTotal.Add(reason, 1)
	return reason
}
