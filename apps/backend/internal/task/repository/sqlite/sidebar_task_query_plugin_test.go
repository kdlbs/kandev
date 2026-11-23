package sqlite

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
func TestSidebarPluginGroupingBeforePagination(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedWorkspace(t, repo, "plugin-ws")
			seedWorkspace(t, repo, "foreign-ws")
			at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
			snap := &pluginsdk.SidebarGroupingSnapshot{Version: 1, Revision: "r1", Complete: true, Groups: []pluginsdk.SidebarGroupingValue{{ID: "late", Label: "Alpha", Color: "#aabbcc"}, {ID: "bulk", Label: "Zulu"}}, Memberships: []pluginsdk.SidebarGroupingMembership{}}
			for i := range 101 {
				id := fmt.Sprintf("root-%03d", i)
				if err := repo.CreateTask(t.Context(), &models.Task{ID: id, WorkspaceID: "plugin-ws", Title: fmt.Sprintf("Task %03d", i), CreatedAt: at, UpdatedAt: at}); err != nil {
					t.Fatal(err)
				}
				snap.Memberships = append(snap.Memberships, pluginsdk.SidebarGroupingMembership{TaskID: id, GroupID: "bulk"})
			}
			for _, task := range []*models.Task{{ID: "child", WorkspaceID: "plugin-ws", ParentID: "root-100", Title: "Child", CreatedAt: at, UpdatedAt: at}, {ID: "foreign", WorkspaceID: "foreign-ws", Title: "Foreign", CreatedAt: at, UpdatedAt: at}} {
				if err := repo.CreateTask(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			snap.Memberships = append(snap.Memberships, pluginsdk.SidebarGroupingMembership{TaskID: "root-100", GroupID: "late"}, pluginsdk.SidebarGroupingMembership{TaskID: "foreign", GroupID: "late"}, pluginsdk.SidebarGroupingMembership{TaskID: "deleted", GroupID: "late"})
			prefs := models.SidebarTaskViewPreferences{PinnedTaskIDs: []string{"root-090"}, PluginGrouping: &models.SidebarPluginGrouping{Reference: "plugin:acme:labels", Generation: "g1", Snapshot: snap}}
			q := sidebarTaskQuery(1)
			q.Group = "plugin:acme:labels"
			q.Sort = models.SidebarTaskViewSort{Key: "title", Direction: "asc"}
			page, err := repo.QuerySidebarTaskPage(t.Context(), "plugin-ws", q, prefs)
			if err != nil {
				t.Fatal(err)
			}
			if page.TotalTasks != 102 || page.TotalVisibleTasks != 104 || len(page.Tasks) != 100 || !page.HasNext {
				t.Fatalf("wrong pre-pagination totals: unique=%d occurrences=%d hydrated=%d next=%v", page.TotalTasks, page.TotalVisibleTasks, len(page.Tasks), page.HasNext)
			}
			if len(page.Entries) < 5 || page.Entries[0].GroupLabel != "Alpha" || page.Entries[0].GroupColor != "#aabbcc" || page.Entries[1].TaskID != "root-100" || page.Entries[2].TaskID != "child" || page.Entries[4].TaskID != "root-090" {
				t.Fatalf("later-source membership/tree/pin ordering lost: %+v", page.Entries[:min(5, len(page.Entries))])
			}
			q.Page = 2
			second, err := repo.QuerySidebarTaskPage(t.Context(), "plugin-ws", q, prefs)
			if err != nil {
				t.Fatal(err)
			}
			if second.TotalVisibleTasks != 104 || len(second.Tasks) != 4 || !second.Entries[0].Continuation || second.HasNext {
				t.Fatalf("second occurrence page: %+v", second)
			}
			q.CollapsedGroupKeys = []string{models.SidebarPluginValueKey(q.Group, "late")}
			collapsed, err := repo.QuerySidebarTaskPage(t.Context(), "plugin-ws", q, prefs)
			if err != nil {
				t.Fatal(err)
			}
			if collapsed.TotalTasks != 102 || collapsed.TotalVisibleTasks != 102 {
				t.Fatalf("collapse changed unique membership or ignored occurrence visibility: %+v", collapsed)
			}
		})
	}
}

func TestSidebarPluginGroupingRequiresVerifiedSnapshot(t *testing.T) {
	repo := newRepoForEntityTests(t)
	seedWorkspace(t, repo, "plugin-ws")
	q := sidebarTaskQuery(1)
	q.Group = "plugin:acme:labels"
	if _, err := repo.QuerySidebarTaskPage(t.Context(), "plugin-ws", q, models.SidebarTaskViewPreferences{}); err == nil {
		t.Fatal("missing provider silently became an untagged success")
	}
}

func TestSidebarPluginGroupingUniqueHydrationAndRevisionIdentity(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedWorkspace(t, repo, "ws")
			for _, task := range []*models.Task{{ID: "root", WorkspaceID: "ws", Title: "Root"}, {ID: "child", WorkspaceID: "ws", Title: "Child", ParentID: "root"}} {
				if err := repo.CreateTask(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			snap := &pluginsdk.SidebarGroupingSnapshot{Version: 1, Complete: true, Revision: "r1", Groups: []pluginsdk.SidebarGroupingValue{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Memberships: []pluginsdk.SidebarGroupingMembership{{TaskID: "root", GroupID: "a"}, {TaskID: "root", GroupID: "b"}}}
			prefs := models.SidebarTaskViewPreferences{PluginGrouping: &models.SidebarPluginGrouping{Reference: "plugin:acme:labels", Generation: "g1", Snapshot: snap}}
			q := sidebarTaskQuery(1)
			q.Group = "plugin:acme:labels"
			first, err := repo.QuerySidebarTaskPage(t.Context(), "ws", q, prefs)
			if err != nil {
				t.Fatal(err)
			}
			if first.TotalTasks != 2 || first.TotalVisibleTasks != 4 || len(first.Tasks) != 2 {
				t.Fatalf("domain task hydration duplicated display occurrences: unique=%d visible=%d hydrated=%d", first.TotalTasks, first.TotalVisibleTasks, len(first.Tasks))
			}
			for _, entry := range first.Entries {
				if entry.TaskID == "root" && entry.SubtaskCount != 1 {
					t.Fatalf("repeated tree doubled descendant counts: %+v", entry)
				}
			}
			snap.Revision = "r2"
			next, err := repo.QuerySidebarTaskPage(t.Context(), "ws", q, prefs)
			if err != nil {
				t.Fatal(err)
			}
			if first.QueryKey == next.QueryKey {
				t.Fatal("source revision missing from authoritative query identity")
			}
		})
	}
}

func TestSidebarPluginGroupingContinuationIsPerOccurrence(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedWorkspace(t, repo, "ws")
			for _, task := range []*models.Task{{ID: "root", WorkspaceID: "ws", Title: "Root"}, {ID: "child-1", WorkspaceID: "ws", Title: "First", ParentID: "root"}, {ID: "child-2", WorkspaceID: "ws", Title: "Second", ParentID: "root"}} {
				if err := repo.CreateTask(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			reference := "plugin:acme:labels"
			snap := &pluginsdk.SidebarGroupingSnapshot{Version: 1, Complete: true, Revision: "r1", Groups: []pluginsdk.SidebarGroupingValue{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}, Memberships: []pluginsdk.SidebarGroupingMembership{{TaskID: "root", GroupID: "a"}, {TaskID: "root", GroupID: "b"}}}
			prefs := models.SidebarTaskViewPreferences{PluginGrouping: &models.SidebarPluginGrouping{Reference: reference, Generation: "g1", Snapshot: snap}}
			q := sidebarTaskQuery(2)
			q.Group = reference
			q.PageSize = 2
			page, err := repo.QuerySidebarTaskPage(t.Context(), "ws", q, prefs)
			if err != nil {
				t.Fatal(err)
			}
			continuation := false
			for _, entry := range page.Entries {
				if entry.Kind == "continuation" && entry.GroupKey == models.SidebarPluginValueKey(reference, "a") && entry.ParentTitle == "Root" {
					continuation = true
				}
				if entry.TaskID == "child-2" && entry.GroupKey == models.SidebarPluginValueKey(reference, "a") && entry.ParentTitle != "Root" {
					t.Fatalf("continuation task lost parent context: %+v", entry)
				}
			}
			if !continuation {
				t.Fatalf("another occurrence suppressed continuation context: %+v", page.Entries)
			}
		})
	}
}

func TestSidebarPluginGroupingClassifiesFilteredRootsAndUnassigned(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedWorkspace(t, repo, "ws")
			for _, task := range []*models.Task{{ID: "parent", WorkspaceID: "ws", Title: "Parent"}, {ID: "child", WorkspaceID: "ws", ParentID: "parent", Title: "Child"}, {ID: "untagged", WorkspaceID: "ws", Title: "Unassigned root"}} {
				if err := repo.CreateTask(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			reference := "plugin:acme:labels"
			snap := &pluginsdk.SidebarGroupingSnapshot{Version: 1, Revision: "r1", Complete: true, Groups: []pluginsdk.SidebarGroupingValue{{ID: "parent", Label: "Parent group"}, {ID: "child", Label: "Child group"}}, Memberships: []pluginsdk.SidebarGroupingMembership{{TaskID: "parent", GroupID: "parent"}, {TaskID: "child", GroupID: "child"}}}
			prefs := models.SidebarTaskViewPreferences{PluginGrouping: &models.SidebarPluginGrouping{Reference: reference, Generation: "g1", Snapshot: snap}}
			query := sidebarTaskQuery(1)
			query.Group = reference
			page, err := repo.QuerySidebarTaskPage(t.Context(), "ws", query, prefs)
			if err != nil {
				t.Fatal(err)
			}
			groups := map[string]string{}
			for _, entry := range page.Entries {
				if entry.Kind == "task" {
					groups[entry.TaskID] = entry.GroupKey
				}
			}
			if page.TotalTasks != 3 || page.TotalVisibleTasks != 3 || groups["child"] != models.SidebarPluginValueKey(reference, "parent") || groups["untagged"] != reference+":unassigned" {
				t.Fatalf("root inheritance or unassigned membership lost: %+v", page)
			}
			query.Filters = []models.SidebarTaskViewClause{{Dimension: "titleMatch", Op: "matches", Value: json.RawMessage(`"Child"`)}}
			filtered, err := repo.QuerySidebarTaskPage(t.Context(), "ws", query, prefs)
			if err != nil {
				t.Fatal(err)
			}
			if filtered.TotalTasks != 1 || filtered.TotalVisibleTasks != 1 || len(filtered.Tasks) != 1 || filtered.Tasks[0].ID != "child" || filtered.Tasks[0].ParentID != "parent" {
				t.Fatalf("filtered root duplicated or canonical parentage changed: %+v", filtered)
			}
			var child models.SidebarTaskPageEntry
			for _, entry := range filtered.Entries {
				if entry.Kind == "task" {
					child = entry
				}
			}
			if child.TaskID != "child" || child.GroupKey != models.SidebarPluginValueKey(reference, "child") || child.Depth != 0 || child.ParentID != "" {
				t.Fatalf("promoted filtered root ignored its own value or display context: %+v", child)
			}
		})
	}
}

func TestSidebarPluginGroupingDoesNotUseAProviderLabelAsTheUnassignedSentinel(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedWorkspace(t, repo, "ws")
			for _, task := range []*models.Task{
				{ID: "tagged", WorkspaceID: "ws", Title: "Tagged"},
				{ID: "unassigned", WorkspaceID: "ws", Title: "Unassigned"},
			} {
				if err := repo.CreateTask(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			reference := "plugin:acme:labels"
			prefs := models.SidebarTaskViewPreferences{PluginGrouping: &models.SidebarPluginGrouping{
				Reference:  reference,
				Generation: "g1",
				Snapshot: &pluginsdk.SidebarGroupingSnapshot{
					Version: 1, Complete: true, Revision: "r1",
					Groups:      []pluginsdk.SidebarGroupingValue{{ID: "literal", Label: "__untagged__"}},
					Memberships: []pluginsdk.SidebarGroupingMembership{{TaskID: "tagged", GroupID: "literal"}},
				},
			}}
			query := sidebarTaskQuery(1)
			query.Group = reference
			page, err := repo.QuerySidebarTaskPage(t.Context(), "ws", query, prefs)
			if err != nil {
				t.Fatal(err)
			}
			labels := make(map[string]string)
			for _, entry := range page.Entries {
				if entry.Kind == "group" {
					labels[entry.GroupKey] = entry.GroupLabel
				}
			}
			if labels[models.SidebarPluginValueKey(reference, "literal")] != "__untagged__" {
				t.Fatalf("provider label changed: %+v", labels)
			}
			unassignedLabel, found := labels[reference+":unassigned"]
			if !found || unassignedLabel != "" {
				t.Fatalf("unassigned bucket reused a provider label: %+v", labels)
			}
		})
	}
}
