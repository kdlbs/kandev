package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestQuerySidebarTaskPageExcludesAgentProjectMembers(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := t.Context()
	const workspaceID = "ws-sidebar-agent-projects"
	seedWorkspace(t, repo, workspaceID)
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO agent_projects (id, workspace_id, name)
		VALUES (?, ?, ?)
	`, "project-1", workspaceID, "Project one")
	require.NoError(t, err)

	for _, task := range []*models.Task{
		{ID: "ordinary-root", WorkspaceID: workspaceID, Title: "A ordinary root"},
		{ID: "ordinary-child", WorkspaceID: workspaceID, Title: "B ordinary child", ParentID: "ordinary-root"},
		{ID: "ordinary-next-root", WorkspaceID: workspaceID, Title: "C ordinary next root"},
		{
			ID: "project-coordinator", WorkspaceID: workspaceID, Title: "Project coordinator",
			AgentProjectID: "project-1", AgentProjectTier: models.AgentProjectTierCoordinator,
			AgentProjectProfileID: "profile-coordinator", Origin: models.TaskOriginAgentProject,
		},
		{
			ID: "project-worker", WorkspaceID: workspaceID, Title: "Project worker", ParentID: "project-coordinator",
			AgentProjectID: "project-1", AgentProjectTier: models.AgentProjectTierEconomy,
			AgentProjectProfileID: "profile-economy", Origin: models.TaskOriginAgentProject,
		},
		{
			ID: "archived-project-worker", WorkspaceID: workspaceID, Title: "Archived project worker", ParentID: "project-coordinator",
			AgentProjectID: "project-1", AgentProjectTier: models.AgentProjectTierFrontier,
			AgentProjectProfileID: "profile-frontier", Origin: models.TaskOriginAgentProject,
		},
		{ID: "ordinary-archived", WorkspaceID: workspaceID, Title: "Ordinary archived"},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id IN (?, ?)`),
		"archived-project-worker", "ordinary-archived")
	require.NoError(t, err)

	query := sidebarTaskQuery(1)
	query.PageSize = 2
	query.Sort = models.SidebarTaskViewSort{Key: "title", Direction: "asc"}
	page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Equal(t, 3, page.TotalTasks)
	require.Equal(t, 3, page.TotalVisibleTasks)
	require.Equal(t, []string{"ordinary-root", "ordinary-child"}, sidebarTaskIDs(page.Tasks))
	require.True(t, page.HasNext)
	query.Page = 2
	page, err = repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Equal(t, 2, page.Page)
	require.Equal(t, 3, page.TotalTasks)
	require.Equal(t, []string{"ordinary-next-root"}, sidebarTaskIDs(page.Tasks))
	require.False(t, page.HasNext)

	query.Page = 1
	query.Filters = []models.SidebarTaskViewClause{{
		Dimension: "archived", Op: "is", Value: json.RawMessage(`true`),
	}}
	page, err = repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Equal(t, 1, page.TotalTasks)
	require.Equal(t, 1, page.TotalVisibleTasks)
	require.Equal(t, []string{"ordinary-archived"}, sidebarTaskIDs(page.Tasks))
}

func TestQuerySidebarTaskPageExcludesAgentProjectMembersFromWIPQueue(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := t.Context()
	const workspaceID = "ws-sidebar-agent-project-queue"
	seedWorkspace(t, repo, workspaceID)
	_, err := repo.db.ExecContext(ctx, `
		INSERT INTO agent_projects (id, workspace_id, name)
		VALUES (?, ?, ?)
	`, "project-queue", workspaceID, "Project queue")
	require.NoError(t, err)
	for _, task := range []*models.Task{
		{ID: "ordinary-queued", WorkspaceID: workspaceID, Title: "Ordinary queued"},
		{
			ID: "project-queued", WorkspaceID: workspaceID, Title: "Project queued",
			AgentProjectID: "project-queue", AgentProjectTier: models.AgentProjectTierCoordinator,
			AgentProjectProfileID: "profile-coordinator", Origin: models.TaskOriginAgentProject,
		},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	for _, queued := range []struct {
		taskID   string
		priority string
	}{
		{taskID: "ordinary-queued", priority: "low"},
		{taskID: "project-queued", priority: "critical"},
	} {
		_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			UPDATE tasks SET workflow_step_id = ?, queued_for_step_id = ?, queued_at = CURRENT_TIMESTAMP,
				wip_admitted = 0, position = 1, priority = ? WHERE id = ?
		`), "step-destination", "step-destination", queued.priority, queued.taskID)
		require.NoError(t, err)
	}

	query := sidebarTaskQuery(1)
	query.Filters = []models.SidebarTaskViewClause{{
		Dimension: "titleMatch", Op: "matches", Value: json.RawMessage(`"ordinary queued"`),
	}}
	page, err := repo.QuerySidebarTaskPage(ctx, workspaceID, query, models.SidebarTaskViewPreferences{})
	require.NoError(t, err)
	require.Equal(t, []string{"ordinary-queued"}, sidebarTaskIDs(page.Tasks))
	for _, entry := range page.Entries {
		if entry.Kind == "task" {
			require.Equal(t, "ordinary-queued", entry.TaskID)
			require.Equal(t, 1, entry.WIPQueuePosition)
			require.Equal(t, 1, entry.WIPQueueTotal)
			return
		}
	}
	t.Fatal("sidebar page has no task entry")
}
