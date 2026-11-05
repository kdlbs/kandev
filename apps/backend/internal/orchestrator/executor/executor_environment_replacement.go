package executor

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type taskEnvironmentReplacementReserver interface {
	ReserveTaskEnvironmentReplacement(context.Context, *models.TaskEnvironment, *models.TaskSession, *models.TaskEnvironment) (*models.TaskEnvironment, error)
}

// Replacement precedes STARTING and every physical preparation. The repository
// validates the persisted terminal session and owns retirement and binding in
// one transaction; inherited owners never enter this path.
func (e *Executor) replaceFailedEnvironmentForResume(ctx context.Context, task *v1.Task, session *models.TaskSession, executorType string) (*models.TaskEnvironment, error) {
	reserver, ok := e.repo.(taskEnvironmentReplacementReserver)
	if !ok || !shouldUseWorktree(executorType) || taskUsesInheritedWorkspace(task) {
		return nil, nil
	}
	var expected *models.TaskEnvironment
	var err error
	if session.TaskEnvironmentID != "" {
		expected, err = e.repo.GetTaskEnvironment(ctx, session.TaskEnvironmentID)
	} else {
		expected, err = e.repo.GetTaskEnvironmentByTaskID(ctx, task.ID)
	}
	if errors.Is(err, repoerrors.ErrTaskEnvironmentNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expected == nil || expected.TaskID != task.ID {
		return nil, nil
	}
	if !expected.IsRetired() && (expected.Status != models.TaskEnvironmentStatusFailed || !taskEnvironmentHasWorkspaceResources(expected)) {
		return nil, nil
	}
	candidate := &models.TaskEnvironment{
		TaskID: task.ID, ExecutorType: executorType, ExecutorID: session.ExecutorID,
		ExecutorProfileID: session.ExecutorProfileID,
		TaskDirName:       worktree.SemanticWorktreeName(task.Title, worktree.TaskDirSuffix(task.ID)),
	}
	selected, err := reserver.ReserveTaskEnvironmentReplacement(ctx, expected, session, candidate)
	if err != nil {
		return nil, fmt.Errorf("reserve replacement environment: %w", err)
	}
	bindSessionToTaskEnvironment(session, selected)
	session.Worktrees = selected.Repos
	return selected, nil
}
