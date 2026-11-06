package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func reconcileLaunchWorkspace(ctx context.Context, rt ExecutorBackend, req *LaunchRequest, instance *ExecutorInstance) error {
	pluginRemote := req.ExecutorType == string(models.ExecutorTypePluginRemote)
	minimumRepos := 2
	if pluginRemote {
		minimumRepos = 1
	}
	if !rt.RequiresCloneURL() || len(req.RepoSpecs()) < minimumRepos || instance == nil || instance.Client == nil {
		return nil
	}
	projection, err := remoteWorkspaceProjectionFromLaunch(req, pluginRemote)
	if err != nil {
		return err
	}
	if instance.RuntimeName == agentruntime.RuntimeRemoteDocker && instance.ReusedWorkspace {
		return verifyRetainedLaunchWorkspace(ctx, req.SessionID, instance, projection)
	}
	return materializeWorkspaceRepositories(ctx, instance.Client, projection)
}
