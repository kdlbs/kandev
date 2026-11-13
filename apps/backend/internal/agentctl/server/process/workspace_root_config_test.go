package process

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestNewManagerUsesTaskWorkspaceRootForLinkedWorktreeDiscovery(t *testing.T) {
	taskRoot := t.TempDir()
	primary := filepath.Join(taskRoot, "primary")
	initGitRepoAt(t, primary)
	manager := NewManager(&config.InstanceConfig{
		TaskID: "task-1", SessionID: "session-1",
		WorkDir: primary, TaskWorkspaceRoot: taskRoot,
	}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	manager.repoTrackersMu.RLock()
	defer manager.repoTrackersMu.RUnlock()
	if manager.workspaceTracker == nil {
		t.Fatal("workspace tracker is nil")
	}
	if !sameTestPath(t, manager.workspaceTracker.workspaceRoot, taskRoot) {
		t.Fatalf("tracker workspace root = %q, want %q", manager.workspaceTracker.workspaceRoot, taskRoot)
	}
}

func TestLinkedWorktreeTrackerNameUsesTaskRelativePath(t *testing.T) {
	taskRoot := "/tmp/tasks/task-1"
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "nested", path: taskRoot + "/.claude/worktrees/fix-b", want: ".claude/worktrees/fix-b"},
		{name: "outside", path: "/tmp/other/fix-b", want: ""},
		{name: "task root", path: taskRoot, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := linkedWorktreeTrackerName(taskRoot, tc.path); got != tc.want {
				t.Fatalf("linkedWorktreeTrackerName() = %q, want %q", got, tc.want)
			}
		})
	}
}
