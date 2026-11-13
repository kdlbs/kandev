package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
)

// TestTaskDTOEnrichmentPendingActionReadFailureSkipsSummaryRepair pins the
// display-only contract of the empty pending-action fallback: a failed
// pending-action read must neither persist a cleared pending action over an
// existing summary nor rebuild a missing summary from the invented empty map.
func TestTaskDTOEnrichmentPendingActionReadFailureSkipsSummaryRepair(t *testing.T) {
	permission := string(models.TaskPendingActionPermission)
	for _, testCase := range []struct {
		name    string
		missing bool
	}{
		{name: "existing summary keeps its pending action"},
		{name: "missing summary is not rebuilt", missing: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			calls := []string{}
			repo := &pendingActionFailureRepo{statusSummarySessionOrderRepo: &statusSummarySessionOrderRepo{
				httpTaskRepo: &httpTaskRepo{},
				calls:        &calls,
				session: &models.TaskSession{
					ID: "waiting", TaskID: "task-p", State: models.TaskSessionStateWaitingForInput,
				},
			}}
			store := &statusSummarySessionOrderStore{
				calls: &calls,
				current: statussummary.TaskStatusSummary{
					Revision: 1, PendingAction: permission, HasRunningSession: boolPointer(true),
				},
			}
			var summaries repository.TaskStatusSummaryRepository = store
			if testCase.missing {
				summaries = &missingStatusSummaryStore{statusSummarySessionOrderStore: store}
			}
			svc := service.NewService(service.Repos{
				Workspaces: repo, Tasks: repo, TaskRepos: repo,
				Workflows: repo, Messages: repo, Turns: repo,
				Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
				Executors: repo, Environments: repo, TaskEnvironments: repo,
				Reviews: repo, ResourceCleanups: repo, StatusSummaries: summaries,
			}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})

			dtos, err := buildTaskDTOsWithSessionInfo(
				context.Background(), svc, newTestLogger(t), nil, nil,
				[]*models.Task{{ID: "task-p", WorkspaceID: "ws-p", Title: "Task"}},
			)
			require.NoError(t, err)
			require.Len(t, dtos, 1)
			require.Positive(t, repo.pendingCalls, "the pending-action read must be attempted")
			require.Zero(t, store.writeCalls,
				"a pending-action read failure must not persist a summary repair or rebuild")
			require.Equal(t, uint64(1), store.current.Revision)
			require.Equal(t, permission, store.current.PendingAction,
				"the stored pending action must survive the failed read")
			if testCase.missing {
				require.Nil(t, dtos[0].StatusSummary, "no summary may be fabricated")
				return
			}
			require.NotNil(t, dtos[0].StatusSummary)
			require.Equal(t, permission, dtos[0].StatusSummary.PendingAction,
				"the response keeps the existing pending-action summary")
		})
	}
}

type pendingActionFailureRepo struct {
	*statusSummarySessionOrderRepo
	pendingCalls int
}

func (r *pendingActionFailureRepo) GetPendingActionsBySessionIDs(
	context.Context,
	[]string,
) (map[string]models.TaskPendingAction, error) {
	r.pendingCalls++
	return nil, errors.New("transient pending-action read failure")
}

// missingStatusSummaryStore reports no stored summary so the handler would
// take the rebuild path; writes still land on the embedded store's counter.
type missingStatusSummaryStore struct {
	*statusSummarySessionOrderStore
}

func (r *missingStatusSummaryStore) LoadTaskStatusSummaries(
	context.Context,
	[]string,
) (map[string]*statussummary.TaskStatusSummary, error) {
	*r.calls = append(*r.calls, "status-summary")
	return map[string]*statussummary.TaskStatusSummary{}, nil
}
