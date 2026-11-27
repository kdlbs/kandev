package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const literalSearchWorkspace = "literal-search"

// @covers AC-TASKS-WORKSPACE-SEARCH-001.3
func TestWorkspaceTaskSearchLiteralPagination(t *testing.T) {
	testLiteralSearchPagination(t, newLiteralSearchSQLite)
}

func newLiteralSearchSQLite(t *testing.T) *Repository {
	t.Helper()
	return newRepoForSidebarConformance(t, "sqlite")
}

func testLiteralSearchPagination(t *testing.T, open func(*testing.T) *Repository) {
	t.Helper()
	t.Run("displaced_match", func(t *testing.T) { testLiteralSearchDisplacedMatch(t, open(t)) })
	t.Run("pages_and_deduplication", func(t *testing.T) { testLiteralSearchPages(t, open(t)) })
}

func testLiteralSearchDisplacedMatch(t *testing.T, repo *Repository) {
	t.Helper()
	seedWorkspace(t, repo, literalSearchWorkspace)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	createLiteralSearchTask(t, repo, &models.Task{ID: "literal", Title: "deploy_production deploy%production uniqueneedle"}, base)
	for i := range 21 {
		createLiteralSearchTask(t, repo, &models.Task{
			ID: fmt.Sprintf("distractor-%02d", i), Title: "deployXproduction",
		}, base.Add(time.Duration(i+1)*time.Hour))
	}
	for _, query := range []string{"deploy_", "deploy%"} {
		t.Run(query, func(t *testing.T) {
			rows, total, err := repo.ListTasksByWorkspace(t.Context(), literalSearchWorkspace, "", "", query, 1, 20, "", false, false, false, false)
			require.NoError(t, err)
			assert.Equal(t, 1, total, "literal query %q", query)
			assert.Equal(t, []string{"literal"}, literalSearchIDs(rows))
		})
	}
	rows, total, err := repo.ListTasksByWorkspace(t.Context(), literalSearchWorkspace, "", "", "production", 1, 20, "", false, false, false, false)
	require.NoError(t, err)
	require.Equal(t, 22, total)
	require.Len(t, rows, 20)
	require.Equal(t, "distractor-20", rows[0].ID)
	assertLiteralSearch(t, repo, literalSearchQuery{query: "uniqueneedle"}, 1, "literal")
}

func createLiteralSearchTask(t *testing.T, repo *Repository, task *models.Task, createdAt time.Time) {
	t.Helper()
	if task.WorkspaceID == "" {
		task.WorkspaceID = literalSearchWorkspace
	}
	require.NoError(t, repo.CreateTask(t.Context(), task))
	setLiteralSearchDates(t, repo, task.ID, createdAt)
}

func setLiteralSearchDates(t *testing.T, repo *Repository, taskID string, at time.Time) {
	t.Helper()
	_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?`), at, at, taskID)
	require.NoError(t, err)
}

func literalSearchIDs(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

// @covers AC-TASKS-WORKSPACE-SEARCH-001.1
// @covers AC-TASKS-WORKSPACE-SEARCH-001.2
func TestWorkspaceTaskSearchLiteralFields(t *testing.T) {
	testLiteralSearchFields(t, newLiteralSearchSQLite)
}

func testLiteralSearchFields(t *testing.T, open func(*testing.T) *Repository) {
	t.Helper()
	for _, field := range []string{"title", "description", "repository_name", "repository_path"} {
		t.Run(field, func(t *testing.T) {
			for _, tc := range []struct{ name, query, match, distractor string }{
				{"percent", "pErCeNt%", "Percent%value", "PercentXvalue"},
				{"underscore", "Under_score", "Under_score", "UnderXscore"},
				{"backslash", `Back\slash`, `Back\slash`, "Backslash"},
				{"adjacent", `Combo\%_`, `Combo\%_value`, "ComboXvalue"},
				{"repeated_backslash", `Repeat\\`, `Repeat\\value`, `Repeat\value`},
				{"trailing_backslash", `Trail\`, `Trail\`, "Trail"},
				{"percent_only", "%", "100% complete", "100 complete"},
				{"underscore_only", "_", "snake_case", "snakeXcase"},
				{"backslash_only", `\`, `path\file`, "pathfile"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					repo := open(t)
					seedWorkspace(t, repo, literalSearchWorkspace)
					createLiteralSearchField(t, repo, field, "match", tc.match)
					createLiteralSearchField(t, repo, field, "distractor", tc.distractor)
					assertLiteralSearch(t, repo, literalSearchQuery{query: tc.query}, 1, "match")
				})
			}
		})
	}
}

func createLiteralSearchField(t *testing.T, repo *Repository, field, id, text string) {
	t.Helper()
	task := &models.Task{ID: id, Title: "ordinary task"}
	switch field {
	case "title":
		task.Title = "before " + text + " after"
	case "description":
		task.Description = "before " + text + " after"
	}
	createLiteralSearchTask(t, repo, task, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if field == "repository_name" || field == "repository_path" {
		repository := &models.Repository{ID: "repo-" + id, WorkspaceID: literalSearchWorkspace, Name: "ordinary", LocalPath: "/ordinary"}
		if field == "repository_name" {
			repository.Name = "before " + text + " after"
		} else {
			repository.LocalPath = "/before/" + text + "/after"
		}
		require.NoError(t, repo.CreateRepository(t.Context(), repository))
		linkLiteralSearchRepository(t, repo, id, repository.ID)
	}
}

func linkLiteralSearchRepository(t *testing.T, repo *Repository, taskID, repositoryID string) {
	t.Helper()
	require.NoError(t, repo.CreateTaskRepository(t.Context(), &models.TaskRepository{
		ID: taskID + "-" + repositoryID, TaskID: taskID, RepositoryID: repositoryID,
	}))
}

type literalSearchQuery struct {
	query, workspace, workflow, repository, sort   string
	page, pageSize                                 int
	includeArchived, onlyArchived                  bool
	includeEphemeral, onlyEphemeral, excludeConfig bool
}

func assertLiteralSearch(t *testing.T, repo *Repository, q literalSearchQuery, wantTotal int, wantIDs ...string) {
	t.Helper()
	if q.workspace == "" {
		q.workspace = literalSearchWorkspace
	}
	if q.page == 0 {
		q.page = 1
	}
	if q.pageSize == 0 {
		q.pageSize = 20
	}
	rows, total, err := repo.ListTasksByWorkspaceWithArchiveMode(t.Context(), q.workspace, q.workflow, q.repository,
		q.query, q.page, q.pageSize, q.sort, q.includeArchived, q.includeEphemeral, q.onlyEphemeral, q.excludeConfig, q.onlyArchived)
	require.NoError(t, err)
	assert.Equal(t, wantTotal, total)
	assert.Equal(t, append([]string{}, wantIDs...), literalSearchIDs(rows))
}

func testLiteralSearchPages(t *testing.T, repo *Repository) {
	t.Helper()
	seedWorkspace(t, repo, literalSearchWorkspace)
	for _, id := range []string{"one", "two"} {
		require.NoError(t, repo.CreateRepository(t.Context(), &models.Repository{
			ID: id, WorkspaceID: literalSearchWorkspace, Name: "shared%repository", LocalPath: "/shared%path",
		}))
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 3 {
		id := fmt.Sprintf("page-%d", i)
		createLiteralSearchTask(t, repo, &models.Task{ID: id, Title: "shared%task"}, base.Add(time.Duration(i)*time.Hour))
		linkLiteralSearchRepository(t, repo, id, "one")
		linkLiteralSearchRepository(t, repo, id, "two")
	}
	for _, query := range []string{"shared%", "%repository", "%path"} {
		t.Run(query, func(t *testing.T) {
			assertLiteralSearch(t, repo, literalSearchQuery{query: query, pageSize: 2}, 3, "page-2", "page-1")
			assertLiteralSearch(t, repo, literalSearchQuery{query: query, pageSize: 2, page: 2}, 3, "page-0")
			assertLiteralSearch(t, repo, literalSearchQuery{query: query, pageSize: 2, page: 3}, 3)
		})
	}
	assertLiteralSearch(t, repo, literalSearchQuery{query: "shared%", sort: usermodels.TasksListSortCreatedAsc}, 3, "page-0", "page-1", "page-2")
}

// @covers AC-TASKS-WORKSPACE-SEARCH-001.2
// @covers AC-TASKS-WORKSPACE-SEARCH-001.4
func TestWorkspaceTaskSearchLiteralFiltersAndControls(t *testing.T) {
	testLiteralSearchFilters(t, newLiteralSearchSQLite(t))
}

func seedLiteralSearchFilters(t *testing.T, repo *Repository) {
	t.Helper()
	seedWorkspace(t, repo, literalSearchWorkspace)
	seedWorkspace(t, repo, "other-workspace")
	for _, id := range []string{"first-workflow", "other-workflow"} {
		require.NoError(t, repo.CreateWorkflow(t.Context(), &models.Workflow{ID: id, WorkspaceID: literalSearchWorkspace, Name: id}))
	}
	for _, id := range []string{"first-repo", "other-repo"} {
		require.NoError(t, repo.CreateRepository(t.Context(), &models.Repository{ID: id, WorkspaceID: literalSearchWorkspace, Name: id}))
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, task := range []*models.Task{
		{ID: "active"},
		{ID: "other-flow", WorkflowID: "other-workflow"},
		{ID: "other-repository"},
		{ID: "archived"},
		{ID: "ephemeral", IsEphemeral: true},
		{ID: "config", Metadata: map[string]interface{}{"config_mode": true}},
		{ID: "automation", Origin: models.TaskOriginAutomationRun, IsEphemeral: true},
		{ID: "foreign", WorkspaceID: "other-workspace"},
		{ID: "wildcard", Title: "deployXproduction"},
	} {
		if task.Title == "" {
			task.Title = "deploy_production " + task.ID
		}
		if task.WorkspaceID == "" && task.WorkflowID == "" {
			task.WorkflowID = "first-workflow"
		}
		at := base.Add(time.Duration(i) * time.Hour)
		createLiteralSearchTask(t, repo, task, at)
		if task.ID == "archived" {
			require.NoError(t, repo.ArchiveTask(t.Context(), task.ID))
			setLiteralSearchDates(t, repo, task.ID, at)
		}
		if task.ID != "foreign" {
			repository := "first-repo"
			if task.ID == "other-repository" {
				repository = "other-repo"
			}
			linkLiteralSearchRepository(t, repo, task.ID, repository)
		}
	}
}

func testLiteralSearchFilters(t *testing.T, repo *Repository) {
	t.Helper()
	seedLiteralSearchFilters(t, repo)
	for _, tc := range []struct {
		name string
		q    literalSearchQuery
		ids  []string
	}{
		{"default", literalSearchQuery{}, []string{"config", "other-repository", "other-flow", "active"}},
		{"workflow", literalSearchQuery{workflow: "first-workflow"}, []string{"config", "other-repository", "active"}},
		{"repository", literalSearchQuery{repository: "first-repo"}, []string{"config", "other-flow", "active"}},
		{"combined", literalSearchQuery{workflow: "first-workflow", repository: "first-repo", excludeConfig: true}, []string{"active"}},
		{"archive_included", literalSearchQuery{includeArchived: true}, []string{"config", "archived", "other-repository", "other-flow", "active"}},
		{"archive_only", literalSearchQuery{onlyArchived: true}, []string{"archived"}},
		{"archive_precedence", literalSearchQuery{onlyArchived: true, includeArchived: true}, []string{"archived"}},
		{"ephemeral_included", literalSearchQuery{includeEphemeral: true}, []string{"config", "ephemeral", "other-repository", "other-flow", "active"}},
		{"ephemeral_only", literalSearchQuery{onlyEphemeral: true}, []string{"ephemeral"}},
		{"ephemeral_precedence", literalSearchQuery{onlyEphemeral: true, includeEphemeral: true}, []string{"ephemeral"}},
		{"config_excluded", literalSearchQuery{excludeConfig: true}, []string{"other-repository", "other-flow", "active"}},
		{"other_workspace", literalSearchQuery{workspace: "other-workspace"}, []string{"foreign"}},
		{"title_sort", literalSearchQuery{sort: usermodels.TasksListSortTitleAsc}, []string{"active", "config", "other-flow", "other-repository"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.q.query = "deploy_"
			assertLiteralSearch(t, repo, tc.q, len(tc.ids), tc.ids...)
			tc.q.query = ""
			if tc.q.workspace == "other-workspace" || tc.q.onlyArchived || tc.q.onlyEphemeral {
				assertLiteralSearch(t, repo, tc.q, len(tc.ids), tc.ids...)
			} else {
				rows, total, err := repo.ListTasksByWorkspaceWithArchiveMode(t.Context(), literalSearchWorkspace,
					tc.q.workflow, tc.q.repository, "", 1, 20, tc.q.sort, tc.q.includeArchived, tc.q.includeEphemeral, false, tc.q.excludeConfig, false)
				require.NoError(t, err)
				require.Equal(t, len(tc.ids)+1, total)
				require.Len(t, rows, total)
				require.Contains(t, literalSearchIDs(rows), "wildcard")
			}
		})
	}
	assertLiteralSearch(t, repo, literalSearchQuery{query: " production"}, 0)
	assertLiteralSearch(t, repo, literalSearchQuery{query: "production", repository: "other-repo"}, 1, "other-repository")
}
