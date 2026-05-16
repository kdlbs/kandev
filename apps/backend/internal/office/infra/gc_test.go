package infra_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/infra"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// mockDockerClient implements infra.DockerClient for testing.
type mockDockerClient struct {
	containers []infra.GCContainerInfo
	removed    []string
	removeErr  error
}

func (m *mockDockerClient) ListContainers(_ context.Context, _ map[string]string) ([]infra.GCContainerInfo, error) {
	return m.containers, nil
}

func (m *mockDockerClient) RemoveContainer(_ context.Context, containerID string, _ bool) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.removed = append(m.removed, containerID)
	return nil
}

// fakeInventory implements infra.WorktreeInventory for tests.
type fakeInventory struct {
	paths []string
	err   error
}

func (f *fakeInventory) ListActiveWorktreePaths(_ context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.paths, nil
}

// newTestGC creates a GarbageCollector with an in-memory SQLite repo and the
// supplied inventory (may be nil to exercise the defensive skip path).
func newTestGC(
	t *testing.T, worktreeBase string, docker infra.DockerClient, inv infra.WorktreeInventory,
) (*infra.GarbageCollector, func(string, ...interface{})) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		project_id TEXT DEFAULT '',
		state TEXT NOT NULL DEFAULT 'TODO',
		title TEXT DEFAULT '',
		description TEXT DEFAULT '',
		identifier TEXT DEFAULT '',
		workflow_id TEXT DEFAULT '',
		workflow_step_id TEXT DEFAULT '',
		priority INTEGER DEFAULT 0,
		position INTEGER DEFAULT 0,
		metadata TEXT DEFAULT '{}',
		is_ephemeral INTEGER DEFAULT 0,
		parent_id TEXT DEFAULT '',
		origin TEXT DEFAULT 'manual',
		labels TEXT DEFAULT '[]',
		execution_policy TEXT DEFAULT '',
		execution_state TEXT DEFAULT '',
		checkout_agent_id TEXT,
		checkout_at DATETIME,
		archived_at DATETIME,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create tasks table: %v", err)
	}

	// ADR 0005 Wave F: GetTaskExecutionFields and other office reads
	// use a correlated subquery to workflow_step_participants /
	// workflow_steps to project the runner. Stub both tables so the
	// projection evaluates cleanly even though tests do not exercise
	// the workflow repo.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS workflow_steps (
		id TEXT PRIMARY KEY,
		agent_profile_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create workflow_steps: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS workflow_step_participants (
		id TEXT PRIMARY KEY,
		step_id TEXT NOT NULL DEFAULT '',
		task_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT '',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		decision_required INTEGER NOT NULL DEFAULT 0,
		position INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatalf("create workflow_step_participants: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	gc := infra.NewGarbageCollector(repo, inv, log, worktreeBase, docker, 0)

	execSQL := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec sql: %v", err)
		}
	}

	return gc, execSQL
}

// ---------------------------------------------------------------------------
// Worktree sweep tests
// ---------------------------------------------------------------------------

func TestGC_WorktreeSweep_KeepsActiveTaskDir(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "mytask_abc", "repo")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	taskDir := filepath.Dir(live)
	// Back-date the parent task dir so the "fresh orphan" path can't
	// accidentally protect it — we want the live/ancestor match to be
	// what keeps it.
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(taskDir, old, old); err != nil {
		t.Fatal(err)
	}

	gc, _ := newTestGC(t, base, nil, &fakeInventory{paths: []string{live}})
	result := gc.Sweep(context.Background())

	if _, err := os.Stat(taskDir); err != nil {
		t.Errorf("live task dir was removed: %v", err)
	}
	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 1 {
		t.Errorf("worktrees_kept = %d, want 1", result.WorktreesKept)
	}
}

func TestGC_WorktreeSweep_DeletesOldOrphan(t *testing.T) {
	base := t.TempDir()
	orphan := filepath.Join(base, "orphan_xyz")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}

	gc, _ := newTestGC(t, base, nil, &fakeInventory{})
	result := gc.Sweep(context.Background())

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected orphan dir removed; stat err = %v", err)
	}
	if result.WorktreesDeleted != 1 {
		t.Errorf("worktrees_deleted = %d, want 1", result.WorktreesDeleted)
	}
}

func TestGC_WorktreeSweep_KeepsFreshOrphan(t *testing.T) {
	base := t.TempDir()
	fresh := filepath.Join(base, "fresh_xyz")
	if err := os.MkdirAll(fresh, 0o755); err != nil {
		t.Fatal(err)
	}
	// mtime is "now" by virtue of just creating the dir.

	gc, _ := newTestGC(t, base, nil, &fakeInventory{})
	result := gc.Sweep(context.Background())

	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh orphan was removed: %v", err)
	}
	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 1 {
		t.Errorf("worktrees_kept = %d, want 1", result.WorktreesKept)
	}
}

func TestGC_WorktreeSweep_KeepsAncestorOfLivePath(t *testing.T) {
	base := t.TempDir()
	taskDir := filepath.Join(base, "task1")
	repoDir := filepath.Join(taskDir, "repoA")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Age the task dir so absent ancestor protection would let it be
	// deleted; the ancestor set must explicitly save it.
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(taskDir, old, old); err != nil {
		t.Fatal(err)
	}

	gc, _ := newTestGC(t, base, nil, &fakeInventory{paths: []string{repoDir}})
	result := gc.Sweep(context.Background())

	if _, err := os.Stat(taskDir); err != nil {
		t.Errorf("ancestor of live path was removed: %v", err)
	}
	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 1 {
		t.Errorf("worktrees_kept = %d, want 1", result.WorktreesKept)
	}
}

func TestGC_WorktreeSweep_FailClosedOnInventoryError(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "would-be-orphan")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}

	gc, _ := newTestGC(t, base, nil, &fakeInventory{err: errors.New("boom")})
	result := gc.Sweep(context.Background())

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir was deleted despite inventory error: %v", err)
	}
	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0 on inventory error", result.WorktreesDeleted)
	}
	if len(result.Errors) == 0 {
		t.Errorf("expected at least one error on inventory failure")
	}
}

func TestGC_WorktreeSweep_PathNormalization(t *testing.T) {
	base := t.TempDir()
	task1 := filepath.Join(base, "task1")
	task2 := filepath.Join(base, "task2")
	if err := os.MkdirAll(task1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(task2, 0o755); err != nil {
		t.Fatal(err)
	}
	// Age both so only the live-set match can save them.
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(task1, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(task2, old, old); err != nil {
		t.Fatal(err)
	}

	// Inventory deliberately returns un-normalized variants.
	paths := []string{
		task1 + "/", // trailing slash
		filepath.Join(base, "redundant", "..", "task2"), // unclean .. component
	}
	gc, _ := newTestGC(t, base, nil, &fakeInventory{paths: paths})
	result := gc.Sweep(context.Background())

	for _, p := range []string{task1, task2} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("dir %s removed despite normalized live match: %v", p, err)
		}
	}
	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 2 {
		t.Errorf("worktrees_kept = %d, want 2", result.WorktreesKept)
	}
}

func TestGC_WorktreeSweep_NilInventoryIsSafe(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "anything")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(dir, old, old); err != nil {
		t.Fatal(err)
	}

	gc, _ := newTestGC(t, base, nil, nil)
	result := gc.Sweep(context.Background())

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir removed despite nil inventory: %v", err)
	}
	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
}

// ---------------------------------------------------------------------------
// Container sweep tests
// ---------------------------------------------------------------------------

func TestGC_OrphanContainerRemoved(t *testing.T) {
	mock := &mockDockerClient{
		containers: []infra.GCContainerInfo{
			{
				ID:    "ctr-orphan-1",
				Name:  "kandev-agent-orphan",
				State: "exited",
				Labels: map[string]string{
					"kandev.managed":    "true",
					"kandev.task_id":    "no-such-task",
					"kandev.session_id": "sess-orphan",
				},
			},
		},
	}

	gc, _ := newTestGC(t, "", mock, nil)
	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 1 {
		t.Errorf("containers_removed = %d, want 1", result.ContainersRemoved)
	}
	if len(mock.removed) != 1 || mock.removed[0] != "ctr-orphan-1" {
		t.Errorf("removed = %v, want [ctr-orphan-1]", mock.removed)
	}
}

func TestGC_ContainerKeptOnDBError(t *testing.T) {
	// Close the underlying DB right after constructing the GC so the
	// next GetTaskExecutionFields call returns an error that is NOT
	// ErrTaskNotFound. The fail-closed contract must keep the container.
	mock := &mockDockerClient{
		containers: []infra.GCContainerInfo{
			{
				ID:    "ctr-unknown",
				Name:  "kandev-agent-unknown",
				State: "exited",
				Labels: map[string]string{
					"kandev.managed":    "true",
					"kandev.task_id":    "some-task",
					"kandev.session_id": "sess-unknown",
				},
			},
		},
	}

	gc, execSQL := newTestGC(t, "", mock, nil)
	// Drop the tasks table so the query errors with "no such table" —
	// not sql.ErrNoRows, so the sentinel does not match.
	execSQL(`DROP TABLE tasks`)

	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 0 {
		t.Errorf("containers_removed = %d, want 0 (DB error must keep container)", result.ContainersRemoved)
	}
	if result.ContainersKept != 1 {
		t.Errorf("containers_kept = %d, want 1", result.ContainersKept)
	}
	if len(mock.removed) != 0 {
		t.Errorf("removed = %v, want none", mock.removed)
	}
}

func TestGC_TerminalStoppedContainerRemoved(t *testing.T) {
	taskID := "task-completed-gc"
	mock := &mockDockerClient{
		containers: []infra.GCContainerInfo{
			{
				ID:    "ctr-stale-1",
				Name:  "kandev-agent-stale",
				State: "exited",
				Labels: map[string]string{
					"kandev.managed":    "true",
					"kandev.task_id":    taskID,
					"kandev.session_id": "sess-stale",
				},
			},
		},
	}

	gc, execSQL := newTestGC(t, "", mock, nil)
	execSQL(`INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'COMPLETED', 'Done', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)

	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 1 {
		t.Errorf("containers_removed = %d, want 1", result.ContainersRemoved)
	}
}

func TestGC_RunningTerminalContainerKept(t *testing.T) {
	taskID := "task-completed-running"
	mock := &mockDockerClient{
		containers: []infra.GCContainerInfo{
			{
				ID:    "ctr-running-1",
				Name:  "kandev-agent-running",
				State: "running",
				Labels: map[string]string{
					"kandev.managed":    "true",
					"kandev.task_id":    taskID,
					"kandev.session_id": "sess-running",
				},
			},
		},
	}

	gc, execSQL := newTestGC(t, "", mock, nil)
	execSQL(`INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'COMPLETED', 'Done', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)

	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 0 {
		t.Errorf("containers_removed = %d, want 0 (running container should be kept)", result.ContainersRemoved)
	}
	if result.ContainersKept != 1 {
		t.Errorf("containers_kept = %d, want 1", result.ContainersKept)
	}
}

func TestGC_InProgressContainerKept(t *testing.T) {
	taskID := "task-in-progress-gc"
	mock := &mockDockerClient{
		containers: []infra.GCContainerInfo{
			{
				ID:    "ctr-active-1",
				Name:  "kandev-agent-active",
				State: "running",
				Labels: map[string]string{
					"kandev.managed":    "true",
					"kandev.task_id":    taskID,
					"kandev.session_id": "sess-active",
				},
			},
		},
	}

	gc, execSQL := newTestGC(t, "", mock, nil)
	execSQL(`INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'IN_PROGRESS', 'Working', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)

	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 0 {
		t.Errorf("containers_removed = %d, want 0", result.ContainersRemoved)
	}
	if result.ContainersKept != 1 {
		t.Errorf("containers_kept = %d, want 1", result.ContainersKept)
	}
}

func TestGC_NoTaskIDLabelKept(t *testing.T) {
	mock := &mockDockerClient{
		containers: []infra.GCContainerInfo{
			{
				ID:    "ctr-no-task",
				Name:  "kandev-agent-notask",
				State: "running",
				Labels: map[string]string{
					"kandev.managed":    "true",
					"kandev.session_id": "sess-notask",
				},
			},
		},
	}

	gc, _ := newTestGC(t, "", mock, nil)
	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 0 {
		t.Errorf("containers_removed = %d, want 0 (no task_id label)", result.ContainersRemoved)
	}
	if result.ContainersKept != 1 {
		t.Errorf("containers_kept = %d, want 1", result.ContainersKept)
	}
}

func TestGC_SweepResultCounts(t *testing.T) {
	base := t.TempDir()

	mock := &mockDockerClient{
		containers: []infra.GCContainerInfo{
			{
				ID: "ctr-1", State: "exited",
				Labels: map[string]string{
					"kandev.managed": "true",
					"kandev.task_id": "t-gone-ctr",
				},
			},
			{
				ID: "ctr-2", State: "running",
				Labels: map[string]string{
					"kandev.managed": "true",
					"kandev.task_id": "t-keep",
				},
			},
		},
	}

	// Two dirs under base: t-keep (live in inventory) and t-gone (old orphan).
	live := filepath.Join(base, "t-keep")
	gone := filepath.Join(base, "t-gone")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(gone, old, old); err != nil {
		t.Fatal(err)
	}

	inv := &fakeInventory{paths: []string{live}}
	gc, execSQL := newTestGC(t, base, mock, inv)
	execSQL(`INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES ('t-keep', 'ws-1', 'IN_PROGRESS', 'Keep', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 1 {
		t.Errorf("worktrees_deleted = %d, want 1", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 1 {
		t.Errorf("worktrees_kept = %d, want 1", result.WorktreesKept)
	}
	if result.ContainersRemoved != 1 {
		t.Errorf("containers_removed = %d, want 1", result.ContainersRemoved)
	}
	if result.ContainersKept != 1 {
		t.Errorf("containers_kept = %d, want 1", result.ContainersKept)
	}
	if len(result.Errors) != 0 {
		t.Errorf("errors = %v, want none", result.Errors)
	}
}

func TestGC_EmptyWorktreeBaseSkipped(t *testing.T) {
	gc, _ := newTestGC(t, "", nil, &fakeInventory{})
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 0 || result.ContainersRemoved != 0 {
		t.Error("empty base path and nil docker should produce zero results")
	}
}

func TestGC_NonexistentWorktreeBaseNoError(t *testing.T) {
	gc, _ := newTestGC(t, "/nonexistent/path/12345", nil, &fakeInventory{})
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("errors = %v, want none for nonexistent base", result.Errors)
	}
}
