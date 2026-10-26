package sandboxruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/processidentity"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// ReconcileAttempt never adopts a process whose private listeners belonged to
// another backend. It releases an attempt only after proving its owned descendants ended.
func (m *Manager) ReconcileAttempt(ctx context.Context, orgID, attemptID string) (*policy.Attempt, error) {
	m.drainRecoveredGroup(ctx, orgID, attemptID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.attempts[attemptID] != nil {
		return nil, policy.ErrConflict
	}
	attempt, err := m.policies.ReadAttempt(ctx, orgID, attemptID)
	if err != nil {
		return nil, err
	}
	if attempt.State == policy.AttemptStopped || attempt.State == policy.AttemptFailed {
		return attempt, nil
	}
	neverDispatched := attempt.State == policy.AttemptPrepared
	if attempt.State != policy.AttemptRecoveryRequired {
		attempt, err = m.policies.TransitionAttempt(ctx, orgID, attempt.ID, attempt.Version, policy.AttemptRecoveryRequired, nil)
		if err != nil {
			return nil, err
		}
	}
	if !neverDispatched && !recoveredSessionTerminated(attempt.Owner) {
		return attempt, policy.ErrConflict
	}
	if err = releaseRecovered(ctx, attempt); err != nil {
		return attempt, err
	}
	if err = m.releaseRecoveredState(attempt); err != nil {
		return attempt, err
	}
	if err = m.removeRecoveredFiles(attempt.Snapshot.Launch); err != nil {
		return attempt, err
	}
	return m.policies.TransitionAttempt(ctx, orgID, attempt.ID, attempt.Version, policy.AttemptStopped, nil)
}

func recoveredSessionTerminated(owner *policy.AttemptOwner) bool {
	if owner == nil {
		return false
	}
	identity := processidentity.Identity{PID: owner.SupervisorPID, GroupID: owner.SupervisorPID, SessionID: owner.SupervisorPID, BirthToken: owner.SupervisorBirth}
	state, err := processidentity.Inspect(identity)
	return err == nil && state == processidentity.StateExited
}

func (m *Manager) releaseRecoveredState(attempt *policy.Attempt) error {
	snapshot := attempt.Snapshot.AgentState
	if snapshot == nil {
		return nil
	}
	state, err := contract.OpenAgentState(m.base, snapshot.Scope)
	if err != nil {
		return err
	}
	defer func() { _ = state.Close() }()
	if state.Snapshot().Home != snapshot.Home {
		return policy.ErrConflict
	}
	if snapshot.Scope.SessionID == "" {
		return state.Remove()
	}
	return nil
}

func (m *Manager) removeRecoveredFiles(launch *contract.Launch) error {
	if launch == nil || filepath.Dir(launch.Root) != m.base || !strings.HasPrefix(filepath.Base(launch.Root), "attempt-") {
		return policy.ErrConflict
	}
	base, err := filepath.EvalSymlinks(m.base)
	if err != nil || base != m.base {
		return policy.ErrConflict
	}
	before, err := os.Lstat(launch.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || launch.Validate() != nil {
		return policy.ErrConflict
	}
	after, err := os.Lstat(launch.Root)
	if err != nil || !os.SameFile(before, after) {
		return policy.ErrConflict
	}
	return os.RemoveAll(launch.Root)
}

// drainRecoveredGroup waits, outside the manager lock, for a process-tree
// attempt's group to empty so the locked pass needs only one read.
func (m *Manager) drainRecoveredGroup(ctx context.Context, orgID, attemptID string) {
	attempt, err := m.policies.ReadAttempt(ctx, orgID, attemptID)
	if err != nil || attempt.Snapshot.Scope() != policy.OwnershipProcessTree || !recoveredSessionTerminated(attempt.Owner) {
		return
	}
	_ = awaitGroupEnded(ctx, attempt.Owner.SupervisorPID)
}

// releaseRecovered proves a stopped attempt has no live process left.
func releaseRecovered(ctx context.Context, attempt *policy.Attempt) error {
	if attempt.Snapshot.Scope() != policy.OwnershipProcessTree {
		return releaseRecoveredContainment(ctx, attempt.Snapshot.Launch)
	}
	if attempt.Owner == nil {
		return nil
	}
	members, err := groupMembers(attempt.Owner.SupervisorPID)
	if err != nil || len(members) > 0 {
		return policy.ErrConflict
	}
	return nil
}

func releaseRecoveredContainment(ctx context.Context, launch *contract.Launch) error {
	if launch == nil || launch.Containment == nil {
		return policy.ErrConflict
	}
	if err := launch.Containment.Kill(ctx); err != nil {
		return err
	}
	return launch.Containment.Remove()
}
