package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func newOrphanReapOwnershipCandidate(pid, ppid int, cwd, root string) orphanReapCandidate {
	return orphanReapCandidate{
		hostProcess: hostProcess{PID: pid, PPID: ppid, Cwd: cwd, Command: "sh"},
		Root:        root,
	}
}

func mustCreateOrphanReapTask(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: taskID, WorkspaceID: "ws-orphan-reap", Title: taskID,
	}); err != nil {
		t.Fatalf("CreateTask(%q): %v", taskID, err)
	}
}

func TestApplyOrphanReapOwnershipAllowsUnownedCandidate(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")

	root := t.TempDir()
	byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(500, 1, root, root)}}
	snap := []hostProcess{{PID: 500, PPID: 1, Cwd: root, Command: "sh"}}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 1 || got[0].PID != 500 {
		t.Fatalf("expected candidate 500 to be signalable, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 0 {
		t.Fatalf("expected no skips, got %+v", snapshot.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-003.2: a root equal to, inside, or containing another
// task's live session workspace is blocked, including an IDLE session, since
// IDLE is one of the five live states this feature treats as live.
func TestApplyOrphanReapOwnershipBlocksRootOverlappingOtherTaskLiveSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")

	otherWorkspace := t.TempDir()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-other", TaskID: "task-other", State: models.TaskSessionStateIdle,
		WorkspacePath: otherWorkspace,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	// Production always calls applyOrphanReapOwnership with already-resolved
	// roots (resolveOrphanReapRoots ran first); mirror that here since a
	// macOS temp dir is itself a symlink and the code compares resolved paths.
	root := resolveOrphanReapPathBestEffort(otherWorkspace)
	byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(500, 1, root, root)}}
	snap := []hostProcess{{PID: 500, PPID: 1, Cwd: root, Command: "sh"}}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected root to be blocked by another task's IDLE session, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 1 || snapshot.OrphanReapSkips[0].Root != root {
		t.Fatalf("expected one root-level skip for %q, got %+v", root, snapshot.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-003.2: an empty workspace_path names no path and must
// never block a root.
func TestApplyOrphanReapOwnershipEmptyWorkspacePathNamesNoPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-other", TaskID: "task-other", State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	root := t.TempDir()
	byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(500, 1, root, root)}}
	snap := []hostProcess{{PID: 500, PPID: 1, Cwd: root, Command: "sh"}}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 1 {
		t.Fatalf("expected candidate to remain signalable when other session names no path, got %+v", got)
	}
}

// AC-TASKS-ORPHAN-REAP-003.4: a root equal to or inside another task's live
// recorded execution's worktree is blocked.
func TestApplyOrphanReapOwnershipBlocksRootInsideOtherTaskLiveWorktree(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")

	worktreeRoot := t.TempDir()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-other", SessionID: "sess-other", TaskID: "task-other", ExecutorID: "executor-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		WorktreePath: worktreeRoot,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	root := filepath.Join(resolveOrphanReapPathBestEffort(worktreeRoot), "nested")
	byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(500, 1, root, root)}}
	snap := []hostProcess{{PID: 500, PPID: 1, Cwd: root, Command: "sh"}}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected root to be blocked by another task's live worktree, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 1 || snapshot.OrphanReapSkips[0].Root != root {
		t.Fatalf("expected one root-level skip for %q, got %+v", root, snapshot.OrphanReapSkips)
	}
}

// F20 safe reading: an executor status not in {failed, stopped, completed} is
// treated as live, so "starting" still blocks the root.
func TestApplyOrphanReapOwnershipTreatsStartingExecutorAsLive(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")

	worktreeRoot := t.TempDir()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-other", SessionID: "sess-other", TaskID: "task-other", ExecutorID: "executor-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
		WorktreePath: worktreeRoot,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	root := resolveOrphanReapPathBestEffort(worktreeRoot)
	byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(500, 1, root, root)}}
	snap := []hostProcess{{PID: 500, PPID: 1, Cwd: root, Command: "sh"}}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected 'starting' executor to be treated as live and block the root, got %+v", got)
	}
}

// F20 safe reading: a "stopped" executor is not live and must not block a root.
func TestApplyOrphanReapOwnershipAllowsRootFromStoppedExecutor(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")

	worktreeRoot := t.TempDir()
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-other", SessionID: "sess-other", TaskID: "task-other", ExecutorID: "executor-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStopped,
		WorktreePath: worktreeRoot,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	root := worktreeRoot
	byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(500, 1, root, root)}}
	snap := []hostProcess{{PID: 500, PPID: 1, Cwd: root, Command: "sh"}}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 1 {
		t.Fatalf("expected a stopped executor to not block the root, got %+v", got)
	}
}

// AC-TASKS-ORPHAN-REAP-003.3: a candidate whose ancestry includes another
// task's local_pid is skipped even though its root is otherwise clear.
func TestApplyOrphanReapOwnershipSkipsCandidateOwnedByOtherTaskLocalPID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-other", SessionID: "sess-other", TaskID: "task-other", ExecutorID: "executor-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		LocalPID: 400,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	root := t.TempDir()
	owned := newOrphanReapOwnershipCandidate(500, 400, root, root) // child of the other task's local_pid
	unowned := newOrphanReapOwnershipCandidate(600, 1, root, root) // unrelated ancestry, same root
	byRoot := map[string][]orphanReapCandidate{root: {owned, unowned}}
	snap := []hostProcess{
		{PID: 400, PPID: 1, Cwd: "/other", Command: "sh"},
		{PID: 500, PPID: 400, Cwd: root, Command: "sh"},
		{PID: 600, PPID: 1, Cwd: root, Command: "sh"},
	}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 1 || got[0].PID != 600 {
		t.Fatalf("expected only pid 600 to remain signalable, got %+v", got)
	}
	foundSkip := false
	for _, rec := range snapshot.OrphanReapRecords {
		if rec.PID == 500 && rec.Outcome == orphanReapOutcomeSkipped {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Fatalf("expected pid 500 to be recorded as skipped, got %+v", snapshot.OrphanReapRecords)
	}
}

// AC-TASKS-ORPHAN-REAP-003.5: the backend's own PID is always protected.
func TestApplyOrphanReapOwnershipSkipsProtectedPID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")

	root := t.TempDir()
	self := os.Getpid()
	byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(self, 1, root, root)}}
	snap := []hostProcess{{PID: self, PPID: 1, Cwd: root, Command: "sh"}}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected the backend's own pid to be protected, got %+v", got)
	}
}

type orphanReapErrExecutorRepo struct {
	repository.ExecutorRepository
	err error
}

func (r orphanReapErrExecutorRepo) ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error) {
	return nil, r.err
}

// AC-TASKS-ORPHAN-REAP-003.6: an executor-repository error fails every
// currently active root closed, not just one.
func TestApplyOrphanReapOwnershipFailsClosedOnExecutorRepositoryError(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	svc.executors = orphanReapErrExecutorRepo{ExecutorRepository: repo, err: errors.New("boom")}

	rootA, rootB := t.TempDir(), t.TempDir()
	byRoot := map[string][]orphanReapCandidate{
		rootA: {newOrphanReapOwnershipCandidate(500, 1, rootA, rootA)},
		rootB: {newOrphanReapOwnershipCandidate(600, 1, rootB, rootB)},
	}
	snap := []hostProcess{
		{PID: 500, PPID: 1, Cwd: rootA, Command: "sh"},
		{PID: 600, PPID: 1, Cwd: rootB, Command: "sh"},
	}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected no signalable candidates after a repository error, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 2 {
		t.Fatalf("expected both roots to be skipped, got %+v", snapshot.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-003.2 + 003.6: an unresolvable stored session
// workspace path cannot be ruled out as containing a root, so every active
// root is inconclusive, not just the one it happens to resemble.
func TestApplyOrphanReapOwnershipFailsClosedOnUnresolvableSessionPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")

	unresolvable := filepath.Join(t.TempDir(), "does-not-exist")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-other", TaskID: "task-other", State: models.TaskSessionStateRunning,
		WorkspacePath: unresolvable,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	rootA, rootB := t.TempDir(), t.TempDir()
	byRoot := map[string][]orphanReapCandidate{
		rootA: {newOrphanReapOwnershipCandidate(500, 1, rootA, rootA)},
		rootB: {newOrphanReapOwnershipCandidate(600, 1, rootB, rootB)},
	}
	snap := []hostProcess{
		{PID: 500, PPID: 1, Cwd: rootA, Command: "sh"},
		{PID: 600, PPID: 1, Cwd: rootB, Command: "sh"},
	}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected no signalable candidates when a stored path is unresolvable, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 2 {
		t.Fatalf("expected both roots to be skipped as inconclusive, got %+v", snapshot.OrphanReapSkips)
	}
}
