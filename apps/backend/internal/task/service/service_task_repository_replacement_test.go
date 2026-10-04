package service

import (
	"context"
	"encoding/json"
	"errors"
	repository "github.com/kandev/kandev/internal/task/repository"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func replacementFixture(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository, []*models.TaskRepository) {
	t.Helper()
	svc, bus, repo := newTitleTestService(t)
	seedPendingTitleTask(t, repo, "task-replacement", "", false)
	ctx := context.Background()
	for _, id := range []string{"original-a", "original-b", "new-a", "new-b"} {
		require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: id, WorkspaceID: "ws-title", Name: id, DefaultBranch: "main"}))
	}
	for i, id := range []string{"original-a", "original-b"} {
		require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: "link-" + id, TaskID: "task-replacement", RepositoryID: id, BaseBranch: "release", CheckoutBranch: "feature/old", Position: i,
			BranchPolicyID: "saved-policy", BranchPolicyName: "Saved", BranchPolicyBaseBranch: "release", BranchPolicyBranchTemplate: "feature/{title}", BranchPolicyPullRequestTarget: "release",
			Metadata: map[string]interface{}{"sentinel": map[string]interface{}{"repository": id, "values": []interface{}{"retain", float64(9)}}},
		}))
	}
	rows, err := repo.ListTaskRepositories(ctx, "task-replacement")
	require.NoError(t, err)
	bus.ClearEvents()
	return svc, bus, repo, rows
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.1
func TestTaskRepositoryReplacementPreservesOriginalSet(t *testing.T) {
	for _, update := range []bool{false, true} {
		for _, failInsert := range []bool{false, true} {
			name := "direct_resolution"
			if update {
				name = "update_resolution"
			}
			if failInsert {
				name += "_second_insert"
			}
			t.Run(name, func(t *testing.T) {
				svc, _, repo, original := replacementFixture(t)
				inputs := []TaskRepositoryInput{{RepositoryID: "new-a", BaseBranch: "main"}, {RepositoryID: "missing", BaseBranch: "main"}}
				if failInsert {
					inputs[1].RepositoryID = "new-b"
					_, err := repo.DB().Exec(`CREATE TRIGGER replacement_second_insert BEFORE INSERT ON task_repositories WHEN NEW.repository_id = 'new-b' BEGIN SELECT RAISE(ABORT, 'replacement second insert rejected'); END`)
					require.NoError(t, err)
				}
				ctx := context.Background()
				var err error
				if update {
					_, err = svc.UpdateTask(ctx, "task-replacement", &UpdateTaskRequest{Repositories: inputs})
				} else {
					err = svc.ReplaceTaskRepositories(ctx, "task-replacement", "ws-title", inputs)
				}
				require.Error(t, err)
				actual, readErr := repo.ListTaskRepositories(ctx, "task-replacement")
				require.NoError(t, readErr)
				require.Equal(t, original, actual, "failed replacement must retain every original column and no candidate row")
			})
		}
	}
	t.Run("successful_update_control", func(t *testing.T) {
		svc, _, repo, _ := replacementFixture(t)
		result, err := svc.UpdateTask(context.Background(), "task-replacement", &UpdateTaskRequest{Repositories: []TaskRepositoryInput{{RepositoryID: "new-a", BaseBranch: "main"}, {RepositoryID: "new-b", BaseBranch: "develop"}}})
		require.NoError(t, err)
		rows, err := repo.ListTaskRepositories(context.Background(), "task-replacement")
		require.NoError(t, err)
		require.Len(t, rows, 2)
		require.Equal(t, "new-a", rows[0].RepositoryID)
		require.Equal(t, "new-b", rows[1].RepositoryID)
		require.Equal(t, rows, result.Repositories)
	})
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.2
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.3
func TestTaskRepositoryReplacementCompleteSet(t *testing.T) {
	for _, update := range []bool{false, true} {
		t.Run(fmtBool(update), func(t *testing.T) {
			svc, _, repo, original := replacementFixture(t)
			ctx := context.Background()
			input := []TaskRepositoryInput{{RepositoryID: "new-b", BaseBranch: "main", CheckoutBranch: "topic", PRNumber: 42}, {RepositoryID: "new-a", BaseBranch: "develop"}, {RepositoryID: "new-b", BaseBranch: "release"}}
			if update {
				_, err := svc.UpdateTask(ctx, "task-replacement", &UpdateTaskRequest{Repositories: input})
				require.NoError(t, err)
			} else {
				require.NoError(t, svc.ReplaceTaskRepositories(ctx, "task-replacement", "ws-title", input))
			}
			rows, err := repo.ListTaskRepositories(ctx, "task-replacement")
			require.NoError(t, err)
			require.Len(t, rows, 3)
			for i, row := range rows {
				require.Equal(t, input[i].RepositoryID, row.RepositoryID)
				require.Equal(t, i, row.Position)
				require.NotEqual(t, original[0].ID, row.ID)
			}
			require.Equal(t, float64(42), rows[0].Metadata["pr_number"])
			// A duplicate effective branch must preserve the complete successful set.
			input[2] = input[0]
			require.Error(t, svc.ReplaceTaskRepositories(ctx, "task-replacement", "ws-title", input))
			unchanged, err := repo.ListTaskRepositories(ctx, "task-replacement")
			require.NoError(t, err)
			require.Equal(t, rows, unchanged)
			if update {
				_, err = svc.UpdateTask(ctx, "task-replacement", &UpdateTaskRequest{Repositories: []TaskRepositoryInput{}})
			} else {
				err = svc.ReplaceTaskRepositories(ctx, "task-replacement", "ws-title", nil)
			}
			require.NoError(t, err)
			cleared, err := repo.ListTaskRepositories(ctx, "task-replacement")
			require.NoError(t, err)
			require.Empty(t, cleared)
		})
	}
}

func fmtBool(update bool) string {
	if update {
		return "update"
	}
	return "direct"
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.4
func TestTaskRepositoryReplacementCompatibility(t *testing.T) {
	testReplacementCanonicalState(t)
	testReplacementContributionMetadata(t)
	svc, _, repo, original := replacementFixture(t)
	ctx := context.Background()
	// Immutable deleted-policy snapshots inherit from the actual canonical rows.
	options := &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadOnDemand, SparseDirectories: []string{"app"}}
	metadata := original[0].Metadata
	require.NoError(t, models.PutRepositoryCheckoutOptions(metadata, options))
	original[0].Metadata = metadata
	require.NoError(t, repo.UpdateTaskRepository(ctx, original[0]))
	inputs := []TaskRepositoryInput{{RepositoryID: "original-a", BaseBranch: "feature/fresh", BranchPolicyID: "saved-policy", PreserveBaseBranch: true}}
	before, err := json.Marshal(inputs)
	require.NoError(t, err)
	_, err = svc.UpdateTask(ctx, "task-replacement", &UpdateTaskRequest{Repositories: inputs})
	require.NoError(t, err)
	after, err := json.Marshal(inputs)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Nil(t, inputs[0].BranchPolicySnapshot)
	rows, err := repo.ListTaskRepositories(ctx, "task-replacement")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "feature/fresh", rows[0].BaseBranch)
	require.Equal(t, "Saved", rows[0].BranchPolicyName)
	inherited, err := models.GetRepositoryCheckoutOptions(rows[0].Metadata)
	require.NoError(t, err)
	require.Equal(t, options, inherited)
	// Omitted/null updates leave every association column unchanged.
	_, err = svc.UpdateTask(ctx, "task-replacement", &UpdateTaskRequest{})
	require.NoError(t, err)
	retained, err := repo.ListTaskRepositories(ctx, "task-replacement")
	require.NoError(t, err)
	require.Equal(t, rows, retained)
	// Explicit nested request data is not normalized in place, even on failure.
	inputs[0].CheckoutOptions = &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadStandard, SparseDirectories: []string{"app", "app"}}
	before, err = json.Marshal(inputs)
	require.NoError(t, err)
	svc.taskEnvironments = nil
	_, err = svc.UpdateTask(ctx, "task-replacement", &UpdateTaskRequest{Repositories: inputs})
	require.ErrorContains(t, err, "mutability")
	after, encodeErr := json.Marshal(inputs)
	require.NoError(t, encodeErr)
	require.Equal(t, before, after)
	retained, err = repo.ListTaskRepositories(ctx, "task-replacement")
	require.NoError(t, err)
	require.Equal(t, rows, retained)
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.7
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-002.8
func TestTaskRepositoryReplacementBoundary(t *testing.T) {
	t.Run("committed_rows_survive_response_read_failure", func(t *testing.T) {
		svc, bus, repo, _ := replacementFixture(t)
		svc.taskRepos = failingTaskRepoRepository{TaskRepoRepository: repo, err: errors.New("postcommit read unavailable")}
		result, err := svc.UpdateTask(context.Background(), "task-replacement", &UpdateTaskRequest{Repositories: []TaskRepositoryInput{{RepositoryID: "new-a"}, {RepositoryID: "new-b"}}})
		require.NoError(t, err)
		rows, err := repo.ListTaskRepositories(context.Background(), "task-replacement")
		require.NoError(t, err)
		require.Equal(t, rows, result.Repositories)
		require.Len(t, bus.GetPublishedEvents(), 1)
	})

	t.Run("early_provider_failure", func(t *testing.T) {
		svc, _, repo, original := replacementFixture(t)
		title := "must not commit"
		_, err := svc.UpdateTask(context.Background(), "task-replacement", &UpdateTaskRequest{Title: &title, Repositories: []TaskRepositoryInput{{Provider: "unavailable-plugin", RemoteURL: "https://forge.example/acme/repo.git"}}})
		require.Error(t, err)
		var selection *RepositorySelectionError
		require.ErrorAs(t, err, &selection)
		task, err := repo.GetTask(context.Background(), "task-replacement")
		require.NoError(t, err)
		require.NotEqual(t, title, task.Title)
		rows, err := repo.ListTaskRepositories(context.Background(), "task-replacement")
		require.NoError(t, err)
		require.Equal(t, original, rows)
	})
	t.Run("early_invalid_checkout", func(t *testing.T) {
		svc, _, repo, original := replacementFixture(t)
		title := "must remain unchanged"
		_, err := svc.UpdateTask(context.Background(), "task-replacement", &UpdateTaskRequest{Title: &title, Repositories: []TaskRepositoryInput{{RepositoryID: "new-a", CheckoutOptions: &models.RepositoryCheckoutOptions{Version: 99}}}})
		require.Error(t, err)
		task, err := repo.GetTask(context.Background(), "task-replacement")
		require.NoError(t, err)
		require.NotEqual(t, title, task.Title)
		rows, err := repo.ListTaskRepositories(context.Background(), "task-replacement")
		require.NoError(t, err)
		require.Equal(t, original, rows)
	})
	t.Run("later_storage_failure", func(t *testing.T) {
		svc, bus, repo, original := replacementFixture(t)
		_, err := repo.DB().Exec(`CREATE TRIGGER replacement_failure BEFORE INSERT ON task_repositories BEGIN SELECT RAISE(ABORT, 'replacement rejected'); END`)
		require.NoError(t, err)
		title := "separately committed task edit"
		_, err = svc.UpdateTask(context.Background(), "task-replacement", &UpdateTaskRequest{Title: &title, Repositories: []TaskRepositoryInput{{RepositoryID: "new-a"}}})
		require.Error(t, err)
		task, err := repo.GetTask(context.Background(), "task-replacement")
		require.NoError(t, err)
		require.Equal(t, title, task.Title)
		rows, err := repo.ListTaskRepositories(context.Background(), "task-replacement")
		require.NoError(t, err)
		require.Equal(t, original, rows)
		require.Empty(t, bus.GetPublishedEvents())
	})
	t.Run("typed_resolution", func(t *testing.T) {
		svc, _, _, _ := replacementFixture(t)
		err := svc.ReplaceTaskRepositories(context.Background(), "task-replacement", "ws-title", []TaskRepositoryInput{{RepositoryID: "missing"}})
		require.True(t, errors.Is(err, ErrTaskReferenceNotFound))
	})
}

// replacementInterleavingStore commits real state after service preparation and
// before the real transaction. It forwards the service's actual pure finalizer.
type replacementInterleavingStore struct {
	repository.TaskRepoRepository
	before func()
}

func (r replacementInterleavingStore) ReplaceTaskRepositories(ctx context.Context, id string, build func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error)) ([]*models.TaskRepository, error) {
	r.before()
	return r.TaskRepoRepository.ReplaceTaskRepositories(ctx, id, build)
}

func testReplacementCanonicalState(t *testing.T) {
	t.Helper()
	for _, scenario := range []string{"policy_checkout", "environment", "equal_environment", "explicit_policy", "ambiguous", "redirect"} {
		t.Run(scenario, func(t *testing.T) {
			svc, _, repo, original := replacementFixture(t)
			ctx := context.Background()
			input := TaskRepositoryInput{RepositoryID: "original-a", BranchPolicyID: "saved-policy"}
			options := &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadOnDemand, SparseDirectories: []string{"latest"}}
			if scenario == "environment" {
				input = TaskRepositoryInput{RepositoryID: "original-a", CheckoutOptions: &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadStandard}}
			}
			if scenario == "equal_environment" {
				input.CheckoutOptions = options
			}
			if scenario == "explicit_policy" {
				input.BranchPolicySnapshot = &models.RepositoryBranchPolicy{ID: "saved-policy", RepositoryID: "original-a", Name: "Explicit", BaseBranch: "explicit-base", BranchTemplate: "explicit/{title}", PullRequestTarget: "explicit-target"}
			}
			if scenario == "ambiguous" {
				input.BranchPolicyID = ""
			}
			if scenario == "redirect" {
				input.BranchPolicyID = ""
				for _, id := range []string{"original-a", "new-a"} {
					entity, err := repo.GetRepository(ctx, id)
					require.NoError(t, err)
					entity.SourceType = "provider"
					entity.Provider = "github"
					entity.ProviderOwner = "acme"
					entity.ProviderName = "repo"
					if id == "original-a" {
						entity.LocalPath = filepath.Join(t.TempDir(), "worktree")
						svc.discoveryConfig.TaskWorktreeRoots = []string{entity.LocalPath}
					}
					require.NoError(t, repo.UpdateRepository(ctx, entity))
				}
			}
			var canonical []*models.TaskRepository
			svc.taskRepos = replacementInterleavingStore{TaskRepoRepository: repo, before: func() {
				row := original[0]
				row.BranchPolicyName = "Latest immutable"
				row.BranchPolicyBaseBranch = "latest-base"
				row.BranchPolicyBranchTemplate = "latest/{title}"
				row.BranchPolicyPullRequestTarget = "latest-target"
				require.NoError(t, models.PutRepositoryCheckoutOptions(row.Metadata, options))
				require.NoError(t, repo.UpdateTaskRepository(ctx, row))
				if scenario == "environment" || scenario == "equal_environment" {
					require.NoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "replacement-env", TaskID: "task-replacement", WorkspacePath: t.TempDir(), ExecutorType: string(models.ExecutorTypeWorktree), Status: models.TaskEnvironmentStatusCreating}))
				}
				if scenario == "ambiguous" {
					require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{RepositoryID: "original-a", TaskID: "task-replacement", BaseBranch: "another"}))
				}
				var err error
				canonical, err = repo.ListTaskRepositories(ctx, "task-replacement")
				require.NoError(t, err)
			}}
			result, err := svc.UpdateTask(ctx, "task-replacement", &UpdateTaskRequest{Repositories: []TaskRepositoryInput{input}})
			if scenario == "environment" || scenario == "ambiguous" {
				if scenario == "environment" {
					require.ErrorContains(t, err, "after environment creation")
				} else {
					require.ErrorContains(t, err, "ambiguous")
				}
				rows, err := repo.ListTaskRepositories(ctx, "task-replacement")
				require.NoError(t, err)
				require.Equal(t, canonical, rows)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.Repositories, 1)
			optionsActual, err := models.GetRepositoryCheckoutOptions(result.Repositories[0].Metadata)
			require.NoError(t, err)
			require.Equal(t, options, optionsActual)
			switch scenario {
			case "redirect":
				require.Equal(t, "new-a", result.Repositories[0].RepositoryID)
			case "explicit_policy":
				require.Equal(t, "Explicit", result.Repositories[0].BranchPolicyName)
				require.Equal(t, "explicit-base", result.Repositories[0].BaseBranch)
				require.Equal(t, "explicit/{title}", result.Repositories[0].BranchPolicyBranchTemplate)
				require.Equal(t, "explicit-target", result.Repositories[0].BranchPolicyPullRequestTarget)
			default:
				require.Equal(t, "Latest immutable", result.Repositories[0].BranchPolicyName)
				require.Equal(t, "latest-base", result.Repositories[0].BaseBranch)
				require.Equal(t, "latest/{title}", result.Repositories[0].BranchPolicyBranchTemplate)
				require.Equal(t, "latest-target", result.Repositories[0].BranchPolicyPullRequestTarget)
			}
		})
	}
}

func testReplacementContributionMetadata(t *testing.T) {
	t.Helper()
	svc, _, repo, _ := replacementFixture(t)
	destination := testContributionDestinationForService()
	destination.CredentialBinding = &models.ContributionDestinationCredentialBinding{Source: "pat", Login: "alice", CredentialGeneration: 3}
	contribution := &models.RemoteContribution{Version: 1, Provider: "github", Kind: "pull_request", CanonicalURL: "https://github.com/acme/remote/pull/7", Number: 7, State: "open", BaseBranch: "release", HeadBranch: "feature/remote", HeadSHA: "0123456789abcdef0123456789abcdef01234567", SourceRepository: models.RemoteContributionRepository{Host: "github.com", Path: "acme/remote", RemoteURL: "https://github.com/acme/remote.git"}}
	input := []TaskRepositoryInput{{RepositoryID: "new-a", RemoteContribution: contribution, ContributionDestination: &destination}}
	require.NoError(t, svc.ReplaceTaskRepositories(context.Background(), "task-replacement", "ws-title", input))
	rows, err := repo.ListTaskRepositories(context.Background(), "task-replacement")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "release", rows[0].BaseBranch)
	require.Equal(t, "feature/remote", rows[0].CheckoutBranch)
	persistedContribution, ok, err := models.LoadRemoteContribution(rows[0].Metadata)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, *contribution, persistedContribution)
	persistedDestination, ok, err := models.LoadContributionDestination(rows[0].Metadata)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, destination, persistedDestination)
	require.Empty(t, input[0].BaseBranch)
	require.Empty(t, input[0].CheckoutBranch)
	destination.CredentialBinding.Login = "caller mutation"
	contribution.HeadBranch = "caller mutation"
	actual, err := repo.ListTaskRepositories(context.Background(), "task-replacement")
	require.NoError(t, err)
	require.Equal(t, rows, actual)
}
