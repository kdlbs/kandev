package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-PLUGIN-002.4, AC-TASKS-TURN-CHANGES-002.1
func TestTurnChangeCheckoutsPluginRemoteMatchMaterializedDestinations(t *testing.T) {
	repositories := []WorkspaceRepositorySpec{
		{
			TaskEnvironmentRepoID: "environment-repo-1", TaskRepositoryID: "task-repo-1",
			RepositoryID: "repository-1", WorktreeID: "worktree-1",
			RepoName: "Acme/Service API", BaseBranch: "main",
			RepositorySubpath: "service-api", RepositorySubpathKnown: false,
		},
		{
			TaskEnvironmentRepoID: "environment-repo-2", TaskRepositoryID: "task-repo-2",
			RepositoryID: "repository-2", WorktreeID: "worktree-2",
			RepoName: "tools/library", BaseBranch: "main", CheckoutBranch: "feature/fast path",
			RepositorySubpath: "library", RepositorySubpathKnown: true,
		},
	}
	request := &LaunchRequest{
		ExecutorType: string(models.ExecutorTypePluginRemote),
		Repositories: []RepoLaunchSpec{
			{RepoName: repositories[0].RepoName, RepositoryURL: "https://example.test/acme/service.git", BaseBranch: repositories[0].BaseBranch},
			{RepoName: repositories[1].RepoName, RepositoryURL: "https://example.test/tools/library.git", BaseBranch: repositories[1].BaseBranch, CheckoutBranch: repositories[1].CheckoutBranch},
		},
	}
	materialized, err := remoteWorkspaceProjectionFromLaunch(request, true)
	require.NoError(t, err)
	require.Len(t, materialized, len(repositories))

	checkouts := turnChangeCheckoutsFromWorkspaceRepositories(repositories, request.ExecutorType)
	require.Len(t, checkouts, len(materialized))
	for index := range materialized {
		require.Equal(t, materialized[index].Destination, checkouts[index].RepositorySubpath)
		require.True(t, checkouts[index].RepositorySubpathKnown)
	}
}

// @covers AC-EXECUTORS-PLUGIN-002.4, AC-TASKS-TURN-CHANGES-002.1
func TestTurnChangeCheckoutsPluginRemotePreserveRecordedScopeWhenBranchMissing(t *testing.T) {
	repositories := []WorkspaceRepositorySpec{{
		TaskEnvironmentRepoID: "environment-repo", RepoName: "service",
		RepositorySubpath: "known-service", RepositorySubpathKnown: true,
	}}

	checkouts := turnChangeCheckoutsFromWorkspaceRepositories(repositories, string(models.ExecutorTypePluginRemote))
	require.Len(t, checkouts, 1)
	require.Equal(t, "known-service", checkouts[0].RepositorySubpath)
	require.True(t, checkouts[0].RepositorySubpathKnown)
}

// @covers AC-EXECUTORS-PLUGIN-002.4, AC-TASKS-TURN-CHANGES-002.1
func TestTurnChangeCheckoutsPluginRemoteNonPluginExecutorsPreserveRecordedSubpaths(t *testing.T) {
	repository := WorkspaceRepositorySpec{
		TaskEnvironmentRepoID: "environment-repo", TaskRepositoryID: "task-repo",
		RepositoryID: "repository", WorktreeID: "worktree", RepoName: "service",
		RepositorySubpath: "recorded/path", RepositorySubpathKnown: true,
	}
	want := []TurnChangeCheckout{{
		ID: "environment-repo", EnvironmentRepoID: "environment-repo", TaskRepositoryID: "task-repo",
		RepositoryID: "repository", WorktreeID: "worktree", DisplayName: "service",
		RepositorySubpath: "recorded/path", RepositorySubpathKnown: true,
	}}

	for _, executorType := range []string{"", "local", "worktree", "k8s", "local_docker", "remote_docker", "sprites", "ssh"} {
		t.Run(executorType, func(t *testing.T) {
			require.Equal(t, want, turnChangeCheckoutsFromWorkspaceRepositories([]WorkspaceRepositorySpec{repository}, executorType))
		})
	}
}

// @covers AC-EXECUTORS-PLUGIN-002.4, AC-TASKS-TURN-CHANGES-002.1
func TestPopulateLaunchTurnChangeCheckoutsResolvesPluginRemoteScope(t *testing.T) {
	manager := newTestManager(t)
	provider := &mockWorkspaceInfoProvider{infos: map[string]*WorkspaceInfo{
		"session-plugin-remote-checkouts": {
			TaskID: "task-plugin-remote-checkouts", SessionID: "session-plugin-remote-checkouts",
			TaskEnvironmentID: "environment-plugin-remote-checkouts",
			WorkspaceRepositories: []WorkspaceRepositorySpec{{
				TaskEnvironmentRepoID: "environment-repo-plugin-remote", RepoName: "owner/service",
				BaseBranch: "main", RepositorySubpath: "owner-service", RepositorySubpathKnown: true,
			}},
		},
	}}
	manager.SetWorkspaceInfoProvider(provider)
	execution := &AgentExecution{
		ID: "execution-plugin-remote-checkouts", TaskID: "task-plugin-remote-checkouts",
		SessionID: "session-plugin-remote-checkouts", TaskEnvironmentID: "environment-plugin-remote-checkouts",
		ExecutorType: string(models.ExecutorTypePluginRemote),
	}

	manager.populateLaunchTurnChangeCheckouts(context.Background(), execution)

	require.Len(t, execution.TurnChangeCheckouts, 1)
	require.Equal(t, "owner-service-main", execution.TurnChangeCheckouts[0].RepositorySubpath)
	require.True(t, execution.TurnChangeCheckouts[0].RepositorySubpathKnown)
	require.Equal(t, 1, provider.sessionCalls)
}

type pluginRemoteInitializationBackend struct{ ExecutorBackend }

func (pluginRemoteInitializationBackend) Name() executor.Name { return executor.Name("test") }

// @covers AC-EXECUTORS-PLUGIN-002.4, AC-TASKS-TURN-CHANGES-002.1
func TestTurnChangeCheckoutsPluginRemoteInitializeCreatedExecution(t *testing.T) {
	for _, test := range []struct {
		name                string
		infoExecutorType    string
		requestExecutorType string
	}{
		{name: "workspace executor type", infoExecutorType: string(models.ExecutorTypePluginRemote)},
		{name: "execution executor type fallback", requestExecutorType: string(models.ExecutorTypePluginRemote)},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := newTestManager(t)
			info := &WorkspaceInfo{
				TaskID: "task-created-checkouts", SessionID: "session-created-checkouts",
				TaskEnvironmentID: "environment-created-checkouts", ExecutorType: test.infoExecutorType,
				WorkspaceRepositories: []WorkspaceRepositorySpec{{
					TaskEnvironmentRepoID: "environment-repo-created", RepoName: "owner/service",
					BaseBranch: "main", RepositorySubpath: "owner-service", RepositorySubpathKnown: true,
				}},
			}
			request := &ExecutorCreateRequest{
				InstanceID: "execution-created-checkouts", TaskID: info.TaskID, SessionID: info.SessionID,
				TaskEnvironmentID: info.TaskEnvironmentID, ExecutorType: test.requestExecutorType,
			}
			execution := manager.initializeCreatedExecution(
				context.Background(), info.TaskID, info, pluginRemoteInitializationBackend{},
				&ExecutorInstance{InstanceID: request.InstanceID, TaskID: info.TaskID, SessionID: info.SessionID},
				&executionCreatePreparation{request: request},
			)

			require.Len(t, execution.TurnChangeCheckouts, 1)
			require.Equal(t, "owner-service-main", execution.TurnChangeCheckouts[0].RepositorySubpath)
			require.True(t, execution.TurnChangeCheckouts[0].RepositorySubpathKnown)
		})
	}
}
