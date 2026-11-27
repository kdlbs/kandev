package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdoptionGeneratedBranchPreservesAttachmentSelector(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	// Ordinary task creation leaves checkout_branch empty and selects the
	// generated worktree through the base-branch identity slug.
	_, err := f.db.Exec(`UPDATE task_repositories SET checkout_branch='',base_branch='main' WHERE id='attachment-adoption'`)
	require.NoError(t, err)
	_, err = f.db.Exec(`UPDATE task_environment_repos SET branch_slug='main' WHERE id='slot-adoption'`)
	require.NoError(t, err)
	p := adoptionPreview(t, f)
	const operation = "11111111-1111-4111-8111-111111111111"
	result := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": operation})
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	var checkoutBranch string
	require.NoError(t, f.db.Get(&checkoutBranch, `SELECT checkout_branch FROM task_repositories WHERE id='attachment-adoption'`))
	require.Empty(t, checkoutBranch)
	require.NoError(t, f.manager.AdmitTaskRecovery(context.Background(), "task-adoption"))
}
