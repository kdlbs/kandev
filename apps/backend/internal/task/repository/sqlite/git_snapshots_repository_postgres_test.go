package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresGitStatusSnapshotsRankEachRepositoryIndependently executes the
// same interleaved ranking scenario as the SQLite regression against the
// Postgres JSON expression and ROW_NUMBER() window. It skips unless the
// Postgres test database is configured.
func TestPostgresGitStatusSnapshotsRankEachRepositoryIndependently(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const (
		taskID    = "task-snapshot-repository-ranking-pg"
		sessionID = "session-snapshot-repository-ranking-pg"
	)
	seedPostgresTask(t, repo, taskID)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-" + taskID, TaskID: taskID,
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, TaskEnvironmentID: "env-" + taskID}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	base := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	snapshots := []*models.GitSnapshot{
		{
			ID: "root-live-old-pg", SessionID: sessionID,
			SnapshotType: models.SnapshotTypeStatusUpdate, TriggeredBy: TriggeredByLiveMonitor,
			HeadCommit: "root-live-old", CreatedAt: base.Add(-time.Hour),
			Metadata: map[string]interface{}{"repository_name": ""},
		},
		{
			ID: "named-completed-pg", SessionID: sessionID,
			SnapshotType: models.SnapshotTypeStatusUpdate, TriggeredBy: "agent_completed",
			HeadCommit: "named-completed", CreatedAt: base,
			Metadata: map[string]interface{}{"repository_name": "named"},
		},
		{
			ID: "root-archive-pg", SessionID: sessionID,
			SnapshotType: models.SnapshotTypeArchive,
			HeadCommit:   "root-archive", CreatedAt: base.Add(time.Hour),
		},
		{
			ID: "named-live-new-pg", SessionID: sessionID,
			SnapshotType: models.SnapshotTypeStatusUpdate, TriggeredBy: TriggeredByLiveMonitor,
			HeadCommit: "named-live-new", CreatedAt: base.Add(2 * time.Hour),
			Metadata: map[string]interface{}{"repository_name": "named"},
		},
	}
	for _, snapshot := range snapshots {
		if err := repo.CreateGitSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("CreateGitSnapshot(%s): %v", snapshot.ID, err)
		}
	}

	got, err := repo.GetLatestGitStatusSnapshotsBySessionIDs(ctx, []string{sessionID})
	if err != nil {
		t.Fatalf("GetLatestGitStatusSnapshotsBySessionIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("selected %d snapshots (%v), want one per repository", len(got), got)
	}
	byRepository := make(map[string]string, len(got))
	for _, snapshot := range got {
		byRepository[gitSnapshotRepositoryName(snapshot)] = snapshot.ID
	}
	if got := byRepository[""]; got != "root-archive-pg" {
		t.Errorf("root repository selected %q, want root-archive-pg", got)
	}
	if got := byRepository["named"]; got != "named-completed-pg" {
		t.Errorf("named repository selected %q, want named-completed-pg", got)
	}
}
