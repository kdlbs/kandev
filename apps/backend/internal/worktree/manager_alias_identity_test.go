package worktree

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestStoreAliasIdentityCleanup(t *testing.T) {
	m, s := newReferenceCleanupTestManager(t)
	seedReferenceCleanupSession(t, s, "task-owner", "session-owner", models.TaskSessionStateCompleted)
	w := createReferenceCleanupWorktree(t, m, "task-owner", "session-owner")
	_, err := s.db.Exec(`INSERT INTO task_environment_repos
 (id,task_environment_id,repository_id,branch_slug,worktree_id,worktree_path,worktree_branch,status,created_at,updated_at)
 VALUES ('alias',?,?,?,?,?,?,'active',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, w.TaskEnvironmentID, w.RepositoryID, "alias", w.ID, w.Path, w.Branch)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var ambiguous *models.WorktreeIdentityAmbiguousError
	if _, err := m.CaptureCleanupHeadOIDs(ctx, []*Worktree{w}); !errors.As(err, &ambiguous) {
		t.Errorf("capture accepted ambiguity: %v", err)
	}
	if _, err := m.removeWorktreeWithReceipt(ctx, w, false, WorktreeCleanupOptions{}); !errors.As(err, &ambiguous) {
		t.Errorf("cleanup accepted ambiguity: %v", err)
	}
	if _, err := os.Stat(w.Path); err != nil {
		t.Fatalf("ambiguous cleanup removed checkout: %v", err)
	}
	// Removing only the disposable legacy alias makes the same cleanup reachable.
	if _, err := s.db.Exec(`DELETE FROM task_environment_repos WHERE id='alias'`); err != nil {
		t.Fatal(err)
	}
	if _, err := m.removeWorktreeWithReceipt(ctx, w, false, WorktreeCleanupOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.Path); !os.IsNotExist(err) {
		t.Fatalf("canonical cleanup did not remove checkout: %v", err)
	}
}

func TestStoreAliasIdentityRecoveryAdmission(t *testing.T) {
	s, w := seedAliasIdentity(t)
	m, err := NewManager(newTestConfig(t), s, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.AdmitRecovery(context.Background(), RecoveryAdmissionRequest{
		TaskID: "alias-task", OwnerTaskID: "alias-task", SessionID: w.SessionID, TaskEnvironmentID: w.TaskEnvironmentID,
		OwnershipGeneration: 1, ExecutorType: "worktree", Slots: []RecoverySlot{{WorktreeID: w.ID, RepositoryID: w.RepositoryID}},
	})
	var ambiguous *models.WorktreeIdentityAmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("admission lost ownership error: %v", err)
	}
}
