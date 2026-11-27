package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kdlbs/kandev/pluginsdk"
)

func TestQuerySidebarTaskPageAuthorizesWorkspaceBeforeQuery(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	query := models.SidebarTaskViewQuery{
		Sort:  models.SidebarTaskViewSort{Key: "updatedAt", Direction: "desc"},
		Group: "none", Page: 1, PageSize: 100, Locale: "en",
	}

	if _, err := svc.QuerySidebarTaskPage(ctxAs("user-a"), "ws-b", query, models.SidebarTaskViewPreferences{}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("query foreign workspace: %v", err)
	}
	page, err := svc.QuerySidebarTaskPage(ctxAs("user-b"), "ws-b", query, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatalf("query own workspace: %v", err)
	}
	if page.TotalTasks != 1 || len(page.Tasks) != 1 || page.Tasks[0].ID != "task-b" {
		t.Fatalf("authorized page = total %d tasks %v, want task-b", page.TotalTasks, sidebarTaskIDsForService(page.Tasks))
	}
}

func sidebarTaskIDsForService(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

type testSidebarGroupingResolver struct{ calls int }

func (r *testSidebarGroupingResolver) ResolveSidebarGrouping(_ context.Context, _ string, reference string) (*models.SidebarPluginGrouping, error) {
	r.calls++
	return &models.SidebarPluginGrouping{Reference: reference, Generation: "g1", Snapshot: &pluginsdk.SidebarGroupingSnapshot{Version: 1, Complete: true, Revision: "r1", Groups: []pluginsdk.SidebarGroupingValue{{ID: "g", Label: "Classified"}}, Memberships: []pluginsdk.SidebarGroupingMembership{{TaskID: "task-b", GroupID: "g"}}}}, nil
}

// @covers AC-UI-SIDEBAR-PLUGIN-GROUPING-001.3
func TestSidebarPluginGroupingServiceResolvesAfterAuthorization(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	resolver := &testSidebarGroupingResolver{}
	svc.sidebarGroupingResolver = resolver
	q := models.SidebarTaskViewQuery{Sort: models.SidebarTaskViewSort{Key: "title", Direction: "asc"}, Group: "plugin:acme:labels", Page: 1, PageSize: 100, Locale: "en"}
	if _, err := svc.QuerySidebarTaskPage(ctxAs("user-a"), "ws-b", q, models.SidebarTaskViewPreferences{}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("foreign query: %v", err)
	}
	if resolver.calls != 0 {
		t.Fatal("foreign workspace reached plugin")
	}
	result, err := svc.QuerySidebarTaskPage(ctxAs("user-b"), "ws-b", q, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 || result.Entries[0].GroupLabel != "Classified" || result.GroupingRevision != "r1" {
		t.Fatalf("query did not use complete resolved snapshot: %+v (calls %d)", result, resolver.calls)
	}
	for i := range 101 {
		if err := repo.CreateTask(t.Context(), &models.Task{ID: fmt.Sprintf("filler-%d", i), WorkspaceID: "ws-b", Title: "Filler"}); err != nil {
			t.Fatal(err)
		}
	}
	q.Page = 2
	q.GroupingRevision = "old"
	q.GroupingGeneration = "g1"
	restarted, err := svc.QuerySidebarTaskPage(ctxAs("user-b"), "ws-b", q, models.SidebarTaskViewPreferences{})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Page != 1 {
		t.Fatalf("changed classification continued a mixed snapshot on page %d", restarted.Page)
	}
}
