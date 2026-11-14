package service

import (
	"context"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

// Cold workspace reconstruction uses the same persisted direct-parent proof as
// native resume before treating the child's deleted local source as historical.
func (s *Service) projectInheritedLocalTaskWorkspace(ctx context.Context, session *models.TaskSession, env *models.TaskEnvironment, info *agentruntime.WorkspaceInfo) error {
	if !inheritedLocalWorkspaceContextMatches(session, env, info) || !deletedInheritedLocalInventoryMatches(session, env) || s.taskRepos == nil || s.repoEntities == nil {
		return nil
	}
	task, err := s.tasks.GetTask(ctx, session.TaskID)
	if err != nil {
		return fmt.Errorf("get inherited workspace task: %w", err)
	}
	if !directParentWorkspaceTaskMatches(task, session, env) {
		return nil
	}
	sourceMatches, err := s.inheritedLocalWorkspaceSourceMatches(ctx, task, session)
	if err != nil || !sourceMatches {
		return err
	}
	ownerMatches, err := s.inheritedLocalWorkspaceOwnerMatches(ctx, task, env)
	if err != nil {
		return err
	}
	if ownerMatches {
		info.WorkspacePath = env.WorkspacePath
		info.WorkspaceRepositories = nil
	}
	return nil
}

func (s *Service) inheritedLocalWorkspaceSourceMatches(ctx context.Context, task *models.Task, session *models.TaskSession) (bool, error) {
	attached, err := s.taskRepos.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		return false, fmt.Errorf("list inherited workspace task repositories: %w", err)
	}
	if len(attached) != 1 || attached[0] == nil || attached[0].RepositoryID != session.RepositoryID {
		return false, nil
	}
	source, err := s.repoEntities.GetRepository(ctx, session.RepositoryID)
	if err != nil {
		return false, fmt.Errorf("get inherited workspace source: %w", err)
	}
	return source != nil && source.ID == session.RepositoryID && source.SourceType == sourceTypeLocal, nil
}

func (s *Service) inheritedLocalWorkspaceOwnerMatches(ctx context.Context, task *models.Task, env *models.TaskEnvironment) (bool, error) {
	owner, err := s.tasks.GetTask(ctx, env.TaskID)
	if err != nil {
		return false, fmt.Errorf("get inherited workspace owner: %w", err)
	}
	if owner == nil || owner.ArchivedAt != nil || owner.WorkspaceID == "" || owner.WorkspaceID != task.WorkspaceID {
		return false, nil
	}
	ownerRepositories, err := s.taskRepos.ListTaskRepositories(ctx, owner.ID)
	if err != nil {
		return false, fmt.Errorf("list inherited workspace owner repositories: %w", err)
	}
	return len(ownerRepositories) == 0, nil
}

func inheritedLocalWorkspaceContextMatches(session *models.TaskSession, env *models.TaskEnvironment, info *agentruntime.WorkspaceInfo) bool {
	if session == nil || env == nil || info == nil || info.TaskID != session.TaskID || info.TaskEnvironmentID != env.ID ||
		(info.ExecutorType != string(models.ExecutorTypeLocal) && info.ExecutorType != "local_pc") {
		return false
	}
	return env.ID != "" && env.WorkspacePath != "" && session.TaskEnvironmentID == env.ID &&
		(env.ExecutorType == "" || env.ExecutorType == info.ExecutorType) &&
		(env.Status == models.TaskEnvironmentStatusReady || env.Status == models.TaskEnvironmentStatusStopped) &&
		inheritedWorkspaceEnvironmentValidationMatches(env, info)
}

func inheritedWorkspaceEnvironmentValidationMatches(env *models.TaskEnvironment, info *agentruntime.WorkspaceInfo) bool {
	return info.ValidatedTaskEnvironmentID == env.ID && info.EnvironmentOwnerTaskID == env.TaskID &&
		info.ValidatedTaskEnvironmentGeneration == env.OwnershipGeneration && info.ValidatedExecutorType == env.ExecutorType
}

func directParentWorkspaceTaskMatches(task *models.Task, session *models.TaskSession, env *models.TaskEnvironment) bool {
	if task == nil || task.ArchivedAt != nil || task.ID != session.TaskID || task.ParentID == "" || env.TaskID != task.ParentID {
		return false
	}
	workspace, _ := task.Metadata["workspace"].(map[string]interface{})
	return workspace["mode"] == workspaceModeInheritParent
}

func deletedInheritedLocalInventoryMatches(session *models.TaskSession, env *models.TaskEnvironment) bool {
	if len(env.Repos) != 1 || env.Repos[0] == nil || session.RepositoryID == "" {
		return false
	}
	inventory := env.Repos[0]
	return inventory.TaskEnvironmentID == env.ID && inventory.RepositoryID == session.RepositoryID && inventory.Status == "deleted" &&
		inventory.WorktreeID == "" && inventory.WorktreePath == "" && inventory.WorktreeSourceClonePath == "" && inventory.WorktreeSourceCommonDir == ""
}
