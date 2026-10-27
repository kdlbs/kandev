package sqlite

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSidebarTaskPageWorkspaceSearch(t *testing.T) {
	testSidebarTaskPageWorkspaceSearch(t, "sqlite")
}

func TestPostgresSidebarTaskPageWorkspaceSearch(t *testing.T) {
	testSidebarTaskPageWorkspaceSearch(t, "postgres")
}

func testSidebarTaskPageWorkspaceSearch(t *testing.T, backend string) {
	t.Helper()
	plainQuery := sidebarTaskQuery(1)
	searchedQuery := plainQuery
	searchedQuery.SearchQuery = "identity"
	require.NotEqual(t,
		sidebarTaskQueryKey("workspace", plainQuery, models.SidebarTaskViewPreferences{}),
		sidebarTaskQueryKey("workspace", searchedQuery, models.SidebarTaskViewPreferences{}),
		"request search must participate in the query digest",
	)
	const workspaceID = "sidebar-workspace-search"
	repo := newRepoForSidebarConformance(t, backend)
	seedWorkspace(t, repo, workspaceID)
	for _, repository := range []*models.Repository{
		{ID: "name-match", WorkspaceID: workspaceID, Name: "needle%_literal", LocalPath: "/repos/name"},
		{ID: "name-match-copy", WorkspaceID: workspaceID, Name: "needle%_literal", LocalPath: "/repos/copy"},
		{ID: "path-match", WorkspaceID: workspaceID, Name: "ordinary", LocalPath: "/repos/needle%_literal"},
	} {
		require.NoError(t, repo.CreateRepository(t.Context(), repository))
	}
	for _, task := range []*models.Task{
		{ID: "description-match", WorkspaceID: workspaceID, Title: "ordinary", Description: "prefix needle%_literal suffix"},
		{ID: "repository-name-match", WorkspaceID: workspaceID, Title: "ordinary"},
		{ID: "repository-path-match", WorkspaceID: workspaceID, Title: "ordinary"},
		{ID: "wildcard-near-match", WorkspaceID: workspaceID, Title: "needleXXliteral"},
		{ID: "backslash-match", WorkspaceID: workspaceID, Title: `back\_slash`},
		{ID: "backslash-near-match", WorkspaceID: workspaceID, Title: "backXslash"},
	} {
		require.NoError(t, repo.CreateTask(t.Context(), task))
	}
	for _, link := range []*models.TaskRepository{
		{ID: "name-link-one", TaskID: "repository-name-match", RepositoryID: "name-match"},
		{ID: "name-link-two", TaskID: "repository-name-match", RepositoryID: "name-match-copy"},
		{ID: "path-link", TaskID: "repository-path-match", RepositoryID: "path-match"},
	} {
		require.NoError(t, repo.CreateTaskRepository(t.Context(), link))
	}

	query := sidebarTaskQuery(1)
	query.PageSize = 2
	setSidebarWorkspaceSearch(t, &query, "needle%_literal")
	first, err := repo.QuerySidebarTaskPage(t.Context(), workspaceID, query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Equal(t, 3, first.TotalTasks)
	require.True(t, first.HasNext)

	query.Page = 2
	second, err := repo.QuerySidebarTaskPage(t.Context(), workspaceID, query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.False(t, second.HasNext)
	got := append(sidebarTaskIDs(first.Tasks), sidebarTaskIDs(second.Tasks)...)
	require.ElementsMatch(t, []string{"description-match", "repository-name-match", "repository-path-match"}, got)
	require.Len(t, got, 3, "matching two repositories must not duplicate a task")

	query.Page = 1
	query.PageSize = 100
	setSidebarWorkspaceSearch(t, &query, `back\_slash`)
	page, err := repo.QuerySidebarTaskPage(t.Context(), workspaceID, query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Equal(t, []string{"backslash-match"}, sidebarTaskIDs(page.Tasks))

	for index := range 105 {
		id := fmt.Sprintf("displaced-%03d", index)
		require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
			ID: id, WorkspaceID: workspaceID, Title: "more recent distractor",
		}))
	}
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
		ID: "displaced-match", WorkspaceID: workspaceID, Title: "older match", Description: "delayed literal result",
	}))
	_, err = repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?`), createdAt, createdAt, "displaced-match")
	require.NoError(t, err)
	query.Page = 1
	query.PageSize = 1
	setSidebarWorkspaceSearch(t, &query, "delayed literal")
	page, err = repo.QuerySidebarTaskPage(t.Context(), workspaceID, query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Equal(t, 1, page.TotalTasks)
	require.Equal(t, []string{"displaced-match"}, sidebarTaskIDs(page.Tasks), "search must filter the complete workspace before page selection")

	for index, row := range []struct {
		id      string
		updated time.Duration
	}{
		{id: "running-first", updated: time.Minute},
		{id: "red-second", updated: 2 * time.Minute},
		{id: "orange-third", updated: 3 * time.Minute},
		{id: "latest-activity", updated: 4 * time.Minute},
	} {
		require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
			ID: row.id, WorkspaceID: workspaceID, Title: "chain-sort needle",
			CreatedAt: createdAt, UpdatedAt: createdAt.Add(row.updated),
		}))
		_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`), createdAt.Add(row.updated), row.id)
		require.NoError(t, err)
		if index == 0 {
			seedSidebarSummaryJSON(t, repo, workspaceID, row.id, `{"has_running_session":true}`)
		}
	}
	prefs := sidebarColorPreferencesForTest()
	prefs.ColorSettings.ManualColors["red-second"] = strptr("red")
	prefs.ColorSettings.ManualColors["orange-third"] = strptr("orange")
	query.Sort = models.SidebarTaskViewSort{
		Key: "running", Direction: "desc",
		ThenBy: []models.SidebarTaskViewSortCriterion{
			{Key: "color", Color: "red", Direction: "desc"},
			{Key: "color", Color: "orange", Direction: "desc"},
			{Key: "lastActivityAt", Direction: "desc"},
		},
	}
	query.PageSize = 2
	setSidebarWorkspaceSearch(t, &query, "chain-sort")
	first, err = repo.QuerySidebarTaskPage(t.Context(), workspaceID, query, prefs)
	require.NoError(t, err)
	require.Equal(t, 4, first.TotalTasks)
	require.Equal(t, []string{"running-first", "red-second"}, sidebarTaskIDs(first.Tasks))
	query.Page = 2
	second, err = repo.QuerySidebarTaskPage(t.Context(), workspaceID, query, prefs)
	require.NoError(t, err)
	require.Equal(t, []string{"orange-third", "latest-activity"}, sidebarTaskIDs(second.Tasks))
}

func setSidebarWorkspaceSearch(t *testing.T, query *models.SidebarTaskViewQuery, value string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"query": value})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(payload, query))
}
