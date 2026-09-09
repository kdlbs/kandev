package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestGitSnapshotEnvironmentPreservesStatusAfterSessionDelete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-git-environment-delete"
		envID     = "env-git-environment-delete"
		sessionID = "session-git-environment-delete"
	)

	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Environment-owned status"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: envID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: envID,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snapshot-environment-delete", SessionID: sessionID,
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main",
	}); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}

	if err := repo.DeleteTaskSession(ctx, sessionID); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE task_environment_id = ?`, envID); got != 1 {
		t.Fatalf("environment snapshot rows after session delete = %d, want 1", got)
	}
	current, err := repo.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx, []string{envID})
	if err != nil {
		t.Fatalf("GetLatestGitStatusSnapshotsByTaskEnvironmentIDs: %v", err)
	}
	if len(current) != 1 || current[0].SessionID != "" {
		t.Fatalf("current snapshot after session delete = %+v, want one row with null provenance", current)
	}

	if err := repo.DeleteTaskEnvironment(ctx, envID); err != nil {
		t.Fatalf("DeleteTaskEnvironment: %v", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_session_git_snapshots WHERE task_environment_id = ?`, envID); got != 0 {
		t.Fatalf("environment snapshot rows after environment delete = %d, want 0", got)
	}
}

func TestGitSnapshotEnvironmentSelectionPrefersNewSiblingAndClearsFiles(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID = "task-git-environment-ranking"
		envID  = "env-git-environment-ranking"
		oldID  = "session-git-environment-old"
		newID  = "session-git-environment-new"
	)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Environment ranking"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: envID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	for _, sessionID := range []string{oldID, newID} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: sessionID, TaskID: taskID, TaskEnvironmentID: envID,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
		}
	}

	base := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snapshot-environment-root-old", TaskEnvironmentID: envID, SessionID: oldID,
		SnapshotType: models.SnapshotTypeStatusUpdate, TriggeredBy: "agent_completed", Branch: "main", CreatedAt: base,
		Files: map[string]interface{}{"removed.go": map[string]interface{}{"status": "modified"}},
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(old): %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snapshot-environment-root-new", TaskEnvironmentID: envID, SessionID: newID,
		SnapshotType: models.SnapshotTypeStatusUpdate, TriggeredBy: TriggeredByLiveMonitor, Branch: "main", CreatedAt: base.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(new): %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID: "snapshot-environment-named", TaskEnvironmentID: envID, SessionID: oldID,
		SnapshotType: models.SnapshotTypeStatusUpdate, Branch: "main", CreatedAt: base.Add(2 * time.Hour),
		Metadata: map[string]interface{}{"repository_name": "named"},
	}); err != nil {
		t.Fatalf("CreateGitSnapshot(named): %v", err)
	}

	got, err := repo.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx, []string{envID})
	if err != nil {
		t.Fatalf("GetLatestGitStatusSnapshotsByTaskEnvironmentIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("selected %d snapshots (%+v), want one per repository", len(got), got)
	}
	byRepository := make(map[string]*models.GitSnapshot, len(got))
	for _, snapshot := range got {
		byRepository[gitSnapshotRepositoryName(snapshot)] = snapshot
	}
	if byRepository[""] == nil || byRepository[""].ID != "snapshot-environment-root-new" {
		t.Fatalf("root snapshot = %+v, want newer sibling row", byRepository[""])
	}
	if byRepository[""].Files != nil {
		t.Fatalf("new sparse root snapshot retained old files: %+v", byRepository[""].Files)
	}
	if byRepository["named"] == nil || byRepository["named"].ID != "snapshot-environment-named" {
		t.Fatalf("named snapshot = %+v, want named row", byRepository["named"])
	}

	batch, err := repo.GetLatestGitSnapshotsByTaskEnvironmentIDs(ctx, []string{envID})
	if err != nil {
		t.Fatalf("GetLatestGitSnapshotsByTaskEnvironmentIDs: %v", err)
	}
	if len(batch) != 1 || batch[envID].ID != "snapshot-environment-named" {
		t.Fatalf("environment batch = %+v, want latest environment row", batch)
	}
}
