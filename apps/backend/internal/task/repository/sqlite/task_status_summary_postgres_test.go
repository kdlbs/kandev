package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresTaskStatusSummarySchemaReplay(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-summary-postgres", "workspace-summary-postgres", "Postgres summary", now, now); err != nil {
		t.Fatalf("seed postgres task: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE task_status_summaries`); err != nil {
		t.Fatalf("drop summary table to simulate legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay postgres migrations: %v", err)
	}

	changed, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID:      "task-summary-postgres",
		WorkspaceID: "workspace-summary-postgres",
		Summary: statussummary.TaskStatusSummary{
			Revision:           1,
			ForegroundActivity: "background",
		},
	})
	if err != nil {
		t.Fatalf("insert postgres summary: %v", err)
	}
	if !changed {
		t.Fatal("postgres summary insert reported no change")
	}
	loaded, err := repo.LoadTaskStatusSummaries(ctx, []string{"task-summary-postgres"})
	if err != nil {
		t.Fatalf("load postgres summary: %v", err)
	}
	if loaded["task-summary-postgres"] == nil || loaded["task-summary-postgres"].Revision != 1 {
		t.Fatalf("postgres summary = %#v", loaded["task-summary-postgres"])
	}
}

func TestPostgresTaskLastActivityBatch(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, time.August, 17, 10, 0, 0, 0, time.UTC)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-activity-postgres", "workspace-activity-postgres", "Postgres activity", base, base.Add(time.Hour)); err != nil {
		t.Fatalf("seed postgres task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-activity-postgres", TaskID: "task-activity-postgres"}); err != nil {
		t.Fatalf("create postgres session: %v", err)
	}
	completedAt := base.Add(3 * time.Hour)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-activity-postgres", TaskSessionID: "session-activity-postgres", TaskID: "task-activity-postgres",
		StartedAt: base.Add(2 * time.Hour), CompletedAt: &completedAt,
	}); err != nil {
		t.Fatalf("create postgres turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-activity-postgres", TaskSessionID: "session-activity-postgres", TaskID: "task-activity-postgres",
		TurnID: "turn-activity-postgres", AuthorType: models.MessageAuthorUser, Content: "prompt",
		CreatedAt: base.Add(4 * time.Hour),
	}); err != nil {
		t.Fatalf("create postgres message: %v", err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("initialize postgres queue repository: %v", err)
	}
	queuedAt := base.Add(5 * time.Hour)
	queuedTarget := &messagequeue.QueuedMessage{
		ID: "queued-activity-postgres-user-target", SessionID: "session-activity-postgres", TaskID: "task-activity-postgres",
		Content: "queued target", QueuedAt: queuedAt.Add(-time.Minute), QueuedBy: "user-1",
	}
	if err := queueRepo.Insert(ctx, queuedTarget, 10); err != nil {
		t.Fatalf("insert postgres queued target: %v", err)
	}
	queuedSource := &messagequeue.QueuedMessage{
		ID: "queued-activity-postgres-user", SessionID: "session-activity-postgres", TaskID: "task-activity-postgres",
		Content: "queued prompt", QueuedAt: queuedAt, QueuedBy: "user-1",
	}
	if err := queueRepo.Insert(ctx, queuedSource, 10); err != nil {
		t.Fatalf("insert postgres queued prompt: %v", err)
	}
	merged, err := queueRepo.MergeIntoAbove(ctx, queuedSource.SessionID, queuedSource.ID, queuedSource.QueuedBy)
	if err != nil {
		t.Fatalf("merge postgres queued prompts: %v", err)
	}
	if merged == nil || !merged.QueuedAt.Equal(queuedAt) {
		t.Fatalf("merged postgres queued activity = %#v, want %s", merged, queuedAt)
	}
	if err := queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
		ID: "queued-activity-postgres-agent", SessionID: "session-activity-postgres", TaskID: "task-activity-postgres",
		Content: "queued agent prompt", QueuedAt: base.Add(6 * time.Hour), QueuedBy: messagequeue.QueuedByAgent,
	}, 10); err != nil {
		t.Fatalf("insert postgres queued agent prompt: %v", err)
	}
	lifecycleCompletedAt := base.Add(8 * time.Hour)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID:            "turn-activity-postgres-lifecycle",
		TaskSessionID: "session-activity-postgres",
		TaskID:        "task-activity-postgres",
		StartedAt:     base.Add(7 * time.Hour),
		CompletedAt:   &lifecycleCompletedAt,
		Metadata:      map[string]interface{}{models.TurnMetaKeyLifecycleOnly: true},
	}); err != nil {
		t.Fatalf("create postgres lifecycle turn: %v", err)
	}

	got, err := repo.LoadTaskLastActivity(ctx, []string{"task-activity-postgres"})
	if err != nil {
		t.Fatalf("load postgres task activity: %v", err)
	}
	if gotAt, ok := got["task-activity-postgres"]; !ok || !gotAt.Equal(queuedAt) {
		t.Fatalf("postgres activity = %v, %v; want %v", gotAt, ok, queuedAt)
	}
}
