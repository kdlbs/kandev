package infra_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

// newTestGC creates a GarbageCollector with an in-memory SQLite repo.
func newTestGC(t *testing.T, worktreeBase string, docker infra.DockerClient) (*infra.GarbageCollector, func(string, ...interface{})) {
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

	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	gc := infra.NewGarbageCollector(repo, log, worktreeBase, docker, 0)

	execSQL := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec sql: %v", err)
		}
	}

	return gc, execSQL
}

func TestGC_OrphanWorktreeDeleted(t *testing.T) {
	base := t.TempDir()
	orphanDir := filepath.Join(base, "nonexistent-task-id")
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		t.Fatal(err)
	}

	gc, _ := newTestGC(t, base, nil)
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 1 {
		t.Errorf("worktrees_deleted = %d, want 1", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 0 {
		t.Errorf("worktrees_kept = %d, want 0", result.WorktreesKept)
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Error("orphan directory should have been removed")
	}
}

func TestGC_ActiveWorktreeKept(t *testing.T) {
	base := t.TempDir()
	taskID := "task-active-1"

	gc, execSQL := newTestGC(t, base, nil)
	execSQL(`INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'IN_PROGRESS', 'Active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)

	if err := os.MkdirAll(filepath.Join(base, taskID), 0o755); err != nil {
		t.Fatal(err)
	}

	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 1 {
		t.Errorf("worktrees_kept = %d, want 1", result.WorktreesKept)
	}
}

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

	gc, _ := newTestGC(t, "", mock)
	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 1 {
		t.Errorf("containers_removed = %d, want 1", result.ContainersRemoved)
	}
	if len(mock.removed) != 1 || mock.removed[0] != "ctr-orphan-1" {
		t.Errorf("removed = %v, want [ctr-orphan-1]", mock.removed)
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

	gc, execSQL := newTestGC(t, "", mock)
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

	gc, execSQL := newTestGC(t, "", mock)
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

	gc, execSQL := newTestGC(t, "", mock)
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

	gc, _ := newTestGC(t, "", mock)
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

	gc, execSQL := newTestGC(t, base, mock)
	execSQL(`INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES ('t-keep', 'ws-1', 'IN_PROGRESS', 'Keep', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	if err := os.MkdirAll(filepath.Join(base, "t-keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "t-gone"), 0o755); err != nil {
		t.Fatal(err)
	}

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
	gc, _ := newTestGC(t, "", nil)
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 0 || result.ContainersRemoved != 0 {
		t.Error("empty base path and nil docker should produce zero results")
	}
}

func TestGC_NonexistentWorktreeBaseNoError(t *testing.T) {
	gc, _ := newTestGC(t, "/nonexistent/path/12345", nil)
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("errors = %v, want none for nonexistent base", result.Errors)
	}
}
