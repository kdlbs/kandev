package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type planCommentRepositoryContract interface {
	ListTaskPlanComments(context.Context, string) (*models.TaskPlanCommentSnapshot, error)
	CreateTaskPlanComment(context.Context, *models.TaskPlanComment) (*models.TaskPlanCommentSnapshot, error)
	UpdateTaskPlanComment(context.Context, *models.TaskPlanComment, int64) (*models.TaskPlanCommentSnapshot, error)
	DeleteTaskPlanComment(context.Context, string, string, string, int64) (*models.TaskPlanCommentSnapshot, error)
}

func TestPlanCommentSchemaReplaysOnLegacyDatabase(t *testing.T) {
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "legacy-plan-comments.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	seedTaskForDocs(t, repo, "task-plan-comments-legacy")
	if err := repo.CreateTaskPlan(context.Background(), &models.TaskPlan{
		ID: "plan-comments-legacy", TaskID: "task-plan-comments-legacy", Content: "Legacy plan",
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	if _, err := db.Exec(`DROP TABLE task_plan_comments`); err != nil {
		t.Fatalf("drop new comment table: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE task_plans DROP COLUMN comments_revision`); err != nil {
		t.Fatalf("rewind task_plans: %v", err)
	}
	if err := repo.initPlansSchema(); err != nil {
		t.Fatalf("initialize plan children on legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	snapshot, err := repo.ListTaskPlanComments(context.Background(), "task-plan-comments-legacy")
	if err != nil {
		t.Fatalf("ListTaskPlanComments after replay: %v", err)
	}
	assertPlanCommentSnapshot(t, snapshot, "task-plan-comments-legacy", "plan-comments-legacy", 0, []*models.TaskPlanComment{})
}

func TestPlanCommentsUseStableOrderAndPlanLifecycle(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-comments-life")
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-comments-life", TaskID: "task-plan-comments-life", Content: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"comment-z", "comment-a"} {
		if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
			ID: id, TaskID: "task-plan-comments-life", PlanID: "plan-comments-life",
			Body: id, SelectedText: "Plan", AnchorFrom: 1, AnchorTo: 5,
		}); err != nil {
			t.Fatalf("CreateTaskPlanComment(%s): %v", id, err)
		}
	}
	sameTime := time.Now().UTC().Add(-time.Hour)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE task_plan_comments SET created_at = ? WHERE task_id = ?
	`), sameTime, "task-plan-comments-life"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.ListTaskPlanComments(ctx, "task-plan-comments-life")
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{snapshot.Comments[0].ID, snapshot.Comments[1].ID}; !reflect.DeepEqual(got, []string{"comment-a", "comment-z"}) {
		t.Fatalf("stable order = %v", got)
	}

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-comments-life", TaskID: "task-plan-comments-life",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteTaskSession(ctx, "session-comments-life"); err != nil {
		t.Fatal(err)
	}
	if snapshot, err = repo.ListTaskPlanComments(ctx, "task-plan-comments-life"); err != nil || len(snapshot.Comments) != 2 {
		t.Fatalf("comments after session delete = %#v, %v", snapshot, err)
	}

	if err := repo.DeleteTaskPlan(ctx, "task-plan-comments-life"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := repo.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM task_plan_comments`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("comments after plan delete = %d, want 0", count)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-comments-life-new", TaskID: "task-plan-comments-life", Content: "New plan",
	}); err != nil {
		t.Fatal(err)
	}
	if snapshot, err = repo.ListTaskPlanComments(ctx, "task-plan-comments-life"); err != nil ||
		snapshot.PlanID != "plan-comments-life-new" || snapshot.Revision != 0 || len(snapshot.Comments) != 0 {
		t.Fatalf("recreated plan snapshot = %#v, %v", snapshot, err)
	}
}

func requirePlanCommentRepository(t *testing.T, repo *Repository) planCommentRepositoryContract {
	t.Helper()
	contract, ok := any(repo).(planCommentRepositoryContract)
	if !ok {
		t.Fatal("Repository does not implement task plan comment persistence")
	}
	return contract
}

func TestPlanCommentSchemaExistsOnFreshDatabase(t *testing.T) {
	repo := newRepoForEntityTests(t)

	var tableName string
	if err := repo.db.Get(&tableName, `
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name = 'task_plan_comments'
	`); err != nil {
		t.Fatalf("task_plan_comments table is missing: %v", err)
	}

	var revisionColumnCount int
	if err := repo.db.Get(&revisionColumnCount, `
		SELECT COUNT(*)
		FROM pragma_table_info('task_plans')
		WHERE name = 'comments_revision'
	`); err != nil {
		t.Fatalf("inspect task_plans columns: %v", err)
	}
	if revisionColumnCount != 1 {
		t.Fatalf("comments_revision column count = %d, want 1", revisionColumnCount)
	}
}

func TestPlanCommentRepositoryCRUDAndOptimisticConflicts(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-comments")
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-comments", TaskID: "task-plan-comments", Content: "Plan body",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	comments := requirePlanCommentRepository(t, repo)

	empty, err := comments.ListTaskPlanComments(ctx, "task-plan-comments")
	if err != nil {
		t.Fatalf("ListTaskPlanComments(empty): %v", err)
	}
	assertPlanCommentSnapshot(t, empty, "task-plan-comments", "plan-comments", 0, []*models.TaskPlanComment{})

	first := &models.TaskPlanComment{
		ID: "comment-b", TaskID: "task-plan-comments", PlanID: "plan-comments",
		Body: "Explain this", SelectedText: "Plan", AnchorFrom: 3, AnchorTo: 7,
	}
	created, err := comments.CreateTaskPlanComment(ctx, first)
	if err != nil {
		t.Fatalf("CreateTaskPlanComment: %v", err)
	}
	if first.Version != 1 || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("created comment was not stamped: %#v", first)
	}
	assertPlanCommentSnapshot(t, created, "task-plan-comments", "plan-comments", 1, []*models.TaskPlanComment{first})

	replayed, err := comments.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-b", TaskID: "task-plan-comments", PlanID: "plan-comments",
		Body: "Explain this", SelectedText: "Plan", AnchorFrom: 3, AnchorTo: 7,
	})
	if err != nil {
		t.Fatalf("idempotent CreateTaskPlanComment replay: %v", err)
	}
	if replayed.Revision != 1 {
		t.Fatalf("idempotent replay revision = %d, want 1", replayed.Revision)
	}

	_, err = comments.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-b", TaskID: "task-plan-comments", PlanID: "plan-comments",
		Body: "Different", SelectedText: "Plan", AnchorFrom: 3, AnchorTo: 7,
	})
	if !errors.Is(err, repoerrors.ErrTaskPlanCommentsChanged) {
		t.Fatalf("conflicting create error = %v, want ErrTaskPlanCommentsChanged", err)
	}

	update := *first
	update.Body = "Explain this carefully"
	updated, err := comments.UpdateTaskPlanComment(ctx, &update, 1)
	if err != nil {
		t.Fatalf("UpdateTaskPlanComment: %v", err)
	}
	if update.Version != 2 {
		t.Fatalf("updated version = %d, want 2", update.Version)
	}
	assertPlanCommentSnapshot(t, updated, "task-plan-comments", "plan-comments", 2, []*models.TaskPlanComment{&update})

	stale := update
	stale.Body = "stale overwrite"
	conflictSnapshot, err := comments.UpdateTaskPlanComment(ctx, &stale, 1)
	if !errors.Is(err, repoerrors.ErrTaskPlanCommentsChanged) {
		t.Fatalf("stale update error = %v, want ErrTaskPlanCommentsChanged", err)
	}
	assertPlanCommentSnapshot(t, conflictSnapshot, "task-plan-comments", "plan-comments", 2, []*models.TaskPlanComment{&update})
	unchanged, err := comments.ListTaskPlanComments(ctx, "task-plan-comments")
	if err != nil {
		t.Fatal(err)
	}
	assertPlanCommentSnapshot(t, unchanged, "task-plan-comments", "plan-comments", 2, []*models.TaskPlanComment{&update})

	deleted, err := comments.DeleteTaskPlanComment(ctx, "task-plan-comments", "plan-comments", "comment-b", 2)
	if err != nil {
		t.Fatalf("DeleteTaskPlanComment: %v", err)
	}
	assertPlanCommentSnapshot(t, deleted, "task-plan-comments", "plan-comments", 3, []*models.TaskPlanComment{})
	if _, err := comments.DeleteTaskPlanComment(ctx, "task-plan-comments", "plan-comments", "comment-b", 2); !errors.Is(err, repoerrors.ErrTaskPlanCommentsChanged) {
		t.Fatalf("repeated delete error = %v, want ErrTaskPlanCommentsChanged", err)
	}
}

func assertPlanCommentSnapshot(
	t *testing.T,
	got *models.TaskPlanCommentSnapshot,
	taskID, planID string,
	revision int64,
	want []*models.TaskPlanComment,
) {
	t.Helper()
	if got == nil {
		t.Fatal("plan comment snapshot is nil")
	}
	if got.TaskID != taskID || got.PlanID != planID || got.Revision != revision {
		t.Errorf("snapshot identity = %q/%q/%d, want %q/%q/%d", got.TaskID, got.PlanID, got.Revision, taskID, planID, revision)
	}
	if !reflect.DeepEqual(got.Comments, want) {
		t.Errorf("snapshot comments = %#v, want %#v", got.Comments, want)
	}
}
