package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

type recordingTaskRepositoryBaseBranchUpdater struct {
	calls []taskRepositoryBaseBranchUpdate
	err   error
}

type taskRepositoryBaseBranchUpdate struct {
	taskID, taskRepositoryID, baseBranch string
}

func (u *recordingTaskRepositoryBaseBranchUpdater) UpdateTaskRepositoryBaseBranch(
	_ context.Context, taskID, taskRepositoryID, baseBranch string,
) error {
	u.calls = append(u.calls, taskRepositoryBaseBranchUpdate{taskID, taskRepositoryID, baseBranch})
	return u.err
}

func TestSelfHealTaskRepositoryBaseBranchesWritesOnlyChangedFallbacks(t *testing.T) {
	updater := &recordingTaskRepositoryBaseBranchUpdater{}
	exec := &Executor{logger: logger.Default(), taskRepositoryBaseBranchUpdater: updater}

	exec.selfHealTaskRepositoryBaseBranches(context.Background(), "task-1", &LaunchAgentRequest{
		TaskRepositoryID: "task-repo-single",
	}, &LaunchAgentResponse{
		RequestedBaseBranch:       "stale",
		BaseBranch:                "main",
		BaseBranchFallbackWarning: "used live default",
	})

	exec.selfHealTaskRepositoryBaseBranches(context.Background(), "task-1", &LaunchAgentRequest{}, &LaunchAgentResponse{
		Worktrees: []RepoWorktreeResult{
			{TaskRepositoryID: "task-repo-a", RequestedBaseBranch: "stale", BaseBranch: "main", BaseBranchFallbackWarning: "fallback"},
			{TaskRepositoryID: "task-repo-b", RequestedBaseBranch: "main", BaseBranch: "main", BaseBranchFallbackWarning: "same value"},
			{TaskRepositoryID: "", RequestedBaseBranch: "stale", BaseBranch: "main", BaseBranchFallbackWarning: "ambiguous"},
		},
	})

	require.Equal(t, []taskRepositoryBaseBranchUpdate{
		{taskID: "task-1", taskRepositoryID: "task-repo-single", baseBranch: "main"},
		{taskID: "task-1", taskRepositoryID: "task-repo-a", baseBranch: "main"},
	}, updater.calls)
}

func TestSelfHealTaskRepositoryBaseBranchesDoesNotFailLaunchOnWriteError(t *testing.T) {
	updater := &recordingTaskRepositoryBaseBranchUpdater{err: errors.New("storage unavailable")}
	exec := &Executor{logger: logger.Default(), taskRepositoryBaseBranchUpdater: updater}

	exec.selfHealTaskRepositoryBaseBranches(context.Background(), "task-1", &LaunchAgentRequest{}, &LaunchAgentResponse{
		Worktrees: []RepoWorktreeResult{{
			TaskRepositoryID:          "task-repo-1",
			RequestedBaseBranch:       "stale",
			BaseBranch:                "main",
			BaseBranchFallbackWarning: "fallback",
		}},
	})

	require.Len(t, updater.calls, 1)
}
