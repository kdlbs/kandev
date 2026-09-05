package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/mattn/go-sqlite3"
)

type planCommentRepositoryContract interface {
	ListTaskPlanComments(context.Context, string) (*models.TaskPlanCommentSnapshot, error)
	CreateTaskPlanComment(context.Context, *models.TaskPlanComment) (*models.TaskPlanCommentSnapshot, error)
	UpdateTaskPlanComment(context.Context, *models.TaskPlanComment, int64) (*models.TaskPlanCommentSnapshot, error)
	DeleteTaskPlanComment(context.Context, string, string, string, int64) (*models.TaskPlanCommentSnapshot, error)
}

func TestListTaskPlanCommentsReturnsOneDatabaseSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "plan-comment-snapshot.db")
	writerConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	writer := sqlx.NewDb(writerConn, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })

	var enabled atomic.Bool
	var gate sync.Once
	commentsRead := make(chan struct{})
	continueRead := make(chan struct{})
	driverName := fmt.Sprintf("sqlite3-plan-comment-snapshot-%d", time.Now().UnixNano())
	sql.Register(driverName, &sqlite3.SQLiteDriver{ConnectHook: func(conn *sqlite3.SQLiteConn) error {
		conn.RegisterAuthorizer(func(op int, table, _, _ string) int {
			if enabled.Load() && op == sqlite3.SQLITE_READ && table == "task_plan_comments" {
				gate.Do(func() {
					close(commentsRead)
					<-continueRead
				})
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}})
	readerConn, err := sql.Open(driverName, "file:"+dbPath+"?_mode=ro&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	readerConn.SetMaxOpenConns(1)
	reader := sqlx.NewDb(readerConn, "sqlite3")
	t.Cleanup(func() { _ = reader.Close() })

	repo, err := NewWithDB(writer, reader, nil)
	if err != nil {
		t.Fatalf("initialize repository: %v", err)
	}
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-comments-snapshot")
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-comments-snapshot", TaskID: "task-plan-comments-snapshot", Content: "Plan",
	}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	type listResult struct {
		snapshot *models.TaskPlanCommentSnapshot
		err      error
	}
	result := make(chan listResult, 1)
	enabled.Store(true)
	go func() {
		snapshot, listErr := repo.ListTaskPlanComments(ctx, "task-plan-comments-snapshot")
		result <- listResult{snapshot: snapshot, err: listErr}
	}()
	select {
	case <-commentsRead:
	case <-time.After(time.Second):
		t.Fatal("comment-row read did not start")
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-snapshot", TaskID: "task-plan-comments-snapshot", PlanID: "plan-comments-snapshot",
		Body: "New", SelectedText: "Plan", AnchorFrom: 0, AnchorTo: 4,
	}); err != nil {
		t.Fatalf("concurrent create: %v", err)
	}
	close(continueRead)

	got := <-result
	if got.err != nil {
		t.Fatalf("ListTaskPlanComments: %v", got.err)
	}
	if got.snapshot.Revision == 0 && len(got.snapshot.Comments) != 0 {
		t.Fatalf("snapshot mixed revision 0 with %d new comments", len(got.snapshot.Comments))
	}
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
	if err := repo.db.Get(&tableName, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'task_plan_comment_admissions'
	`); err != nil {
		t.Fatalf("task_plan_comment_admissions table is missing: %v", err)
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

func TestPlanCommentCreateReplayDoesNotResurrectRetiredComment(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-comment-replay")
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-comment-replay", TaskID: "task-plan-comment-replay", Content: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	original := &models.TaskPlanComment{
		ID: "comment-retired", TaskID: "task-plan-comment-replay", PlanID: "plan-comment-replay",
		Body: "Keep once", SelectedText: "Plan", AnchorFrom: 0, AnchorTo: 4,
	}
	created, err := repo.CreateTaskPlanComment(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := repo.DeleteTaskPlanComment(ctx, original.TaskID, original.PlanID, original.ID, 1)
	if err != nil {
		t.Fatal(err)
	}

	replayed, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: original.ID, TaskID: original.TaskID, PlanID: original.PlanID,
		Body: original.Body, SelectedText: original.SelectedText, AnchorFrom: original.AnchorFrom, AnchorTo: original.AnchorTo,
	})
	if err != nil {
		t.Fatalf("retired create replay: %v", err)
	}
	if replayed.Revision != deleted.Revision || len(replayed.Comments) != 0 {
		t.Fatalf("retired replay resurrected comment: %#v (created revision %d)", replayed, created.Revision)
	}

	conflict, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: original.ID, TaskID: original.TaskID, PlanID: original.PlanID,
		Body: "different", SelectedText: original.SelectedText, AnchorFrom: original.AnchorFrom, AnchorTo: original.AnchorTo,
	})
	if !errors.Is(err, repoerrors.ErrTaskPlanCommentsChanged) || conflict.Revision != deleted.Revision {
		t.Fatalf("mismatched retired replay = %#v, %v", conflict, err)
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
