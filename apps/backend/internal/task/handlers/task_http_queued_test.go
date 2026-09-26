package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
)

type fakeQueuedPromptCounter struct {
	byTask map[string]int
	err    error
}

func (f fakeQueuedPromptCounter) CountPendingByTaskIDs(_ context.Context, taskIDs []string) (map[string]int, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]int, len(taskIDs))
	for _, id := range taskIDs {
		out[id] = f.byTask[id]
	}
	return out, nil
}

func newQueuedTaskDTOBuilder(t *testing.T) (*service.Service, *TaskHandlers, *taskrepo.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() {
		_ = sqlxDB.Close()
	})
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = cleanup()
	})
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "Workspace"}))
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo, StatusSummaries: repo,
	}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	return svc, h, repo
}

func createQueuedTestTask(
	t *testing.T,
	svc *service.Service,
	repo *taskrepo.Repository,
	counter fakeQueuedPromptCounter,
) *models.Task {
	t.Helper()
	ctx := context.Background()
	svc.SetQueuedPromptCounter(counter)
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "Queued badge task"}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s1", TaskID: "task-1", State: models.TaskSessionStateIdle, IsPrimary: true,
	}))
	return task
}

func TestTaskDTOBuilderStampsQueuedPromptCount(t *testing.T) {
	svc, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	task := createQueuedTestTask(t, svc, repo, fakeQueuedPromptCounter{byTask: map[string]int{"task-1": 3}})

	result, err := h.toTaskDTOsWithSessionInfo(ctx, []*models.Task{task})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].StatusSummary, "expected a status summary on the DTO")
	assert.Equal(t, 3, result[0].StatusSummary.QueuedPromptCount)

	payload, err := json.Marshal(result[0])
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"queued_prompt_count":3`)
}

func TestTaskDTOBuilderOmitsZeroQueuedPromptCount(t *testing.T) {
	svc, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	task := createQueuedTestTask(t, svc, repo, fakeQueuedPromptCounter{byTask: map[string]int{"task-1": 0}})

	result, err := h.toTaskDTOsWithSessionInfo(ctx, []*models.Task{task})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].StatusSummary)
	assert.Zero(t, result[0].StatusSummary.QueuedPromptCount)

	payload, err := json.Marshal(result[0])
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "queued_prompt_count")
}

func TestTaskDTOBuilderDegradesWhenQueuedCounterFails(t *testing.T) {
	svc, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	task := createQueuedTestTask(t, svc, repo, fakeQueuedPromptCounter{err: errors.New("queue store down")})

	result, err := h.toTaskDTOsWithSessionInfo(ctx, []*models.Task{task})
	require.NoError(t, err, "a failed queued counter must not fail the task list")
	require.Len(t, result, 1)
	require.NotNil(t, result[0].StatusSummary, "the summary must survive a queued counter failure")
	assert.Zero(t, result[0].StatusSummary.QueuedPromptCount,
		"a failed counter must clear the badge in the response (documented fallback)")
}

func TestTaskDTOBuilderPreservesProjectedCountWhenCounterUnwired(t *testing.T) {
	_, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1", Title: "Unwired counter task"}
	require.NoError(t, repo.CreateTask(ctx, task))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s1", TaskID: "task-1", State: models.TaskSessionStateIdle, IsPrimary: true,
	}))
	// A projected summary with a positive count, as if the projector had
	// persisted it. With no counter wired, the assembly must NOT overwrite it.
	changed, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID:      task.ID,
		WorkspaceID: "ws-1",
		Summary:     statussummary.TaskStatusSummary{Revision: 3, QueuedPromptCount: 5},
	})
	require.NoError(t, err)
	require.True(t, changed)

	result, err := h.toTaskDTOsWithSessionInfo(ctx, []*models.Task{task})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].StatusSummary)
	assert.Equal(t, 5, result[0].StatusSummary.QueuedPromptCount,
		"an unwired counter must preserve the projected count, not zero it")
}

func TestTaskDTOBuilderReconcilesExistingPendingSummary(t *testing.T) {
	svc, h, repo := newQueuedTaskDTOBuilder(t)
	ctx := context.Background()
	task := createQueuedTestTask(t, svc, repo, fakeQueuedPromptCounter{byTask: map[string]int{}})
	changed, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID:      task.ID,
		WorkspaceID: task.WorkspaceID,
		Summary: statussummary.TaskStatusSummary{
			Revision:      5,
			PendingAction: string(models.TaskPendingActionClarification),
			Git:           &statussummary.GitSummary{ChangedFiles: 2},
		},
	})
	require.NoError(t, err)
	require.True(t, changed)

	result, err := h.toTaskDTOsWithSessionInfo(ctx, []*models.Task{task})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result[0].StatusSummary)
	assert.Equal(t, uint64(6), result[0].StatusSummary.Revision)
	assert.Empty(t, result[0].StatusSummary.PendingAction)
	require.NotNil(t, result[0].StatusSummary.Git)
	assert.Equal(t, 2, result[0].StatusSummary.Git.ChangedFiles)
}
