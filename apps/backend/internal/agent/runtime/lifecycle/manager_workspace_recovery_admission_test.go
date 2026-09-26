package lifecycle

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

func TestManagerAdmitWorkspaceRecoveryForwardsSessionIncarnation(t *testing.T) {
	mgr := newTestManager(t)
	worktreeMgr, err := worktree.NewManager(worktree.Config{
		Enabled:       true,
		TasksBasePath: filepath.Join(t.TempDir(), "tasks"),
		BranchPrefix:  "feature/",
	}, newInMemoryWorktreeStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	mgr.SetWorktreeManager(worktreeMgr)

	claim := &models.TaskEnvironmentRecoveryClaim{
		TaskEnvironmentID:    "environment-1",
		OwnerTaskID:          "owner-task-1",
		OwnershipGeneration:  7,
		SessionID:            "session-1",
		SessionIncarnationID: "incarnation-1",
		ExecutorType:         string(models.ExecutorTypeWorktree),
	}
	info := &WorkspaceInfo{
		TaskID:                 "task-1",
		SessionID:              "session-1",
		SessionIncarnationID:   "incarnation-1",
		TaskEnvironmentID:      "environment-1",
		EnvironmentOwnerTaskID: "owner-task-1",
		OwnershipGeneration:    7,
		ExecutorType:           string(models.ExecutorTypeWorktree),
		WorkspaceRepositories: []WorkspaceRepositorySpec{{
			RepositoryID: "repository-1",
			WorktreeID:   "worktree-1",
			BranchSlug:   "main",
		}},
	}

	admission, err := mgr.admitWorkspaceRecovery(worktree.WithRecoveryClaim(context.Background(), claim), info)
	if err != nil {
		t.Fatalf("admitWorkspaceRecovery: %v", err)
	}
	if admission == nil || admission.Claim() != claim {
		t.Fatalf("admission claim = %#v, want carried claim", admission)
	}
}
