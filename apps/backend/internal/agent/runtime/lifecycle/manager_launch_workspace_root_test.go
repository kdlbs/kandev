package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskWorkspaceRootForLaunch(t *testing.T) {
	manager := &Manager{}
	cases := []struct {
		name string
		req  *LaunchRequest
		want string
	}{
		{
			name: "single worktree uses task folder",
			req: &LaunchRequest{
				ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: "/tasks/task-1/primary",
				TaskDirName: "task-1",
			},
			want: "/tasks/task-1",
		},
		{
			name: "legacy flat worktree keeps containment at the checkout",
			req: &LaunchRequest{
				ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: "/shared/worktrees/repo-feature",
			},
			want: "/shared/worktrees/repo-feature",
		},
		{
			name: "task-root layout uses workspace path",
			req: &LaunchRequest{
				ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: "/tasks/task-1",
				WorkspaceLayout: workspaceLayoutTaskRoot,
			},
			want: "/tasks/task-1",
		},
		{
			name: "local executor uses workspace path",
			req:  &LaunchRequest{ExecutorType: string(models.ExecutorTypeLocal), WorkspacePath: "/home/user/repo"},
			want: "/home/user/repo",
		},
		{
			name: "remote executor has no host root",
			req:  &LaunchRequest{ExecutorType: string(models.ExecutorTypeSSH), WorkspacePath: "/remote/ws"},
			want: "",
		},
		{name: "nil request", req: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := manager.taskWorkspaceRootForLaunch(tc.req); got != tc.want {
				t.Fatalf("taskWorkspaceRootForLaunch() = %q, want %q", got, tc.want)
			}
		})
	}
}
