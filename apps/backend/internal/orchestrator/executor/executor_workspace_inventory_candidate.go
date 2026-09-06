package executor

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func (e *Executor) workspaceInventoryRepairSession(
	ctx context.Context,
	req *LaunchAgentRequest,
	env *models.TaskEnvironment,
	session *models.TaskSession,
	repositories []*repoInfo,
) (*models.TaskSession, *models.ExecutorRunning, error) {
	if len(env.Repos) != 0 || len(session.Worktrees) != 0 {
		return session, nil, nil
	}
	spec, ok := singleWorkspaceInventoryRepairSpec(req)
	if !ok {
		return nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	position, ok := workspaceInventoryRepairPosition(spec, repositories)
	if !ok {
		return nil, nil, models.ErrWorkspaceInventoryRecoveryConflict
	}
	running, err := e.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, nil, fmt.Errorf("%w: cannot load server-owned checkout identity", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	if running != nil && !workspaceInventoryRuntimeMatchesSession(running, session) {
		return nil, nil, fmt.Errorf("%w: no server-owned checkout identity", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	if running == nil {
		running, err = e.workspaceInventoryPriorRuntime(ctx, env, session)
		if err != nil {
			return nil, nil, err
		}
	}
	if running.TaskID != session.TaskID || !workspaceInventoryRuntimeHasCheckout(running) {
		return nil, nil, fmt.Errorf("%w: no server-owned checkout identity", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	return workspaceInventorySessionWithRuntime(session, env.ID, spec.RepositoryID, position, running), running, nil
}

func (e *Executor) workspaceInventoryPriorRuntime(
	ctx context.Context,
	env *models.TaskEnvironment,
	session *models.TaskSession,
) (*models.ExecutorRunning, error) {
	sessions, err := e.repo.ListTaskSessions(ctx, session.TaskID)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot load prior workspace owners", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	environmentSessions := workspaceInventoryEnvironmentSessions(sessions, session, env.ID)
	runners, err := e.repo.ListExecutorsRunningByTaskID(ctx, session.TaskID)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot load prior workspace runtime", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	return selectWorkspaceInventoryPriorRuntime(env, session.TaskID, environmentSessions, runners)
}

func workspaceInventoryEnvironmentSessions(
	sessions []*models.TaskSession,
	current *models.TaskSession,
	environmentID string,
) map[string]struct{} {
	matched := make(map[string]struct{})
	for _, session := range sessions {
		if session != nil && session.ID != current.ID && session.TaskID == current.TaskID &&
			session.TaskEnvironmentID == environmentID {
			matched[session.ID] = struct{}{}
		}
	}
	return matched
}

func selectWorkspaceInventoryPriorRuntime(
	env *models.TaskEnvironment,
	taskID string,
	environmentSessions map[string]struct{},
	runners []*models.ExecutorRunning,
) (*models.ExecutorRunning, error) {
	var candidate *models.ExecutorRunning
	for _, running := range runners {
		if !workspaceInventoryRuntimeOwnedByEnvironment(running, taskID, environmentSessions) {
			continue
		}
		if env.ExecutorID != "" && running.ExecutorID != "" && running.ExecutorID != env.ExecutorID {
			return nil, fmt.Errorf("%w: prior runtime executor does not match environment", models.ErrWorkspaceInventoryRecoveryConflict)
		}
		if candidate != nil && !sameWorkspaceInventoryRuntimeCheckout(candidate, running) {
			return nil, fmt.Errorf("%w: multiple prior checkout identities", models.ErrWorkspaceInventoryRecoveryConflict)
		}
		candidate = running
	}
	if candidate == nil {
		return nil, fmt.Errorf("%w: no server-owned checkout identity", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	return candidate, nil
}

func workspaceInventoryRuntimeOwnedByEnvironment(
	running *models.ExecutorRunning,
	taskID string,
	environmentSessions map[string]struct{},
) bool {
	if running == nil || running.TaskID != taskID || !workspaceInventoryRuntimeHasCheckout(running) {
		return false
	}
	_, ok := environmentSessions[running.SessionID]
	return ok
}

func workspaceInventoryRuntimeMatchesSession(running *models.ExecutorRunning, session *models.TaskSession) bool {
	return running != nil && session != nil && running.TaskID == session.TaskID &&
		running.SessionID == session.ID && workspaceInventoryRuntimeHasCheckout(running)
}

func workspaceInventoryRuntimeHasCheckout(running *models.ExecutorRunning) bool {
	return running.WorktreeID != "" && running.WorktreePath != "" && running.WorktreeBranch != ""
}

func sameWorkspaceInventoryRuntimeCheckout(a, b *models.ExecutorRunning) bool {
	return a.WorktreeID == b.WorktreeID && a.WorktreePath == b.WorktreePath && a.WorktreeBranch == b.WorktreeBranch
}

func workspaceInventorySessionWithRuntime(
	session *models.TaskSession,
	environmentID, repositoryID string,
	position int,
	running *models.ExecutorRunning,
) *models.TaskSession {
	copySession := *session
	copySession.Worktrees = []*models.TaskEnvironmentRepo{{
		TaskEnvironmentID: environmentID, RepositoryID: repositoryID,
		WorktreeID: running.WorktreeID, WorktreePath: running.WorktreePath,
		WorktreeBranch: running.WorktreeBranch, Position: position,
	}}
	return &copySession
}
