package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresPlanCommentSchemaReplayAndCRUD pins the dialect-sensitive
// composite foreign key, optimistic updates, and additive plan migration.
func TestPostgresPlanCommentSchemaReplayAndCRUD(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTask(t, repo, "task-plan-comments-pg")
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-comments-pg", TaskID: "task-plan-comments-pg", Content: "Plan",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-comments-pg", TaskID: "task-plan-comments-pg", PlanID: "plan-comments-pg",
		Body: "Clarify", SelectedText: "Plan", AnchorFrom: 1, AnchorTo: 5,
	}); err != nil {
		t.Fatalf("CreateTaskPlanComment: %v", err)
	}

	if _, err := db.Exec(`DROP TABLE task_plan_comments`); err != nil {
		t.Fatalf("drop comment table: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE task_plans DROP COLUMN comments_revision`); err != nil {
		t.Fatalf("rewind comments_revision: %v", err)
	}
	if err := repo.initPlansSchema(); err != nil {
		t.Fatalf("initialize plan schema on legacy database: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy database: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	snapshot, err := repo.ListTaskPlanComments(ctx, "task-plan-comments-pg")
	if err != nil {
		t.Fatalf("ListTaskPlanComments: %v", err)
	}
	if snapshot.PlanID != "plan-comments-pg" || snapshot.Revision != 0 || len(snapshot.Comments) != 0 {
		t.Fatalf("snapshot after migration = %#v", snapshot)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-comments-pg-new", TaskID: "task-plan-comments-pg", PlanID: "plan-comments-pg",
		Body: "Retained", SelectedText: "Plan", AnchorFrom: 1, AnchorTo: 5,
	}); err != nil {
		t.Fatalf("create after migration: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`DELETE FROM tasks WHERE id = ?`), "task-plan-comments-pg"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM task_plan_comments`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("comment rows after task cascade = %d, want 0", count)
	}
}
