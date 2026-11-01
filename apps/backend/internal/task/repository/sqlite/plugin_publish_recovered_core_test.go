package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

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

// Core materializes repositories through agentctl only after the instance
// exists, and a recovery never does, so a repo-backed task is refused and the
// environment stays creating, never published ready with an empty workspace.
func TestPluginPublishCoreModeRefusesARepoBackedTaskWithNoEnvironmentRows(t *testing.T) {
	repo := newRepoForSessionTests(t)
	published, err := publishCore(t, repo, coreEnvelope, func(*models.PublishRecoveredTaskEnvironmentParams) { seedTaskRepository(t, repo) })
	requireMismatch(t, err, models.PluginPublishReasonRepos)
	if published {
		t.Fatal("published a repo-backed environment with no repository rows")
	}
	if env := publishedEnvironment(t, repo); env.Status != models.TaskEnvironmentStatusCreating {
		t.Fatalf("status = %q, want creating; the caller then fails the environment", env.Status)
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

// Rows left on the environment by an earlier materialization say nothing about
// what is on disk after a recovery, so none of them lets a repo-backed task
// publish: a complete row, a partial set, or a tombstoned one.
func TestPluginPublishCoreModeRefusesARepoBackedTaskWhateverRowsTheEnvironmentHas(t *testing.T) {
	for name, rows := range map[string][]*models.TaskEnvironmentRepo{
		"complete": {{TaskEnvironmentID: "environment-descriptor-1", RepositoryID: "repo-core-1", WorktreePath: publishWorkspace + "/core"}},
		"partial":  {{TaskEnvironmentID: "environment-descriptor-1", RepositoryID: "repo-core-1", WorktreePath: publishWorkspace + "/core"}},
		"tombstoned": {{TaskEnvironmentID: "environment-descriptor-1", RepositoryID: "repo-core-1", WorktreePath: publishWorkspace + "/core",
			DeletedAt: func() *time.Time { now := time.Now().UTC(); return &now }()}},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			revision := seedCreatingEnvironment(t, repo, seedDescriptorEnvironmentSQLite, coreEnvelope)
			seedTaskRepository(t, repo)
			if name == "partial" {
				seedSecondTaskRepository(t, repo)
			}
			for _, row := range rows {
				if err := repo.CreateTaskEnvironmentRepo(context.Background(), row); err != nil {
					t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
				}
			}
			claim := acquireClaimForDescriptorEnvironment(t, repo, publishOperationID+":recovery")
			params := publishParams(claim, revision)
			params.CorePreparedWorkspace, params.WorkspacePath = true, "/workspace"
			published, err := repo.PublishRecoveredTaskEnvironment(context.Background(), params)
			requireMismatch(t, err, models.PluginPublishReasonRepos)
			if published {
				t.Fatal("published a repo-backed core-prepared environment")
			}
			if env := publishedEnvironment(t, repo); env.Status != models.TaskEnvironmentStatusCreating {
				t.Fatalf("status = %q, want creating", env.Status)
			}
		})
	}
}

func seedSecondTaskRepository(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-core-2", WorkspaceID: "workspace-core", Name: "core2"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-core-2", TaskID: "task-descriptor-1", RepositoryID: "repo-core-2", BaseBranch: "main"}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
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
