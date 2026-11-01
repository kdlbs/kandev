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

var coreEnvelope = map[string]interface{}{"workspace_preparation": "core"}

func publishCore(t *testing.T, repo *Repository, extra map[string]interface{}, mutate func(*models.PublishRecoveredTaskEnvironmentParams)) (bool, error) {
	t.Helper()
	revision := seedCreatingEnvironment(t, repo, seedDescriptorEnvironmentSQLite, extra)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	params := publishParams(claim, revision)
	params.CorePreparedWorkspace, params.WorkspacePath = true, "/workspace"
	if mutate != nil {
		mutate(&params)
	}
	return repo.PublishRecoveredTaskEnvironment(context.Background(), params)
}

func requireMismatch(t *testing.T, err error, reason string) {
	t.Helper()
	var mismatch *models.PluginPublishMismatchError
	if !errors.As(err, &mismatch) || mismatch.Reason != reason {
		t.Fatalf("err = %v, want mismatch %q", err, reason)
	}
}

// Core writes task_environment_repos only when it materializes the workspace,
// which a recovery never does. A repo-backed task whose environment has no rows
// must be refused, never published ready with an empty workspace.
func TestPluginPublishCoreModeRefusesARepoBackedTaskWithNoEnvironmentRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	published, err := publishCore(t, repo, coreEnvelope, func(*models.PublishRecoveredTaskEnvironmentParams) { seedTaskRepository(t, repo) })
	requireMismatch(t, err, models.PluginPublishReasonRepos)
	if published {
		t.Fatal("published a repo-backed environment with no repository rows")
	}
	if env := publishedEnvironment(t, repo); env.Status != models.TaskEnvironmentStatusCreating {
		t.Fatalf("status = %q, want creating so cleanup and relaunch handle it", env.Status)
	}
	if ids := environmentRepoIDs(t, repo); len(ids) != 0 {
		t.Fatalf("environment repos = %v, want none", ids)
	}
}

func TestPluginPublishCoreModePublishesARepoLessTask(t *testing.T) {
	repo := newRepoForSessionTests(t)
	published, err := publishCore(t, repo, coreEnvelope, nil)
	if err != nil || !published {
		t.Fatalf("publish = %v, %v; want published", published, err)
	}
	if env := publishedEnvironment(t, repo); env.Status != models.TaskEnvironmentStatusReady {
		t.Fatalf("status = %q, want ready", env.Status)
	}
}

// Rows already on the environment (a prior materialization) are kept.
func TestPluginPublishCoreModeKeepsRowsTheEnvironmentAlreadyHas(t *testing.T) {
	repo := newRepoForSessionTests(t)
	revision := seedCreatingEnvironment(t, repo, seedDescriptorEnvironmentSQLite, coreEnvelope)
	seedTaskRepository(t, repo)
	if err := repo.CreateTaskEnvironmentRepo(context.Background(), &models.TaskEnvironmentRepo{
		TaskEnvironmentID: "environment-descriptor-1", RepositoryID: "repo-core-1", WorktreePath: publishWorkspace + "/core",
	}); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
	}
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	params := publishParams(claim, revision)
	params.CorePreparedWorkspace, params.WorkspacePath = true, "/workspace"
	published, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	if err != nil || !published {
		t.Fatalf("publish = %v, %v; want published", published, err)
	}
	if ids := environmentRepoIDs(t, repo); len(ids) != 1 || ids[0] != "repo-core-1" {
		t.Fatalf("environment repos = %v, want the existing row kept", ids)
	}
}

func dropDescriptor(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.ExecContext(context.Background(),
		`UPDATE executors_running SET metadata = REPLACE(metadata, '"workspace_descriptor"', '"workspace_descriptor_x"') WHERE session_id = 'session-descriptor-1'`); err != nil {
		t.Fatalf("drop descriptor: %v", err)
	}
}

// With no recorded descriptor the publication uses only the fixed /workspace
// the provider contract names; any other caller path is refused.
func TestPluginPublishCoreModeWithoutADescriptorRequiresTheFixedPath(t *testing.T) {
	repo := newRepoForSessionTests(t)
	revision := seedCreatingEnvironment(t, repo, seedDescriptorEnvironmentSQLite, coreEnvelope)
	dropDescriptor(t, repo)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	params := publishParams(claim, revision)
	params.CorePreparedWorkspace, params.WorkspacePath = true, "/etc"
	_, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	requireMismatch(t, err, models.PluginPublishReasonWorkspace)
	params.WorkspacePath = "/workspace"
	published, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
	if err != nil || !published {
		t.Fatalf("publish = %v, %v; want published", published, err)
	}
	if env := publishedEnvironment(t, repo); env.WorkspacePath != "/workspace" {
		t.Fatalf("workspace path = %q, want /workspace", env.WorkspacePath)
	}
}

func TestPluginPublishProviderModeWithoutADescriptorIsRefused(t *testing.T) {
	repo := newRepoForSessionTests(t)
	revision := seedCreatingEnvironment(t, repo, seedDescriptorEnvironmentSQLite, nil)
	dropDescriptor(t, repo)
	claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
	_, err := repo.PublishRecoveredTaskEnvironment(context.Background(), publishParams(claim, revision))
	requireMismatch(t, err, models.PluginPublishReasonWorkspace)
}

// The caller's CorePreparedWorkspace flag is checked against the mode the
// envelope recorded, so it cannot bypass the provider repository checks.
func TestPluginPublishRefusesACorePreparedFlagThatDisagreesWithTheEnvelope(t *testing.T) {
	repo := newRepoForSessionTests(t)
	published, err := publishCore(t, repo, map[string]interface{}{"workspace_preparation": "provider"},
		func(p *models.PublishRecoveredTaskEnvironmentParams) {
			seedTaskRepository(t, repo)
			p.Repos = []*models.TaskEnvironmentRepo{{RepositoryID: "wrong-repo"}}
		})
	requireMismatch(t, err, models.PluginPublishReasonWorkspace)
	if published {
		t.Fatal("published a provider envelope as core-prepared")
	}
}

func TestPluginPublishRefusesAProviderFlagForACoreEnvelope(t *testing.T) {
	repo := newRepoForSessionTests(t)
	_, err := publishCore(t, repo, coreEnvelope, func(p *models.PublishRecoveredTaskEnvironmentParams) {
		p.CorePreparedWorkspace = false
	})
	requireMismatch(t, err, models.PluginPublishReasonWorkspace)
}

func TestPluginPublishRefusesACoreFlagWhenTheEnvelopeRecordsRepositories(t *testing.T) {
	repo := newRepoForSessionTests(t)
	_, err := publishCore(t, repo, map[string]interface{}{
		"workspace_preparation": "core", "repositories": []interface{}{map[string]interface{}{"id": "repo-core-1"}},
	}, nil)
	requireMismatch(t, err, models.PluginPublishReasonRepos)
}
