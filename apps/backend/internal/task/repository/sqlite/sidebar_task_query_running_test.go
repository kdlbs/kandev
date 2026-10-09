package sqlite

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSidebarTaskWideRunningRank(t *testing.T) {
	// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.14, AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.16
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			const workspace = "sidebar-running-rank"
			seedWorkspace(t, repo, workspace)
			base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			for _, task := range []struct {
				id      string
				updated time.Duration
			}{
				{id: "secondary-running", updated: time.Minute},
				{id: "idle-red", updated: 10 * time.Minute},
			} {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: task.id, WorkspaceID: workspace, Title: task.id,
					State: "TODO", CreatedAt: base, UpdatedAt: base.Add(task.updated),
				}))
			}
			seedSidebarLegacyPrimarySummary(t, repo, workspace, "secondary-running", "")
			seedSidebarLegacyPrimarySummary(t, repo, workspace, "idle-red", "WAITING_FOR_INPUT")

			require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{
				ID: "running-executor", Name: "running-executor", Type: models.ExecutorTypeLocal,
			}))
			require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{
				ID: "secondary-session", TaskID: "secondary-running", ExecutorID: "running-executor",
				State: models.TaskSessionStateRunning, IsPrimary: false,
			}))

			prefs := sidebarColorPreferencesForTest()
			prefs.ColorSettings.ManualColors["idle-red"] = strptr("red")
			query := sidebarTaskQuery(1)
			query.PageSize = 1
			query.Sort = models.SidebarTaskViewSort{
				Key: "running", Direction: "desc",
				ThenBy: []models.SidebarTaskViewSortCriterion{
					{Key: "color", Color: "red", Direction: "desc"},
					{Key: "lastActivityAt", Direction: "desc"},
				},
			}
			page, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, prefs)
			require.NoError(t, err)
			require.Equal(t, []string{"secondary-running"}, sidebarTaskIDs(page.Tasks),
				"a secondary RUNNING session must rank before preferred color and activity on page one")
			require.True(t, page.HasNext)
		})
	}
}

func TestSidebarRunningRankLegacyFallbackAndExplicitFalse(t *testing.T) {
	// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.14, AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.16
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			const workspace = "sidebar-running-fallback"
			seedWorkspace(t, repo, workspace)
			base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			cases := []struct {
				id      string
				summary string
			}{
				{id: "legacy-missing", summary: `{}`},
				{id: "legacy-null", summary: `{"has_running_session":null}`},
				{id: "legacy-string", summary: `{"has_running_session":"true"}`},
				{id: "legacy-number", summary: `{"has_running_session":1}`},
				{id: "explicit-false", summary: `{"has_running_session":false}`},
			}
			for index, testCase := range cases {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: testCase.id, WorkspaceID: workspace, Title: testCase.id,
					State: "TODO", CreatedAt: base, UpdatedAt: base.Add(time.Duration(index) * time.Minute),
				}))
				seedSidebarSummaryJSON(t, repo, workspace, testCase.id, testCase.summary)
				executorID := testCase.id + "-executor"
				require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{
					ID: executorID, Name: executorID, Type: models.ExecutorTypeLocal,
				}))
				require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{
					ID: testCase.id + "-session", TaskID: testCase.id, ExecutorID: executorID,
					State: models.TaskSessionStateRunning,
				}))
			}

			query := sidebarTaskQuery(1)
			query.Sort = models.SidebarTaskViewSort{
				Key: "running", Direction: "desc",
				ThenBy: []models.SidebarTaskViewSortCriterion{{Key: "title", Direction: "asc"}},
			}
			page, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, sidebarColorPreferencesForTest())
			require.NoError(t, err)
			require.Equal(t, []string{
				"legacy-missing", "legacy-null", "legacy-number", "legacy-string", "explicit-false",
			}, sidebarTaskIDs(page.Tasks), "invalid or missing flags use session rows, while false remains authoritative")
		})
	}
}

func TestSidebarRunningFirstSortPromotesRunningDescendantsAtEveryTreeLevel(t *testing.T) {
	// @covers AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.14, AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.16
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			const workspace = "sidebar-running-subtree"
			seedWorkspace(t, repo, workspace)
			base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			for _, task := range []struct {
				id, parent string
				activity   time.Duration
				running    bool
			}{
				{id: "running-tree", activity: time.Minute},
				{id: "nested-parent", parent: "running-tree", activity: 2 * time.Minute},
				{id: "running-grandchild", parent: "nested-parent", activity: 3 * time.Minute, running: true},
				{id: "idle-child", parent: "running-tree", activity: 30 * time.Minute},
				{id: "idle-tree", activity: 40 * time.Minute},
			} {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: task.id, WorkspaceID: workspace, Title: task.id, ParentID: task.parent,
					State: "TODO", CreatedAt: base, UpdatedAt: base.Add(task.activity),
				}))
				summary, err := json.Marshal(map[string]any{
					"last_activity_at":    base.Add(task.activity).Format(time.RFC3339Nano),
					"has_running_session": task.running,
				})
				require.NoError(t, err)
				_, err = repo.db.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO task_status_summaries
					(task_id, workspace_id, revision, summary, updated_at) VALUES (?, ?, 1, ?, ?)`),
					task.id, workspace, string(summary), base)
				require.NoError(t, err)
			}

			query := sidebarTaskQuery(1)
			query.Group = "none"
			query.PageSize = 20
			query.Sort = models.SidebarTaskViewSort{Key: "runningFirstActivity", Direction: "desc"}
			page, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, sidebarColorPreferencesForTest())
			require.NoError(t, err)
			require.Equal(t, []string{
				"running-tree", "nested-parent", "running-grandchild", "idle-child", "idle-tree",
			}, sidebarTaskIDs(page.Tasks), "a running descendant promotes each ancestor before activity")
		})
	}
}

func seedSidebarLegacyPrimarySummary(t *testing.T, repo *Repository, workspace, taskID, primaryState string) {
	t.Helper()
	summary := map[string]any{}
	if primaryState != "" {
		summary["primary_session"] = map[string]string{"state": primaryState}
	}
	encoded, err := json.Marshal(summary)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO task_status_summaries
		(task_id, workspace_id, revision, summary, updated_at) VALUES (?, ?, 1, ?, ?)`),
		taskID, workspace, string(encoded), time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
}

func seedSidebarSummaryJSON(t *testing.T, repo *Repository, workspace, taskID, summary string) {
	t.Helper()
	_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO task_status_summaries
		(task_id, workspace_id, revision, summary, updated_at) VALUES (?, ?, 1, ?, ?)`),
		taskID, workspace, summary, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
}
