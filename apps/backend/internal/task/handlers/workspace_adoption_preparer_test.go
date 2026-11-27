package handlers

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdoptionNativePreparerRetainsPublishedMetadata(t *testing.T) {
	f := newAdoptionHTTPFixture(t)
	_, err := f.db.Exec(`UPDATE task_environments SET workspace_path=?`, f.old)
	require.NoError(t, err)
	_, err = f.db.Exec(`UPDATE task_sessions SET workspace_path=?`, f.old)
	require.NoError(t, err)
	p := adoptionPreview(t, f)
	rec := adoptionRequest(f.router, "/api/v1/tasks/task-adoption/workspace-adoption/apply", map[string]any{"preview": p, "operation_id": "11111111-1111-4111-8111-111111111111"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	preparer := lifecycle.NewWorktreePreparer(f.manager, newTestLogger(t))
	result, err := preparer.Prepare(context.Background(), &lifecycle.EnvPrepareRequest{
		TaskID: "task-adoption", SessionID: "session-adoption", WorkspaceID: "ws-adoption",
		TaskEnvironmentID: "env-adoption", TaskDirName: "task-adoption", RepositoryID: "repo-adoption",
		RepositoryPath: f.source, WorkspacePath: f.clone, WorktreeID: "wt-adoption",
		UseWorktree: true, WorkspaceReuseRequired: true, BaseBranch: "main",
		CheckoutBranch: "feature/adoption", BranchIdentitySlug: "feature-adoption",
	}, nil)
	require.NoError(t, err)
	require.True(t, result.Success, result.ErrorMessage)
	require.Equal(t, f.clone, result.WorkspacePath)
	require.Equal(t, filepath.Join(f.clone, ".git"), result.MainRepoGitDir)
	require.Equal(t, "wt-adoption", result.WorktreeID)
}
