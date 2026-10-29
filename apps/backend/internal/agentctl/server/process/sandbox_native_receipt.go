package process

import (
	"errors"
	"os"

	"github.com/kandev/kandev/internal/common/sandbox"
)

func (r *InteractiveRunner) reserveSandboxAttempt(launch *sandbox.Launch) error {
	if launch == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sandboxAttempts == nil {
		r.sandboxAttempts = map[string]*sandboxStartRecord{}
	}
	if _, exists := r.sandboxAttempts[launch.AttemptID]; exists {
		return sandbox.ErrInvalidPolicy
	}
	for attemptID, record := range r.sandboxAttempts {
		if _, err := os.Lstat(record.launch.Root); record.completed && errors.Is(err, os.ErrNotExist) {
			delete(r.sandboxAttempts, attemptID)
		}
	}
	if len(r.sandboxAttempts) >= 256 {
		return sandbox.ErrInvalidPolicy
	}
	r.sandboxAttempts[launch.AttemptID] = &sandboxStartRecord{launch: launch.Clone()}
	return nil
}

func nativeSandboxStartFailure(launch *sandbox.Launch, spawned bool, err error) error {
	if err == nil || launch == nil || spawned {
		return err
	}
	return &sandbox.StartFailure{Receipt: &sandbox.StartReceipt{Version: 1, AttemptID: launch.AttemptID, Generation: launch.Generation, PolicyDigest: launch.PolicyDigest, Outcome: sandbox.StartNotStarted}}
}

func (r *InteractiveRunner) completeNativeSandboxStart(launch *sandbox.Launch, owner *sandbox.ProcessOwner, spawned bool) {
	if launch == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.sandboxAttempts[launch.AttemptID]
	if record == nil {
		return
	}
	record.completed = true
	record.dispatched = spawned
	if owner != nil {
		copied := *owner
		record.owner = &copied
	}
}

// SandboxStartReceipt survives removal of the short-lived PTY process record.
func (r *InteractiveRunner) SandboxStartReceipt(attemptID string) (*sandbox.StartReceipt, error) {
	r.mu.RLock()
	record := r.sandboxAttempts[attemptID]
	if record == nil || !record.completed {
		r.mu.RUnlock()
		return nil, sandbox.ErrInvalidPolicy
	}
	snapshot := *record
	r.mu.RUnlock()
	return snapshot.receipt()
}
