package sqlite

import (
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestQuerySidebarTaskPageSelectsIntersectingRootTrees(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedSidebarPageWindowTrees(t, repo)
			query := sidebarTaskQuery(1)
			query.Sort = models.SidebarTaskViewSort{Key: "title", Direction: "asc"}
			query.PageSize = 3
			for index, want := range [][]string{
				{"a", "a1", "a2"}, {"a3", "a4", "b"}, {"b1", "b2", "b3"},
				{"b4", "c", "c1"}, {"c2", "c3", "c4"},
			} {
				query.Page = index + 1
				page, err := repo.QuerySidebarTaskPage(t.Context(), "page-windows", query, models.SidebarTaskViewPreferences{})
				require.NoError(t, err)
				require.Equal(t, want, sidebarTaskIDs(page.Tasks))
				require.Equal(t, 15, page.TotalVisibleTasks)
				require.Equal(t, index+1, page.Page)
			}
			query.Page = 2
			pinned, err := repo.QuerySidebarTaskPage(t.Context(), "page-windows", query,
				models.SidebarTaskViewPreferences{PinnedTaskIDs: []string{"c"}})
			require.NoError(t, err)
			require.Equal(t, []string{"c3", "c4", "a"}, sidebarTaskIDs(pinned.Tasks))
			query.CollapsedTaskIDs = []string{"b"}
			query.Page = 99
			collapsed, err := repo.QuerySidebarTaskPage(t.Context(), "page-windows", query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{"c3", "c4"}, sidebarTaskIDs(collapsed.Tasks))
			require.Equal(t, 11, collapsed.TotalVisibleTasks)
			require.Equal(t, 4, collapsed.Page)
		})
	}
}

func seedSidebarPageWindowTrees(t *testing.T, repo *Repository) {
	t.Helper()
	seedWorkspace(t, repo, "page-windows")
	for _, root := range []string{"a", "b", "c"} {
		for child := range 5 {
			id, parent := root, ""
			if child > 0 {
				id, parent = fmt.Sprintf("%s%d", root, child), root
			}
			require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
				ID: id, WorkspaceID: "page-windows", Title: id, ParentID: parent,
			}))
		}
	}
}
