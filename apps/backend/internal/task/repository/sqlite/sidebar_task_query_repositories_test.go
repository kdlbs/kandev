package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSidebarRepositoryProjectionDeduplicatesBranchesAndKeepsPrimaryFilter(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedWorkspace(t, repo, "repository-projection")
			for _, id := range []string{"Alpha", "Beta"} {
				require.NoError(t, repo.CreateRepository(t.Context(), &models.Repository{
					ID: id, WorkspaceID: "repository-projection", Name: id, LocalPath: "/fixture/" + id,
				}))
			}
			for _, id := range []string{"multi", "single", "unlinked"} {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: id, WorkspaceID: "repository-projection", Title: id,
				}))
			}
			for _, link := range []*models.TaskRepository{
				{ID: "a", TaskID: "multi", RepositoryID: "Alpha", CheckoutBranch: "later", Position: 2},
				{ID: "z", TaskID: "multi", RepositoryID: "Alpha", CheckoutBranch: "first", Position: 0},
				{ID: "b", TaskID: "multi", RepositoryID: "Beta", Position: 0},
				{ID: "s1", TaskID: "single", RepositoryID: "Alpha", CheckoutBranch: "first"},
				{ID: "s2", TaskID: "single", RepositoryID: "Alpha", CheckoutBranch: "later", Position: 1},
			} {
				require.NoError(t, repo.CreateTaskRepository(t.Context(), link))
			}
			query := sidebarTaskQuery(1)
			query.Group = "repository"
			page, err := repo.QuerySidebarTaskPage(t.Context(), "repository-projection", query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			groups := map[string]string{}
			for _, entry := range page.Entries {
				if entry.TaskID != "" {
					groups[entry.TaskID] = entry.GroupKey
				}
			}
			require.Equal(t, `__repo_combination__:["Alpha","Beta"]`, groups["multi"])
			require.Equal(t, "Alpha", groups["single"])
			require.Equal(t, "Alpha", groups["unlinked"])
			for _, group := range []string{"none", "repository"} {
				query.Group = group
				query.Filters = []models.SidebarTaskViewClause{{Dimension: "repository", Op: "is", Value: json.RawMessage(`"Beta"`)}}
				page, err = repo.QuerySidebarTaskPage(t.Context(), "repository-projection", query, models.SidebarTaskViewPreferences{})
				require.NoError(t, err)
				require.Equal(t, []string{"multi"}, sidebarTaskIDs(page.Tasks))
				query.Filters[0].Value = json.RawMessage(`"Alpha"`)
				page, err = repo.QuerySidebarTaskPage(t.Context(), "repository-projection", query, models.SidebarTaskViewPreferences{})
				require.NoError(t, err)
				require.Equal(t, []string{"single"}, sidebarTaskIDs(page.Tasks))
			}
		})
	}
}

func TestSidebarRepositoryProjectionFollowsFilteredRoots(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedWorkspace(t, repo, "repository-roots")
			for index, id := range []string{"parent", "child"} {
				require.NoError(t, repo.CreateRepository(t.Context(), &models.Repository{
					ID: id, WorkspaceID: "repository-roots", Name: id, LocalPath: "/fixture/" + id,
				}))
				parent := ""
				if index > 0 {
					parent = "parent"
				}
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: id, WorkspaceID: "repository-roots", Title: id, ParentID: parent,
				}))
				require.NoError(t, repo.CreateTaskRepository(t.Context(), &models.TaskRepository{
					ID: id, TaskID: id, RepositoryID: id,
				}))
			}
			query := sidebarTaskQuery(1)
			query.Group = "repository"
			page, err := repo.QuerySidebarTaskPage(t.Context(), "repository-roots", query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{"parent", "child"}, sidebarTaskIDs(page.Tasks))
			require.Equal(t, "parent", page.Entries[0].GroupKey)
			query.Filters = []models.SidebarTaskViewClause{{Dimension: "titleMatch", Op: "matches", Value: json.RawMessage(`"child"`)}}
			page, err = repo.QuerySidebarTaskPage(t.Context(), "repository-roots", query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{"child"}, sidebarTaskIDs(page.Tasks))
			require.Equal(t, "child", page.Entries[0].GroupKey)
		})
	}
}
