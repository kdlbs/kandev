package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestQuerySidebarTaskPageCollapseFollowsFilteredDisplayGraph(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		tasks []*models.Task
		want  []string
	}{
		{
			name: "collapsed parent excluded by filter",
			tasks: []*models.Task{
				{ID: "parent", WorkspaceID: "ws-sidebar-collapse-filtered-parent", Title: "Parent"},
				{ID: "child", WorkspaceID: "ws-sidebar-collapse-filtered-parent", ParentID: "parent", Title: "Needle child"},
			},
			want: []string{"child"},
		},
		{
			name: "excluded intermediate ancestor",
			tasks: []*models.Task{
				{ID: "parent", WorkspaceID: "ws-sidebar-collapse-filtered-intermediate", Title: "Needle parent"},
				{ID: "middle", WorkspaceID: "ws-sidebar-collapse-filtered-intermediate", ParentID: "parent", Title: "Middle"},
				{ID: "child", WorkspaceID: "ws-sidebar-collapse-filtered-intermediate", ParentID: "middle", Title: "Needle child"},
			},
			want: []string{"parent", "child"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			workspaceID := testCase.tasks[0].WorkspaceID
			seedWorkspace(t, repo, workspaceID)
			for _, task := range testCase.tasks {
				if err := repo.CreateTask(ctx, task); err != nil {
					t.Fatalf("create task %s: %v", task.ID, err)
				}
			}
			query := sidebarTaskQuery(1)
			query.CollapsedTaskIDs = []string{"parent"}
			query.Filters = []models.SidebarTaskViewClause{{
				Dimension: "titleMatch", Op: "matches", Value: json.RawMessage(`"needle"`),
			}}
			page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
			if err != nil {
				t.Fatalf("query collapsed filtered graph: %v", err)
			}
			got := sidebarTaskIDs(page.Tasks)
			gotSet := make(map[string]bool, len(got))
			for _, id := range got {
				gotSet[id] = true
			}
			if len(gotSet) != len(testCase.want) {
				t.Fatalf("visible filtered tasks = %v, want %v", got, testCase.want)
			}
			for _, id := range testCase.want {
				if !gotSet[id] {
					t.Fatalf("visible filtered tasks = %v, want %v", got, testCase.want)
				}
			}
			for _, entry := range page.Entries {
				if entry.Kind == "task" && entry.TaskID == "child" && entry.Depth != 0 {
					t.Fatalf("filtered child entry = %+v, want promoted root", entry)
				}
			}
		})
	}
}

func TestQuerySidebarTaskPageCollapsedParentsDoNotHideAcrossArchiveFilters(t *testing.T) {
	for _, queryArchived := range []bool{false, true} {
		t.Run(fmt.Sprintf("archived=%t", queryArchived), func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			ctx := context.Background()
			const workspaceID = "ws-sidebar-collapse-archive-filter"
			seedWorkspace(t, repo, workspaceID)
			archivedAt := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
			parent := &models.Task{ID: "parent", WorkspaceID: workspaceID, Title: "Parent"}
			child := &models.Task{ID: "child", WorkspaceID: workspaceID, ParentID: "parent", Title: "Child"}
			if queryArchived {
				child.ArchivedAt = &archivedAt
			} else {
				parent.ArchivedAt = &archivedAt
			}
			for _, task := range []*models.Task{parent, child} {
				if err := repo.CreateTask(ctx, task); err != nil {
					t.Fatalf("create task %s: %v", task.ID, err)
				}
			}
			archivedTaskID := parent.ID
			if queryArchived {
				archivedTaskID = child.ID
			}
			if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET archived_at = ? WHERE id = ?`), archivedAt, archivedTaskID); err != nil {
				t.Fatalf("archive task %s: %v", archivedTaskID, err)
			}
			query := sidebarTaskQuery(1)
			query.CollapsedTaskIDs = []string{"parent"}
			query.Filters = []models.SidebarTaskViewClause{{
				Dimension: "archived", Op: "is", Value: json.RawMessage(fmt.Sprintf("%t", queryArchived)),
			}}
			page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
			if err != nil {
				t.Fatalf("query archive-filtered collapsed graph: %v", err)
			}
			if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != "[child]" {
				t.Fatalf("archive-filtered child visibility = %v, want [child]", got)
			}
		})
	}
}

func TestQuerySidebarTaskPageBreaksMalformedParentCycleAtSmallestID(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-cycle")
	for _, task := range []*models.Task{
		{ID: "cycle-a", WorkspaceID: "ws-sidebar-cycle", Title: "Cycle A"},
		{ID: "cycle-b", WorkspaceID: "ws-sidebar-cycle", Title: "Cycle B"},
		{ID: "cycle-child", WorkspaceID: "ws-sidebar-cycle", ParentID: "cycle-a", Title: "Cycle child"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create malformed-cycle task %s: %v", task.ID, err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET parent_id = ? WHERE id = ?`), "cycle-b", "cycle-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET parent_id = ? WHERE id = ?`), "cycle-a", "cycle-b"); err != nil {
		t.Fatal(err)
	}
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-cycle", sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query malformed parent cycle: %v", err)
	}
	if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != "[cycle-a cycle-child cycle-b]" {
		t.Fatalf("cycle task order = %v, want smallest cycle ID promoted as root", got)
	}
}

func TestQuerySidebarTaskPageRanksCompleteTreeActivityBeforePaging(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-activity")
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for _, task := range []*models.Task{
		{ID: "parent", WorkspaceID: "ws-sidebar-activity", Title: "Parent", UpdatedAt: base, CreatedAt: base},
		{ID: "child", WorkspaceID: "ws-sidebar-activity", ParentID: "parent", Title: "Child", UpdatedAt: base, CreatedAt: base},
		{ID: "peer", WorkspaceID: "ws-sidebar-activity", Title: "Peer", UpdatedAt: base, CreatedAt: base},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	for taskID, activity := range map[string]string{
		"parent": base.Format(time.RFC3339Nano),
		"child":  base.Add(10 * time.Minute).Format(time.RFC3339Nano),
		"peer":   base.Add(5 * time.Minute).Format(time.RFC3339Nano),
	} {
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			INSERT INTO task_status_summaries (task_id, workspace_id, revision, summary, updated_at)
			VALUES (?, ?, 1, ?, ?)
		`), taskID, "ws-sidebar-activity", fmt.Sprintf(`{"last_activity_at":%q}`, activity), base); err != nil {
			t.Fatalf("insert activity summary for %s: %v", taskID, err)
		}
	}

	query := sidebarTaskQuery(1)
	query.PageSize = 1
	query.Sort = models.SidebarTaskViewSort{Key: "lastActivityAt", Direction: "desc"}
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-activity", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query activity-ranked page: %v", err)
	}
	if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != "[parent]" {
		t.Fatalf("first tree-activity task = %v, want parent with the newest included child", got)
	}
}

func TestQuerySidebarTaskPageOrdersActivityByInstant(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			ctx := context.Background()
			workspaceID := "ws-sidebar-activity-instant-" + backend
			seedWorkspace(t, repo, workspaceID)
			base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
			tasks := []*models.Task{
				{ID: "fractional-whole", WorkspaceID: workspaceID, Title: "Fractional whole", CreatedAt: base, UpdatedAt: base},
				{ID: "fractional-new", WorkspaceID: workspaceID, Title: "Fractional new", CreatedAt: base, UpdatedAt: base},
				{ID: "offset-old", WorkspaceID: workspaceID, Title: "Offset old", CreatedAt: base, UpdatedAt: base},
				{ID: "offset-new", WorkspaceID: workspaceID, Title: "Offset new", CreatedAt: base, UpdatedAt: base},
				{ID: "tree-root", WorkspaceID: workspaceID, Title: "Tree root", CreatedAt: base, UpdatedAt: base},
				{ID: "tree-child", WorkspaceID: workspaceID, ParentID: "tree-root", Title: "Tree child", CreatedAt: base, UpdatedAt: base},
				{ID: "same-instant", WorkspaceID: workspaceID, Title: "Same instant", CreatedAt: base, UpdatedAt: base},
				{ID: "instant-newer", WorkspaceID: workspaceID, Title: "Instant newer", CreatedAt: base, UpdatedAt: base},
				{ID: "instant-older", WorkspaceID: workspaceID, Title: "Instant older", CreatedAt: base, UpdatedAt: base},
				{ID: "invalid-day-30", WorkspaceID: workspaceID, Title: "Invalid day 30", CreatedAt: base, UpdatedAt: base},
				{ID: "invalid-day-31", WorkspaceID: workspaceID, Title: "Invalid day 31", CreatedAt: base, UpdatedAt: base},
				{ID: "fallback-whole", WorkspaceID: workspaceID, Title: "Fallback whole", CreatedAt: base, UpdatedAt: base},
				{ID: "fallback-fraction", WorkspaceID: workspaceID, Title: "Fallback fraction", CreatedAt: base, UpdatedAt: base.Add(900 * time.Millisecond)},
			}
			for _, task := range tasks {
				if err := repo.CreateTask(ctx, task); err != nil {
					t.Fatalf("create task %s: %v", task.ID, err)
				}
			}
			for taskID, updatedAt := range map[string]time.Time{
				"fallback-whole":    base,
				"fallback-fraction": base.Add(900 * time.Millisecond),
			} {
				if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`), updatedAt, taskID); err != nil {
					t.Fatalf("set fallback activity timestamp for %s: %v", taskID, err)
				}
			}
			activities := map[string]string{
				"fractional-whole": "2026-09-26T12:00:00Z",
				"fractional-new":   "2026-09-26T12:00:00.9Z",
				"offset-old":       "2026-09-26T13:00:00+01:00",
				"offset-new":       "2026-09-26T12:00:01Z",
				"tree-root":        "2026-09-26T11:00:00.12Z",
				"tree-child":       "2026-09-26T12:00:00.12+01:00",
				"same-instant":     "2026-09-26T11:00:00.12Z",
				"instant-newer":    "2026-09-26T11:00:00.121Z",
				"instant-older":    "2026-09-26T11:00:00.119Z",
				"invalid-day-30":   "2026-02-30T12:00:00Z",
				"invalid-day-31":   "2026-02-31T12:00:00Z",
			}
			for taskID, activity := range activities {
				if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
					INSERT INTO task_status_summaries (task_id, workspace_id, revision, summary, updated_at)
					VALUES (?, ?, 1, ?, ?)
				`), taskID, workspaceID, fmt.Sprintf(`{"last_activity_at":%q}`, activity), base); err != nil {
					t.Fatalf("insert activity summary for %s: %v", taskID, err)
				}
			}

			query := sidebarTaskQuery(1)
			query.Sort = models.SidebarTaskViewSort{Key: "lastActivityAt", Direction: "desc"}
			page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
			if err != nil {
				t.Fatalf("query activity-ranked page: %v", err)
			}
			positions := make(map[string]int, len(page.Tasks))
			for index, task := range page.Tasks {
				positions[task.ID] = index
			}
			assertBefore := func(newer, older string) {
				t.Helper()
				if positions[newer] >= positions[older] {
					t.Fatalf("activity order %v places %s at %d and %s at %d", sidebarTaskIDs(page.Tasks), newer, positions[newer], older, positions[older])
				}
			}
			assertBefore("fractional-new", "fractional-whole")
			assertBefore("offset-new", "offset-old")
			assertBefore("instant-newer", "tree-root")
			assertBefore("tree-root", "instant-older")
			assertBefore("invalid-day-31", "invalid-day-30")
			assertBefore("fallback-fraction", "fallback-whole")
			if positions["same-instant"] < positions["instant-newer"] || positions["same-instant"] > positions["instant-older"] {
				t.Fatalf("equivalent offset instants not kept between neighbors: %v", sidebarTaskIDs(page.Tasks))
			}
		})
	}
}

func TestQuerySidebarTaskPageUsesGlobalRankForGroupOrder(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			ctx := context.Background()
			workspaceID := "ws-sidebar-global-group-order-" + backend
			seedWorkspace(t, repo, workspaceID)
			for _, workflow := range []*models.Workflow{
				{ID: "workflow-a", WorkspaceID: workspaceID, Name: "Older workflow"},
				{ID: "workflow-z", WorkspaceID: workspaceID, Name: "Newest workflow"},
			} {
				if err := repo.CreateWorkflow(ctx, workflow); err != nil {
					t.Fatalf("create workflow %s: %v", workflow.ID, err)
				}
			}
			base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
			for _, task := range []*models.Task{
				{ID: "z-newest", WorkspaceID: workspaceID, WorkflowID: "workflow-z", Title: "Z newest", CreatedAt: base, UpdatedAt: base.Add(2 * time.Minute)},
				{ID: "a-unpinned", WorkspaceID: workspaceID, WorkflowID: "workflow-a", Title: "A unpinned", CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
				{ID: "a-pinned", WorkspaceID: workspaceID, WorkflowID: "workflow-a", Title: "A pinned", CreatedAt: base, UpdatedAt: base},
			} {
				if err := repo.CreateTask(ctx, task); err != nil {
					t.Fatalf("create task %s: %v", task.ID, err)
				}
			}
			for taskID, updatedAt := range map[string]time.Time{
				"z-newest":   base.Add(2 * time.Minute),
				"a-unpinned": base.Add(time.Minute),
				"a-pinned":   base,
			} {
				if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`), updatedAt, taskID); err != nil {
					t.Fatalf("set ordering timestamp for %s: %v", taskID, err)
				}
			}
			query := sidebarTaskQuery(1)
			query.Group = "workflow"
			page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{PinnedTaskIDs: []string{"a-pinned"}})
			if err != nil {
				t.Fatalf("query workflow groups: %v", err)
			}
			var groups []string
			var groupedTasks []string
			for _, entry := range page.Entries {
				if entry.Kind == "group" {
					groups = append(groups, entry.GroupKey)
				} else {
					groupedTasks = append(groupedTasks, entry.TaskID)
				}
			}
			if fmt.Sprint(groups) != "[workflow-z workflow-a]" {
				t.Fatalf("group order = %v, entries %+v, want global newest-root order", groups, page.Entries)
			}
			if fmt.Sprint(groupedTasks) != "[z-newest a-pinned a-unpinned]" {
				t.Fatalf("within-group pinned task order = %v", groupedTasks)
			}
		})
	}
}

func TestQuerySidebarTaskPageKeepsGroupAndContinuationContext(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-groups")
	for _, task := range []*models.Task{
		{ID: "group-a", WorkspaceID: "ws-sidebar-groups", Title: "Group A"},
		{ID: "group-b", WorkspaceID: "ws-sidebar-groups", Title: "Group B"},
		{ID: "group-c", WorkspaceID: "ws-sidebar-groups", Title: "Group C"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	query := sidebarTaskQuery(1)
	query.PageSize = 2
	first, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-groups", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query first group page: %v", err)
	}
	if len(first.Entries) != 3 || first.Entries[0].Kind != "group" || first.Entries[1].Kind != "task" {
		t.Fatalf("first page group entries = %+v", first.Entries)
	}

	query.Page = 2
	second, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-groups", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query continuation page: %v", err)
	}
	if len(second.Entries) != 2 || second.Entries[0].Kind != "group" || !second.Entries[0].Continuation {
		t.Fatalf("continuation page entries = %+v", second.Entries)
	}
}

func TestQuerySidebarTaskPageRetainsCollapsedGroupHeadersWithoutCountingThemAsTasks(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-collapsed-groups")
	for _, task := range []*models.Task{
		{ID: "active", WorkspaceID: "ws-sidebar-collapsed-groups", Title: "Active", State: "IN_PROGRESS"},
		{ID: "completed", WorkspaceID: "ws-sidebar-collapsed-groups", Title: "Completed", State: "COMPLETED"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	query := sidebarTaskQuery(1)
	query.Group = "state"
	query.PageSize = 1
	query.CollapsedGroupKeys = []string{"COMPLETED"}
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-collapsed-groups", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query collapsed groups: %v", err)
	}
	if page.TotalTasks != 2 || page.TotalVisibleTasks != 1 || page.TotalEntries != 3 {
		t.Fatalf("collapsed group totals = tasks %d visible %d entries %d", page.TotalTasks, page.TotalVisibleTasks, page.TotalEntries)
	}
	if len(page.Entries) != 3 || page.Entries[0].Kind != "group" || page.Entries[1].Kind != "task" ||
		page.Entries[2].Kind != "group" || page.Entries[2].GroupKey != "COMPLETED" {
		t.Fatalf("collapsed group entries = %+v", page.Entries)
	}
}

func TestQuerySidebarTaskPageKeepsWipQueuePositionAcrossViewFilters(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-wip")
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for _, task := range []*models.Task{
		{ID: "queued-critical", WorkspaceID: "ws-sidebar-wip", Title: "Queued critical"},
		{ID: "queued-high", WorkspaceID: "ws-sidebar-wip", Title: "Queued high"},
		{ID: "queued-filtered", WorkspaceID: "ws-sidebar-wip", Title: "Needle queued low"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create queued task %s: %v", task.ID, err)
		}
	}
	for _, queued := range []struct {
		taskID   string
		priority string
	}{
		{taskID: "queued-critical", priority: "critical"},
		{taskID: "queued-high", priority: "high"},
		{taskID: "queued-filtered", priority: "low"},
	} {
		_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			UPDATE tasks SET workflow_step_id = ?, queued_for_step_id = ?, queued_at = ?,
				wip_admitted = 0, position = 1, priority = ? WHERE id = ?
		`), "step-destination", "step-destination", base, queued.priority, queued.taskID)
		if err != nil {
			t.Fatalf("queue task %s: %v", queued.taskID, err)
		}
	}

	query := sidebarTaskQuery(1)
	query.Filters = []models.SidebarTaskViewClause{{
		Dimension: "titleMatch", Op: "matches", Value: json.RawMessage(`"needle"`),
	}}
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-wip", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query filtered WIP task: %v", err)
	}
	if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != "[queued-filtered]" {
		t.Fatalf("filtered WIP tasks = %v, want only queued-filtered", got)
	}
	encoded, err := json.Marshal(page.Entries)
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(encoded, &entries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry["kind"] != "task" {
			continue
		}
		if entry["task_id"] != "queued-filtered" || entry["wip_queue_position"] != float64(3) || entry["wip_queue_total"] != float64(3) {
			t.Fatalf("filtered task WIP status = %+v, want position 3 of 3", entry)
		}
		return
	}
	t.Fatalf("filtered page has no task entry: %+v", page.Entries)
}

func TestQuerySidebarTaskPagePostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres repository: %v", err)
	}
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sidebar-postgres")
	base := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for index, task := range []*models.Task{
		{ID: "parent", WorkspaceID: "ws-sidebar-postgres", Title: "Parent", UpdatedAt: base},
		{ID: "child-b", WorkspaceID: "ws-sidebar-postgres", ParentID: "parent", Title: "Child B", UpdatedAt: base.Add(time.Minute)},
		{ID: "child-a", WorkspaceID: "ws-sidebar-postgres", ParentID: "parent", Title: "Child A", UpdatedAt: base.Add(2 * time.Minute)},
		{ID: "peer", WorkspaceID: "ws-sidebar-postgres", Title: "Peer", UpdatedAt: base.Add(3 * time.Minute)},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create postgres task %d: %v", index, err)
		}
	}

	query := sidebarTaskQuery(1)
	query.PageSize = 2
	page, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-postgres", query, models.SidebarTaskViewPreferences{
		PinnedTaskIDs:          []string{"parent"},
		SubtaskOrderByParentID: map[string][]string{"parent": {"child-a", "child-b"}},
	})
	if err != nil {
		t.Fatalf("query postgres task page: %v", err)
	}
	if got := sidebarTaskIDs(page.Tasks); fmt.Sprint(got) != "[parent child-a]" {
		t.Fatalf("postgres first page order = %v, want pinned parent then first manual child", got)
	}
	if len(page.Entries) != 3 || page.Entries[2].Depth != 1 || page.Entries[2].ParentID != "parent" {
		t.Fatalf("postgres tree entries = %+v", page.Entries)
	}

	query = sidebarTaskQuery(1)
	query.Group = "state"
	query.Sort = models.SidebarTaskViewSort{Key: "state", Direction: "asc"}
	statePage, err := repo.QuerySidebarTaskPage(ctx, "ws-sidebar-postgres", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query postgres effective-state groups: %v", err)
	}
	if len(statePage.Tasks) != 4 || len(statePage.Entries) < 2 || statePage.Entries[0].Kind != "group" {
		t.Fatalf("postgres effective-state page = %+v", statePage.Entries)
	}
}

func sidebarTaskQuery(page int) models.SidebarTaskViewQuery {
	return models.SidebarTaskViewQuery{
		Sort:  models.SidebarTaskViewSort{Key: "updatedAt", Direction: "desc"},
		Group: "none", Page: page, PageSize: 100, Locale: "en",
	}
}

func sidebarTaskIDs(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
