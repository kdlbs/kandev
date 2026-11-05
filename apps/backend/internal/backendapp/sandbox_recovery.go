package backendapp

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agent/runtime"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
)

// Live sandbox endpoints belong to the previous backend. Recovery refuses
// adoption before commands, journal replay, or prompt admission can run.
func (a sandboxLaunchAdmission) recover(ctx context.Context, execution *runtime.AgentExecution) error {
	task, err := a.tasks.GetTask(ctx, execution.TaskID)
	if err != nil {
		return err
	}
	if task == nil {
		return policy.ErrNotFound
	}
	workspace, err := a.tasks.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil {
		return err
	}
	if workspace == nil {
		return policy.ErrNotFound
	}
	attempt, err := a.policies.LatestSessionAttempt(ctx, workspace.OrgID, execution.SessionID)
	if errors.Is(err, policy.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if attempt.Snapshot.ExecutionID != execution.ID || attempt.Snapshot.TaskID != execution.TaskID {
		return policy.ErrConflict
	}
	switch attempt.State {
	case policy.AttemptPrepared, policy.AttemptStarting, policy.AttemptRunning:
		if _, err = a.policies.TransitionAttempt(ctx, workspace.OrgID, attempt.ID, attempt.Version, policy.AttemptRecoveryRequired, nil); err != nil {
			return err
		}
	}
	return errSandboxRuntimeUnavailable
}
