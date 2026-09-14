package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestManagerAdmitRecoveryExcludedExecutorsDoNotInspectHostCheckout(t *testing.T) {
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, recoveryGitDirName), []byte("not a git pointer"), 0o600); err != nil {
		t.Fatalf("create invalid checkout metadata: %v", err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	for _, executorType := range []models.ExecutorType{
		models.ExecutorTypeLocal,
		models.ExecutorTypeLocalDocker,
		models.ExecutorTypeSSH,
		models.ExecutorTypeSprites,
		models.ExecutorTypeKubernetes,
		models.ExecutorTypeMockRemote,
	} {
		admission, err := mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
			ExecutorType: string(executorType),
			Slots: []RecoverySlot{{
				Worktree: &Worktree{Path: checkout},
			}},
		})
		if err != nil {
			t.Fatalf("executor %q inspected excluded checkout: %v", executorType, err)
		}
		if admission != nil {
			t.Fatalf("executor %q returned recovery admission", executorType)
		}
	}
}

func TestManagerAdmitRecoveryRepoFreeWorktreeInventoryIsNoop(t *testing.T) {
	checkout := t.TempDir()
	if err := os.WriteFile(filepath.Join(checkout, recoveryGitDirName), []byte("not a git pointer"), 0o600); err != nil {
		t.Fatalf("create invalid checkout metadata: %v", err)
	}
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	admission, err := mgr.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID:              "task-repo-free",
		SessionID:           "session-repo-free",
		TaskEnvironmentID:   "environment-repo-free",
		OwnerTaskID:         "task-repo-free",
		OwnershipGeneration: 1,
		ExecutorType:        string(models.ExecutorTypeWorktree),
	})
	if err != nil {
		t.Fatalf("repo-free admission: %v", err)
	}
	if admission != nil {
		t.Fatal("repo-free admission returned a recovery claim")
	}
}
