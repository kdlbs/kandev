package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/service"
)

// mockDockerClient implements service.DockerClient for testing.
type mockDockerClient struct {
	containers []service.GCContainerInfo
	removed    []string
	removeErr  error
}

func (m *mockDockerClient) ListContainers(_ context.Context, _ map[string]string) ([]service.GCContainerInfo, error) {
	return m.containers, nil
}

func (m *mockDockerClient) RemoveContainer(_ context.Context, containerID string, _ bool) error {
	if m.removeErr != nil {
		return m.removeErr
	}
	m.removed = append(m.removed, containerID)
	return nil
}

func TestGC_OrphanWorktreeDeleted(t *testing.T) {
	svc := newTestService(t)

	// Create a temp worktree base with an orphan directory.
	base := t.TempDir()
	orphanDir := filepath.Join(base, "nonexistent-task-id")
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		t.Fatal(err)
	}

	gc := service.NewGarbageCollector(svc, base, nil, 0)
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 1 {
		t.Errorf("worktrees_deleted = %d, want 1", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 0 {
		t.Errorf("worktrees_kept = %d, want 0", result.WorktreesKept)
	}

	// Verify directory was actually removed.
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Error("orphan directory should have been removed")
	}
}

func TestGC_ActiveWorktreeKept(t *testing.T) {
	svc := newTestService(t)

	// Insert a task whose ID matches the directory name.
	taskID := "task-active-1"
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'IN_PROGRESS', 'Active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)

	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, taskID), 0o755); err != nil {
		t.Fatal(err)
	}

	gc := service.NewGarbageCollector(svc, base, nil, 0)
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if result.WorktreesKept != 1 {
		t.Errorf("worktrees_kept = %d, want 1", result.WorktreesKept)
	}
}

func TestGC_OrphanContainerRemoved(t *testing.T) {
	svc := newTestService(t)

	mock := &mockDockerClient{
		containers: []service.GCContainerInfo{
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

	gc := service.NewGarbageCollector(svc, "", mock, 0)
	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 1 {
		t.Errorf("containers_removed = %d, want 1", result.ContainersRemoved)
	}
	if len(mock.removed) != 1 || mock.removed[0] != "ctr-orphan-1" {
		t.Errorf("removed = %v, want [ctr-orphan-1]", mock.removed)
	}
}

func TestGC_TerminalStoppedContainerRemoved(t *testing.T) {
	svc := newTestService(t)

	taskID := "task-completed-gc"
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'COMPLETED', 'Done', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)

	mock := &mockDockerClient{
		containers: []service.GCContainerInfo{
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

	gc := service.NewGarbageCollector(svc, "", mock, 0)
	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 1 {
		t.Errorf("containers_removed = %d, want 1", result.ContainersRemoved)
	}
}

func TestGC_RunningTerminalContainerKept(t *testing.T) {
	svc := newTestService(t)

	taskID := "task-completed-running"
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'COMPLETED', 'Done', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)

	mock := &mockDockerClient{
		containers: []service.GCContainerInfo{
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

	gc := service.NewGarbageCollector(svc, "", mock, 0)
	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 0 {
		t.Errorf("containers_removed = %d, want 0 (running container should be kept)", result.ContainersRemoved)
	}
	if result.ContainersKept != 1 {
		t.Errorf("containers_kept = %d, want 1", result.ContainersKept)
	}
}

func TestGC_InProgressContainerKept(t *testing.T) {
	svc := newTestService(t)

	taskID := "task-in-progress-gc"
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES (?, 'ws-1', 'IN_PROGRESS', 'Working', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, taskID)

	mock := &mockDockerClient{
		containers: []service.GCContainerInfo{
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

	gc := service.NewGarbageCollector(svc, "", mock, 0)
	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 0 {
		t.Errorf("containers_removed = %d, want 0", result.ContainersRemoved)
	}
	if result.ContainersKept != 1 {
		t.Errorf("containers_kept = %d, want 1", result.ContainersKept)
	}
}

func TestGC_NoTaskIDLabelKept(t *testing.T) {
	svc := newTestService(t)

	mock := &mockDockerClient{
		containers: []service.GCContainerInfo{
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

	gc := service.NewGarbageCollector(svc, "", mock, 0)
	result := gc.Sweep(context.Background())

	if result.ContainersRemoved != 0 {
		t.Errorf("containers_removed = %d, want 0 (no task_id label)", result.ContainersRemoved)
	}
	if result.ContainersKept != 1 {
		t.Errorf("containers_kept = %d, want 1", result.ContainersKept)
	}
}

func TestGC_SweepResultCounts(t *testing.T) {
	svc := newTestService(t)

	// One active task, one missing task.
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, title, created_at, updated_at)
		VALUES ('t-keep', 'ws-1', 'IN_PROGRESS', 'Keep', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "t-keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "t-gone"), 0o755); err != nil {
		t.Fatal(err)
	}

	mock := &mockDockerClient{
		containers: []service.GCContainerInfo{
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

	gc := service.NewGarbageCollector(svc, base, mock, 0)
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
	svc := newTestService(t)
	gc := service.NewGarbageCollector(svc, "", nil, 0)
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 0 || result.ContainersRemoved != 0 {
		t.Error("empty base path and nil docker should produce zero results")
	}
}

func TestGC_NonexistentWorktreeBaseNoError(t *testing.T) {
	svc := newTestService(t)
	gc := service.NewGarbageCollector(svc, "/nonexistent/path/12345", nil, 0)
	result := gc.Sweep(context.Background())

	if result.WorktreesDeleted != 0 {
		t.Errorf("worktrees_deleted = %d, want 0", result.WorktreesDeleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("errors = %v, want none for nonexistent base", result.Errors)
	}
}
