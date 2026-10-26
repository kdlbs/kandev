package sandboxruntime

import (
	"context"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// CleanupSessionState requires the caller's session-deletion admission fence.
// All attempts are checked before any retained child allocation is removed.
func (m *Manager) CleanupSessionState(ctx context.Context, taskID, sessionID string) error {
	if taskID == "" || sessionID == "" {
		return policy.ErrConflict
	}
	states := map[string]*contract.AgentStateSnapshot{}
	cursor := policy.CleanupCursor{}
	for {
		page, err := m.policies.SessionCleanupAttempts(ctx, taskID, sessionID, cursor)
		if err != nil {
			return err
		}
		if len(page) == 0 {
			break
		}
		for _, attempt := range page {
			state, err := m.stoppedSessionState(ctx, attempt)
			if err != nil {
				return err
			}
			if state != nil {
				states[state.Home] = state
			}
			cursor = policy.CleanupCursor{OrgID: attempt.Snapshot.OrgID, AttemptID: attempt.ID}
		}
	}
	for _, state := range states {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := contract.RemoveCapturedAgentState(m.base, state); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) stoppedSessionState(ctx context.Context, attempt *policy.Attempt) (*contract.AgentStateSnapshot, error) {
	m.mu.Lock()
	blocked := m.closed || m.attempts[attempt.ID] != nil
	m.mu.Unlock()
	if blocked {
		return nil, policy.ErrConflict
	}
	if attempt.State != policy.AttemptStopped && attempt.State != policy.AttemptFailed {
		var err error
		attempt, err = m.ReconcileAttempt(ctx, attempt.Snapshot.OrgID, attempt.ID)
		if err != nil {
			return nil, err
		}
	}
	if attempt.Owner != nil && !recoveredSessionTerminated(attempt.Owner) {
		return nil, policy.ErrConflict
	}
	snapshot := attempt.Snapshot
	state := snapshot.AgentState
	if state == nil {
		return nil, nil
	}
	expected := contract.AgentStateScope{OrgID: snapshot.OrgID, WorkspaceID: snapshot.WorkspaceID, SessionID: snapshot.SessionID,
		AgentProfileID: snapshot.AgentProfileID, InstallationID: snapshot.Provider.InstallationID, ProviderKey: snapshot.Provider.Key}
	if state.Scope != expected || snapshot.Launch.Containment == nil {
		return nil, policy.ErrConflict
	}
	if attempt.State == policy.AttemptFailed {
		empty, err := snapshot.Launch.Containment.Empty()
		if err != nil || !empty {
			return nil, policy.ErrConflict
		}
	}
	return state, nil
}
