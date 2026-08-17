package sqlite

import (
	"context"
	"testing"
	"time"

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

	got, err := repo.LoadTaskLastActivity(ctx, []string{"task-activity-postgres"})
	if err != nil {
		t.Fatalf("load postgres task activity: %v", err)
	}
	if gotAt, ok := got["task-activity-postgres"]; !ok || !gotAt.Equal(base.Add(4*time.Hour)) {
		t.Fatalf("postgres activity = %v, %v; want %v", gotAt, ok, base.Add(4*time.Hour))
	}
}
