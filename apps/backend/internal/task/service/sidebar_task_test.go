package service

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
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
