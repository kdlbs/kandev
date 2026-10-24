package worktree

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

type retainedCleanupStore struct{ Store }

func (retainedCleanupStore) IsWorkspaceRetained(context.Context, string, string, string) (bool, error) {
	return true, nil
}
func TestExplicitRetentionBlocksWorktreeRemoval(t *testing.T) {
	mgr, store := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, store, "task-retain", "session-retain", models.TaskSessionStateCompleted)
	wt := createReferenceCleanupWorktree(t, mgr, "task-retain", "session-retain")
	mgr.store = retainedCleanupStore{Store: mgr.store}
	require.Error(t, mgr.CleanupWorktrees(context.Background(), []*Worktree{wt}))
	_, err := os.Stat(wt.Path)
	require.NoError(t, err)
	assertCleanupBranchPresent(t, wt.RepositoryPath, wt.Branch)
}
