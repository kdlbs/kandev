package executor

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-RUNTIME-CLEANUP-001.7
func TestWorkspaceReuseAllowedRequiresLiveWorktreeRepo(t *testing.T) {
	deletedAt := time.Unix(123, 0)
	tests := []struct {
		name             string
		env              *models.TaskEnvironment
		requestedType    string
		reuseRequired    bool
		repoBacked       bool
		wantReuseAllowed bool
	}{
		{
			name: "persisted worktree with deleted row",
			env: &models.TaskEnvironment{
				ExecutorType: string(models.ExecutorTypeWorktree),
				Repos: []*models.TaskEnvironmentRepo{{
					WorktreeID: "wt-deleted",
					Status:     "active",
					DeletedAt:  &deletedAt,
				}},
			},
			requestedType:    string(models.ExecutorTypeWorktree),
			reuseRequired:    true,
			wantReuseAllowed: false,
		},
		{
			name: "persisted worktree with failed row",
			env: &models.TaskEnvironment{
				ExecutorType: string(models.ExecutorTypeWorktree),
				Repos: []*models.TaskEnvironmentRepo{{
					WorktreeID: "wt-failed",
					Status:     taskEnvironmentRepoStatusFailed,
				}},
			},
			requestedType:    string(models.ExecutorTypeWorktree),
			reuseRequired:    true,
			wantReuseAllowed: false,
		},
		{
			name: "persisted worktree with deleted status",
			env: &models.TaskEnvironment{
				ExecutorType: string(models.ExecutorTypeWorktree),
				Repos: []*models.TaskEnvironmentRepo{{
					WorktreeID: "wt-deleted-status",
					Status:     taskEnvironmentRepoStatusDeleted,
				}},
			},
			requestedType:    string(models.ExecutorTypeWorktree),
			reuseRequired:    true,
			wantReuseAllowed: false,
		},
		{
			name: "persisted worktree without physical id",
			env: &models.TaskEnvironment{
				ExecutorType: string(models.ExecutorTypeWorktree),
				Repos: []*models.TaskEnvironmentRepo{{
					Status: "active",
				}},
			},
			requestedType:    string(models.ExecutorTypeWorktree),
			reuseRequired:    true,
			wantReuseAllowed: false,
		},
		{
			name: "persisted worktree with live row",
			env: &models.TaskEnvironment{
				ExecutorType: string(models.ExecutorTypeWorktree),
				Repos: []*models.TaskEnvironmentRepo{{
					WorktreeID: "wt-live",
					Status:     "active",
				}},
			},
			requestedType:    string(models.ExecutorTypeWorktree),
			reuseRequired:    true,
			wantReuseAllowed: true,
		},
		{
			name: "executor type mismatch",
			env: &models.TaskEnvironment{
				ExecutorType: string(models.ExecutorTypeLocal),
				Repos: []*models.TaskEnvironmentRepo{{
					WorktreeID: "wt-live",
					Status:     "active",
				}},
			},
			requestedType:    string(models.ExecutorTypeWorktree),
			reuseRequired:    true,
			wantReuseAllowed: false,
		},
		{
			name:             "matching non-worktree executor",
			env:              &models.TaskEnvironment{ExecutorType: string(models.ExecutorTypeLocal)},
			requestedType:    string(models.ExecutorTypeLocal),
			reuseRequired:    true,
			wantReuseAllowed: true,
		},
		{
			name: "reuse not required",
			env: &models.TaskEnvironment{
				ExecutorType: string(models.ExecutorTypeWorktree),
				Repos: []*models.TaskEnvironmentRepo{{
					WorktreeID: "wt-live",
					Status:     "active",
				}},
			},
			requestedType:    string(models.ExecutorTypeWorktree),
			reuseRequired:    false,
			wantReuseAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workspaceReuseAllowed(tt.env, tt.requestedType, tt.reuseRequired, tt.repoBacked); got != tt.wantReuseAllowed {
				t.Fatalf("workspaceReuseAllowed() = %t, want %t", got, tt.wantReuseAllowed)
			}
		})
	}
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.7
func TestWorkspaceReuseAllowedRequiresSSHInventory(t *testing.T) {
	tests := []struct {
		name             string
		env              *models.TaskEnvironment
		repoBacked       bool
		wantReuseAllowed bool
	}{
		{
			name:             "repo-backed with empty inventory forces fresh materialization",
			env:              &models.TaskEnvironment{ExecutorType: string(models.ExecutorTypeSSH)},
			repoBacked:       true,
			wantReuseAllowed: false,
		},
		{
			name: "repo-backed with recorded inventory reuses",
			env: &models.TaskEnvironment{
				ExecutorType: string(models.ExecutorTypeSSH),
				Repos:        []*models.TaskEnvironmentRepo{{RepositoryID: "repo-1"}},
			},
			repoBacked:       true,
			wantReuseAllowed: true,
		},
		{
			name:             "repo-less task reuses despite empty inventory",
			env:              &models.TaskEnvironment{ExecutorType: string(models.ExecutorTypeSSH)},
			repoBacked:       false,
			wantReuseAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workspaceReuseAllowed(tt.env, string(models.ExecutorTypeSSH), true, tt.repoBacked)
			if got != tt.wantReuseAllowed {
				t.Fatalf("workspaceReuseAllowed() = %t, want %t", got, tt.wantReuseAllowed)
			}
		})
	}
}
