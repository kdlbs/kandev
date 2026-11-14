package executor

import (
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// A detached local repository preference is historical, not a current task
// attachment. Resume uses the already-owned task workspace without
// recreating its deleted repository inventory or changing the stored preference.
func detachedLocalResumeUsesTaskWorkspace(
	task *v1.Task,
	session *models.TaskSession,
	req *LaunchAgentRequest,
	env *models.TaskEnvironment,
	repositories []*repoInfo,
	repository *models.Repository,
) bool {
	if !localExistingWorkspaceResume(req, env) || task == nil || session == nil || repository == nil {
		return false
	}
	return len(repositories) == 0 && repository.SourceType == sourceTypeLocal && repository.ID != "" &&
		session.RepositoryID == repository.ID && session.TaskID == task.ID && env.TaskID == task.ID &&
		session.TaskEnvironmentID == env.ID
}

func localExistingWorkspaceResume(req *LaunchAgentRequest, env *models.TaskEnvironment) bool {
	// Ordinary cold resume and explicit recovery share persisted workspace
	// ownership. Recovery fences are checked independently before prompt admission.
	if req == nil || env == nil || !req.WorkspaceReuseRequired ||
		req.UseWorktree || env.ID == "" || env.WorkspacePath == "" {
		return false
	}
	if req.ExecutorType != string(models.ExecutorTypeLocal) && req.ExecutorType != "local_pc" {
		return false
	}
	if env.ExecutorType != "" && env.ExecutorType != req.ExecutorType {
		return false
	}
	return env.Status == models.TaskEnvironmentStatusReady || env.Status == models.TaskEnvironmentStatusStopped
}
