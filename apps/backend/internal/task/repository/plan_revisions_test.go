package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func seedPlanRevisionTask(t *testing.T, ctx context.Context, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}, taskID string) {
	t.Helper()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-rev", Name: "Rev WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-rev", WorkspaceID: "ws-rev", Name: "WF"})
	now := time.Now().UTC()
	_ = repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-rev", WorkflowID: "wf-rev",
		Title: "T", State: v1.TaskStateCreated, CreatedAt: now, UpdatedAt: now,
	})
}

func TestPlanRevisions_InsertAndList(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedPlanRevisionTask(t, ctx, repo, "task-r1")

	for i := 1; i <= 3; i++ {
		rev := &models.TaskPlanRevision{
			TaskID: "task-r1", RevisionNumber: i,
			Title: "Plan", Content: "c",
			AuthorKind: "agent", AuthorName: "Agent",
		}
		if err := repo.InsertTaskPlanRevision(ctx, rev); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	list, err := repo.ListTaskPlanRevisions(ctx, "task-r1", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].RevisionNumber != 3 || list[2].RevisionNumber != 1 {
		t.Errorf("expected newest-first ordering, got %d..%d", list[0].RevisionNumber, list[2].RevisionNumber)
	}
}

func TestPlanRevisions_NextRevisionNumber(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedPlanRevisionTask(t, ctx, repo, "task-r2")

	n, err := repo.NextTaskPlanRevisionNumber(ctx, "task-r2")
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1, got %d", n)
	}

	_ = repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		TaskID: "task-r2", RevisionNumber: 1, AuthorKind: "agent",
	})
	_ = repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		TaskID: "task-r2", RevisionNumber: 2, AuthorKind: "agent",
	})

	n, _ = repo.NextTaskPlanRevisionNumber(ctx, "task-r2")
	if n != 3 {
		t.Errorf("expected 3, got %d", n)
	}
}

func TestPlanRevisions_UniqueConstraint(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedPlanRevisionTask(t, ctx, repo, "task-r3")

	if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		TaskID: "task-r3", RevisionNumber: 1, AuthorKind: "agent",
	}); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		TaskID: "task-r3", RevisionNumber: 1, AuthorKind: "agent",
	})
	if err == nil {
		t.Fatal("expected UNIQUE violation on duplicate (task_id, revision_number)")
	}
}

func TestPlanRevisions_GetLatest(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedPlanRevisionTask(t, ctx, repo, "task-r4")

	latest, _ := repo.GetLatestTaskPlanRevision(ctx, "task-r4")
	if latest != nil {
		t.Errorf("expected nil before any insert, got %+v", latest)
	}

	_ = repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{TaskID: "task-r4", RevisionNumber: 1, AuthorKind: "agent", Content: "a"})
	_ = repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{TaskID: "task-r4", RevisionNumber: 2, AuthorKind: "user", Content: "b"})

	latest, err := repo.GetLatestTaskPlanRevision(ctx, "task-r4")
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest == nil || latest.RevisionNumber != 2 || latest.Content != "b" {
		t.Errorf("expected newest rev 2 with content 'b', got %+v", latest)
	}
}

func TestPlanRevisions_UpdateCoalesceMerge(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedPlanRevisionTask(t, ctx, repo, "task-r5")

	rev := &models.TaskPlanRevision{
		TaskID: "task-r5", RevisionNumber: 1,
		Title: "Plan", Content: "v1",
		AuthorKind: "agent", AuthorName: "Agent",
	}
	_ = repo.InsertTaskPlanRevision(ctx, rev)
	originalID := rev.ID

	rev.Content = "v2"
	if err := repo.UpdateTaskPlanRevision(ctx, rev); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := repo.GetTaskPlanRevision(ctx, originalID)
	if got.Content != "v2" {
		t.Errorf("expected merged content 'v2', got %q", got.Content)
	}
	if !got.UpdatedAt.After(got.CreatedAt) && !got.UpdatedAt.Equal(got.CreatedAt) {
		t.Errorf("updated_at should be >= created_at")
	}

	list, _ := repo.ListTaskPlanRevisions(ctx, "task-r5", 0)
	if len(list) != 1 {
		t.Errorf("coalesce merge should not add rows; have %d", len(list))
	}
}

func TestPlanRevisions_RevertOfRevisionID(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()
	seedPlanRevisionTask(t, ctx, repo, "task-r6")

	target := &models.TaskPlanRevision{
		TaskID: "task-r6", RevisionNumber: 1,
		Content: "original", AuthorKind: "agent", AuthorName: "Agent",
	}
	_ = repo.InsertTaskPlanRevision(ctx, target)

	refID := target.ID
	revertRev := &models.TaskPlanRevision{
		TaskID: "task-r6", RevisionNumber: 2,
		Content: "original", AuthorKind: "user", AuthorName: "User",
		RevertOfRevisionID: &refID,
	}
	_ = repo.InsertTaskPlanRevision(ctx, revertRev)

	got, _ := repo.GetTaskPlanRevision(ctx, revertRev.ID)
	if got.RevertOfRevisionID == nil || *got.RevertOfRevisionID != refID {
		t.Errorf("expected revert_of_revision_id=%q, got %v", refID, got.RevertOfRevisionID)
	}
}

func TestPlanRevisions_BackfillsExistingPlan(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "backfill.db")

	// First open: seed a task + plan, then close (simulates pre-migration state plus
	// a subsequent reopen — backfill runs once per newRepository call).
	firstConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	firstDB := sqlx.NewDb(firstConn, "sqlite3")
	firstRepo, err := sqlite.NewWithDB(firstDB, firstDB)
	if err != nil {
		t.Fatalf("repo 1: %v", err)
	}
	seedPlanRevisionTask(t, ctx, firstRepo, "task-r7")
	_ = firstRepo.CreateTaskPlan(ctx, &models.TaskPlan{
		TaskID: "task-r7", Title: "Legacy", Content: "legacy content", CreatedBy: "agent",
	})
	// Remove the revision the plan creation would have left (if any) so backfill has real work.
	_, _ = firstDB.Exec(`DELETE FROM task_plan_revisions WHERE task_id = ?`, "task-r7")
	if err := firstDB.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	// Second open: backfill should run during initSchema and synthesize a revision.
	secondConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	secondDB := sqlx.NewDb(secondConn, "sqlite3")
	secondRepo, err := sqlite.NewWithDB(secondDB, secondDB)
	if err != nil {
		t.Fatalf("repo 2: %v", err)
	}
	defer func() {
		if err := secondDB.Close(); err != nil {
			t.Errorf("close 2: %v", err)
		}
	}()

	list, err := secondRepo.ListTaskPlanRevisions(ctx, "task-r7", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 backfilled revision, got %d", len(list))
	}
	if list[0].Content != "legacy content" {
		t.Errorf("expected backfilled content 'legacy content', got %q", list[0].Content)
	}
	if list[0].AuthorName != "legacy" {
		t.Errorf("expected author_name 'legacy', got %q", list[0].AuthorName)
	}
	if list[0].AuthorKind != "agent" {
		t.Errorf("expected author_kind 'agent' (carried over from created_by), got %q", list[0].AuthorKind)
	}
	if list[0].RevisionNumber != 1 {
		t.Errorf("expected revision_number=1, got %d", list[0].RevisionNumber)
	}
}
