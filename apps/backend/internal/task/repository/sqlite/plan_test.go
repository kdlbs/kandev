package sqlite

//revive:disable:file-length-limit // Task plan HEAD + revision persistence coverage is intentionally scenario-heavy.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func assertTaskPlanEqual(t *testing.T, got, want *models.TaskPlan) {
	t.Helper()
	if got == nil {
		t.Fatal("plan is nil")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.TaskID != want.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, want.TaskID)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Content != want.Content {
		t.Errorf("Content = %q, want %q", got.Content, want.Content)
	}
	if got.CreatedBy != want.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, want.CreatedBy)
	}
	assertTimeEqual(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertTimeEqual(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
	assertOptionalString(t, "ImplementationStartedSessionID", got.ImplementationStartedSessionID, want.ImplementationStartedSessionID)
	assertOptionalString(t, "ImplementationStartedBy", got.ImplementationStartedBy, want.ImplementationStartedBy)
	switch {
	case want.ImplementationStartedAt == nil && got.ImplementationStartedAt != nil:
		t.Errorf("ImplementationStartedAt = %v, want nil", *got.ImplementationStartedAt)
	case want.ImplementationStartedAt != nil && got.ImplementationStartedAt == nil:
		t.Errorf("ImplementationStartedAt = nil, want %v", *want.ImplementationStartedAt)
	case want.ImplementationStartedAt != nil && !got.ImplementationStartedAt.Truncate(time.Microsecond).Equal(want.ImplementationStartedAt.Truncate(time.Microsecond)):
		t.Errorf("ImplementationStartedAt = %v, want %v", *got.ImplementationStartedAt, *want.ImplementationStartedAt)
	}
}

func assertOptionalString(t *testing.T, label string, got, want *string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %q, want nil", label, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %q", label, *want)
	case want != nil && *got != *want:
		t.Errorf("%s = %q, want %q", label, *got, *want)
	}
}

func assertPlanRevisionEqual(t *testing.T, got, want *models.TaskPlanRevision) {
	t.Helper()
	if got == nil {
		t.Fatal("revision is nil")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.TaskID != want.TaskID {
		t.Errorf("TaskID = %q, want %q", got.TaskID, want.TaskID)
	}
	if got.RevisionNumber != want.RevisionNumber {
		t.Errorf("RevisionNumber = %d, want %d", got.RevisionNumber, want.RevisionNumber)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Content != want.Content {
		t.Errorf("Content = %q, want %q", got.Content, want.Content)
	}
	if got.AuthorKind != want.AuthorKind {
		t.Errorf("AuthorKind = %q, want %q", got.AuthorKind, want.AuthorKind)
	}
	if got.AuthorName != want.AuthorName {
		t.Errorf("AuthorName = %q, want %q", got.AuthorName, want.AuthorName)
	}
	assertOptionalString(t, "RevertOfRevisionID", got.RevertOfRevisionID, want.RevertOfRevisionID)
	assertTimeEqual(t, "CreatedAt", got.CreatedAt, want.CreatedAt)
	assertTimeEqual(t, "UpdatedAt", got.UpdatedAt, want.UpdatedAt)
}

func TestCreateTaskPlanRoundTripsEveryField(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-full")

	before := time.Now().UTC().Add(-time.Second)
	want := &models.TaskPlan{
		ID: "plan-full", TaskID: "task-plan-full",
		Title: "Implementation plan", Content: "## Steps\n1. do it", CreatedBy: "user",
	}
	if err := repo.CreateTaskPlan(ctx, want); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	if want.CreatedAt.Before(before) || !want.CreatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("CreateTaskPlan stamped CreatedAt=%v UpdatedAt=%v, want equal times after %v",
			want.CreatedAt, want.UpdatedAt, before)
	}

	got, err := repo.GetTaskPlan(ctx, "task-plan-full")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	assertTaskPlanEqual(t, got, want)
}

func TestCreateTaskPlanAppliesDefaultsForEmptyFields(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-defaults")

	plan := &models.TaskPlan{TaskID: "task-plan-defaults", Content: "body"}
	if err := repo.CreateTaskPlan(ctx, plan); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	if plan.ID == "" {
		t.Fatal("ID left empty; a UUID should have been generated")
	}
	if plan.Title != "Plan" {
		t.Errorf("Title = %q, want the %q default", plan.Title, "Plan")
	}
	if plan.CreatedBy != authorKindAgent {
		t.Errorf("CreatedBy = %q, want the %q default", plan.CreatedBy, authorKindAgent)
	}

	got, err := repo.GetTaskPlan(ctx, "task-plan-defaults")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if got.Title != "Plan" || got.CreatedBy != authorKindAgent {
		t.Errorf("persisted defaults = %q/%q, want Plan/%s", got.Title, got.CreatedBy, authorKindAgent)
	}
	if got.ImplementationStartedAt != nil || got.ImplementationStartedSessionID != nil || got.ImplementationStartedBy != nil {
		t.Errorf("implementation markers = %+v, want all nil on a fresh plan", got)
	}
}

func TestCreateTaskPlanRejectsSecondPlanForSameTask(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-unique")

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-unique-1", TaskID: "task-plan-unique", Title: "One", Content: "one",
	}); err != nil {
		t.Fatalf("CreateTaskPlan(first): %v", err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-unique-2", TaskID: "task-plan-unique", Title: "Two", Content: "two",
	}); err == nil {
		t.Fatal("CreateTaskPlan accepted a second plan for the same task; task_id is UNIQUE")
	}
	got, err := repo.GetTaskPlan(ctx, "task-plan-unique")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if got.Title != "One" {
		t.Errorf("Title = %q, want the original %q", got.Title, "One")
	}
}

func TestGetTaskPlanReturnsNilNilWhenMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)

	got, err := repo.GetTaskPlan(context.Background(), "task-plan-missing")
	if err != nil {
		t.Fatalf("GetTaskPlan returned error %v, want the nil,nil not-found contract", err)
	}
	if got != nil {
		t.Errorf("GetTaskPlan = %+v, want nil", got)
	}
}

func TestUpdateTaskPlanWritesMutableFieldsAndReportsMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-update")

	plan := &models.TaskPlan{
		ID: "plan-update", TaskID: "task-plan-update", Title: "Before", Content: "old", CreatedBy: "agent",
	}
	if err := repo.CreateTaskPlan(ctx, plan); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	createdAt := plan.CreatedAt

	plan.Title = "After"
	plan.Content = "new"
	plan.CreatedBy = "user"
	if err := repo.UpdateTaskPlan(ctx, plan); err != nil {
		t.Fatalf("UpdateTaskPlan: %v", err)
	}

	got, err := repo.GetTaskPlan(ctx, "task-plan-update")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	assertTaskPlanEqual(t, got, plan)
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want it preserved at %v", got.CreatedAt, createdAt)
	}

	err = repo.UpdateTaskPlan(ctx, &models.TaskPlan{TaskID: "task-plan-nowhere", Title: "x"})
	if err == nil {
		t.Fatal("UpdateTaskPlan on a missing row returned nil")
	}
	if !strings.Contains(err.Error(), "task plan not found") {
		t.Errorf("UpdateTaskPlan error = %q, want a task-plan-not-found message", err)
	}
}

func TestDeleteTaskPlanRemovesRowAndReportsMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-delete")
	seedTaskForDocs(t, repo, "task-plan-keep")

	for _, taskID := range []string{"task-plan-delete", "task-plan-keep"} {
		if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
			ID: "plan-" + taskID, TaskID: taskID, Title: "Plan", Content: taskID,
		}); err != nil {
			t.Fatalf("CreateTaskPlan(%s): %v", taskID, err)
		}
	}

	if err := repo.DeleteTaskPlan(ctx, "task-plan-delete"); err != nil {
		t.Fatalf("DeleteTaskPlan: %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_plans WHERE task_id = ?`, "task-plan-delete"); got != 0 {
		t.Errorf("rows after delete = %d, want 0", got)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_plans WHERE task_id = ?`, "task-plan-keep"); got != 1 {
		t.Errorf("other task's plan rows = %d, want 1 (delete must be task-scoped)", got)
	}

	err := repo.DeleteTaskPlan(ctx, "task-plan-delete")
	if err == nil {
		t.Fatal("second DeleteTaskPlan returned nil; a missing row must be reported")
	}
	if !strings.Contains(err.Error(), "task plan not found") {
		t.Errorf("DeleteTaskPlan error = %q, want a task-plan-not-found message", err)
	}
}

func TestMarkTaskPlanImplementationStartedIsIdempotent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-mark")

	plan := &models.TaskPlan{ID: "plan-mark", TaskID: "task-plan-mark", Title: "Plan", Content: "body"}
	if err := repo.CreateTaskPlan(ctx, plan); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	createdUpdatedAt := plan.UpdatedAt

	marked, err := repo.MarkTaskPlanImplementationStarted(ctx, "task-plan-mark", "session-first", "jcfs")
	if err != nil {
		t.Fatalf("MarkTaskPlanImplementationStarted: %v", err)
	}
	if marked.ImplementationStartedAt == nil {
		t.Fatal("ImplementationStartedAt = nil, want the first-start marker set")
	}
	assertOptionalString(t, "ImplementationStartedSessionID", marked.ImplementationStartedSessionID, strptr("session-first"))
	assertOptionalString(t, "ImplementationStartedBy", marked.ImplementationStartedBy, strptr("jcfs"))
	if !marked.UpdatedAt.After(createdUpdatedAt) {
		t.Errorf("UpdatedAt = %v, want it bumped past %v on the first mark", marked.UpdatedAt, createdUpdatedAt)
	}
	firstStartedAt := *marked.ImplementationStartedAt
	firstUpdatedAt := marked.UpdatedAt

	second, err := repo.MarkTaskPlanImplementationStarted(ctx, "task-plan-mark", "session-second", "someone-else")
	if err != nil {
		t.Fatalf("second MarkTaskPlanImplementationStarted: %v", err)
	}
	if !second.ImplementationStartedAt.Equal(firstStartedAt) {
		t.Errorf("ImplementationStartedAt = %v, want the first value %v preserved", *second.ImplementationStartedAt, firstStartedAt)
	}
	assertOptionalString(t, "ImplementationStartedSessionID", second.ImplementationStartedSessionID, strptr("session-first"))
	assertOptionalString(t, "ImplementationStartedBy", second.ImplementationStartedBy, strptr("jcfs"))
	if !second.UpdatedAt.Equal(firstUpdatedAt) {
		t.Errorf("UpdatedAt = %v, want it unchanged at %v on a repeat mark", second.UpdatedAt, firstUpdatedAt)
	}
}

func TestMarkTaskPlanImplementationStartedReturnsSentinelWhenPlanMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)

	got, err := repo.MarkTaskPlanImplementationStarted(context.Background(), "task-plan-absent", "session", "actor")
	if got != nil {
		t.Errorf("plan = %+v, want nil", got)
	}
	if !errors.Is(err, ErrTaskPlanNotFound) {
		t.Fatalf("error = %v, want ErrTaskPlanNotFound", err)
	}
	if !strings.Contains(err.Error(), "task-plan-absent") {
		t.Errorf("error = %q, want the task id included for diagnostics", err)
	}
}

func TestInsertTaskPlanRevisionRoundTripsEveryField(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-full")

	revertOf := "rev-earlier"
	want := &models.TaskPlanRevision{
		ID: "planrev-full", TaskID: "task-planrev-full", RevisionNumber: 4,
		Title: "Plan v4", Content: "four", AuthorKind: "user", AuthorName: "jcfs",
		RevertOfRevisionID: &revertOf,
		CreatedAt:          time.Date(2026, 6, 6, 6, 6, 6, 789000000, time.UTC),
	}
	if err := repo.InsertTaskPlanRevision(ctx, want); err != nil {
		t.Fatalf("InsertTaskPlanRevision: %v", err)
	}

	got, err := repo.GetTaskPlanRevision(ctx, "planrev-full")
	if err != nil {
		t.Fatalf("GetTaskPlanRevision: %v", err)
	}
	assertPlanRevisionEqual(t, got, want)

	latest, err := repo.GetLatestTaskPlanRevision(ctx, "task-planrev-full")
	if err != nil {
		t.Fatalf("GetLatestTaskPlanRevision: %v", err)
	}
	assertPlanRevisionEqual(t, latest, want)

	listed, err := repo.ListTaskPlanRevisions(ctx, "task-planrev-full", 0)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListTaskPlanRevisions returned %d rows, want 1", len(listed))
	}
	assertPlanRevisionEqual(t, listed[0], want)
}

func TestInsertTaskPlanRevisionDefaultsIDAndCreatedAt(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-defaults")

	before := time.Now().UTC().Add(-time.Second)
	rev := &models.TaskPlanRevision{
		TaskID: "task-planrev-defaults", RevisionNumber: 1, Title: "Plan", Content: "v1", AuthorKind: "agent",
	}
	if err := repo.InsertTaskPlanRevision(ctx, rev); err != nil {
		t.Fatalf("InsertTaskPlanRevision: %v", err)
	}
	if rev.ID == "" {
		t.Fatal("ID left empty; a UUID should have been generated")
	}
	if rev.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, want a freshly stamped time after %v", rev.CreatedAt, before)
	}
	got, err := repo.GetTaskPlanRevision(ctx, rev.ID)
	if err != nil {
		t.Fatalf("GetTaskPlanRevision: %v", err)
	}
	if got.RevertOfRevisionID != nil {
		t.Errorf("RevertOfRevisionID = %q, want nil for a NULL column", *got.RevertOfRevisionID)
	}
}

func TestInsertTaskPlanRevisionRejectsDuplicateRevisionNumber(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-dupe")

	if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		ID: "planrev-dupe-1", TaskID: "task-planrev-dupe", RevisionNumber: 1,
		Title: "Plan", Content: "one", AuthorKind: "agent",
	}); err != nil {
		t.Fatalf("InsertTaskPlanRevision(first): %v", err)
	}
	err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		ID: "planrev-dupe-2", TaskID: "task-planrev-dupe", RevisionNumber: 1,
		Title: "Plan", Content: "two", AuthorKind: "agent",
	})
	if err == nil {
		t.Fatal("InsertTaskPlanRevision accepted a duplicate (task_id, revision_number)")
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_plan_revisions WHERE task_id = ?`, "task-planrev-dupe"); got != 1 {
		t.Errorf("rows = %d, want 1 (the duplicate must not land)", got)
	}
}

func TestUpdateTaskPlanRevisionMergesTitleAndContentOnly(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-update")

	rev := &models.TaskPlanRevision{
		ID: "planrev-update", TaskID: "task-planrev-update", RevisionNumber: 2,
		Title: "Before", Content: "old", AuthorKind: "agent", AuthorName: "claude",
		CreatedAt: time.Date(2026, 1, 1, 1, 1, 1, 0, time.UTC),
	}
	if err := repo.InsertTaskPlanRevision(ctx, rev); err != nil {
		t.Fatalf("InsertTaskPlanRevision: %v", err)
	}

	rev.Title = "After"
	rev.Content = "new"
	rev.AuthorKind = "user"
	rev.AuthorName = "ignored"
	if err := repo.UpdateTaskPlanRevision(ctx, rev); err != nil {
		t.Fatalf("UpdateTaskPlanRevision: %v", err)
	}

	got, err := repo.GetTaskPlanRevision(ctx, "planrev-update")
	if err != nil {
		t.Fatalf("GetTaskPlanRevision: %v", err)
	}
	if got.Title != "After" || got.Content != "new" {
		t.Errorf("merged revision = %q/%q, want After/new", got.Title, got.Content)
	}
	if got.AuthorKind != "agent" || got.AuthorName != "claude" {
		t.Errorf("author = %q/%q, want the stored agent/claude — the UPDATE must not touch author columns",
			got.AuthorKind, got.AuthorName)
	}
	if got.RevisionNumber != 2 {
		t.Errorf("RevisionNumber = %d, want 2 preserved", got.RevisionNumber)
	}
	if !got.CreatedAt.Equal(time.Date(2026, 1, 1, 1, 1, 1, 0, time.UTC)) {
		t.Errorf("CreatedAt = %v, want it preserved", got.CreatedAt)
	}
	if !got.UpdatedAt.Equal(rev.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want the refreshed %v", got.UpdatedAt, rev.UpdatedAt)
	}

	err = repo.UpdateTaskPlanRevision(ctx, &models.TaskPlanRevision{ID: "planrev-nowhere", Title: "x"})
	if err == nil {
		t.Fatal("UpdateTaskPlanRevision on a missing row returned nil")
	}
	if !strings.Contains(err.Error(), "task plan revision not found") {
		t.Errorf("error = %q, want a task-plan-revision-not-found message", err)
	}
}

func TestGetTaskPlanRevisionReturnsNilNilWhenMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()

	got, err := repo.GetTaskPlanRevision(ctx, "planrev-missing")
	if err != nil {
		t.Fatalf("GetTaskPlanRevision returned error %v, want nil,nil", err)
	}
	if got != nil {
		t.Errorf("GetTaskPlanRevision = %+v, want nil", got)
	}
	latest, err := repo.GetLatestTaskPlanRevision(ctx, "task-planrev-missing")
	if err != nil {
		t.Fatalf("GetLatestTaskPlanRevision returned error %v, want nil,nil", err)
	}
	if latest != nil {
		t.Errorf("GetLatestTaskPlanRevision = %+v, want nil", latest)
	}
}

func TestListTaskPlanRevisionsOrdersDescendingAndHonoursLimit(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-list")
	seedTaskForDocs(t, repo, "task-planrev-noise")

	for number := 1; number <= 3; number++ {
		if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
			ID: "planrev-list-" + string(rune('0'+number)), TaskID: "task-planrev-list",
			RevisionNumber: number, Title: "Plan", Content: "v", AuthorKind: "agent",
		}); err != nil {
			t.Fatalf("InsertTaskPlanRevision(%d): %v", number, err)
		}
	}
	if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		ID: "planrev-noise", TaskID: "task-planrev-noise", RevisionNumber: 99,
		Title: "Plan", Content: "noise", AuthorKind: "agent",
	}); err != nil {
		t.Fatalf("InsertTaskPlanRevision(noise): %v", err)
	}

	all, err := repo.ListTaskPlanRevisions(ctx, "task-planrev-list", 0)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions(limit=0): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("limit=0 returned %d rows, want 3", len(all))
	}
	for i, wantNumber := range []int{3, 2, 1} {
		if all[i].RevisionNumber != wantNumber {
			t.Fatalf("revision numbers = %d at index %d, want %d (newest first)", all[i].RevisionNumber, i, wantNumber)
		}
	}

	limited, err := repo.ListTaskPlanRevisions(ctx, "task-planrev-list", 1)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions(limit=1): %v", err)
	}
	if len(limited) != 1 || limited[0].RevisionNumber != 3 {
		t.Errorf("limit=1 returned %+v, want only revision 3", limited)
	}

	empty, err := repo.ListTaskPlanRevisions(ctx, "task-planrev-nothing", 0)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions(unknown task): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("unknown task returned %+v, want empty", empty)
	}
}

func TestNextTaskPlanRevisionNumber(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-planrev-next")

	next, err := repo.NextTaskPlanRevisionNumber(ctx, "task-planrev-next")
	if err != nil {
		t.Fatalf("NextTaskPlanRevisionNumber: %v", err)
	}
	if next != 1 {
		t.Errorf("NextTaskPlanRevisionNumber with no history = %d, want 1", next)
	}

	if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		ID: "planrev-next-7", TaskID: "task-planrev-next", RevisionNumber: 7,
		Title: "Plan", Content: "v7", AuthorKind: "agent",
	}); err != nil {
		t.Fatalf("InsertTaskPlanRevision: %v", err)
	}
	next, err = repo.NextTaskPlanRevisionNumber(ctx, "task-planrev-next")
	if err != nil {
		t.Fatalf("NextTaskPlanRevisionNumber: %v", err)
	}
	if next != 8 {
		t.Errorf("NextTaskPlanRevisionNumber = %d, want 8 (MAX(7)+1)", next)
	}
}

func TestWritePlanRevisionCreatesHeadAndFirstRevision(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-writeplan")

	head := &models.TaskPlan{TaskID: "task-writeplan", Content: "first"}
	rev := &models.TaskPlanRevision{
		TaskID: "task-writeplan", Title: "Plan", Content: "first", AuthorKind: "agent", AuthorName: "claude",
	}
	if err := repo.WritePlanRevision(ctx, head, rev, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision: %v", err)
	}
	if head.ID == "" {
		t.Fatal("head ID left empty; a UUID should have been generated")
	}
	if head.Title != "Plan" || head.CreatedBy != authorKindAgent {
		t.Errorf("head defaults = %q/%q, want Plan/%s", head.Title, head.CreatedBy, authorKindAgent)
	}
	if rev.RevisionNumber != 1 {
		t.Errorf("RevisionNumber = %d, want 1", rev.RevisionNumber)
	}

	gotHead, err := repo.GetTaskPlan(ctx, "task-writeplan")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	assertTaskPlanEqual(t, gotHead, head)

	gotRev, err := repo.GetLatestTaskPlanRevision(ctx, "task-writeplan")
	if err != nil {
		t.Fatalf("GetLatestTaskPlanRevision: %v", err)
	}
	assertPlanRevisionEqual(t, gotRev, rev)
}

func TestWritePlanRevisionMissingTask(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	taskID := "task-writeplan-missing"

	err := repo.WritePlanRevision(ctx,
		&models.TaskPlan{ID: "plan-missing", TaskID: taskID, Title: "Plan", Content: "body"},
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

func TestWritePlanRevisionUpsertsHeadAndAppendsRevisions(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-writeplan-append")

	head := &models.TaskPlan{
		ID: "plan-append", TaskID: "task-writeplan-append", Title: "v1", Content: "one", CreatedBy: "agent",
	}
	if err := repo.WritePlanRevision(ctx, head, &models.TaskPlanRevision{
		TaskID: "task-writeplan-append", Title: "v1", Content: "one", AuthorKind: "agent",
	}, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(first): %v", err)
	}
	createdAt := head.CreatedAt

	head.Title = "v2"
	head.Content = "two"
	head.CreatedBy = "user"
	second := &models.TaskPlanRevision{
		TaskID: "task-writeplan-append", Title: "v2", Content: "two", AuthorKind: "user", AuthorName: "jcfs",
	}
	if err := repo.WritePlanRevision(ctx, head, second, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(second): %v", err)
	}
	if second.RevisionNumber != 2 {
		t.Errorf("second RevisionNumber = %d, want 2", second.RevisionNumber)
	}

	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_plans WHERE task_id = ?`, "task-writeplan-append"); got != 1 {
		t.Errorf("HEAD rows = %d, want 1 (the upsert must not insert a second row)", got)
	}
	gotHead, err := repo.GetTaskPlan(ctx, "task-writeplan-append")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if gotHead.ID != "plan-append" {
		t.Errorf("HEAD id = %q, want plan-append", gotHead.ID)
	}
	if gotHead.Title != "v2" || gotHead.Content != "two" || gotHead.CreatedBy != "user" {
		t.Errorf("HEAD = %+v, want the v2 values", gotHead)
	}
	if !gotHead.CreatedAt.Equal(createdAt) {
		t.Errorf("HEAD CreatedAt = %v, want it preserved at %v", gotHead.CreatedAt, createdAt)
	}

	history, err := repo.ListTaskPlanRevisions(ctx, "task-writeplan-append", 0)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history has %d revisions, want 2", len(history))
	}
	if history[0].RevisionNumber != 2 || history[0].Content != "two" || history[0].AuthorName != "jcfs" {
		t.Errorf("newest revision = %+v, want revision 2 by jcfs", history[0])
	}
	if history[1].RevisionNumber != 1 || history[1].Content != "one" {
		t.Errorf("oldest revision = %+v, want revision 1 with the original content", history[1])
	}
}

func TestWritePlanRevisionPreservesImplementationMarkerAcrossUpsert(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-writeplan-marker")

	head := &models.TaskPlan{ID: "plan-marker", TaskID: "task-writeplan-marker", Title: "v1", Content: "one"}
	if err := repo.WritePlanRevision(ctx, head, &models.TaskPlanRevision{
		TaskID: "task-writeplan-marker", Title: "v1", Content: "one", AuthorKind: "agent",
	}, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(first): %v", err)
	}
	if _, err := repo.MarkTaskPlanImplementationStarted(ctx, "task-writeplan-marker", "session-x", "jcfs"); err != nil {
		t.Fatalf("MarkTaskPlanImplementationStarted: %v", err)
	}

	head.Title = "v2"
	head.Content = "two"
	if err := repo.WritePlanRevision(ctx, head, &models.TaskPlanRevision{
		TaskID: "task-writeplan-marker", Title: "v2", Content: "two", AuthorKind: "agent",
	}, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(second): %v", err)
	}

	got, err := repo.GetTaskPlan(ctx, "task-writeplan-marker")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if got.ImplementationStartedAt == nil {
		t.Fatal("ImplementationStartedAt = nil; the ON CONFLICT clause must not clear the marker")
	}
	assertOptionalString(t, "ImplementationStartedSessionID", got.ImplementationStartedSessionID, strptr("session-x"))
	assertOptionalString(t, "ImplementationStartedBy", got.ImplementationStartedBy, strptr("jcfs"))
}

func TestWritePlanRevisionCoalescesIntoExistingRevision(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-writeplan-merge")

	head := &models.TaskPlan{ID: "plan-merge", TaskID: "task-writeplan-merge", Title: "v1", Content: "one"}
	first := &models.TaskPlanRevision{
		ID: "planrev-merge", TaskID: "task-writeplan-merge", Title: "v1", Content: "one",
		AuthorKind: "agent", AuthorName: "claude",
		CreatedAt: time.Date(2026, 2, 2, 2, 2, 2, 0, time.UTC),
	}
	if err := repo.WritePlanRevision(ctx, head, first, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(first): %v", err)
	}

	head.Title = "v1 edited"
	head.Content = "one edited"
	merged := &models.TaskPlanRevision{
		TaskID: "task-writeplan-merge", Title: "v1 edited", Content: "one edited",
		AuthorKind: "ignored", AuthorName: "ignored",
	}
	coalesceID := "planrev-merge"
	if err := repo.WritePlanRevision(ctx, head, merged, &coalesceID, false, false); err != nil {
		t.Fatalf("WritePlanRevision(merge): %v", err)
	}
	if merged.ID != "planrev-merge" {
		t.Errorf("merged ID = %q, want planrev-merge", merged.ID)
	}

	history, err := repo.ListTaskPlanRevisions(ctx, "task-writeplan-merge", 0)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history has %d revisions, want 1 (coalesce must merge, not append)", len(history))
	}
	got := history[0]
	if got.Title != "v1 edited" || got.Content != "one edited" {
		t.Errorf("merged revision = %+v, want the edited title/content", got)
	}
	if got.RevisionNumber != 1 {
		t.Errorf("RevisionNumber = %d, want 1 preserved", got.RevisionNumber)
	}
	if got.AuthorKind != "agent" || got.AuthorName != "claude" {
		t.Errorf("author = %q/%q, want the original agent/claude preserved", got.AuthorKind, got.AuthorName)
	}
	if !got.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v preserved", got.CreatedAt, first.CreatedAt)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("UpdatedAt = %v, want it bumped past CreatedAt %v", got.UpdatedAt, got.CreatedAt)
	}
}

func TestWritePlanRevisionRollsBackWhenCoalesceTargetMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-writeplan-rollback")

	missingID := "planrev-does-not-exist"
	err := repo.WritePlanRevision(ctx,
		&models.TaskPlan{ID: "plan-rollback", TaskID: "task-writeplan-rollback", Title: "nope", Content: "nope"},
		&models.TaskPlanRevision{TaskID: "task-writeplan-rollback", Title: "x", Content: "y"},
		&missingID, false, false)
	if err == nil {
		t.Fatal("WritePlanRevision with an unknown coalesce id returned nil")
	}
	if !strings.Contains(err.Error(), "task plan revision not found") {
		t.Errorf("error = %q, want a task-plan-revision-not-found message", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_plans WHERE task_id = ?`, "task-writeplan-rollback"); got != 0 {
		t.Errorf("HEAD rows after a failed merge = %d, want 0 (transaction must roll back)", got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_plan_revisions WHERE task_id = ?`, "task-writeplan-rollback"); got != 0 {
		t.Errorf("revision rows after a failed merge = %d, want 0", got)
	}
}

func TestTaskPlanAndRevisionsCascadeOnTaskDelete(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-cascade")

	if err := repo.WritePlanRevision(ctx,
		&models.TaskPlan{ID: "plan-cascade", TaskID: "task-plan-cascade", Title: "Plan", Content: "body"},
		&models.TaskPlanRevision{TaskID: "task-plan-cascade", Title: "Plan", Content: "body", AuthorKind: "agent"},
		nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision: %v", err)
	}

	if err := repo.DeleteTask(ctx, "task-plan-cascade"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM task_plans WHERE task_id = ?`, "task-plan-cascade"); got != 0 {
		t.Errorf("plan rows after task delete = %d, want 0 (FK cascade)", got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM task_plan_revisions WHERE task_id = ?`, "task-plan-cascade"); got != 0 {
		t.Errorf("plan revision rows after task delete = %d, want 0 (FK cascade)", got)
	}
}

func TestTaskPlanMethodsPropagateBackendErrors(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-plan-broken")
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-broken", TaskID: "task-plan-broken", Title: "Plan", Content: "body",
	}); err != nil {
		t.Fatalf("CreateTaskPlan: %v", err)
	}
	closeRepoDB(t, repo)

	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{ID: "plan-after-close", TaskID: "task-plan-broken"}); err == nil {
		t.Error("CreateTaskPlan on a closed database returned nil")
	}
	if _, err := repo.GetTaskPlan(ctx, "task-plan-broken"); err == nil {
		t.Error("GetTaskPlan on a closed database returned nil")
	}
	if err := repo.UpdateTaskPlan(ctx, &models.TaskPlan{TaskID: "task-plan-broken"}); err == nil {
		t.Error("UpdateTaskPlan on a closed database returned nil")
	}
	if _, err := repo.MarkTaskPlanImplementationStarted(ctx, "task-plan-broken", "s", "a"); err == nil {
		t.Error("MarkTaskPlanImplementationStarted on a closed database returned nil")
	}
	if err := repo.DeleteTaskPlan(ctx, "task-plan-broken"); err == nil {
		t.Error("DeleteTaskPlan on a closed database returned nil")
	}
	if err := repo.InsertTaskPlanRevision(ctx, &models.TaskPlanRevision{
		TaskID: "task-plan-broken", RevisionNumber: 1,
	}); err == nil {
		t.Error("InsertTaskPlanRevision on a closed database returned nil")
	}
	if err := repo.UpdateTaskPlanRevision(ctx, &models.TaskPlanRevision{ID: "planrev-broken"}); err == nil {
		t.Error("UpdateTaskPlanRevision on a closed database returned nil")
	}
	if _, err := repo.GetTaskPlanRevision(ctx, "planrev-broken"); err == nil {
		t.Error("GetTaskPlanRevision on a closed database returned nil")
	}
	if _, err := repo.ListTaskPlanRevisions(ctx, "task-plan-broken", 0); err == nil {
		t.Error("ListTaskPlanRevisions on a closed database returned nil")
	}
	if _, err := repo.NextTaskPlanRevisionNumber(ctx, "task-plan-broken"); err == nil {
		t.Error("NextTaskPlanRevisionNumber on a closed database returned nil")
	}
	if err := repo.WritePlanRevision(ctx,
		&models.TaskPlan{TaskID: "task-plan-broken"},
		&models.TaskPlanRevision{TaskID: "task-plan-broken"},
		nil, false, false); err == nil {
		t.Error("WritePlanRevision on a closed database returned nil")
	}
}

func TestWritePlanRevisionPreserveFlagsGateOnlyTheUpdateBranch(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-writeplan-preserve")

	head := &models.TaskPlan{
		ID: "plan-preserve", TaskID: "task-writeplan-preserve", Title: "v1", Content: "one", CreatedBy: "agent",
	}
	if err := repo.WritePlanRevision(ctx, head, &models.TaskPlanRevision{
		TaskID: "task-writeplan-preserve", Title: "v1", Content: "one", AuthorKind: "agent",
	}, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(first): %v", err)
	}

	// An existing row with preserveTitle/preserveCreatedBy set keeps its stored title and
	// created_by even though head carries different values; content still updates.
	head.Title = "v2"
	head.Content = "two"
	head.CreatedBy = "user"
	if err := repo.WritePlanRevision(ctx, head, &models.TaskPlanRevision{
		TaskID: "task-writeplan-preserve", Title: "v2", Content: "two", AuthorKind: "agent",
	}, nil, true, true); err != nil {
		t.Fatalf("WritePlanRevision(preserve): %v", err)
	}

	gotHead, err := repo.GetTaskPlan(ctx, "task-writeplan-preserve")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if gotHead.Title != "v1" {
		t.Errorf("HEAD title = %q, want v1 preserved", gotHead.Title)
	}
	if gotHead.CreatedBy != "agent" {
		t.Errorf("HEAD created_by = %q, want agent preserved", gotHead.CreatedBy)
	}
	if gotHead.Content != "two" {
		t.Errorf("HEAD content = %q, want two (content is never preserved)", gotHead.Content)
	}
	if head.Title != "v1" || head.CreatedBy != "agent" {
		t.Errorf("returned HEAD metadata = %q/%q, want authoritative preserved values v1/agent", head.Title, head.CreatedBy)
	}

	revisions, err := repo.ListTaskPlanRevisions(ctx, "task-writeplan-preserve", 0)
	if err != nil {
		t.Fatalf("ListTaskPlanRevisions: %v", err)
	}
	latest, err := repo.GetTaskPlanRevision(ctx, revisions[0].ID)
	if err != nil {
		t.Fatalf("GetTaskPlanRevision: %v", err)
	}
	if latest.Title != "v1" {
		t.Errorf("revision title = %q, want authoritative preserved title v1", latest.Title)
	}
}

func TestWritePlanRevisionPreserveFlagsIgnoredOnFreshInsert(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-writeplan-preserve-insert")

	head := &models.TaskPlan{
		TaskID: "task-writeplan-preserve-insert", Title: "fresh", Content: "body", CreatedBy: "agent",
	}
	if err := repo.WritePlanRevision(ctx, head, &models.TaskPlanRevision{
		TaskID: "task-writeplan-preserve-insert", Title: "fresh", Content: "body", AuthorKind: "agent",
	}, nil, true, true); err != nil {
		t.Fatalf("WritePlanRevision: %v", err)
	}

	gotHead, err := repo.GetTaskPlan(ctx, "task-writeplan-preserve-insert")
	if err != nil {
		t.Fatalf("GetTaskPlan: %v", err)
	}
	if gotHead.Title != "fresh" || gotHead.CreatedBy != "agent" {
		t.Errorf("HEAD = %+v, want head's own values used on a fresh insert regardless of preserve flags", gotHead)
	}
}
