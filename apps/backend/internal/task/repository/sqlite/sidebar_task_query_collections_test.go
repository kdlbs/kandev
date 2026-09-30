package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestQuerySidebarTaskPageCollapseListsPreserveMembership(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			const workspaceID = "sidebar-collapse-lists"
			const rootID = "root'\""
			seedWorkspace(t, repo, workspaceID)
			for _, task := range []*models.Task{
				{ID: rootID, WorkspaceID: workspaceID, Title: "Root", State: "TODO"},
				{ID: "child", WorkspaceID: workspaceID, ParentID: rootID, Title: "Child", State: "TODO"},
				{ID: "other", WorkspaceID: workspaceID, Title: "Other", State: "COMPLETED"},
			} {
				require.NoError(t, repo.CreateTask(t.Context(), task))
			}
			query := sidebarTaskQuery(99)
			query.Group = "state"
			query.CollapsedTaskIDs = []string{rootID, rootID, "missing'\""}
			query.CollapsedGroupKeys = []string{"COMPLETED", "COMPLETED", "missing'\""}
			page, err := repo.QuerySidebarTaskPage(t.Context(), workspaceID, query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{rootID}, sidebarTaskIDs(page.Tasks))
			require.Equal(t, 3, page.TotalTasks)
			require.Equal(t, 1, page.TotalVisibleTasks)
			require.Equal(t, 1, page.Page)
			groups := make(map[string]int)
			for _, entry := range page.Entries {
				if entry.Kind == "group" {
					groups[entry.GroupKey] = entry.MatchingCount
				}
			}
			require.Equal(t, map[string]int{"TODO": 1, "COMPLETED": 1}, groups)
		})
	}
}
