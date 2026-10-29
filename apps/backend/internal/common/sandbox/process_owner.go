package sandbox

import "github.com/kandev/kandev/internal/common/processidentity"

// ProcessOwner binds the supervisor captured at spawn to an immutable launch.
// It does not assert that the child is confined.
type ProcessOwner struct {
	AttemptID    string                   `json:"attempt_id"`
	Generation   uint64                   `json:"generation"`
	PolicyDigest string                   `json:"policy_digest"`
	Supervisor   processidentity.Identity `json:"supervisor"`
}

func CaptureProcessOwner(launch *Launch, pid int) (*ProcessOwner, error) {
	if launch == nil {
		return nil, nil
	}
	identity, err := processidentity.Capture(pid)
	if err != nil {
		return nil, ErrInvalidPolicy
	}
	return &ProcessOwner{AttemptID: launch.AttemptID, Generation: launch.Generation, PolicyDigest: launch.PolicyDigest, Supervisor: identity}, nil
}

func (owner *ProcessOwner) LiveSnapshot() (*ProcessOwner, error) {
	if owner == nil {
		return nil, ErrInvalidPolicy
	}
	state, err := processidentity.Inspect(owner.Supervisor)
	if err != nil || state != processidentity.StateAlive {
		return nil, ErrInvalidPolicy
	}
	result := *owner
	return &result, nil
}
