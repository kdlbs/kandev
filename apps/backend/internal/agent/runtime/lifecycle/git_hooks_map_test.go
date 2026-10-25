package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func hooksSpec(name, base string, hooks bool) RepoLaunchSpec {
	return RepoLaunchSpec{RepositoryID: name, RepoName: name, BaseBranch: base, RunGitHooks: hooks}
}

func TestCollectGitHooks(t *testing.T) {
	remote := string(models.ExecutorTypeSSH)
	local := string(models.ExecutorTypeWorktree)
	cases := []struct {
		name string
		req  *LaunchRequest
		want map[string]bool
	}{
		{"nil request", nil, nil},
		{"no repositories", &LaunchRequest{ExecutorType: local}, nil},
		{"single repository off", &LaunchRequest{ExecutorType: local, RepositoryID: "r", RepoName: "app"}, nil},
		{
			"single repository opted in is addressed by the root key and its name",
			&LaunchRequest{ExecutorType: local, RepositoryID: "r", RepoName: "app", RunGitHooks: true},
			map[string]bool{"": true, "app": true},
		},
		{
			"single repository without a base branch still carries its decision",
			&LaunchRequest{ExecutorType: remote, RepositoryID: "r", RepoName: "app", RunGitHooks: true},
			map[string]bool{"": true},
		},
		{
			"multi repository: only the opted-in entry is present and there is no root key",
			&LaunchRequest{ExecutorType: local, Repositories: []RepoLaunchSpec{
				hooksSpec("front", "main", false), hooksSpec("back", "", true),
			}},
			map[string]bool{"back": true},
		},
		{
			"remote multi repository keys later entries by materialized directory",
			&LaunchRequest{ExecutorType: remote, Repositories: []RepoLaunchSpec{
				hooksSpec("front", "main", false), hooksSpec("back", "dev", true),
			}},
			map[string]bool{"back-dev": true},
		},
		{
			"remote multi repository first entry owns the root key",
			&LaunchRequest{ExecutorType: remote, Repositories: []RepoLaunchSpec{
				hooksSpec("front", "main", true), hooksSpec("back", "dev", false),
			}},
			map[string]bool{"": true},
		},
		{
			"two repositories deriving one key: an opted-in one does not enable the other",
			&LaunchRequest{ExecutorType: local, Repositories: []RepoLaunchSpec{
				{RepositoryID: "a", RepoName: "svc", BaseBranch: "main", RunGitHooks: true},
				{RepositoryID: "b", RepoName: "svc", BaseBranch: "dev", RunGitHooks: false},
			}},
			nil,
		},
		{
			"two repositories deriving one key, both opted in",
			&LaunchRequest{ExecutorType: local, Repositories: []RepoLaunchSpec{
				{RepositoryID: "a", RepoName: "svc", BaseBranch: "main", RunGitHooks: true},
				{RepositoryID: "b", RepoName: "svc", BaseBranch: "dev", RunGitHooks: true},
			}},
			map[string]bool{"svc": true},
		},
		{
			"shared key is off whichever repository comes first",
			&LaunchRequest{ExecutorType: local, Repositories: []RepoLaunchSpec{
				{RepositoryID: "b", RepoName: "svc", BaseBranch: "dev", RunGitHooks: false},
				{RepositoryID: "a", RepoName: "svc", BaseBranch: "main", RunGitHooks: true},
			}},
			nil,
		},
		{
			"plugin remote carries no map",
			&LaunchRequest{ExecutorType: string(models.ExecutorTypePluginRemote), Repositories: []RepoLaunchSpec{
				hooksSpec("front", "main", true),
			}},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, collectGitHooks(tc.req))
		})
	}
}

func TestGitHooksFromWorkspaceRepositoriesMatchesLaunchProjection(t *testing.T) {
	specs := []WorkspaceRepositorySpec{
		{RepoName: "front", BaseBranch: "main"},
		{RepoName: "back", BaseBranch: "dev", RunGitHooks: true},
	}
	launch := &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH), Repositories: []RepoLaunchSpec{
		hooksSpec("front", "main", false), hooksSpec("back", "dev", true),
	}}
	got := gitHooksFromWorkspaceRepositories(specs, launch.ExecutorType)
	assert.Equal(t, collectGitHooks(launch), got)
	assert.Equal(t, map[string]bool{"back-dev": true}, got)
	assert.Nil(t, gitHooksFromWorkspaceRepositories(nil, launch.ExecutorType))
}

func TestPrepareExecutionCreateRequestRebuildsGitHooksFromRepositoryRecords(t *testing.T) {
	mgr := newTestManager(t)
	info := &WorkspaceInfo{
		TaskID:        "task-1",
		SessionID:     "session-2",
		WorkspacePath: "/workspace",
		AgentID:       "auggie",
		ExecutorType:  string(models.ExecutorTypeLocalDocker),
		Metadata:      map[string]interface{}{MetadataKeyGitHooks: map[string]bool{"": true, "other": true}},
		WorkspaceRepositories: []WorkspaceRepositorySpec{
			{RepoName: "app", BaseBranch: "main", RunGitHooks: false},
		},
	}

	prepared, err := mgr.prepareExecutionCreateRequest(context.Background(), "task-1", info, "execution-2")
	require.NoError(t, err)
	assert.Nil(t, prepared.request.GitHooks, "a metadata-carried map must not widen the repository's setting")
	assert.NotContains(t, prepared.request.Metadata, MetadataKeyGitHooks)

	info.WorkspaceRepositories[0].RunGitHooks = true
	prepared, err = mgr.prepareExecutionCreateRequest(context.Background(), "task-1", info, "execution-3")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"": true}, prepared.request.GitHooks)
}
