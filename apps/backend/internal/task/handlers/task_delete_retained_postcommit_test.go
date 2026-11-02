package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type retainedPostCommitJobs struct {
	repository.TaskResourceCleanupRepository
	state models.TaskResourceCleanupState
}

func (r retainedPostCommitJobs) ListTaskResourceCleanupJobs(context.Context, string) ([]*models.TaskResourceCleanupJob, error) {
	return []*models.TaskResourceCleanupJob{{OperationID: "retention", State: r.state, ResourceSnapshot: `{"workspace_cleanup":"retain","stop_targets":[{"session_id":"session","execution_id":"execution"}]}`}}, nil
}

type retainedPostCommitFailure struct{ postCommitCleanupFailure }

func (*retainedPostCommitFailure) PrepareTaskResourceCleanupWithDeleteOptions(context.Context, string, models.TaskResourceCleanupTrigger, string, bool, service.DeleteTaskOptions) error {
	return nil
}

func TestHTTPRetainedDeletePreservesStopStatusAfterPostCommitFailure(t *testing.T) {
	for _, state := range []models.TaskResourceCleanupState{models.TaskResourceCleanupStateSucceeded, models.TaskResourceCleanupStatePending} {
		t.Run(string(state), func(t *testing.T) {
			repo := &authzDeleteRepo{}
			log := newTestLogger(t)
			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo,
				TaskEnvironments: repo, Reviews: repo, ResourceCleanups: retainedPostCommitJobs{state: state},
			}, nil, log, service.RepositoryDiscoveryConfig{})
			svc.SetWorktreeCleanup(authzDeleteCleanup{})
			h := &TaskHandlers{service: svc, repo: repo, logger: log}
			handoff := service.NewHandoffService(repo, nil, nil, nil, nil, log)
			handoff.SetTaskResourceCleaner(&retainedPostCommitFailure{postCommitCleanupFailure{err: errors.New("housekeeping unavailable")}})
			h.SetHandoffService(handoff)
			c, rec := authzDeleteRequest(t, "user-b", "task-b")
			c.Request.URL.RawQuery = "workspace_cleanup=retain"
			preview, err := svc.TaskDeletePreflightWithOptions(c.Request.Context(), []string{"task-b"}, false, service.DeleteTaskOptions{WorkspaceCleanup: models.WorkspaceCleanupRetain})
			require.NoError(t, err)
			c.Request.Header.Set(taskDeleteConfirmationHeader, preview.ConfirmationID)
			h.httpDeleteTask(c)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var result service.RetainedDeletionResult
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			require.True(t, result.Success)
			require.Equal(t, state != models.TaskResourceCleanupStateSucceeded, result.RuntimeStopPending)
			require.Equal(t, []string{"retention"}, result.RetentionOperationIDs)
			require.Equal(t, []string{"task-b"}, repo.deletes())
		})
	}
}

type unverifiedPostCommitJobs struct {
	retainedPostCommitJobs
	state models.TaskResourceCleanupState
}

func (r unverifiedPostCommitJobs) ListTaskResourceCleanupJobs(context.Context, string) ([]*models.TaskResourceCleanupJob, error) {
	return []*models.TaskResourceCleanupJob{{OperationID: "cleanup", State: r.state, ResourceSnapshot: `{"workspace_cleanup":"delete","allow_unverified_workspace_changes":true,"stop_targets":[{"session_id":"session"}]}`}}, nil
}
func TestHTTPUnverifiedDeletePreservesCommittedResult(t *testing.T) {
	for _, state := range []models.TaskResourceCleanupState{models.TaskResourceCleanupStateSucceeded, models.TaskResourceCleanupStatePending} {
		t.Run(string(state), func(t *testing.T) {
			repo := &authzDeleteRepo{}
			log := newTestLogger(t)
			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo, Executors: repo, Environments: repo,
				TaskEnvironments: repo, Reviews: repo, ResourceCleanups: unverifiedPostCommitJobs{state: state},
			}, nil, log, service.RepositoryDiscoveryConfig{})
			svc.SetWorktreeCleanup(authzDeleteCleanup{})
			h := &TaskHandlers{service: svc, repo: repo, logger: log}
			handoff := service.NewHandoffService(repo, nil, nil, nil, nil, log)
			handoff.SetTaskResourceCleaner(&retainedPostCommitFailure{postCommitCleanupFailure{err: errors.New("housekeeping unavailable")}})
			h.SetHandoffService(handoff)
			c, rec := authzDeleteRequest(t, "user-b", "task-b")
			c.Request.URL.RawQuery = "workspace_cleanup=delete&discard_worktree_changes=true&allow_unverified_workspace_changes=true"
			preview, err := svc.TaskDeletePreflightWithOptions(c.Request.Context(), []string{"task-b"}, false, service.DeleteTaskOptions{DiscardWorktreeChanges: true, AllowUnverifiedWorkspaceChanges: true})
			require.NoError(t, err)
			c.Request.Header.Set(taskDeleteConfirmationHeader, preview.ConfirmationID)
			h.httpDeleteTask(c)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var result map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			require.Equal(t, true, result["success"])
			require.Equal(t, state != models.TaskResourceCleanupStateSucceeded, result["runtime_stop_pending"])
			require.Equal(t, []any{"cleanup"}, result["cleanup_operation_ids"])
			require.Equal(t, state != models.TaskResourceCleanupStateSucceeded, result["workspace_cleanup_pending"])
			require.Equal(t, []string{"task-b"}, repo.deletes())
		})
	}
}

func (*retainedPostCommitFailure) ValidateTaskDeleteWorktreesWithOptions(context.Context, []string, service.DeleteTaskOptions) error {
	return nil
}
