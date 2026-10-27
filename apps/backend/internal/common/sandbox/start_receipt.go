package sandbox

import "github.com/kandev/kandev/internal/common/processidentity"

// StartReceipt is emitted only by the authenticated execution host after its
// start operation finishes. An absent receipt never proves that no process ran.
type StartReceipt struct {
	Version      int           `json:"version"`
	AttemptID    string        `json:"attempt_id"`
	Generation   uint64        `json:"generation"`
	PolicyDigest string        `json:"policy_digest"`
	Outcome      string        `json:"outcome"`
	Owner        *ProcessOwner `json:"owner,omitempty"`
}

const (
	StartNotStarted = "not_started"
	StartTerminated = "terminated"
)

func (r *StartReceipt) ValidateStopped(launch *Launch) error {
	if r == nil || launch == nil || r.Version != 1 || r.AttemptID != launch.AttemptID || r.Generation != launch.Generation || r.PolicyDigest != launch.PolicyDigest {
		return ErrInvalidPolicy
	}
	switch r.Outcome {
	case StartNotStarted:
		if r.Owner != nil {
			return ErrInvalidPolicy
		}
		return nil
	case StartTerminated:
		return r.validateTerminated(launch)
	default:
		return ErrInvalidPolicy
	}
}

func receiptOwnerMatches(owner *ProcessOwner, launch *Launch) bool {
	return owner != nil && owner.AttemptID == launch.AttemptID && owner.Generation == launch.Generation && owner.PolicyDigest == launch.PolicyDigest && owner.Supervisor.SessionID == owner.Supervisor.PID && owner.Supervisor.GroupID == owner.Supervisor.PID
}

func (r *StartReceipt) validateTerminated(launch *Launch) error {
	if !receiptOwnerMatches(r.Owner, launch) {
		return ErrInvalidPolicy
	}
	stopped, err := processidentity.OwnedSessionTerminated(r.Owner.Supervisor)
	if err != nil || !stopped {
		return ErrInvalidPolicy
	}
	empty, err := launch.Containment.Empty()
	if err != nil || !empty {
		return ErrInvalidPolicy
	}
	return nil
}
