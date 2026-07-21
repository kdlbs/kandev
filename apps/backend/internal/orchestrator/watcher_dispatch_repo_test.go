package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRepositoryChecker answers a fixed verdict for any repository.
type fakeRepositoryChecker struct {
	exists bool
	err    error
	calls  int
}

func (f *fakeRepositoryChecker) RepositoryExists(_ context.Context, _, _ string) (bool, error) {
	f.calls++
	return f.exists, f.err
}

// TestWatcherDispatchCoordinator_SelfHealsOnDeletedRepository: when the bound
// repository was soft-deleted after the watch was configured, the coordinator
// must NOT create a task (which would leave an orphan row, since CreateTask
// inserts the task before repository association), must release the reservation,
// and must self-heal the watch so a later poll doesn't repeat.
func TestWatcherDispatchCoordinator_SelfHealsOnDeletedRepository(t *testing.T) {
	src := &stubWatcherSource{
		name:         "stub",
		workspaceID:  "ws-1",
		repositories: []IssueTaskRepository{{RepositoryID: "repo-gone", BaseBranch: "main"}},
	}
	checker := &fakeRepositoryChecker{exists: false}
	creator := &countingIssueTaskCreator{taskID: "task-1"}

	c := &WatcherDispatchCoordinator{
		repoChecker:     checker,
		shouldAutoStart: func(_ context.Context, _ string) bool { return false },
		logger:          newTestLogger(),
	}
	c.SetTaskCreator(creator)

	c.Dispatch(context.Background(), src, struct{}{})

	if checker.calls != 1 {
		t.Errorf("expected 1 repository check, got %d", checker.calls)
	}
	if creator.calls != 0 {
		t.Errorf("CreateIssueTask must not run when bound repo is deleted, got %d", creator.calls)
	}
	if src.selfHealCalls != 1 {
		t.Fatalf("expected 1 SelfHeal call, got %d", src.selfHealCalls)
	}
	if !strings.Contains(src.selfHealCause, "repo-gone") {
		t.Errorf("SelfHeal cause should name the removed repository, got %q", src.selfHealCause)
	}
	if src.releaseCalls != 1 {
		t.Errorf("reservation must be released on self-heal, got %d Release calls", src.releaseCalls)
	}
}

// A repository-check error must fail open (run the pipeline), not self-heal —
// a transient DB hiccup shouldn't disable a watch.
func TestWatcherDispatchCoordinator_RepoCheckErrorFailsOpen(t *testing.T) {
	src := &stubWatcherSource{
		name:         "stub",
		workspaceID:  "ws-1",
		repositories: []IssueTaskRepository{{RepositoryID: "repo-1", BaseBranch: "main"}},
	}
	checker := &fakeRepositoryChecker{err: errors.New("db unavailable")}
	creator := &countingIssueTaskCreator{taskID: "task-1"}

	c := &WatcherDispatchCoordinator{
		repoChecker:     checker,
		shouldAutoStart: func(_ context.Context, _ string) bool { return false },
		logger:          newTestLogger(),
	}
	c.SetTaskCreator(creator)

	c.Dispatch(context.Background(), src, struct{}{})

	if src.selfHealCalls != 0 {
		t.Errorf("SelfHeal must NOT run on a repo-check error, got %d", src.selfHealCalls)
	}
	if creator.calls != 1 {
		t.Errorf("pipeline must fall through on repo-check error, got %d CreateIssueTask calls", creator.calls)
	}
}

// A live bound repository passes the pre-flight and the task is created.
func TestWatcherDispatchCoordinator_LiveRepositoryPassesPreflight(t *testing.T) {
	src := &stubWatcherSource{
		name:         "stub",
		workspaceID:  "ws-1",
		repositories: []IssueTaskRepository{{RepositoryID: "repo-1", BaseBranch: "main"}},
	}
	checker := &fakeRepositoryChecker{exists: true}
	creator := &countingIssueTaskCreator{taskID: "task-1"}

	c := &WatcherDispatchCoordinator{
		repoChecker:     checker,
		shouldAutoStart: func(_ context.Context, _ string) bool { return false },
		logger:          newTestLogger(),
	}
	c.SetTaskCreator(creator)

	c.Dispatch(context.Background(), src, struct{}{})

	if src.selfHealCalls != 0 {
		t.Errorf("SelfHeal must NOT run for a live repo, got %d", src.selfHealCalls)
	}
	if creator.calls != 1 {
		t.Errorf("expected 1 CreateIssueTask call on the happy path, got %d", creator.calls)
	}
}
