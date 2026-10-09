package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

func TestBackgroundWorkRepositoryRoundTrip(t *testing.T) {
	repo := newRepoForSessionTests(t)

	ctx := context.Background()
	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()

	err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Test WS"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	err = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Test Task"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	err = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID})
	if err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	workID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	workload := &models.BackgroundWorkload{
		ID:        workID,
		TaskID:    taskID,
		SessionID: sessionID,
		Kind:      "shell",
		Title:     "npm test",
		State:     "running",
		Output:    "Running tests...\n",
		StartedAt: &now,
		Revision:  1,
	}

	if err := repo.UpsertBackgroundWorkload(ctx, workload); err != nil {
		t.Fatalf("UpsertBackgroundWorkload: %v", err)
	}

	got, err := repo.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil {
		t.Fatalf("GetBackgroundWorkload: %v", err)
	}
	if got == nil || got.ID != workID || got.Title != "npm test" || got.State != "running" {
		t.Fatalf("GetBackgroundWorkload returned unexpected record: %#v", got)
	}

	list, err := repo.ListBackgroundWorkloadsBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListBackgroundWorkloadsBySession: %v", err)
	}
	if len(list) != 1 || list[0].ID != workID {
		t.Fatalf("ListBackgroundWorkloadsBySession count = %d, want 1", len(list))
	}

	runID := uuid.New().String()
	run := &models.BackgroundRun{
		ID:         runID,
		WorkloadID: workID,
		SessionID:  sessionID,
		State:      "running",
		StartedAt:  &now,
	}
	if err := repo.UpsertBackgroundRun(ctx, run); err != nil {
		t.Fatalf("UpsertBackgroundRun: %v", err)
	}

	runs, err := repo.ListBackgroundRunsByWorkload(ctx, sessionID, workID)
	if err != nil {
		t.Fatalf("ListBackgroundRunsByWorkload: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != runID {
		t.Fatalf("ListBackgroundRunsByWorkload count = %d, want 1", len(runs))
	}

	// Test update state and finished_at
	finished := now.Add(10 * time.Second)
	exitCode := 0
	workload.State = "completed"
	workload.FinishedAt = &finished
	workload.ExitCode = &exitCode
	workload.Output += "All tests passed.\n"
	workload.Revision = 2

	if err := repo.UpsertBackgroundWorkload(ctx, workload); err != nil {
		t.Fatalf("UpsertBackgroundWorkload update: %v", err)
	}

	updated, err := repo.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil {
		t.Fatalf("GetBackgroundWorkload after update: %v", err)
	}
	if updated.State != "completed" || updated.ExitCode == nil || *updated.ExitCode != 0 || updated.Revision != 2 {
		t.Fatalf("updated workload = %#v, want completed state and exit code 0", updated)
	}

	// Test session isolation (R06): another session with the same workID
	sessionID2 := uuid.New().String()
	err = repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID2, TaskID: taskID})
	if err != nil {
		t.Fatalf("CreateTaskSession 2: %v", err)
	}
	workload2 := &models.BackgroundWorkload{
		ID:        workID,
		TaskID:    taskID,
		SessionID: sessionID2,
		Kind:      "shell",
		Title:     "npm test session 2",
		State:     "running",
		Output:    "Session 2 output\n",
		StartedAt: &now,
		Revision:  1,
	}
	if err := repo.UpsertBackgroundWorkload(ctx, workload2); err != nil {
		t.Fatalf("UpsertBackgroundWorkload session 2: %v", err)
	}

	s1Work, err := repo.GetBackgroundWorkload(ctx, sessionID, workID)
	if err != nil || s1Work == nil || s1Work.Title != "npm test" || s1Work.State != "completed" {
		t.Fatalf("session 1 workload corrupted by session 2: %#v", s1Work)
	}
	s2Work, err := repo.GetBackgroundWorkload(ctx, sessionID2, workID)
	if err != nil || s2Work == nil || s2Work.Title != "npm test session 2" || s2Work.State != "running" {
		t.Fatalf("session 2 workload not found or incorrect: %#v", s2Work)
	}

	// Test session deletion cleanup
	if err := repo.DeleteBackgroundWorkloadsBySession(ctx, sessionID); err != nil {
		t.Fatalf("DeleteBackgroundWorkloadsBySession: %v", err)
	}
	afterDelete, err := repo.ListBackgroundWorkloadsBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListBackgroundWorkloadsBySession after delete: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Fatalf("expected 0 workloads after delete, got %d", len(afterDelete))
	}
	// Session 2 workloads must still exist
	s2List, err := repo.ListBackgroundWorkloadsBySession(ctx, sessionID2)
	if err != nil || len(s2List) != 1 {
		t.Fatalf("session 2 workloads deleted by session 1 cleanup: %v", s2List)
	}
}
