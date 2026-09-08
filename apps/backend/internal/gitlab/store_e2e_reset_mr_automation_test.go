package gitlab

import "testing"

// TestStoreResetWorkspaceE2E_ClearsMRAutomationState covers AC38-adjacent
// discipline for the E2E reset invariant (apps/backend/AGENTS.md): every MR
// automation table must be wiped for the reset workspace and left untouched
// for other workspaces, ahead of the task rows they reference.
func TestStoreResetWorkspaceE2E_ClearsMRAutomationState(t *testing.T) {
	store := newTestStore(t)
	for _, workspaceID := range []string{"ws-a", "ws-b"} {
		seedWorkspace(t, store, workspaceID)
		seedTask(t, store, "task-"+workspaceID, workspaceID)
		if _, err := store.UpdateTaskMRAutomationOptions(t.Context(), "task-"+workspaceID, TaskMRAutomationPatch{
			AutoFixPromptOverride: stringPtr("custom"),
		}, nil); err != nil {
			t.Fatalf("seed options %s: %v", workspaceID, err)
		}
		setMRSwitches(t, store, "task-"+workspaceID, mrIdentity("group/project", 7),
			TaskMRAutomationSwitchPatch{PromptOnMerged: boolPtr(true)})
		if err := store.SetTaskMRObservedState(t.Context(), "task-"+workspaceID, "", "group/project", 7, "open"); err != nil {
			t.Fatalf("seed state %s: %v", workspaceID, err)
		}
	}

	if _, err := store.ResetWorkspaceE2E(t.Context(), "ws-a"); err != nil {
		t.Fatalf("reset workspace: %v", err)
	}

	assertMRAutomationRowCount(t, store, "task-ws-a", 0)
	assertMRAutomationRowCount(t, store, "task-ws-b", 1)
}

func assertMRAutomationRowCount(t *testing.T, store *Store, taskID string, want int) {
	t.Helper()
	for _, query := range []string{
		`SELECT COUNT(*) FROM gitlab_task_mr_options WHERE task_id = ?`,
		`SELECT COUNT(*) FROM gitlab_task_mr_automation_options WHERE task_id = ?`,
		`SELECT COUNT(*) FROM gitlab_task_mr_state WHERE task_id = ?`,
	} {
		var got int
		if err := store.ro.Get(&got, query, taskID); err != nil {
			t.Fatalf("count %q: %v", query, err)
		}
		if got != want {
			t.Fatalf("count %q for %s = %d, want %d", query, taskID, got, want)
		}
	}
}
