package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommandPanelEligibleServiceUsesWorkspaceList(t *testing.T) {
	svc, _, repo := newTitleTestService(t)
	_, err := repo.DB().ExecContext(t.Context(), `INSERT INTO workflow_steps (id, workflow_id, name, position, show_in_command_panel) VALUES (?, ?, ?, 0, 1)`, "step-1", "wf-title", "Shown")
	require.NoError(t, err)
	seedPendingTitleTask(t, repo, "task-visible", "", false)
	rows, total, err := svc.ListTasksByWorkspaceForCommandPanel(t.Context(), "ws-title", "wf-title", "", "", 1, 20, "", false, false, false, false, false)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, rows, 1)
	require.Equal(t, "task-visible", rows[0].ID)
	_, _, err = svc.ListTasksByWorkspaceForCommandPanel(t.Context(), "ws-title", "wf-title", "", "search", 1, 20, "", false, false, false, false, false)
	require.Error(t, err)
}
