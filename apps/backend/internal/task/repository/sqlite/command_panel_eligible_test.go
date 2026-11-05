package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type commandPanelTestLister interface {
	ListTasksByWorkspaceForCommandPanel(context.Context, string, string, string, string, int, int, string, bool, bool, bool, bool, bool) ([]*models.Task, int, error)
}

// @covers AC-TASKS-WORKSPACE-SEARCH-002.1
func TestCommandPanelEligibleSQLite(t *testing.T) {
	testCommandPanelEligible(t, newLiteralSearchSQLite(t))
}

func TestCommandPanelEligiblePostgres(t *testing.T) {
	testCommandPanelEligible(t, openPostgresRepo(t))
}

func testCommandPanelEligible(t *testing.T, repo *Repository) {
	t.Helper()
	lister, ok := any(repo).(commandPanelTestLister)
	require.True(t, ok, "repository must provide the opt-in listing")
	seedWorkspace(t, repo, "palette")
	require.NoError(t, repo.CreateWorkflow(t.Context(), &models.Workflow{ID: "flow", WorkspaceID: "palette", Name: "Flow"}))
	for _, step := range []struct {
		id      string
		visible int
	}{{"shown", 1}, {"hidden", 0}} {
		_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO workflow_steps (id, workflow_id, name, position, show_in_command_panel) VALUES (?, ?, ?, 0, ?)`), step.id, "flow", step.id, step.visible)
		require.NoError(t, err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 23; index++ {
		step := "hidden"
		if index < 3 {
			step = "shown"
		}
		item := &models.Task{ID: fmt.Sprintf("item-%02d", index), WorkspaceID: "palette", WorkflowID: "flow", WorkflowStepID: step, Title: "Task"}
		createLiteralSearchTask(t, repo, item, base.Add(time.Duration(index)*time.Minute))
	}
	list := func(page, size int) ([]*models.Task, int) {
		t.Helper()
		rows, total, err := lister.ListTasksByWorkspaceForCommandPanel(t.Context(), "palette", "flow", "", "", page, size, "", false, false, false, false, false)
		require.NoError(t, err)
		return rows, total
	}
	first, total := list(1, 2)
	require.Equal(t, 3, total)
	require.Equal(t, []string{"item-02", "item-01"}, literalSearchIDs(first))
	second, total := list(2, 2)
	require.Equal(t, 3, total)
	require.Equal(t, []string{"item-00"}, literalSearchIDs(second))
	regular, total, err := repo.ListTasksByWorkspace(t.Context(), "palette", "flow", "", "", 1, 20, "", false, false, false, false)
	require.NoError(t, err)
	require.Equal(t, 23, total)
	require.Equal(t, "item-22", regular[0].ID)

	seedWorkspace(t, repo, "foreign")
	for _, item := range []*models.Task{
		{ID: "foreign", WorkspaceID: "foreign", WorkflowID: "flow", WorkflowStepID: "shown"},
		{ID: "ephemeral", WorkspaceID: "palette", WorkflowID: "flow", WorkflowStepID: "shown", IsEphemeral: true},
		{ID: "config", WorkspaceID: "palette", WorkflowID: "flow", WorkflowStepID: "shown", Metadata: map[string]interface{}{"config_mode": true}},
		{ID: "automation", WorkspaceID: "palette", WorkflowID: "flow", WorkflowStepID: "shown", Origin: models.TaskOriginAutomationRun},
		{ID: "archived", WorkspaceID: "palette", WorkflowID: "flow", WorkflowStepID: "shown"},
	} {
		item.Title = item.ID
		createLiteralSearchTask(t, repo, item, base.Add(24*time.Hour))
	}
	require.NoError(t, repo.ArchiveTask(t.Context(), "archived"))
	rows, total, err := lister.ListTasksByWorkspaceForCommandPanel(t.Context(), "palette", "flow", "", "", 1, 20, "", false, false, false, true, false)
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Equal(t, []string{"item-02", "item-01", "item-00"}, literalSearchIDs(rows))
	rows, total, err = lister.ListTasksByWorkspaceForCommandPanel(t.Context(), "palette", "flow", "", "", 1, 20, "", false, false, false, false, false)
	require.NoError(t, err)
	require.Equal(t, 4, total)
	require.Contains(t, literalSearchIDs(rows), "config")
	_, err = repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE workflow_steps SET show_in_command_panel = 0 WHERE id = ?`), "shown")
	require.NoError(t, err)
	rows, total = list(1, 20)
	require.Zero(t, total)
	require.Empty(t, rows)
}
