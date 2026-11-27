package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestQuerySidebarTaskPageAcceptsMoreThanFiveHundredCollapsedIDs(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-many-collapsed")
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "visible", WorkspaceID: "ws-sidebar-many-collapsed", Title: "Visible",
	}); err != nil {
		t.Fatalf("create visible task: %v", err)
	}

	query := sidebarTaskQuery(1)
	query.CollapsedTaskIDs = make([]string, 501)
	for index := range query.CollapsedTaskIDs {
		query.CollapsedTaskIDs[index] = fmt.Sprintf("collapsed-%03d", index)
	}
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-many-collapsed", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query with 501 collapsed task IDs: %v", err)
	}
	if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != "[visible]" {
		t.Fatalf("visible tasks = %v, want [visible]", got)
	}
}

func TestQuerySidebarTaskPageKeepsDescendantCountsWhenSubtasksAreCollapsed(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const workspaceID = "ws-sidebar-collapsed-descendant-counts"
	seedWorkspace(t, repo, workspaceID)
	for _, task := range []*models.Task{
		{ID: "root", WorkspaceID: workspaceID, Title: "Root"},
		{ID: "child", WorkspaceID: workspaceID, Title: "Child", ParentID: "root"},
		{ID: "grandchild", WorkspaceID: workspaceID, Title: "Grandchild", ParentID: "child"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	query := sidebarTaskQuery(1)
	query.CollapsedTaskIDs = []string{"child"}
	page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query collapsed task page: %v", err)
	}
	counts := make(map[string]int, len(page.Entries))
	for _, entry := range page.Entries {
		if entry.Kind == "task" {
			counts[entry.TaskID] = entry.SubtaskCount
		}
	}
	if counts["root"] != 2 || counts["child"] != 1 {
		t.Fatalf("descendant counts = %v, want root=2 child=1 while grandchild is hidden", counts)
	}
	if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != "[root child]" {
		t.Fatalf("visible tasks = %v, want [root child]", got)
	}
}

func TestQuerySidebarTaskPageHydratesTaskRepositoryLinks(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const workspaceID = "ws-sidebar-page-repository-links"
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-page-link", WorkspaceID: workspaceID, Name: "page-link",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-page-link", WorkspaceID: workspaceID, Title: "Repository-linked task",
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "link-page-task", TaskID: "task-page-link", RepositoryID: "repo-page-link", Position: 0,
	}); err != nil {
		t.Fatalf("attach repository to task: %v", err)
	}

	page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query sidebar page: %v", err)
	}
	if len(page.Tasks) != 1 || len(page.Tasks[0].Repositories) != 1 {
		t.Fatalf("task repository links = %+v, want one link", page.Tasks)
	}
	if got := page.Tasks[0].Repositories[0].RepositoryID; got != "repo-page-link" {
		t.Fatalf("repository ID = %q, want repo-page-link", got)
	}
}

func TestBuildSidebarTaskPageContinuationMarkersAreScopedToPageBoundaries(t *testing.T) {
	query := sidebarTaskQuery(1)
	rows := []sidebarPageRow{
		{taskID: "root", groupKey: "all"},
		{taskID: "first-child", groupKey: "all", parentID: "off-page-parent", parentTitle: "Parent", depth: 1, continuesGroup: true},
		{taskID: "second-child", groupKey: "all", parentID: "off-page-parent", parentTitle: "Parent", depth: 1, continuesGroup: true},
	}
	result := buildSidebarTaskPageResult("workspace", query, models.SidebarTaskViewPreferences{}, 1, 3, 3, 1,
		[]sidebarGroupRow{{groupKey: "all", groupLabel: "All", count: 3}}, rows, nil)

	if result.Entries[0].Continuation {
		t.Fatal("first page group was marked as a continuation")
	}
	continuations := 0
	for _, entry := range result.Entries {
		if entry.Kind == "continuation" && entry.ParentID == "off-page-parent" {
			continuations++
		}
	}
	if continuations != 1 {
		t.Fatalf("off-page parent continuation count = %d, want exactly one; entries: %+v", continuations, result.Entries)
	}
}

func TestQuerySidebarTaskPageReranksRootsAfterRepositoryGroupMerge(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const workspaceID = "ws-sidebar-repository-merge-order"
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-only", WorkspaceID: workspaceID, Name: "only-repository",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for _, task := range []*models.Task{
		{ID: "a-pinned", WorkspaceID: workspaceID, Title: "Pinned old", UpdatedAt: base},
		{ID: "b-named-old", WorkspaceID: workspaceID, Title: "Named old", UpdatedAt: base.Add(time.Minute)},
		{ID: "z-unassigned-new", WorkspaceID: workspaceID, Title: "Unassigned new", UpdatedAt: base.Add(2 * time.Minute)},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "link-pinned", TaskID: "a-pinned", RepositoryID: "repo-only", Position: 0,
	}); err != nil {
		t.Fatalf("attach repository to pinned task: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "link-named-old", TaskID: "b-named-old", RepositoryID: "repo-only", Position: 0,
	}); err != nil {
		t.Fatalf("attach repository to named task: %v", err)
	}
	for id, updatedAt := range map[string]time.Time{
		"a-pinned":         base,
		"b-named-old":      base.Add(time.Minute),
		"z-unassigned-new": base.Add(2 * time.Minute),
	} {
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`), updatedAt, id); err != nil {
			t.Fatalf("set activity for %s: %v", id, err)
		}
	}

	query := sidebarTaskQuery(1)
	query.Group = "repository"
	page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{
		PinnedTaskIDs: []string{"a-pinned"},
	})
	if err != nil {
		t.Fatalf("query merged repository group: %v", err)
	}
	if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != "[a-pinned z-unassigned-new b-named-old]" {
		t.Fatalf("merged repository root order = %v, want pinned then descending activity", got)
	}
}
