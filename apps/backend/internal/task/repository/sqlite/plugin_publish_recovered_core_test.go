package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func seedTaskRepository(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-core")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-core-1", WorkspaceID: "workspace-core", Name: "core"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-core-1", TaskID: "task-descriptor-1", RepositoryID: "repo-core-1", BaseBranch: "main"}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
}

func environmentRepoIDs(t *testing.T, repo *Repository) []string {
	t.Helper()
	rows, err := repo.ListTaskEnvironmentRepos(context.Background(), "environment-descriptor-1")
	if err != nil {
		t.Fatalf("list environment repos: %v", err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RepositoryID)
	}
	return ids
}

// A provider-prepared workspace publishes the repository set the provider
// recorded. A task with repositories and an empty projection is still refused.
func TestPluginPublishProviderModeStillRefusesAnEmptyProjectionForARepoBackedTask(t *testing.T) {
	repo := newRepoForSessionTests(t)
	revision := seedCreatingEnvironment(t, repo, seedDescriptorEnvironmentSQLite, nil)
	seedTaskRepository(t, repo)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")

	_, err := repo.PublishRecoveredTaskEnvironment(context.Background(), publishParams(claim, revision))
	var mismatch *models.PluginPublishMismatchError
	if !errors.As(err, &mismatch) || mismatch.Reason != models.PluginPublishReasonRepos {
		t.Fatalf("err = %v, want the repository mismatch", err)
	}
}

// A core-prepared workspace has no provider-recorded repository set: core wrote
// the environment's repository rows itself, so the publication keeps them.
func TestPluginPublishCorePreparedWorkspaceKeepsCoreRepositoryRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	revision := seedCreatingEnvironment(t, repo, seedDescriptorEnvironmentSQLite, nil)
	seedTaskRepository(t, repo)
	if err := repo.CreateTaskEnvironmentRepo(context.Background(), &models.TaskEnvironmentRepo{
		TaskEnvironmentID: "environment-descriptor-1", RepositoryID: "repo-core-1", WorktreePath: publishWorkspace + "/core",
	}); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
	}
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	params := publishParams(claim, revision)
	params.CorePreparedWorkspace, params.WorkspacePath = true, publishWorkspace

	published, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	if err != nil || !published {
		t.Fatalf("publish = %v, %v; want published", published, err)
	}
	if env := publishedEnvironment(t, repo); env.Status != models.TaskEnvironmentStatusReady {
		t.Fatalf("status = %q, want ready", env.Status)
	}
	if ids := environmentRepoIDs(t, repo); len(ids) != 1 || ids[0] != "repo-core-1" {
		t.Fatalf("environment repos = %v, want core's row kept", ids)
	}
}

// A core-prepared provider that declares no workspace_descriptor records none;
// the publication then uses the fixed workspace path the host supplies.
func TestPluginPublishCorePreparedWorkspaceWithoutADescriptorUsesTheHostPath(t *testing.T) {
	repo := newRepoForSessionTests(t)
	revision := seedCreatingEnvironment(t, repo, seedDescriptorEnvironmentSQLite, nil)
	if _, err := repo.db.ExecContext(context.Background(),
		`UPDATE executors_running SET metadata = REPLACE(metadata, '"workspace_descriptor"', '"workspace_descriptor_x"') WHERE session_id = 'session-descriptor-1'`); err != nil {
		t.Fatalf("drop descriptor: %v", err)
	}
	seedTaskRepository(t, repo)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	params := publishParams(claim, revision)

	_, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	var mismatch *models.PluginPublishMismatchError
	if !errors.As(err, &mismatch) || mismatch.Reason != models.PluginPublishReasonWorkspace {
		t.Fatalf("provider mode err = %v, want the workspace mismatch", err)
	}
	params.CorePreparedWorkspace, params.WorkspacePath = true, "/workspace"
	published, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	if err != nil || !published {
		t.Fatalf("core publish = %v, %v; want published", published, err)
	}
	if env := publishedEnvironment(t, repo); env.WorkspacePath != "/workspace" {
		t.Fatalf("workspace path = %q, want the host path", env.WorkspacePath)
	}
}
