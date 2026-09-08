package github

import (
	"context"
	"errors"
	"testing"
)

type recordingTaskRepositoryUpdater struct {
	err   error
	calls []taskRepositoryUpdateCall
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.12
func TestSyncTaskPR_DoesNotPropagateUnchangedBase(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	updater := &recordingTaskRepositoryUpdater{}
	svc.SetTaskRepositoryBaseBranchUpdater(updater)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widgets", PRNumber: 42,
		HeadBranch: "feature/child", BaseBranch: "main", State: "open",
	}); err != nil {
		t.Fatalf("CreateTaskPR() error: %v", err)
	}

	if err := svc.SyncTaskPR(ctx, "task-1", &PRStatus{PR: &PR{
		Number: 42, RepoOwner: "acme", RepoName: "widgets", HeadBranch: "feature/child", BaseBranch: "main", State: "open",
	}}); err != nil {
		t.Fatalf("SyncTaskPR() error: %v", err)
	}
	if len(updater.calls) != 0 {
		t.Fatalf("updater calls = %#v, want none", updater.calls)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.12
func TestSyncTaskPR_TaskRepositoryUpdateFailureIsBestEffort(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	updater := &recordingTaskRepositoryUpdater{err: errors.New("task store unavailable")}
	svc.SetTaskRepositoryBaseBranchUpdater(updater)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "task-1", RepositoryID: "repo-1", Owner: "acme", Repo: "widgets", PRNumber: 42,
		HeadBranch: "feature/child", BaseBranch: "feature/parent", State: "open",
	}); err != nil {
		t.Fatalf("CreateTaskPR() error: %v", err)
	}

	if err := svc.SyncTaskPR(ctx, "task-1", &PRStatus{PR: &PR{
		Number: 42, RepoOwner: "acme", RepoName: "widgets", HeadBranch: "feature/child", BaseBranch: "main", State: "open",
	}}); err != nil {
		t.Fatalf("SyncTaskPR() error: %v", err)
	}
	if len(updater.calls) != 1 {
		t.Fatalf("updater calls = %#v, want one best-effort attempt", updater.calls)
	}
	got, err := svc.GetTaskPRByOwnerRepoNumber(ctx, "task-1", "acme", "widgets", 42)
	if err != nil {
		t.Fatalf("GetTaskPRByOwnerRepoNumber() error: %v", err)
	}
	if got == nil || got.BaseBranch != "main" {
		t.Fatalf("persisted TaskPR = %#v, want base main", got)
	}
}

type taskRepositoryUpdateCall struct {
	taskID, repositoryID, headBranch, baseBranch string
}

func (u *recordingTaskRepositoryUpdater) UpdateTaskRepositoryBaseBranch(
	_ context.Context, taskID, repositoryID, headBranch, baseBranch string,
) error {
	u.calls = append(u.calls, taskRepositoryUpdateCall{
		taskID: taskID, repositoryID: repositoryID, headBranch: headBranch, baseBranch: baseBranch,
	})
	return u.err
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.12
func TestSyncTaskPR_PropagatesChangedBaseToTaskRepository(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()
	updater := &recordingTaskRepositoryUpdater{}
	svc.SetTaskRepositoryBaseBranchUpdater(updater)
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		Owner:        "acme",
		Repo:         "widgets",
		PRNumber:     42,
		HeadBranch:   "feature/stacked-child",
		BaseBranch:   "feature/deleted-parent",
		State:        "open",
	}); err != nil {
		t.Fatalf("CreateTaskPR() error: %v", err)
	}

	err := svc.SyncTaskPR(ctx, "task-1", &PRStatus{PR: &PR{
		Number:     42,
		RepoOwner:  "acme",
		RepoName:   "widgets",
		HeadBranch: "feature/stacked-child",
		BaseBranch: "main",
		State:      "open",
	}})
	if err != nil {
		t.Fatalf("SyncTaskPR() error: %v", err)
	}
	want := taskRepositoryUpdateCall{
		taskID: "task-1", repositoryID: "repo-1", headBranch: "feature/stacked-child", baseBranch: "main",
	}
	if len(updater.calls) != 1 || updater.calls[0] != want {
		t.Fatalf("updater calls = %#v, want %#v", updater.calls, want)
	}
}
