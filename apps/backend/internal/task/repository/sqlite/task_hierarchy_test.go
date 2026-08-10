package sqlite

// Coverage for the parent/child/sibling read surface of task.go plus the
// batch GetTasksByIDs fetch. These share one fixture shape: a parent with
// active, archived, ephemeral and automation-origin children, so every
// method's filter clause is exercised against rows that must be excluded.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	hierarchyWorkspaceID = "ws-hierarchy"
	hierarchyParentID    = "task-hierarchy-parent"
)

// seedHierarchy builds a parent with four children: two ordinary active
// children, one archived, one ephemeral, plus one automation-origin child.
// Ordering is pinned by explicit created_at values so the ASC assertions
// cannot pass by accident.
func seedHierarchy(t *testing.T) *Repository {
	t.Helper()
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, hierarchyWorkspaceID)

	rows := []struct {
		id      string
		created time.Time
		mutate  func(*models.Task)
	}{
		{id: hierarchyParentID, created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{id: "task-child-b", created: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
			mutate: func(task *models.Task) { task.ParentID = hierarchyParentID }},
		{id: "task-child-a", created: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
			mutate: func(task *models.Task) { task.ParentID = hierarchyParentID }},
		{id: "task-child-archived", created: time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
			mutate: func(task *models.Task) { task.ParentID = hierarchyParentID }},
		{id: "task-child-ephemeral", created: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
			mutate: func(task *models.Task) { task.ParentID = hierarchyParentID; task.IsEphemeral = true }},
		{id: "task-child-automation", created: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
			mutate: func(task *models.Task) {
				task.ParentID = hierarchyParentID
				task.Origin = models.TaskOriginAutomationRun
			}},
		{id: "task-unrelated-root", created: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC)},
	}
	for _, row := range rows {
		task := &models.Task{ID: row.id, WorkspaceID: hierarchyWorkspaceID, Title: row.id}
		if row.mutate != nil {
			row.mutate(task)
		}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask(%s): %v", row.id, err)
		}
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
			`UPDATE tasks SET created_at = ? WHERE id = ?`), row.created, row.id); err != nil {
			t.Fatalf("pin created_at for %s: %v", row.id, err)
		}
	}
	if err := repo.ArchiveTask(ctx, "task-child-archived"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	return repo
}

func assertIDsEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}

func TestListChildrenExcludesArchivedEphemeralAndAutomationRows(t *testing.T) {
	repo := seedHierarchy(t)
	ctx := context.Background()

	got, err := repo.ListChildren(ctx, hierarchyParentID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	assertIDsEqual(t, "ListChildren", taskIDs(got), []string{"task-child-a", "task-child-b"})
	for _, child := range got {
		if child.ParentID != hierarchyParentID {
			t.Errorf("child %s ParentID = %q, want %q", child.ID, child.ParentID, hierarchyParentID)
		}
		if child.Title != child.ID {
			t.Errorf("child %s Title = %q, want the row's own title", child.ID, child.Title)
		}
	}

	empty, err := repo.ListChildren(ctx, "")
	if err != nil {
		t.Fatalf("ListChildren(\"\"): %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ListChildren(\"\") = %v, want a non-nil empty slice", empty)
	}
	none, err := repo.ListChildren(ctx, "task-unrelated-root")
	if err != nil {
		t.Fatalf("ListChildren(childless): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("ListChildren(childless) = %v, want empty", taskIDs(none))
	}
}

func TestListChildrenIncludingArchivedKeepsArchivedButStillDropsEphemeral(t *testing.T) {
	repo := seedHierarchy(t)
	ctx := context.Background()

	got, err := repo.ListChildrenIncludingArchived(ctx, hierarchyParentID)
	if err != nil {
		t.Fatalf("ListChildrenIncludingArchived: %v", err)
	}
	assertIDsEqual(t, "ListChildrenIncludingArchived", taskIDs(got),
		[]string{"task-child-a", "task-child-b", "task-child-archived"})

	var archived *models.Task
	for _, child := range got {
		if child.ID == "task-child-archived" {
			archived = child
		}
	}
	if archived == nil || archived.ArchivedAt == nil {
		t.Fatalf("archived child = %+v, want ArchivedAt set", archived)
	}

	empty, err := repo.ListChildrenIncludingArchived(ctx, "")
	if err != nil {
		t.Fatalf("ListChildrenIncludingArchived(\"\"): %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ListChildrenIncludingArchived(\"\") = %v, want a non-nil empty slice", empty)
	}
}

func TestListChildCompletionRowsReturnsCompactActiveChildren(t *testing.T) {
	repo := seedHierarchy(t)
	ctx := context.Background()

	if err := repo.UpdateTaskState(ctx, "task-child-a", "DONE"); err != nil {
		t.Fatalf("UpdateTaskState: %v", err)
	}

	got, err := repo.ListChildCompletionRows(ctx, hierarchyParentID)
	if err != nil {
		t.Fatalf("ListChildCompletionRows: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListChildCompletionRows returned %d rows (%+v), want 2", len(got), got)
	}
	if got[0].ID != "task-child-a" || got[1].ID != "task-child-b" {
		t.Fatalf("row order = %q,%q, want task-child-a,task-child-b", got[0].ID, got[1].ID)
	}
	if string(got[0].State) != "DONE" {
		t.Errorf("task-child-a State = %q, want DONE", got[0].State)
	}
	if got[0].Title != "task-child-a" {
		t.Errorf("task-child-a Title = %q, want the row's own title", got[0].Title)
	}
	if got[0].UpdatedAt.IsZero() {
		t.Error("task-child-a UpdatedAt is zero; the column must be projected")
	}

	empty, err := repo.ListChildCompletionRows(ctx, "")
	if err != nil {
		t.Fatalf("ListChildCompletionRows(\"\"): %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ListChildCompletionRows(\"\") = %v, want a non-nil empty slice", empty)
	}
}

func TestListSiblingsExcludesSelfRootsAndOtherWorkspaces(t *testing.T) {
	repo := seedHierarchy(t)
	ctx := context.Background()

	got, err := repo.ListSiblings(ctx, "task-child-a")
	if err != nil {
		t.Fatalf("ListSiblings: %v", err)
	}
	assertIDsEqual(t, "ListSiblings", taskIDs(got), []string{"task-child-b"})

	// A root task has no parent, so it deliberately reports no siblings even
	// though other roots exist in the same workspace.
	roots, err := repo.ListSiblings(ctx, hierarchyParentID)
	if err != nil {
		t.Fatalf("ListSiblings(root): %v", err)
	}
	if roots == nil || len(roots) != 0 {
		t.Errorf("ListSiblings(root) = %v, want a non-nil empty slice", taskIDs(roots))
	}

	// A child of the same parent living in a different workspace is not a
	// sibling: the query pins workspace_id as well as parent_id.
	seedWorkspace(t, repo, "ws-hierarchy-other")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-child-foreign", WorkspaceID: "ws-hierarchy-other",
		ParentID: hierarchyParentID, Title: "foreign",
	}); err != nil {
		t.Fatalf("CreateTask(foreign): %v", err)
	}
	got, err = repo.ListSiblings(ctx, "task-child-a")
	if err != nil {
		t.Fatalf("ListSiblings after foreign child: %v", err)
	}
	assertIDsEqual(t, "ListSiblings (cross-workspace)", taskIDs(got), []string{"task-child-b"})

	// ListSiblings resolves the task first, so an unknown id surfaces
	// GetTask's sentinel rather than an empty list.
	missing, err := repo.ListSiblings(ctx, "task-does-not-exist")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("ListSiblings(missing) error = %v, want ErrTaskNotFound", err)
	}
	if missing != nil {
		t.Errorf("ListSiblings(missing) = %v, want nil alongside the error", taskIDs(missing))
	}
}

func TestReparentDirectChildrenMovesEveryChildIncludingArchived(t *testing.T) {
	repo := seedHierarchy(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-hierarchy-new-parent", WorkspaceID: hierarchyWorkspaceID, Title: "new parent",
	}); err != nil {
		t.Fatalf("CreateTask(new parent): %v", err)
	}

	before := countRows(t, repo, `SELECT COUNT(1) FROM tasks WHERE parent_id = ?`, hierarchyParentID)
	if before != 5 {
		t.Fatalf("children before reparent = %d, want 5", before)
	}

	if err := repo.ReparentDirectChildren(ctx, hierarchyParentID, "task-hierarchy-new-parent"); err != nil {
		t.Fatalf("ReparentDirectChildren: %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM tasks WHERE parent_id = ?`, hierarchyParentID); got != 0 {
		t.Errorf("children left on the old parent = %d, want 0", got)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM tasks WHERE parent_id = ?`, "task-hierarchy-new-parent"); got != 5 {
		t.Errorf("children on the new parent = %d, want all 5 (archived and ephemeral included)", got)
	}

	// Reparenting to the empty id turns the children into roots.
	if err := repo.ReparentDirectChildren(ctx, "task-hierarchy-new-parent", ""); err != nil {
		t.Fatalf("ReparentDirectChildren(to root): %v", err)
	}
	if got := countRows(t, repo,
		`SELECT COUNT(1) FROM tasks WHERE parent_id = ?`, "task-hierarchy-new-parent"); got != 0 {
		t.Errorf("children after rooting = %d, want 0", got)
	}

	// An empty old parent is a no-op guard, not a mass update of every root.
	rootsBefore := countRows(t, repo, `SELECT COUNT(1) FROM tasks WHERE parent_id = ''`)
	if err := repo.ReparentDirectChildren(ctx, "", "task-hierarchy-new-parent"); err != nil {
		t.Fatalf("ReparentDirectChildren(\"\"): %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(1) FROM tasks WHERE parent_id = ''`); got != rootsBefore {
		t.Errorf("root rows = %d, want %d unchanged — an empty oldParentID must be a no-op", got, rootsBefore)
	}
}

func TestGetTasksByIDsFetchesRequestedRowsAndSkipsUnknown(t *testing.T) {
	repo := seedHierarchy(t)
	ctx := context.Background()

	got, err := repo.GetTasksByIDs(ctx, []string{"task-child-a", "task-child-archived", "task-nope"})
	if err != nil {
		t.Fatalf("GetTasksByIDs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("GetTasksByIDs returned %d rows (%v), want 2", len(got), taskIDs(got))
	}
	byID := make(map[string]*models.Task, len(got))
	for _, task := range got {
		byID[task.ID] = task
	}
	if byID["task-child-a"] == nil || byID["task-child-archived"] == nil {
		t.Fatalf("GetTasksByIDs = %v, want both requested rows", taskIDs(got))
	}
	// Unlike the list reads, this one is deliberately filter-free: the
	// archived row must come back.
	if byID["task-child-archived"].ArchivedAt == nil {
		t.Error("archived task ArchivedAt = nil, want it populated")
	}
	if byID["task-child-a"].WorkspaceID != hierarchyWorkspaceID {
		t.Errorf("WorkspaceID = %q, want %q", byID["task-child-a"].WorkspaceID, hierarchyWorkspaceID)
	}

	empty, err := repo.GetTasksByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("GetTasksByIDs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("GetTasksByIDs(nil) = %v, want empty", taskIDs(empty))
	}
}
