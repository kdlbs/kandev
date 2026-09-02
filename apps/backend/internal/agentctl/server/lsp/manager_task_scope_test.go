package lsp

import (
	"context"
	"testing"
	"time"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

func TestManagerScopesSameLanguageByTask(t *testing.T) {
	manager, processes := newManagerForTest(t, func(int) *fakeLSPServer {
		return newFakeLSPServer()
	})
	if _, err := manager.UpdateWorkspaceForTask(
		"task-parent", "/workspace/parent", []string{"/workspace/parent"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.UpdateWorkspaceForTask(
		"task-child", "/workspace/child", []string{"/workspace/child"},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), StartRequest{
		TaskID: "task-parent", Language: "kotlin", Generation: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), StartRequest{
		TaskID: "task-child", Language: "kotlin", Generation: 1,
	}); err != nil {
		t.Fatal(err)
	}
	parent := waitForTaskPhase(t, manager, "task-parent", "kotlin", sharedlsp.PhaseReady)
	child := waitForTaskPhase(t, manager, "task-child", "kotlin", sharedlsp.PhaseReady)
	if processes.activeCount() != 2 || len(parent.WorkspaceFolders) != 1 || parent.WorkspaceFolders[0].Name != "parent" ||
		len(child.WorkspaceFolders) != 1 || child.WorkspaceFolders[0].Name != "child" {
		t.Fatalf("parent=%#v child=%#v active=%d", parent, child, processes.activeCount())
	}
	if got := manager.DiscoveryRootsForTask("task-parent"); len(got) != 1 || got[0] != "/workspace/parent" {
		t.Fatalf("parent discovery roots = %v", got)
	}
	if got := manager.DiscoveryRootsForTask("task-child"); len(got) != 1 || got[0] != "/workspace/child" {
		t.Fatalf("child discovery roots = %v", got)
	}
	if _, err := manager.Stop(context.Background(), StopRequest{
		TaskID: "task-child", Language: "kotlin", Generation: 1, Reason: "task_stop",
	}); err != nil {
		t.Fatal(err)
	}
	if child = manager.SnapshotForTask("task-child", "kotlin"); child.Phase != sharedlsp.PhaseOff {
		t.Fatalf("child snapshot after stop = %#v", child)
	}
	if parent = manager.SnapshotForTask("task-parent", "kotlin"); parent.Phase != sharedlsp.PhaseReady {
		t.Fatalf("parent snapshot after child stop = %#v", parent)
	}
	if processes.activeCount() != 1 {
		t.Fatalf("active processes after child stop = %d, want 1", processes.activeCount())
	}
}

func waitForTaskPhase(
	t *testing.T,
	manager *Manager,
	taskID, language string,
	phase sharedlsp.Phase,
) Snapshot {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := manager.SnapshotForTask(taskID, language)
		if snapshot.Phase == phase {
			return snapshot
		}
		select {
		case <-deadline.C:
			t.Fatalf("task=%s phase=%s want=%s snapshot=%#v", taskID, snapshot.Phase, phase, snapshot)
		case <-ticker.C:
		}
	}
}
