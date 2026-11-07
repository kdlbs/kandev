package executor

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (e *Executor) applyResumeRepositoryContext(
	ctx context.Context, task *v1.Task, session *models.TaskSession,
	req *LaunchAgentRequest, env *models.TaskEnvironment, repositories []*repoInfo,
) (string, []*repoInfo, error) {
	inherit, err := e.inheritedLocalResumeUsesTaskWorkspace(ctx, task, session, req, env, repositories)
	if err != nil {
		return "", nil, err
	}
	if inherit {
		req.WorkspacePath = env.WorkspacePath
		return "", nil, nil
	}
	id, err := e.applyResumeRepoConfig(ctx, task, session, req, env, repositories)
	return id, repositories, err
}

// A verified parent workspace remains reusable after its sole historical local
// repository inventory was deleted. Recovery preserves the child's attachment
// and session history without rematerializing that source in the shared folder.
func (e *Executor) inheritedLocalResumeUsesTaskWorkspace(
	ctx context.Context, task *v1.Task, session *models.TaskSession,
	req *LaunchAgentRequest, env *models.TaskEnvironment, repositories []*repoInfo,
) (bool, error) {
	if !inheritedLocalResumeIdentityMatches(task, session, req, env) ||
		!hasDeletedInheritedLocalSource(session, env, repositories) {
		return false, nil
	}
	owner, err := e.repo.GetTask(ctx, env.TaskID)
	if err != nil {
		return false, err
	}
	if owner == nil || owner.ArchivedAt != nil || owner.WorkspaceID == "" || owner.WorkspaceID != task.WorkspaceID {
		return false, nil
	}
	attached, err := e.repo.ListTaskRepositories(ctx, owner.ID)
	if err != nil {
		return false, err
	}
	return len(attached) == 0, nil
}

func inheritedLocalResumeIdentityMatches(task *v1.Task, session *models.TaskSession, req *LaunchAgentRequest, env *models.TaskEnvironment) bool {
	if !localExistingWorkspaceResume(req, env) || task == nil || session == nil {
		return false
	}
	return taskUsesDeferredEnvironmentInheritance(task.Metadata) && task.ParentID != "" &&
		env.TaskID == task.ParentID && session.TaskID == task.ID && session.TaskEnvironmentID == env.ID
}

func hasDeletedInheritedLocalSource(session *models.TaskSession, env *models.TaskEnvironment, repositories []*repoInfo) bool {
	if len(repositories) != 1 || len(env.Repos) != 1 || repositories[0] == nil || env.Repos[0] == nil {
		return false
	}
	inventory := env.Repos[0]
	return inheritedLocalSourceMatches(session, repositories[0]) &&
		inventory.RepositoryID == session.RepositoryID && inventory.TaskEnvironmentID == env.ID && inventory.Status == taskEnvironmentRepoStatusDeleted &&
		inventory.WorktreeID == "" && inventory.WorktreePath == "" &&
		inventory.WorktreeSourceClonePath == "" && inventory.WorktreeSourceCommonDir == ""
}

func inheritedLocalSourceMatches(session *models.TaskSession, repository *repoInfo) bool {
	source := repository.Repository
	return source != nil && source.SourceType == sourceTypeLocal && source.ID != "" &&
		session.RepositoryID == source.ID && repository.RepositoryID == source.ID
}
