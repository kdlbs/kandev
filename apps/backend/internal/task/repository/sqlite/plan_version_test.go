package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskPlanWriteVersionIsPersisted(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-version")

	plan := &models.TaskPlan{
		ID: "plan-version", TaskID: "task-plan-version", Title: "Plan", Content: "first",
	}
	if err := repo.CreateTaskPlan(ctx, plan); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	first := plan.WriteVersion
	if first == "" {
		t.Fatal("CreateTaskPlan did not assign a write version")
	}

	got, err := repo.GetTaskPlan(ctx, plan.TaskID)
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if gotVersion := got.WriteVersion; gotVersion != first {
		t.Fatalf("persisted write version = %q, want %q", gotVersion, first)
	}

	plan.Content = "second"
	if err := repo.UpdateTaskPlan(ctx, plan); err != nil {
		t.Fatalf("UpdateTaskPlan: %v", err)
	}
	second := plan.WriteVersion
	if second == "" || second == first {
		t.Fatalf("UpdateTaskPlan write version = %q, want a new non-empty version after %q", second, first)
	}
}

func TestTaskPlanWriteVersionChangesWhenRevisionCoalesces(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-version-coalesce")

	head := &models.TaskPlan{ID: "plan-version-coalesce", TaskID: "task-plan-version-coalesce", Content: "first"}
	firstRevision := &models.TaskPlanRevision{
		TaskID: head.TaskID, Title: "Plan", Content: "first", AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.WritePlanRevision(ctx, head, firstRevision, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(first): %v", err)
	}
	firstVersion := head.WriteVersion

	head.Content = "second"
	secondRevision := &models.TaskPlanRevision{
		TaskID: head.TaskID, Title: "Plan", Content: "second", AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.WritePlanRevision(ctx, head, secondRevision, &firstRevision.ID, false, false); err != nil {
		t.Fatalf("WritePlanRevision(coalesce): %v", err)
	}
	if head.WriteVersion == firstVersion || head.WriteVersion == "" {
		t.Fatalf("coalesced write version = %q, want a new non-empty version after %q", head.WriteVersion, firstVersion)
	}
	if secondRevision.ID != firstRevision.ID {
		t.Fatalf("coalesced revision ID = %q, want %q", secondRevision.ID, firstRevision.ID)
	}
}

func TestTaskPlanWriteVersionSurvivesMarkerAndCommentWrites(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-version-markers")

	plan := &models.TaskPlan{ID: "plan-version-markers", TaskID: "task-plan-version-markers", Content: "body"}
	if err := repo.CreateTaskPlan(ctx, plan); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	version := plan.WriteVersion
	if _, err := repo.MarkTaskPlanImplementationStarted(ctx, plan.TaskID, "session-version", "user"); err != nil {
		t.Fatalf("MarkTaskPlanImplementationStarted: %v", err)
	}
	comment := &models.TaskPlanComment{
		ID: "comment-version", TaskID: plan.TaskID, PlanID: plan.ID, Body: "review", SelectedText: "body",
		AnchorTo: len("body"),
	}
	if _, err := repo.CreateTaskPlanComment(ctx, comment); err != nil {
		t.Fatalf("CreateTaskPlanComment: %v", err)
	}
	got, err := repo.GetTaskPlan(ctx, plan.TaskID)
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if got.WriteVersion != version {
		t.Fatalf("marker/comment write version = %q, want %q", got.WriteVersion, version)
	}
}

func TestTaskPlanWriteVersionMigrationBackfillsAndReplays(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-version-migration")
	plan := &models.TaskPlan{
		ID: "plan-version-migration", TaskID: "task-plan-version-migration", Content: "legacy body",
	}
	if err := repo.CreateTaskPlan(ctx, plan); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `ALTER TABLE task_plans DROP COLUMN write_version`); err != nil {
		t.Fatalf("drop write_version for legacy fixture: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations(backfill): %v", err)
	}
	migrated, err := repo.GetTaskPlan(ctx, plan.TaskID)
	if err != nil {
		t.Fatalf("GetTaskPlan after migration: %v", err)
	}
	if migrated.WriteVersion == "" {
		t.Fatal("migration left legacy plan without a write version")
	}
	version := migrated.WriteVersion
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations(replay): %v", err)
	}
	replayed, err := repo.GetTaskPlan(ctx, plan.TaskID)
	if err != nil {
		t.Fatalf("GetTaskPlan after replay: %v", err)
	}
	if replayed.WriteVersion != version {
		t.Fatalf("replayed write version = %q, want %q", replayed.WriteVersion, version)
	}
}

func TestTaskPlanWriteVersionRollbackLeavesHeadUnchanged(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-version-rollback")

	head := &models.TaskPlan{ID: "plan-version-rollback", TaskID: "task-plan-version-rollback", Content: "before"}
	firstRevision := &models.TaskPlanRevision{
		TaskID: head.TaskID, Title: "Plan", Content: "before", AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.WritePlanRevision(ctx, head, firstRevision, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(first): %v", err)
	}
	firstVersion := head.WriteVersion
	missingRevisionID := "missing-plan-revision"
	head.Content = "after"
	err := repo.WritePlanRevision(ctx, head, &models.TaskPlanRevision{
		TaskID: head.TaskID, Title: "Plan", Content: "after", AuthorKind: "agent", AuthorName: "Agent",
	}, &missingRevisionID, false, false)
	if err == nil {
		t.Fatal("WritePlanRevision accepted a missing coalesce target")
	}
	got, err := repo.GetTaskPlan(ctx, head.TaskID)
	if err != nil {
		t.Fatalf("GetTaskPlan after rollback: %v", err)
	}
	if got.Content != "before" || got.WriteVersion != firstVersion {
		t.Fatalf("HEAD after rollback = content %q/version %q, want before/%q", got.Content, got.WriteVersion, firstVersion)
	}
}

func TestTaskPlanWriteVersionDeleteRecreateIsFresh(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-version-recreate")

	first := &models.TaskPlan{ID: "plan-version-recreate-1", TaskID: "task-plan-version-recreate", Content: "first"}
	if err := repo.CreateTaskPlan(ctx, first); err != nil {
		t.Fatalf("CreateTaskPlan(first): %v", err)
	}
	if err := repo.DeleteTaskPlan(ctx, first.TaskID); err != nil {
		t.Fatalf("DeleteTaskPlan: %v", err)
	}
	second := &models.TaskPlan{ID: "plan-version-recreate-2", TaskID: first.TaskID, Content: "second"}
	if err := repo.CreateTaskPlan(ctx, second); err != nil {
		t.Fatalf("CreateTaskPlan(second): %v", err)
	}
	if second.WriteVersion == "" || second.WriteVersion == first.WriteVersion {
		t.Fatalf("recreated write version = %q, want a fresh token after %q", second.WriteVersion, first.WriteVersion)
	}
}

func TestTaskPlanWriteVersionSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "plan-version-restart.db")

	firstConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open first database: %v", err)
	}
	firstDB := sqlx.NewDb(firstConn, "sqlite3")
	firstRepo, err := NewWithDB(firstDB, firstDB, nil)
	if err != nil {
		_ = firstDB.Close()
		t.Fatalf("create first repository: %v", err)
	}
	seedWorkspace(t, firstRepo, "workspace-plan-version-restart")
	if err := firstRepo.CreateTask(ctx, &models.Task{
		ID: "task-plan-version-restart", WorkspaceID: "workspace-plan-version-restart", Title: "Restart task",
	}); err != nil {
		_ = firstDB.Close()
		t.Fatalf("CreateTask: %v", err)
	}
	plan := &models.TaskPlan{
		ID: "plan-version-restart", TaskID: "task-plan-version-restart", Content: "persistent body",
	}
	if err := firstRepo.CreateTaskPlan(ctx, plan); err != nil {
		_ = firstDB.Close()
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	version := plan.WriteVersion
	if err := firstDB.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	secondConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open second database: %v", err)
	}
	secondDB := sqlx.NewDb(secondConn, "sqlite3")
	t.Cleanup(func() { _ = secondDB.Close() })
	secondRepo, err := NewWithDB(secondDB, secondDB, nil)
	if err != nil {
		t.Fatalf("create second repository: %v", err)
	}
	got, err := secondRepo.GetTaskPlan(ctx, plan.TaskID)
	if err != nil {
		t.Fatalf("GetTaskPlan after restart: %v", err)
	}
	if got.WriteVersion != version || got.Content != plan.Content {
		t.Fatalf("plan after restart = content %q/version %q, want %q/%q", got.Content, got.WriteVersion, plan.Content, version)
	}
}

func TestTaskPlanRevisionMetadataIsBoundedAndTaskScoped(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-metadata")
	seedTaskForDocs(t, repo, "task-plan-metadata-other")

	head := &models.TaskPlan{
		ID: "plan-metadata", TaskID: "task-plan-metadata", Title: "Plan", Content: "first 内容",
	}
	first := &models.TaskPlanRevision{
		TaskID: head.TaskID, Title: head.Title, Content: head.Content, AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.WritePlanRevision(ctx, head, first, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(first): %v", err)
	}
	head.Content = "第二 body"
	second := &models.TaskPlanRevision{
		TaskID: head.TaskID, Title: head.Title, Content: head.Content, AuthorKind: "agent", AuthorName: "Agent",
	}
	if err := repo.WritePlanRevision(ctx, head, second, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(second): %v", err)
	}

	page, err := repo.ListTaskPlanRevisionMetadata(ctx, head.TaskID, 0, 1)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisionMetadata(first page): %v", err)
	}
	if len(page) != 1 || page[0].Content != "" || page[0].ContentBytes != len(second.Content) {
		t.Fatalf("metadata page = %+v, want one content-free row with byte size %d", page, len(second.Content))
	}
	if page[0].RevisionNumber != second.RevisionNumber {
		t.Fatalf("metadata revision number = %d, want %d", page[0].RevisionNumber, second.RevisionNumber)
	}

	older, err := repo.ListTaskPlanRevisionMetadata(ctx, head.TaskID, page[0].RevisionNumber, 1)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisionMetadata(cursor): %v", err)
	}
	if len(older) != 1 || older[0].ID != first.ID {
		t.Fatalf("cursor page = %+v, want first revision %q", older, first.ID)
	}

	foreign, err := repo.GetTaskPlanRevisionForTask(ctx, "task-plan-metadata-other", second.ID)
	if err != nil {
		t.Fatalf("GetTaskPlanRevisionForTask(foreign): %v", err)
	}
	if foreign != nil {
		t.Fatalf("foreign task received revision: %+v", foreign)
	}
	got, err := repo.GetTaskPlanRevisionForTask(ctx, head.TaskID, second.ID)
	if err != nil {
		t.Fatalf("GetTaskPlanRevisionForTask(owner): %v", err)
	}
	if got == nil || got.Content != second.Content {
		t.Fatalf("owner revision = %+v, want exact content %q", got, second.Content)
	}
}
