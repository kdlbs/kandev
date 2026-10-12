package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSidebarTaskWorkflowCoverage(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, driver)
			seedWorkspace(t, repo, "coverage")
			seedWorkspace(t, repo, "other")
			for _, id := range []string{"visible", "hidden", "excluded", "empty"} {
				require.NoError(t, repo.CreateWorkflow(t.Context(), &models.Workflow{ID: id, WorkspaceID: "coverage", Name: id, Hidden: id == "hidden"}))
			}
			for _, task := range []*models.Task{
				{ID: "visible", WorkflowID: "visible"},
				{ID: "hidden", WorkflowID: "hidden"},
				{ID: "unassigned"},
				{ID: "archived", WorkflowID: "excluded"},
				{ID: "ephemeral", WorkflowID: "excluded", IsEphemeral: true},
				{ID: "automation", WorkflowID: "excluded", Origin: models.TaskOriginAutomationRun},
				{ID: "config", WorkflowID: "excluded", Metadata: map[string]interface{}{"config_mode": true}},
			} {
				task.WorkspaceID, task.Title = "coverage", task.ID
				require.NoError(t, repo.CreateTask(t.Context(), task))
			}
			require.NoError(t, repo.ArchiveTask(t.Context(), "archived"))
			require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "other", WorkspaceID: "other", Title: "Other workspace"}))
			ids, err := repo.SidebarTaskWorkflowIDs(t.Context(), "coverage")
			require.NoError(t, err)
			require.ElementsMatch(t, []string{"visible", "hidden", ""}, ids)
			profile := "sqlite_nocase_v1"
			if driver == "postgres" {
				profile = "server_only"
			}
			require.Equal(t, profile, repo.SidebarTaskOrderingProfile())
		})
	}
}
