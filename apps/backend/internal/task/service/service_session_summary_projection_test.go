package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type fallbackSessionSummaryRepository struct{ repository.SessionRepository }

type countingSessionSummaryRepository struct {
	*sqliterepo.Repository
	batchSummaryCalls int
	listSummaryCalls  int
	fullBatchCalls    int
	fullListCalls     int
}

func (r *countingSessionSummaryRepository) BatchGetTaskSessionSummaryObservations(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSessionSummaryObservation, error) {
	r.batchSummaryCalls++
	return r.Repository.BatchGetTaskSessionSummaryObservations(ctx, taskIDs)
}

func (r *countingSessionSummaryRepository) ListTaskSessionSummaryObservations(
	ctx context.Context,
	taskID string,
) ([]*models.TaskSessionSummaryObservation, error) {
	r.listSummaryCalls++
	return r.Repository.ListTaskSessionSummaryObservations(ctx, taskID)
}

func (r *countingSessionSummaryRepository) BatchGetSessionsByTaskIDs(
	ctx context.Context,
	taskIDs []string,
) (map[string][]*models.TaskSession, error) {
	r.fullBatchCalls++
	return r.Repository.BatchGetSessionsByTaskIDs(ctx, taskIDs)
}

func (r *countingSessionSummaryRepository) ListTaskSessions(
	ctx context.Context,
	taskID string,
) ([]*models.TaskSession, error) {
	r.fullListCalls++
	return r.Repository.ListTaskSessions(ctx, taskID)
}

func TestSessionSummaryServiceUsesNarrowReads(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	createTaskWithoutRepositories(t, ctx, repo)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-summary-service", TaskID: "task-1", IsPrimary: true,
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	reader := &countingSessionSummaryRepository{Repository: repo}
	svc.sessions = reader

	batch, err := svc.BatchGetTaskSessionSummaryObservations(ctx, []string{"task-1"})
	if err != nil {
		t.Fatalf("batch session summary observations: %v", err)
	}
	if len(batch["task-1"]) != 1 || batch["task-1"][0].ID != "session-summary-service" {
		t.Fatalf("batch summary observations = %+v", batch)
	}
	listed, err := svc.ListTaskSessionSummaryObservations(ctx, "task-1")
	if err != nil {
		t.Fatalf("list session summary observations: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "session-summary-service" {
		t.Fatalf("listed summary observations = %+v", listed)
	}
	if reader.batchSummaryCalls != 1 || reader.listSummaryCalls != 1 || reader.fullBatchCalls != 0 || reader.fullListCalls != 0 {
		t.Fatalf("summary/full read counts = batch %d/list %d/full batch %d/full list %d",
			reader.batchSummaryCalls, reader.listSummaryCalls, reader.fullBatchCalls, reader.fullListCalls)
	}
}

func TestSessionSummaryServiceFallbackPreservesArchivedAt(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	createTaskWithoutRepositories(t, ctx, repo)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-archived-fallback", TaskID: "task-1",
		State: models.TaskSessionStateCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	archivedAt := time.Date(2026, time.October, 25, 0, 0, 0, 0, time.UTC)
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE task_sessions SET archived_at = ? WHERE id = ?`, archivedAt, "session-archived-fallback"); err != nil {
		t.Fatal(err)
	}
	svc.sessions = &fallbackSessionSummaryRepository{SessionRepository: repo}
	listed, err := svc.ListTaskSessionSummaryObservations(ctx, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ArchivedAt == nil || !listed[0].ArchivedAt.Equal(archivedAt) {
		t.Fatalf("fallback summary archive timestamp = %+v", listed)
	}
}
