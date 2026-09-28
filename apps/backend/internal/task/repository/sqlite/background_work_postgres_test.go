package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresBackgroundWorkRepositoryRoundTrip(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("NewWithDB on postgres: %v", err)
	}

	ctx := context.Background()
	wsID := uuid.New().String()
	taskID := uuid.New().String()
	sessionID := uuid.New().String()

	err = repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "Postgres WS"})
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	err = repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Postgres Task"})
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
		Title:     "pg test",
		State:     "running",
		Output:    "Running pg test...\n",
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
	if got == nil || got.ID != workID || got.Title != "pg test" || got.State != "running" {
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

	finished := now.Add(10 * time.Second)
	exitCode := 0
	workload.State = "completed"
	workload.FinishedAt = &finished
	workload.ExitCode = &exitCode
	workload.Output += "Done.\n"
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
}
