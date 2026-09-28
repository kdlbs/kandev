package orchestrator

import "github.com/kandev/kandev/internal/task/models"

// ManagedCloneRelocationRecoveryError is the path-free websocket contract for
// the explicit file-preserving action. Stamp is the durable session error
// identity that must be echoed by the client.
type ManagedCloneRelocationRecoveryError struct {
	Stamp string
	Stale bool
}

func (e *ManagedCloneRelocationRecoveryError) Error() string {
	if e != nil && e.Stale {
		return "workspace recovery options changed; reload the session before continuing"
	}
	return "task workspace needs explicit file-preserving recovery"
}

func (e *ManagedCloneRelocationRecoveryError) Details() map[string]interface{} {
	if e == nil {
		return nil
	}
	kind := "managed_clone_relocation_required"
	if e.Stale {
		kind = "managed_clone_relocation_stale"
	}
	details := map[string]interface{}{"kind": kind}
	if e.Stamp != "" {
		details["error_stamp"] = e.Stamp
	}
	if !e.Stale {
		details["recovery_action"] = models.RecoveryActionRelocateAndResume
	}
	return details
}
