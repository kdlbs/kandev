package sqlite

// Postgres parity coverage for the document and plan revision write paths.
// Both go through `INSERT ... ON CONFLICT (...) DO UPDATE SET ... excluded.*`
// inside an explicit transaction, and MarkTaskPlanImplementationStarted uses
// COALESCE plus CASE WHEN over a nullable TIMESTAMP — all dialect-sensitive.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set; CI runs these in postgres-boot.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// seedPostgresTask inserts a tasks row directly: the FK targets of documents
// and plans are all this table, and a raw insert keeps the fixture minimal.
func seedPostgresTask(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), taskID, "ws-"+taskID, taskID, now, now); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}

func TestPostgresWriteDocumentRevisionUpsertAndCoalesce(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTask(t, repo, "task-doc-pg")

	head := &models.TaskDocument{
		ID: "doc-pg", TaskID: "task-doc-pg", Key: "plan", Type: "plan",
		Title: "v1", Content: "one", AuthorKind: "agent", AuthorName: "claude",
		Filename: "plan.md", MimeType: "text/markdown", SizeBytes: 3, DiskPath: "/plan.md",
	}
	first := &models.TaskDocumentRevision{
		TaskID: "task-doc-pg", DocumentKey: "plan",
		Title: "v1", Content: "one", AuthorKind: "agent", AuthorName: "claude",
	}
	if err := repo.WriteDocumentRevision(ctx, head, first, nil); err != nil {
		t.Fatalf("WriteDocumentRevision(first): %v", err)
	}
	if first.RevisionNumber != 1 {
		t.Fatalf("RevisionNumber = %d, want 1", first.RevisionNumber)
	}
	createdAt := head.CreatedAt

	// Second write: ON CONFLICT (task_id, key) must update the same HEAD row
	// and the revision number must advance to 2.
	head.Title = "v2"
	head.Content = "two"
	head.SizeBytes = 3
	second := &models.TaskDocumentRevision{
		TaskID: "task-doc-pg", DocumentKey: "plan", Title: "v2", Content: "two", AuthorKind: "user",
	}
	if err := repo.WriteDocumentRevision(ctx, head, second, nil); err != nil {
		t.Fatalf("WriteDocumentRevision(second): %v", err)
	}
	if second.RevisionNumber != 2 {
		t.Errorf("second RevisionNumber = %d, want 2", second.RevisionNumber)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_documents WHERE task_id = ?`, "task-doc-pg"); got != 1 {
		t.Errorf("HEAD rows = %d, want 1 (ON CONFLICT must update, not insert)", got)
	}
	gotHead, err := repo.GetDocument(ctx, "task-doc-pg", "plan")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if gotHead.ID != "doc-pg" || gotHead.Title != "v2" || gotHead.Content != "two" {
		t.Errorf("HEAD = %+v, want doc-pg at v2/two", gotHead)
	}
	assertTimeEqual(t, "HEAD CreatedAt", gotHead.CreatedAt, createdAt)

	// Coalesce merge into the newest revision.
	coalesceID := second.ID
	merged := &models.TaskDocumentRevision{
		TaskID: "task-doc-pg", DocumentKey: "plan", Title: "v2 edited", Content: "two edited",
	}
	head.Title = "v2 edited"
	head.Content = "two edited"
	if err := repo.WriteDocumentRevision(ctx, head, merged, &coalesceID); err != nil {
		t.Fatalf("WriteDocumentRevision(merge): %v", err)
	}
	history, err := repo.ListDocumentRevisions(ctx, "task-doc-pg", "plan", 0)
	if err != nil {
		t.Fatalf("ListDocumentRevisions: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history has %d revisions, want 2 (coalesce must merge, not append)", len(history))
	}
	if history[0].RevisionNumber != 2 || history[0].Content != "two edited" {
		t.Errorf("newest revision = %+v, want revision 2 with the merged content", history[0])
	}
	if history[0].AuthorKind != "user" {
		t.Errorf("AuthorKind = %q, want the stored user preserved across the merge", history[0].AuthorKind)
	}

	next, err := repo.NextDocumentRevisionNumber(ctx, "task-doc-pg", "plan")
	if err != nil {
		t.Fatalf("NextDocumentRevisionNumber: %v", err)
	}
	if next != 3 {
		t.Errorf("NextDocumentRevisionNumber = %d, want 3 (MAX+1 over the nullable aggregate)", next)
	}

	// The NULL revert_of_revision_id column must scan back as a nil pointer.
	if history[0].RevertOfRevisionID != nil {
		t.Errorf("RevertOfRevisionID = %q, want nil", *history[0].RevertOfRevisionID)
	}
}

func TestPostgresWritePlanRevisionUpsertAndImplementationMarker(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTask(t, repo, "task-plan-pg")

	head := &models.TaskPlan{ID: "plan-pg", TaskID: "task-plan-pg", Content: "one"}
	first := &models.TaskPlanRevision{
		TaskID: "task-plan-pg", Title: "Plan", Content: "one", AuthorKind: "agent", AuthorName: "claude",
	}
	if err := repo.WritePlanRevision(ctx, head, first, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(first): %v", err)
	}
	if head.Title != "Plan" || head.CreatedBy != authorKindAgent {
		t.Errorf("head defaults = %q/%q, want Plan/%s", head.Title, head.CreatedBy, authorKindAgent)
	}
	if first.RevisionNumber != 1 {
		t.Fatalf("RevisionNumber = %d, want 1", first.RevisionNumber)
	}

	// The nullable implementation marker: COALESCE + CASE WHEN over TIMESTAMP.
	marked, err := repo.MarkTaskPlanImplementationStarted(ctx, "task-plan-pg", "session-pg", "jcfs")
	if err != nil {
		t.Fatalf("MarkTaskPlanImplementationStarted: %v", err)
	}
	if marked.ImplementationStartedAt == nil {
		t.Fatal("ImplementationStartedAt = nil, want the marker set")
	}
	startedAt := *marked.ImplementationStartedAt
	if marked.ImplementationStartedSessionID == nil || *marked.ImplementationStartedSessionID != "session-pg" {
		t.Errorf("ImplementationStartedSessionID = %v, want session-pg", marked.ImplementationStartedSessionID)
	}
	again, err := repo.MarkTaskPlanImplementationStarted(ctx, "task-plan-pg", "session-other", "someone")
	if err != nil {
		t.Fatalf("second MarkTaskPlanImplementationStarted: %v", err)
	}
	if !again.ImplementationStartedAt.Equal(startedAt) {
		t.Errorf("ImplementationStartedAt = %v, want the first value %v preserved", *again.ImplementationStartedAt, startedAt)
	}
	if again.ImplementationStartedBy == nil || *again.ImplementationStartedBy != "jcfs" {
		t.Errorf("ImplementationStartedBy = %v, want jcfs preserved", again.ImplementationStartedBy)
	}

	// A second WritePlanRevision must hit ON CONFLICT (task_id) and leave the
	// marker columns alone (they are not in the DO UPDATE SET list).
	head.Title = "Plan v2"
	head.Content = "two"
	second := &models.TaskPlanRevision{
		TaskID: "task-plan-pg", Title: "Plan v2", Content: "two", AuthorKind: "user", AuthorName: "jcfs",
	}
	if err := repo.WritePlanRevision(ctx, head, second, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(second): %v", err)
	}
	if second.RevisionNumber != 2 {
		t.Errorf("second RevisionNumber = %d, want 2", second.RevisionNumber)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_plans WHERE task_id = ?`, "task-plan-pg"); got != 1 {
		t.Errorf("HEAD rows = %d, want 1 (ON CONFLICT must update, not insert)", got)
	}
	gotHead, err := repo.GetTaskPlan(ctx, "task-plan-pg")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if gotHead.Title != "Plan v2" || gotHead.Content != "two" {
		t.Errorf("HEAD = %+v, want Plan v2/two", gotHead)
	}
	if gotHead.ImplementationStartedAt == nil || !gotHead.ImplementationStartedAt.Equal(startedAt) {
		t.Errorf("ImplementationStartedAt = %v, want %v preserved across the upsert",
			gotHead.ImplementationStartedAt, startedAt)
	}

	// The dialect-sensitive preservation branches must return the stored metadata and use
	// the stored title in the revision snapshot.
	head.Title = "ignored title"
	head.Content = "three"
	head.CreatedBy = "user"
	preserved := &models.TaskPlanRevision{
		TaskID: "task-plan-pg", Title: "ignored title", Content: "three", AuthorKind: "agent", AuthorName: "claude",
	}
	if err := repo.WritePlanRevision(ctx, head, preserved, nil, true, true); err != nil {
		t.Fatalf("WritePlanRevision(preserve): %v", err)
	}
	if head.Title != "Plan v2" || head.CreatedBy != authorKindAgent {
		t.Errorf("preserved HEAD metadata = %q/%q, want Plan v2/%s", head.Title, head.CreatedBy, authorKindAgent)
	}
	if preserved.Title != "Plan v2" {
		t.Errorf("preserved revision title = %q, want Plan v2", preserved.Title)
	}
	gotHead, err = repo.GetTaskPlan(ctx, "task-plan-pg")
	if err != nil {
		t.Fatalf("GetTaskPlan after preserve: %v", err)
	}
	if gotHead.Title != "Plan v2" || gotHead.CreatedBy != authorKindAgent || gotHead.Content != "three" {
		t.Errorf("preserved HEAD = %+v, want Plan v2/%s/three", gotHead, authorKindAgent)
	}

	// The sentinel path on a task with no plan row.
	if _, err := repo.MarkTaskPlanImplementationStarted(ctx, "task-plan-pg-absent", "s", "a"); !errors.Is(err, ErrTaskPlanNotFound) {
		t.Errorf("error = %v, want ErrTaskPlanNotFound", err)
	}
	// And the nil,nil not-found contract on the read path.
	missing, err := repo.GetTaskPlan(ctx, "task-plan-pg-absent")
	if err != nil || missing != nil {
		t.Errorf("GetTaskPlan(missing) = %+v, %v; want nil, nil", missing, err)
	}
}

func TestPostgresWritePlanRevisionMissingTask(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	taskID := "task-plan-pg-missing"

	err = repo.WritePlanRevision(ctx,
		&models.TaskPlan{ID: "plan-pg-missing", TaskID: taskID, Title: "Plan", Content: "body"},
		&models.TaskPlanRevision{TaskID: taskID, Title: "Plan", Content: "body", AuthorKind: "agent"},
		nil, false, false)
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("WritePlanRevision error = %v, want ErrTaskNotFound", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_plans WHERE task_id = ?`, taskID); got != 0 {
		t.Fatalf("plan HEAD rows = %d, want 0 after rollback", got)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_plan_revisions WHERE task_id = ?`, taskID); got != 0 {
		t.Fatalf("plan revision rows = %d, want 0 after rollback", got)
	}
}

// TestPostgresPlanRevisionWorkflowColumnsReplayMigration is the Postgres
// counterpart to the SQLite legacy-schema test. It verifies that an existing
// revision survives the additive columns and that new writes use the repaired
// schema to capture the current workflow step.
func TestPostgresPlanRevisionWorkflowColumnsReplayMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresTask(t, repo, "task-plan-replay-pg")

	for _, column := range []string{"workflow_step_id", "workflow_step_name", "workflow_step_color"} {
		if _, err := db.Exec(`ALTER TABLE task_plan_revisions DROP COLUMN ` + column); err != nil {
			t.Fatalf("drop legacy column %s: %v", column, err)
		}
	}
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_plan_revisions
			(id, task_id, revision_number, title, content, author_kind, author_name, revert_of_revision_id, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?, ?, ?, NULL, ?, ?)
	`), "legacy-plan-revision-pg", "task-plan-replay-pg", "Plan", "legacy", "agent", "Agent", now, now); err != nil {
		t.Fatalf("insert legacy plan revision: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("run legacy plan migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay legacy plan migrations: %v", err)
	}

	var stepID, stepName, stepColor string
	if err := db.QueryRow(db.Rebind(`
		SELECT workflow_step_id, workflow_step_name, workflow_step_color
		FROM task_plan_revisions WHERE id = ?
	`), "legacy-plan-revision-pg").Scan(&stepID, &stepName, &stepColor); err != nil {
		t.Fatalf("read migrated legacy revision: %v", err)
	}
	if stepID != "" || stepName != "" || stepColor != "" {
		t.Fatalf("legacy workflow fields = %q/%q/%q, want empty defaults", stepID, stepName, stepColor)
	}

	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-plan-replay-pg", Name: "Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position, color)
		VALUES (?, ?, ?, 0, ?)
	`), "step-review-pg", "workflow-plan-replay-pg", "Review", "bg-purple-500"); err != nil {
		t.Fatalf("insert workflow step: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		UPDATE tasks SET workflow_id = ?, workflow_step_id = ? WHERE id = ?
	`), "workflow-plan-replay-pg", "step-review-pg", "task-plan-replay-pg"); err != nil {
		t.Fatalf("set task workflow step: %v", err)
	}

	rev := &models.TaskPlanRevision{
		TaskID: "task-plan-replay-pg", Title: "Plan", Content: "new", AuthorKind: "user", AuthorName: "You",
	}
	if err := repo.WritePlanRevision(ctx, &models.TaskPlan{TaskID: "task-plan-replay-pg", Content: "new"}, rev, nil, false, false); err != nil {
		t.Fatalf("write stamped revision after migration: %v", err)
	}
	if rev.WorkflowStepID != "step-review-pg" || rev.WorkflowStepName != "Review" || rev.WorkflowStepColor != "bg-purple-500" {
		t.Fatalf("post-migration workflow stamp = %q/%q/%q, want step-review-pg/Review/bg-purple-500", rev.WorkflowStepID, rev.WorkflowStepName, rev.WorkflowStepColor)
	}
}
