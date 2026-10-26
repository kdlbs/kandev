package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func seedGitHooksTask(t *testing.T, svc *Service, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateRepository(context.Context, *models.Repository) error
}, repos map[string]bool, inputs []TaskRepositoryInput) string {
	t.Helper()
	ctx := context.Background()
	setBranchUpdateWorkflowStep(svc)
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	for id, hooks := range repos {
		require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-1", Name: id, DefaultBranch: "main", RunGitHooks: hooks,
		}))
	}
	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "hooks", Repositories: inputs,
	})
	require.NoError(t, err)
	return result.Task.ID
}

func TestTaskGitHooksSingleRepositoryIsAddressedByRootAndTrackerKey(t *testing.T) {
	svc, _, repo := createTestService(t)
	taskID := seedGitHooksTask(t, svc, repo, map[string]bool{"app": true},
		[]TaskRepositoryInput{{RepositoryID: "app", BaseBranch: "main", CheckoutBranch: "feature/a"}})

	hooks, err := svc.TaskGitHooks(context.Background(), taskID)
	require.NoError(t, err)
	branches, err := svc.TaskBaseBranches(context.Background(), taskID)
	require.NoError(t, err)
	want := map[string]bool{}
	for key := range branches {
		want[key] = true
	}
	assert.Contains(t, want, "", "launch path writes the root key for a single repository")
	assert.Contains(t, want, "app")
	assert.Equal(t, want, hooks, "hooks keys equal the base-branch keys")
}

func TestTaskGitHooksMultiRepositoryHasNoRootAndDoesNotSkipEmptyBaseBranch(t *testing.T) {
	svc, _, repo := createTestService(t)
	taskID := seedGitHooksTask(t, svc, repo, map[string]bool{"front": false, "back": true},
		[]TaskRepositoryInput{
			{RepositoryID: "front", BaseBranch: "main", CheckoutBranch: "feature/a"},
			{RepositoryID: "back", BaseBranch: "", CheckoutBranch: "feature/b"},
		})

	hooks, err := svc.TaskGitHooks(context.Background(), taskID)
	require.NoError(t, err)
	assert.Len(t, hooks, 1)
	assert.NotContains(t, hooks, "")
	for key, on := range hooks {
		assert.True(t, on)
		assert.Contains(t, key, "back")
	}
}

func TestTaskGitHooksEmptyCases(t *testing.T) {
	svc, _, repo := createTestService(t)
	hooks, err := svc.TaskGitHooks(context.Background(), "")
	require.NoError(t, err)
	assert.NotNil(t, hooks)
	assert.Empty(t, hooks)

	taskID := seedGitHooksTask(t, svc, repo, map[string]bool{"app": false},
		[]TaskRepositoryInput{{RepositoryID: "app", BaseBranch: "main"}})
	hooks, err = svc.TaskGitHooks(context.Background(), taskID)
	require.NoError(t, err)
	assert.NotNil(t, hooks)
	assert.Empty(t, hooks)
}

func TestTaskGitHooksPluginRemoteCarriesNoEntries(t *testing.T) {
	svc, _, repo := createTestService(t)
	taskID := seedGitHooksTask(t, svc, repo, map[string]bool{"app": true},
		[]TaskRepositoryInput{{RepositoryID: "app", BaseBranch: "main", CheckoutBranch: "feature/a"}})
	require.NoError(t, repo.CreateTaskEnvironment(context.Background(), &models.TaskEnvironment{
		ID: "env-plugin", TaskID: taskID, ExecutorType: string(models.ExecutorTypePluginRemote),
		Status: models.TaskEnvironmentStatusReady,
		Repos:  []*models.TaskEnvironmentRepo{{ID: "env-plugin-repo", RepositoryID: "app"}},
	}))
	hooks, err := svc.TaskGitHooks(context.Background(), taskID)
	require.NoError(t, err)
	assert.Empty(t, hooks)
}

func TestFoldGitHooksEntriesSharedKeyIsOffAndWarnsOnce(t *testing.T) {
	var warned []string
	got := foldGitHooksEntries([]gitHooksEntry{
		{key: "dup", enabled: true},
		{key: "dup", enabled: true},
		{key: "mixed", enabled: true},
		{key: "mixed", enabled: false},
		{key: "solo", enabled: true},
		{key: "off", enabled: false},
	}, func(key string) { warned = append(warned, key) })

	assert.Equal(t, map[string]bool{"solo": true}, got)
	assert.ElementsMatch(t, []string{"dup", "mixed"}, warned)
}

func TestFoldGitHooksEntriesEmptyIsNonNilAndSilent(t *testing.T) {
	got := foldGitHooksEntries(nil, func(string) { t.Fatal("unexpected warning") })
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestTaskGitHooksUnreadableRepositoryIsOffOthersKeepValue(t *testing.T) {
	svc, _, repo := createTestService(t)
	taskID := seedGitHooksTask(t, svc, repo, map[string]bool{"front": true, "back": true},
		[]TaskRepositoryInput{
			{RepositoryID: "front", BaseBranch: "main", CheckoutBranch: "feature/a"},
			{RepositoryID: "back", BaseBranch: "main", CheckoutBranch: "feature/b"},
		})
	require.NoError(t, repo.DeleteRepository(context.Background(), "back"))

	hooks, err := svc.TaskGitHooks(context.Background(), taskID)
	require.NoError(t, err)
	require.Len(t, hooks, 1)
	for key, on := range hooks {
		assert.True(t, on)
		assert.Contains(t, key, "front")
	}
}
