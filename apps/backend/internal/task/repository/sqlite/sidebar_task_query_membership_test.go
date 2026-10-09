package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
// @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.14
func TestQuerySidebarTaskPageLargeMembership(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			ctx := context.Background()
			seedWorkspace(t, repo, "membership")
			values := make([]string, 1000)
			for i := range values {
				values[i] = fmt.Sprintf("organization-long-repository-%04d", i)
			}
			for i := range 102 {
				id := fmt.Sprintf("member-%03d", i)
				require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "membership", Title: id}))
				name := values[i]
				if i == 101 {
					name = "excluded"
				}
				seedRunnerSwitchRepository(t, repo, name, "membership")
				seedRunnerSwitchTaskRepository(t, repo, id, name)
			}
			raw, err := json.Marshal(values)
			require.NoError(t, err)
			q := sidebarTaskQuery(1)
			q.Filters = []models.SidebarTaskViewClause{{Dimension: "repository", Op: "in", Value: raw}}
			page, err := repo.QuerySidebarTaskPage(ctx, "membership", q, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, 101, page.TotalTasks)
			require.Len(t, page.Tasks, 100)
			q.Page = 2
			page, err = repo.QuerySidebarTaskPage(ctx, "membership", q, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Len(t, page.Tasks, 1)
			q.Page = 1
			q.Filters[0].Op = "not_in"
			page, err = repo.QuerySidebarTaskPage(ctx, "membership", q, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{"member-101"}, sidebarTaskIDs(page.Tasks))
			// Maximum combined binding count, with short values inside the HTTP body budget.
			for i := range values {
				values[i] = fmt.Sprintf("x%d", i)
			}
			raw, err = json.Marshal(values)
			require.NoError(t, err)
			q.Filters = make([]models.SidebarTaskViewClause, 20)
			for i := range q.Filters {
				q.Filters[i] = models.SidebarTaskViewClause{Dimension: "repository", Op: "not_in", Value: raw}
			}
			page, err = repo.QuerySidebarTaskPage(ctx, "membership", q, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, 102, page.TotalTasks)
		})
	}
}
