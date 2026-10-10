package service

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type countingCompletionGateBatchRepository struct {
	*sqliterepo.Repository
	calls     int
	requested [][]string
}

func (r *countingCompletionGateBatchRepository) GetTaskCompletionGateSummaries(
	ctx context.Context,
	taskIDs []string,
) (models.TaskCompletionGateSummaryBatch, error) {
	r.calls++
	r.requested = append(r.requested, append([]string(nil), taskIDs...))
	return r.Repository.GetTaskCompletionGateSummaries(ctx, taskIDs)
}

func TestTaskStatusSummaryBatchRepair(t *testing.T) {
	ctx := context.Background()
	svc, eventBus, repo := createTestService(t)
	createTaskWithoutRepositories(t, ctx, repo)
	svc.statusSummaries = repo
	reader := &countingCompletionGateBatchRepository{Repository: repo}
	svc.tasks = reader

	const taskCount = 130
	tasks := make([]*models.Task, 0, taskCount)
	for index := 0; index < taskCount; index++ {
		taskID := fmt.Sprintf("status-batch-task-%03d", index)
		task := &models.Task{ID: taskID, WorkspaceID: "ws-1", Title: taskID}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %q: %v", taskID, err)
		}
		tasks = append(tasks, task)
	}
	if _, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID:      tasks[0].ID,
		WorkspaceID: "ws-1",
		ActorKind:   "human",
		ActorID:     "status-batch-test",
		Criteria: []models.TaskCompletionCriterion{{
			ID: "tests", Description: "Required tests pass",
			EvidenceSubject: models.TaskCompletionEvidenceSubject{
				Kind: models.TaskCompletionEvidenceArtifact, ID: "status-batch-artifact",
			},
		}},
	}); err != nil {
		t.Fatalf("set task completion criteria: %v", err)
	}

	got, err := svc.ReconcileTaskStatusSummaries(ctx, tasks, nil, nil, nil)
	if err != nil {
		t.Fatalf("reconcile missing task summaries: %v", err)
	}
	if reader.calls != 1 || !reflect.DeepEqual(reader.requested[0], taskIDsForSummaryBatch(tasks)) {
		t.Fatalf("gate batch calls = %d, requested task IDs = %v", reader.calls, reader.requested)
	}
	if len(got) != taskCount || got[tasks[0].ID] == nil || got[tasks[0].ID].CompletionGate == nil || !got[tasks[0].ID].CompletionGate.Blocked {
		t.Fatalf("repaired task summaries = %d, gated summary = %+v", len(got), got[tasks[0].ID])
	}
	for _, task := range tasks[1:] {
		if got[task.ID] == nil || got[task.ID].CompletionGate != nil {
			t.Fatalf("task %q without a gate has summary %+v", task.ID, got[task.ID])
		}
	}

	stored, err := repo.LoadTaskStatusSummaries(ctx, taskIDsForSummaryBatch(tasks))
	if err != nil {
		t.Fatalf("load repaired summaries: %v", err)
	}
	eventBus.ClearEvents()
	reader.calls = 0
	reader.requested = nil
	warm, err := svc.ReconcileTaskStatusSummaries(ctx, tasks, nil, nil, stored)
	if err != nil {
		t.Fatalf("reconcile warm summaries: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("warm repair gate batch calls = %d, want one", reader.calls)
	}
	for taskID, before := range stored {
		if after := warm[taskID]; after == nil || after.Revision != before.Revision {
			t.Fatalf("warm summary %q changed from revision %d to %+v", taskID, before.Revision, after)
		}
	}
	if events := eventBus.GetPublishedEvents(); len(events) != 0 {
		t.Fatalf("warm no-op repair published %d events", len(events))
	}
}

func taskIDsForSummaryBatch(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

var _ repository.TaskCompletionGateSummaryReader = (*countingCompletionGateBatchRepository)(nil)
