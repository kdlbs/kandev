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
// AC-TASKS-ORPHAN-REAP-003.4: the worktree, not the root, is the thing that
// must be equal to or inside the other -- another task's live worktree
// nested inside this reap root blocks it (a child task's worktree
// surviving the removal of a parent's directory it lived under).
func TestApplyOrphanReapOwnershipBlocksRootContainingOtherTaskLiveWorktree(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")

	root := t.TempDir()
	nestedWorktree := filepath.Join(root, "nested")
	if err := os.MkdirAll(nestedWorktree, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-other", SessionID: "sess-other", TaskID: "task-other", ExecutorID: "executor-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		WorktreePath: nestedWorktree,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	resolvedRoot := resolveOrphanReapPathBestEffort(root)
	byRoot := map[string][]orphanReapCandidate{resolvedRoot: {newOrphanReapOwnershipCandidate(500, 1, resolvedRoot, resolvedRoot)}}
	snap := []hostProcess{{PID: 500, PPID: 1, Cwd: resolvedRoot, Command: "sh"}}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected root to be blocked by another task's live worktree nested inside it, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 1 || snapshot.OrphanReapSkips[0].Root != resolvedRoot {
		t.Fatalf("expected one root-level skip for %q, got %+v", resolvedRoot, snapshot.OrphanReapSkips)
	}
}

// The mirror image of the above: this task's reap root nested inside
// another task's live worktree is NOT a containment match under
// AC-003.4's one-directional text (only AC-003.2's bidirectional session
// overlap check reaches this shape).
func TestApplyOrphanReapOwnershipDoesNotBlockRootNestedInsideOtherTaskLiveWorktree(t *testing.T) {
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
	if len(got) != 1 {
		t.Fatalf("expected root nested inside another task's worktree to remain signalable under AC-003.4, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 0 {
		t.Fatalf("expected no root-level skip, got %+v", snapshot.OrphanReapSkips)
	}
}

// An executor status not in {failed, stopped, completed} is treated as live,
// so "starting" still blocks the root.
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

// A "stopped" executor is not live and must not block a root.
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

// The ancestry walk must not break on an intermediate process whose cwd
// is unreadable (so the host snapshotter can only report its PID/PPID, with
// an empty Cwd). A candidate reached only through such a hop is still owned.
func TestApplyOrphanReapOwnershipWalksThroughAncestryOnlyHop(t *testing.T) {
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
	owned := newOrphanReapOwnershipCandidate(500, 450, root, root) // child of an ancestry-only hop
	byRoot := map[string][]orphanReapCandidate{root: {owned}}
	snap := []hostProcess{
		{PID: 400, PPID: 1, Cwd: "/other", Command: "sh"},
		{PID: 450, PPID: 400, Cwd: "", Command: ""}, // cwd unreadable: ancestry-only entry
		{PID: 500, PPID: 450, Cwd: root, Command: "sh"},
	}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected pid 500 to be skipped via the ancestry-only hop, got %+v", got)
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

// A hop ps did not report an ancestry entry for (the host snapshot marks it
// with the unresolved-ppid sentinel, not a defaulted 0) must fail the whole
// candidate closed rather than let the walk read the sentinel as "no more
// ancestors, not owned".
func TestApplyOrphanReapOwnershipFailsClosedOnUnresolvedAncestryHop(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")

	root := t.TempDir()
	cand := newOrphanReapOwnershipCandidate(500, 450, root, root)
	byRoot := map[string][]orphanReapCandidate{root: {cand}}
	snap := []hostProcess{
		{PID: 450, PPID: orphanReapUnresolvedPPID, Cwd: "", Command: ""},
		{PID: 500, PPID: 450, Cwd: root, Command: "sh"},
	}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected pid 500 to be skipped as ownership-inconclusive, not signaled, got %+v", got)
	}
	found := false
	for _, rec := range snapshot.OrphanReapRecords {
		if rec.PID == 500 && rec.Outcome == orphanReapOutcomeSkipped {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pid 500 to be recorded as skipped, got %+v", snapshot.OrphanReapRecords)
	}
}

// AC-TASKS-ORPHAN-REAP-003.5 + 003.6: an unresolved hop in the backend's own
// ancestry cannot be ruled out as hiding a protected ancestor, so every
// active root is inconclusive, not just skipped for one unrelated candidate.
func TestApplyOrphanReapOwnershipFailsClosedOnUnresolvedBackendAncestry(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")

	self := os.Getpid()
	root := t.TempDir()
	byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(500, 1, root, root)}}
	snap := []hostProcess{
		{PID: self, PPID: orphanReapUnresolvedPPID, Cwd: "", Command: ""},
		{PID: 500, PPID: 1, Cwd: root, Command: "sh"},
	}
	snapshot := &taskResourceCleanupSnapshot{}

	got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
	if len(got) != 0 {
		t.Fatalf("expected no signalable candidates when the backend's own ancestry is unresolved, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 1 {
		t.Fatalf("expected the root to be skipped phase-wide, got %+v", snapshot.OrphanReapSkips)
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

// An unresolvable live executor worktree path must fail every active
// root closed, the same way an unresolvable session path already does —
// falling back to the raw, unresolved path would let a symlinked or
// reparented process pass the AC-TASKS-ORPHAN-REAP-003.4 containment check.
func TestApplyOrphanReapOwnershipFailsClosedOnUnresolvableExecutorWorktreePath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")
	mustCreateOrphanReapTask(t, repo, "task-other")

	unresolvable := filepath.Join(t.TempDir(), "does-not-exist")
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "exec-other", SessionID: "sess-other", TaskID: "task-other", ExecutorID: "executor-1",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		WorktreePath: unresolvable,
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
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
		t.Fatalf("expected no signalable candidates when a live executor worktree path is unresolvable, got %+v", got)
	}
	if len(snapshot.OrphanReapSkips) != 2 {
		t.Fatalf("expected both roots to be skipped as inconclusive, got %+v", snapshot.OrphanReapSkips)
	}
}

// AC-TASKS-ORPHAN-REAP-003.2 vs 003.4: orphanReapFindOverlap matches "equal
// to, inside, or containing" while orphanReapFindContainment only matches
// the narrower "equal to or inside" (AC-003.4's own direction: another
// task's worktree nested inside root blocks it) -- root nested inside
// another task's path is an overlap but not a containment.
func TestOrphanReapFindOverlapAndContainmentDiffer(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tasks", "root")

	t.Run("path nested inside root: both match", func(t *testing.T) {
		nested := filepath.Join(root, "child")
		paths := []orphanReapOwnedPath{{taskID: "other", path: nested}}

		if _, found := orphanReapFindOverlap(paths, root); !found {
			t.Fatalf("expected overlap to detect a path nested inside root")
		}
		if _, found := orphanReapFindContainment(paths, root); !found {
			t.Fatalf("expected containment to detect another task's worktree nested inside root")
		}
	})

	t.Run("root nested inside path: overlap yes, containment no", func(t *testing.T) {
		ancestor := filepath.Dir(root)
		paths := []orphanReapOwnedPath{{taskID: "other", path: ancestor}}

		if _, found := orphanReapFindOverlap(paths, root); !found {
			t.Fatalf("expected overlap to detect root nested inside path")
		}
		if _, found := orphanReapFindContainment(paths, root); found {
			t.Fatalf("expected containment to not match root nested inside another task's path")
		}
	})

	t.Run("equal paths: both match", func(t *testing.T) {
		paths := []orphanReapOwnedPath{{taskID: "other", path: root}}

		if owner, found := orphanReapFindOverlap(paths, root); !found || owner != "other" {
			t.Fatalf("expected overlap to match an equal path, got owner=%q found=%v", owner, found)
		}
		if owner, found := orphanReapFindContainment(paths, root); !found || owner != "other" {
			t.Fatalf("expected containment to match an equal path, got owner=%q found=%v", owner, found)
		}
	})

	t.Run("unrelated paths: neither matches", func(t *testing.T) {
		paths := []orphanReapOwnedPath{{taskID: "other", path: filepath.Join(string(filepath.Separator), "unrelated")}}

		if _, found := orphanReapFindOverlap(paths, root); found {
			t.Fatalf("expected overlap to not match an unrelated path")
		}
		if _, found := orphanReapFindContainment(paths, root); found {
			t.Fatalf("expected containment to not match an unrelated path")
		}
	})
}

// AC-TASKS-ORPHAN-REAP-003.5: PID 0, 1, and any negative PID are always
// protected regardless of ancestry, since none of them can be a real
// process this task launched.
func TestApplyOrphanReapOwnershipProtectsPIDsAtOrBelowOne(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	mustCreateOrphanReapTask(t, repo, "task-a")

	for _, pid := range []int{0, 1, -1} {
		root := t.TempDir()
		byRoot := map[string][]orphanReapCandidate{root: {newOrphanReapOwnershipCandidate(pid, 1, root, root)}}
		snap := []hostProcess{{PID: pid, PPID: 1, Cwd: root, Command: "sh"}}
		snapshot := &taskResourceCleanupSnapshot{}

		got := svc.applyOrphanReapOwnership(ctx, "task-a", snap, byRoot, snapshot)
		if len(got) != 0 {
			t.Fatalf("expected pid %d to be protected, got %+v", pid, got)
		}
	}
}

// orphanReapProtectedPIDs must protect not just the backend's own pid but
// every ancestor of it, walked over the same ppid chain every other check
// in this phase uses.
func TestOrphanReapProtectedPIDsWalksBackendAncestryChain(t *testing.T) {
	self := os.Getpid()
	parent := self + 10000
	grandparent := self + 20000
	ppidByPID := map[int]int{self: parent, parent: grandparent}

	got, inconclusive := orphanReapProtectedPIDs(ppidByPID)
	if inconclusive {
		t.Fatalf("expected a fully-resolved chain to not be inconclusive, got %+v", got)
	}
	for _, pid := range []int{self, parent, grandparent} {
		if !got[pid] {
			t.Fatalf("expected pid %d to be protected, got %+v", pid, got)
		}
	}
}

// A ppid cycle must not spin orphanReapProtectedPIDs forever: the walk
// stops the moment it revisits a pid it has already marked protected.
func TestOrphanReapProtectedPIDsStopsOnCycle(t *testing.T) {
	self := os.Getpid()
	other := self + 10000
	ppidByPID := map[int]int{self: other, other: self}

	got, inconclusive := orphanReapProtectedPIDs(ppidByPID)
	if inconclusive {
		t.Fatalf("expected a resolved cycle to not be inconclusive, got %+v", got)
	}
	if !got[self] || !got[other] {
		t.Fatalf("expected both pids in the cycle to be protected, got %+v", got)
	}
	if len(got) != 2 {
		t.Fatalf("expected exactly the two cycle pids protected (no runaway walk), got %+v", got)
	}
}

// Hitting the unresolved-ppid sentinel while walking the backend's own
// ancestry must report inconclusive, not fall through to "no more
// ancestors, protect what was found and stop" -- mirrors
// TestOrphanReapAncestorOwnerReturnsInconclusiveOnUnresolvedHop for the
// sibling ownership walk.
func TestOrphanReapProtectedPIDsReturnsInconclusiveOnUnresolvedHop(t *testing.T) {
	self := os.Getpid()
	parent := self + 10000
	ppidByPID := map[int]int{self: parent, parent: orphanReapUnresolvedPPID}

	got, inconclusive := orphanReapProtectedPIDs(ppidByPID)
	if !inconclusive {
		t.Fatalf("expected inconclusive=true when an ancestor's ppid is unresolved, got protected=%+v", got)
	}
}

// Hitting the unresolved-ppid sentinel partway up the chain must report
// inconclusive, not fall through to "walk ended, not owned".
func TestOrphanReapAncestorOwnerReturnsInconclusiveOnUnresolvedHop(t *testing.T) {
	ppidByPID := map[int]int{
		500: 450,
		450: orphanReapUnresolvedPPID,
	}
	owner, blocked, inconclusive := orphanReapAncestorOwner(500, ppidByPID, map[int]string{})
	if !inconclusive || blocked || owner != "" {
		t.Fatalf("expected an inconclusive result, got owner=%q blocked=%v inconclusive=%v", owner, blocked, inconclusive)
	}
}

// An owner found before the walk reaches an unresolved hop still reports as
// blocked, not inconclusive: the sentinel only matters when the walk needs
// to cross it.
func TestOrphanReapAncestorOwnerFindsOwnerBeforeUnresolvedHop(t *testing.T) {
	ppidByPID := map[int]int{
		500: 400,
		400: orphanReapUnresolvedPPID,
	}
	owner, blocked, inconclusive := orphanReapAncestorOwner(500, ppidByPID, map[int]string{400: "task-other"})
	if inconclusive || !blocked || owner != "task-other" {
		t.Fatalf("expected pid 400 to be found owned before the walk needed to cross the unresolved hop, got owner=%q blocked=%v inconclusive=%v", owner, blocked, inconclusive)
	}
}
