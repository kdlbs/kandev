package executor

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
)

func validateTaskEnvironmentLaunchSnapshot(launch, fresh *models.TaskEnvironment, sessionID, executorType string) error {
	if launch == nil {
		if fresh.ExecutorType != "" && fresh.ExecutorType != executorType {
			return fmt.Errorf("%w: task environment was claimed by executor %q before this launch completed", models.ErrWorkspaceReuseUnsafe, fresh.ExecutorType)
		}
		return nil
	}
	if fresh.ID != launch.ID || (launch.ExecutorType != "" && fresh.ExecutorType != launch.ExecutorType) {
		return fmt.Errorf("%w: task environment changed while launch was in progress", models.ErrWorkspaceReuseUnsafe)
	}
	if launch.Status == models.TaskEnvironmentStatusCreating && launch.MaterializationSessionID == sessionID &&
		(fresh.Status != models.TaskEnvironmentStatusCreating || fresh.MaterializationSessionID != sessionID) {
		return fmt.Errorf("%w: task environment materialization passed to another session during this launch", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

func (e *Executor) bindBorrowedTaskEnvironment(ctx context.Context, environment *models.TaskEnvironment, req *LaunchAgentRequest, session *models.TaskSession) error {
	// A borrowed environment can materialize its first worktree after its owner
	// starts. Preserve that physical root identity without rewriting its inventory.
	if environment.TaskDirName != "" || !req.UseWorktree || req.TaskDirName == "" {
		bindSessionToTaskEnvironment(session, environment)
		return nil
	}
	if _, ok := e.repo.(taskEnvironmentTaskDirNameStamper); ok {
		if err := e.claimSharedTaskEnvironmentTaskDirName(ctx, environment, req); err != nil {
			return err
		}
	} else {
		environment.TaskDirName = req.TaskDirName
		if err := e.repo.UpdateTaskEnvironment(ctx, environment); err != nil {
			return fmt.Errorf("persist shared task directory name: %w", err)
		}
	}
	bindSessionToTaskEnvironment(session, environment)
	return nil
}
