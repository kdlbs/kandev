package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.1
func TestMissingCheckoutRecoveryPreservesCanonicalIdentity(t *testing.T) {
	ctx := context.Background()
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	admission, err := fixture.manager.AdmitRecovery(ctx, fixture.request)
	if err != nil {
		t.Fatalf("admit missing-checkout recovery: %v", err)
	}
	if admission == nil {
		t.Fatal("AdmitRecovery returned no admission for a missing canonical checkout")
	}
	defer func() {
		if err := admission.Release(ctx); err != nil {
			t.Errorf("release recovery admission: %v", err)
		}
	}()

	got, err := fixture.manager.Create(WithRecoveryAdmission(ctx, admission), CreateRequest{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, TaskEnvironmentID: fixture.environmentID,
		RepositoryID: fixture.repositoryID, RepositoryPath: fixture.repositoryPath, BaseBranch: "main",
		WorktreeID: fixture.worktreeID, ReuseRequired: true,
	})
	if err != nil {
		t.Fatalf("attach restored worktree: %v", err)
	}
	if got == nil || got.ID != fixture.worktreeID || filepath.Clean(got.Path) != filepath.Clean(fixture.worktreePath) || got.Branch != fixture.branch {
		t.Fatalf("attached worktree = %+v, want original identity %q at %q on %q", got, fixture.worktreeID, fixture.worktreePath, fixture.branch)
	}
	if head := strings.TrimSpace(runGit(t, fixture.worktreePath, "rev-parse", "HEAD")); head != fixture.branchHead {
		t.Fatalf("restored HEAD = %q, want recorded branch head %q", head, fixture.branchHead)
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.5
func TestMissingCheckoutRecoveryClearsOnlyTheProvenStaleRegistration(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, true)

	assertMissingCheckoutRecovered(t, fixture)
	registered, err := worktreeRegistrationExists(context.Background(), fixture.repositoryPath, fixture.worktreePath)
	if err != nil {
		t.Fatalf("inspect recovered Git registration: %v", err)
	}
	if !registered {
		t.Fatal("restored checkout has no Git worktree registration")
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.1
func TestMissingCheckoutRecoveryRebuildsMissingTasksBase(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	tasksBase := filepath.Dir(filepath.Dir(fixture.worktreePath))
	if err := os.RemoveAll(tasksBase); err != nil {
		t.Fatalf("remove managed tasks base: %v", err)
	}

	assertMissingCheckoutRecovered(t, fixture)
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
func TestMissingCheckoutRecoveryKeepsAttachOnlyReadOnly(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)

	_, err := fixture.manager.Create(context.Background(), CreateRequest{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, TaskEnvironmentID: fixture.environmentID,
		RepositoryID: fixture.repositoryID, RepositoryPath: fixture.repositoryPath, BaseBranch: "main",
		WorktreeID: fixture.worktreeID, ReuseRequired: true,
	})
	if err == nil || !strings.Contains(err.Error(), ErrReuseWorktreeUnavailable.Error()) {
		t.Fatalf("attach-only Create error = %v, want %v", err, ErrReuseWorktreeUnavailable)
	}
	assertMissingCheckoutUnchanged(t, fixture)
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.2
func TestMissingCheckoutRecoveryRefusesBusyEnvironmentBeforeMutation(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *missingCheckoutFixture)
	}{
		{
			name: "requester runtime",
			setup: func(t *testing.T, fixture *missingCheckoutFixture) {
				t.Helper()
				repo, err := tasksqlite.NewWithDB(fixture.store.db, fixture.store.db, nil)
				if err != nil {
					t.Fatalf("open task repository: %v", err)
				}
				if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
					ID: "runtime-missing-checkout", SessionID: fixture.sessionID, TaskID: fixture.taskID,
					AgentExecutionID: "execution-missing-checkout", Status: "ready",
				}); err != nil {
					t.Fatalf("seed requesting session runtime: %v", err)
				}
			},
		},
		{
			name: "same-task session",
			setup: func(t *testing.T, fixture *missingCheckoutFixture) {
				t.Helper()
				fixture.addSession(t, "session-sibling", fixture.taskID, "RUNNING")
			},
		},
		{
			name: "borrowing task session",
			setup: func(t *testing.T, fixture *missingCheckoutFixture) {
				t.Helper()
				fixture.addSession(t, "session-borrower", "task-borrower", "RUNNING")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMissingCheckoutFixture(t)
			fixture.removeCheckout(t, false)
			tt.setup(t, fixture)

			if _, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
				t.Fatal("AdmitRecovery succeeded while a live environment consumer remained")
			}
			assertMissingCheckoutUnchanged(t, fixture)
		})
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.2
func TestMissingCheckoutRecoveryRefusesLivenessReadFailureBeforeMutation(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	if _, err := fixture.store.db.ExecContext(context.Background(), `DROP TABLE executors_running`); err != nil {
		t.Fatalf("disable runtime liveness query: %v", err)
	}

	if _, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
		t.Fatal("AdmitRecovery succeeded when the environment liveness query failed")
	}
	assertMissingCheckoutUnchanged(t, fixture)
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.2
func TestMissingCheckoutRecoveryRefusesCompetingClaimBeforeMutation(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	claim, err := fixture.store.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: fixture.environmentID, OwnerTaskID: fixture.taskID, OwnershipGeneration: 1,
		SessionID: "session-competing-recovery", OperationID: "123e4567-e89b-12d3-a456-426614174000",
		ExecutorType: "worktree",
	})
	if err != nil {
		t.Fatalf("seed competing recovery claim: %v", err)
	}
	defer func() {
		if err := fixture.store.ReleaseTaskEnvironmentRecoveryClaim(context.Background(), claim); err != nil {
			t.Errorf("release competing recovery claim: %v", err)
		}
	}()

	if _, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
		t.Fatal("AdmitRecovery succeeded while another recovery claim owned the environment")
	}
	assertMissingCheckoutUnchanged(t, fixture)
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.3
func TestMissingCheckoutRecoveryRequiresAnActiveCanonicalRow(t *testing.T) {
	for _, state := range []string{"failed", "deleted"} {
		t.Run(state, func(t *testing.T) {
			fixture := newMissingCheckoutFixture(t)
			fixture.removeCheckout(t, false)
			worktree, err := fixture.store.GetWorktreeByID(context.Background(), fixture.worktreeID)
			if err != nil || worktree == nil {
				t.Fatalf("load canonical worktree: worktree=%+v error=%v", worktree, err)
			}
			if state == "failed" {
				worktree.Status = "failed"
			} else {
				deletedAt := time.Now().UTC()
				worktree.DeletedAt = &deletedAt
			}
			if err := fixture.store.UpdateWorktree(context.Background(), worktree); err != nil {
				t.Fatalf("mark canonical worktree %s: %v", state, err)
			}

			if _, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
				t.Fatalf("AdmitRecovery accepted a %s worktree row", state)
			}
			if _, err := os.Lstat(fixture.worktreePath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s row recovery changed missing checkout path: %v", state, err)
			}
		})
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.3
func TestMissingCheckoutRecoveryRefusesMismatchedTaskRootMarker(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	marker, err := json.Marshal(storageworkspaces.OwnershipMarker{
		TaskID: fixture.taskID, TaskDirName: "another-task-root",
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	})
	if err != nil {
		t.Fatalf("encode conflicting ownership marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(fixture.worktreePath), storageworkspaces.OwnershipMarkerFilename), marker, 0o600); err != nil {
		t.Fatalf("replace task-root ownership marker: %v", err)
	}

	if _, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
		t.Fatal("AdmitRecovery accepted a managed root with a different stable task-directory identity")
	}
	assertMissingCheckoutUnchanged(t, fixture)
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.3
func TestMissingCheckoutRecoveryAllowsTransferredEnvironmentOwner(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	const newOwnerTaskID = "task-missing-checkout-new-owner"
	ctx := context.Background()
	if _, err := fixture.store.db.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, 'workspace', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, newOwnerTaskID, newOwnerTaskID); err != nil {
		t.Fatalf("seed transferred task owner: %v", err)
	}
	if _, err := fixture.store.db.ExecContext(ctx, `
		UPDATE task_environments SET task_id = ?, ownership_generation = 2 WHERE id = ?
	`, newOwnerTaskID, fixture.environmentID); err != nil {
		t.Fatalf("transfer task environment owner: %v", err)
	}
	fixture.request.TaskID = newOwnerTaskID
	fixture.request.OwnerTaskID = newOwnerTaskID
	fixture.request.OwnershipGeneration = 2

	assertMissingCheckoutRecovered(t, fixture)
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.5
func TestMissingCheckoutRecoveryResumesInterruptedOperation(t *testing.T) {
	for _, phase := range []string{"before_git_add", "after_git_add"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newMissingCheckoutFixture(t)
			fixture.removeCheckout(t, false)
			operationID := "123e4567-e89b-12d3-a456-426614174001"
			fixture.writePendingOperationRecord(t, operationID)
			_, err := fixture.store.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
				TaskEnvironmentID: fixture.environmentID, OwnerTaskID: fixture.taskID, OwnershipGeneration: 1,
				SessionID: fixture.sessionID, OperationID: operationID, ExecutorType: "worktree",
			})
			if err != nil {
				t.Fatalf("seed interrupted operation claim: %v", err)
			}
			if phase == "after_git_add" {
				runGit(t, fixture.repositoryPath, "worktree", "add", fixture.worktreePath, fixture.branch)
			}

			admission, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
			if err != nil {
				t.Fatalf("resume missing-checkout operation: %v", err)
			}
			if admission == nil || admission.Claim().OperationID != operationID {
				t.Fatalf("resumed admission claim = %+v, want operation %q", admission, operationID)
			}
			defer func() {
				if err := admission.Release(context.Background()); err != nil {
					t.Errorf("release resumed admission: %v", err)
				}
			}()
			if head := strings.TrimSpace(runGit(t, fixture.worktreePath, "rev-parse", "HEAD")); head != fixture.branchHead {
				t.Fatalf("resumed checkout HEAD = %q, want %q", head, fixture.branchHead)
			}
			record := fixture.readOperationRecord(t)
			if record.State != missingCheckoutRecordComplete || record.OperationID != operationID {
				t.Fatalf("completed operation record = %+v, want completed %q", record, operationID)
			}
		})
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.8
func TestMissingCheckoutRecoveryRejectsStaleGenerationWithoutMutation(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	fixture.writePendingOperationRecord(t, "123e4567-e89b-12d3-a456-426614174002")
	fixture.request.OwnershipGeneration++

	if _, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
		t.Fatal("AdmitRecovery accepted an operation from a different ownership generation")
	}
	assertMissingCheckoutUnchanged(t, fixture)
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.4
func TestMissingCheckoutRecoveryPreflightsEverySlotBeforeMutation(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	invalid := fixture.addWorktreeSlot(t, "wt-z-invalid", "repo-invalid", "invalid", "feature-unavailable", false)
	fixture.request.Slots = append(fixture.request.Slots, RecoverySlot{
		WorktreeID: invalid.ID, RepositoryID: invalid.RepositoryID, BranchSlug: invalid.BranchSlug,
		RepositoryPath: fixture.repositoryPath, Worktree: invalid,
	})

	if _, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request); err == nil {
		t.Fatal("AdmitRecovery accepted an inventory with an unavailable branch")
	}
	assertMissingCheckoutUnchanged(t, fixture)
	if _, err := os.Lstat(invalid.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid slot path changed after failed preflight: %v", err)
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.4
func TestMissingCheckoutRecoveryPreservesCompletedSlotWhenLaterSlotFails(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	invalid := fixture.addWorktreeSlot(t, "wt-z-invalid", "repo-invalid", "invalid", "feature-unavailable", false)
	request := fixture.request
	request.Slots = append(request.Slots, RecoverySlot{
		WorktreeID: invalid.ID, RepositoryID: invalid.RepositoryID, BranchSlug: invalid.BranchSlug,
		RepositoryPath: fixture.repositoryPath, Worktree: invalid,
	})
	claim, err := fixture.store.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: fixture.environmentID, OwnerTaskID: fixture.taskID, OwnershipGeneration: 1,
		SessionID: fixture.sessionID, OperationID: "123e4567-e89b-12d3-a456-426614174003", ExecutorType: "worktree",
	})
	if err != nil {
		t.Fatalf("acquire partial recovery claim: %v", err)
	}
	defer func() {
		if err := fixture.store.ReleaseTaskEnvironmentRecoveryClaim(context.Background(), claim); err != nil {
			t.Errorf("release partial recovery claim: %v", err)
		}
	}()

	indices := []int{0, 1}
	locks, err := fixture.manager.lockRecoverySlots(context.Background(), &request, indices)
	if err != nil {
		t.Fatalf("lock recovery slots: %v", err)
	}
	defer func() {
		for index := len(locks) - 1; index >= 0; index-- {
			locks[index].Unlock()
		}
	}()
	var outcome string
	if err := fixture.manager.recoverClaimedRecoverySlots(context.Background(), &request, indices, claim, &outcome); err == nil {
		t.Fatal("recovery succeeded despite the later unavailable branch")
	}
	if _, err := os.Stat(fixture.worktreePath); err != nil {
		t.Fatalf("completed first slot was not preserved: %v", err)
	}
	if _, err := os.Lstat(invalid.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed later slot path changed: %v", err)
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.4
func TestMissingCheckoutRecoveryLeavesHealthySlotUnchanged(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	secondary := fixture.addWorktreeSlot(t, "wt-secondary", "repo-secondary", "secondary", "feature/secondary", true)
	secondaryHead := strings.TrimSpace(runGit(t, secondary.Path, "rev-parse", "HEAD"))
	fixture.request.Slots = append(fixture.request.Slots, RecoverySlot{
		WorktreeID: secondary.ID, RepositoryID: secondary.RepositoryID, BranchSlug: secondary.BranchSlug,
		RepositoryPath: fixture.repositoryPath, Worktree: secondary,
	})
	fixture.removeCheckout(t, false)

	admission, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	if err != nil || admission == nil {
		t.Fatalf("admit multi-slot recovery: admission=%+v error=%v", admission, err)
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release multi-slot admission: %v", err)
		}
	}()
	if len(fixture.request.Slots) != 2 {
		t.Fatalf("selected recovery inventory has %d slots, want 2", len(fixture.request.Slots))
	}
	for index, slot := range fixture.request.Slots {
		if slot.Worktree == nil || slot.Worktree.ID != slot.WorktreeID || slot.Worktree.RepositoryID != slot.RepositoryID {
			t.Fatalf("selected slot %d is not fully projected: %+v", index, slot)
		}
	}
	rows, err := fixture.store.ListActiveWorktrees(context.Background())
	if err != nil {
		t.Fatalf("list canonical worktree inventory: %v", err)
	}
	canonical := make(map[string]*Worktree)
	for _, row := range rows {
		if row.TaskEnvironmentID == fixture.environmentID {
			canonical[row.ID] = row
		}
	}
	if len(canonical) != 2 || canonical[fixture.worktreeID] == nil || canonical[secondary.ID] == nil {
		t.Fatalf("canonical environment inventory = %+v, want both recovered and healthy slots", canonical)
	}
	restored := canonical[fixture.worktreeID]
	if filepath.Clean(restored.Path) != filepath.Clean(fixture.worktreePath) || restored.Branch != fixture.branch {
		t.Fatalf("restored canonical slot = %+v, want original path %q and branch %q", restored, fixture.worktreePath, fixture.branch)
	}
	if head := strings.TrimSpace(runGit(t, secondary.Path, "rev-parse", "HEAD")); head != secondaryHead {
		t.Fatalf("healthy slot HEAD = %q after recovery, want unchanged %q", head, secondaryHead)
	}
}

type missingCheckoutFixture struct {
	store          *SQLiteStore
	manager        *Manager
	request        RecoveryAdmissionRequest
	taskID         string
	sessionID      string
	environmentID  string
	worktreeID     string
	taskDirName    string
	repositoryID   string
	branchSlug     string
	branch         string
	repositoryPath string
	worktreePath   string
	branchHead     string
}

func newMissingCheckoutFixture(t *testing.T) *missingCheckoutFixture {
	t.Helper()
	ctx := context.Background()
	f := &missingCheckoutFixture{
		store: newTestStore(t), taskID: "task-missing-checkout", sessionID: "session-missing-checkout",
		environmentID: "env-session-missing-checkout", worktreeID: "wt-missing-checkout",
		taskDirName: "task-missing-checkout-root", repositoryID: "repo-missing-checkout",
		branchSlug: "primary", branch: "feature/pr-branch",
	}
	f.store.seedSessionWithEnvironment(t, f.sessionID, f.taskID)
	if _, err := f.store.db.ExecContext(ctx,
		`UPDATE task_environments SET task_dir_name = ? WHERE id = ?`, f.taskDirName, f.environmentID); err != nil {
		t.Fatalf("set stable task directory name: %v", err)
	}
	f.repositoryPath = initGitRepoWithRemote(t)
	cfg := newTestConfig(t)
	manager, err := NewManager(cfg, f.store, newTestLogger())
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}
	f.manager = manager
	f.worktreePath, err = cfg.TaskWorktreePath(f.taskDirName, "repository", f.branchSlug)
	if err != nil {
		t.Fatalf("build worktree path: %v", err)
	}
	taskRoot := filepath.Dir(f.worktreePath)
	if err := os.MkdirAll(taskRoot, 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	if err := storageworkspaces.WriteOwnershipMarker(taskRoot, storageworkspaces.OwnershipMarker{
		TaskID: f.taskID, TaskDirName: f.taskDirName, LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write task root ownership marker: %v", err)
	}
	runGit(t, f.repositoryPath, "worktree", "add", f.worktreePath, f.branch)
	f.branchHead = strings.TrimSpace(runGit(t, f.repositoryPath, "rev-parse", "refs/heads/"+f.branch))
	worktree := &Worktree{
		ID: f.worktreeID, SessionID: f.sessionID, TaskID: f.taskID, TaskDirName: f.taskDirName,
		TaskEnvironmentID: f.environmentID, RepositoryID: f.repositoryID, BranchSlug: f.branchSlug,
		RepositoryPath: f.repositoryPath, Path: f.worktreePath, Branch: f.branch,
		BaseBranch: "main", Status: StatusActive,
	}
	if err := f.store.CreateWorktree(ctx, worktree); err != nil {
		t.Fatalf("persist canonical worktree: %v", err)
	}
	f.request = RecoveryAdmissionRequest{
		TaskID: f.taskID, SessionID: f.sessionID, TaskEnvironmentID: f.environmentID,
		OwnerTaskID: f.taskID, OwnershipGeneration: 1, ExecutorType: "worktree",
		Slots: []RecoverySlot{{WorktreeID: f.worktreeID, RepositoryID: f.repositoryID,
			BranchSlug: f.branchSlug, RepositoryPath: f.repositoryPath, Worktree: worktree}},
	}
	return f
}

func assertMissingCheckoutRecovered(t *testing.T, fixture *missingCheckoutFixture) {
	t.Helper()
	ctx := context.Background()
	admission, err := fixture.manager.AdmitRecovery(ctx, fixture.request)
	if err != nil {
		t.Fatalf("admit missing-checkout recovery: %v", err)
	}
	if admission == nil {
		t.Fatal("AdmitRecovery returned no admission for a missing canonical checkout")
	}
	defer func() {
		if err := admission.Release(ctx); err != nil {
			t.Errorf("release recovery admission: %v", err)
		}
	}()
	if head := strings.TrimSpace(runGit(t, fixture.worktreePath, "rev-parse", "HEAD")); head != fixture.branchHead {
		t.Fatalf("restored HEAD = %q, want recorded branch head %q", head, fixture.branchHead)
	}
	persisted, err := fixture.store.GetWorktreeByID(ctx, fixture.worktreeID)
	if err != nil {
		t.Fatalf("reload canonical worktree: %v", err)
	}
	if persisted == nil || persisted.ID != fixture.worktreeID || filepath.Clean(persisted.Path) != filepath.Clean(fixture.worktreePath) || persisted.Branch != fixture.branch {
		t.Fatalf("persisted worktree = %+v, want original ID %q path %q branch %q", persisted, fixture.worktreeID, fixture.worktreePath, fixture.branch)
	}
}

func (f *missingCheckoutFixture) removeCheckout(t *testing.T, retainGitRegistration bool) {
	t.Helper()
	if retainGitRegistration {
		if err := os.RemoveAll(f.worktreePath); err != nil {
			t.Fatalf("remove checkout while retaining Git registration: %v", err)
		}
		return
	}
	runGit(t, f.repositoryPath, "worktree", "remove", "--force", f.worktreePath)
	runGit(t, f.repositoryPath, "worktree", "prune")
}

func (f *missingCheckoutFixture) addWorktreeSlot(
	t *testing.T,
	worktreeID, repositoryID, branchSlug, branch string,
	materialize bool,
) *Worktree {
	t.Helper()
	path := filepath.Join(filepath.Dir(f.worktreePath), "repository-"+branchSlug)
	if materialize {
		runGit(t, f.repositoryPath, "branch", branch, f.branchHead)
		runGit(t, f.repositoryPath, "worktree", "add", path, branch)
	}
	worktree := &Worktree{
		ID: worktreeID, SessionID: f.sessionID, TaskID: f.taskID, TaskDirName: f.taskDirName,
		TaskEnvironmentID: f.environmentID, RepositoryID: repositoryID, BranchSlug: branchSlug,
		RepositoryPath: f.repositoryPath, Path: path, Branch: branch, BaseBranch: "main", Status: StatusActive,
	}
	if err := f.store.CreateWorktree(context.Background(), worktree); err != nil {
		t.Fatalf("persist additional worktree slot %q: %v", worktreeID, err)
	}
	return worktree
}

func (f *missingCheckoutFixture) addSession(t *testing.T, sessionID, taskID, state string) {
	t.Helper()
	ctx := context.Background()
	if taskID != f.taskID {
		if _, err := f.store.db.ExecContext(ctx, `
			INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
			VALUES (?, 'workspace', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, taskID, taskID); err != nil {
			t.Fatalf("seed borrower task: %v", err)
		}
	}
	if _, err := f.store.db.ExecContext(ctx, `
		INSERT INTO task_sessions (id, task_id, state, task_environment_id, started_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, sessionID, taskID, state, f.environmentID); err != nil {
		t.Fatalf("seed live environment consumer %q: %v", sessionID, err)
	}
}

func (f *missingCheckoutFixture) writePendingOperationRecord(t *testing.T, operationID string) {
	t.Helper()
	f.writeOperationRecord(t, f.request.Slots[0].Worktree, operationID, missingCheckoutRecordInProgress)
}

func (f *missingCheckoutFixture) writeOperationRecord(
	t *testing.T,
	worktree *Worktree,
	operationID string,
	state missingCheckoutRecordState,
) {
	t.Helper()
	location, managed, err := f.manager.missingCheckoutLocation(worktree)
	if err != nil || !managed {
		t.Fatalf("resolve task root for operation record: managed=%v error=%v", managed, err)
	}
	parent, err := storageworkspaces.OpenDirectoryNoFollow(location.tasksBase, location.taskRoot)
	if err != nil {
		t.Fatalf("open task root for operation record: %v", err)
	}
	defer func() { _ = parent.Close() }()
	head := f.branchHead
	if worktree.Branch != f.branch {
		head = strings.TrimSpace(runGit(t, f.repositoryPath, "rev-parse", "refs/heads/"+worktree.Branch))
	}
	record := missingCheckoutRecoveryRecord{
		Version: 1, OperationID: operationID, State: state,
		TaskID: worktree.TaskID, TaskEnvironmentID: worktree.TaskEnvironmentID, OwnershipGeneration: 1,
		TaskDirName: worktree.TaskDirName, WorktreeID: worktree.ID, RepositoryID: worktree.RepositoryID,
		RepositoryPath: worktree.RepositoryPath, BranchSlug: worktree.BranchSlug, Path: worktree.Path,
		Branch: worktree.Branch, Head: head, Source: missingCheckoutSourceLocal, UpdatedAt: time.Now().UTC(),
	}
	if err := writeMissingCheckoutRecord(parent, location.name+missingCheckoutRecordSuffix, record); err != nil {
		t.Fatalf("write pending missing-checkout record: %v", err)
	}
}

type directorySnapshotEntry struct {
	Mode    os.FileMode
	Content string
	Link    string
}

func snapshotDirectoryTree(t *testing.T, root string) map[string]directorySnapshotEntry {
	t.Helper()
	snapshot := make(map[string]directorySnapshotEntry)
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := directorySnapshotEntry{Mode: info.Mode()}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			entry.Link, err = os.Readlink(path)
		case info.Mode().IsRegular():
			var content []byte
			content, err = os.ReadFile(path)
			entry.Content = string(content)
		}
		if err != nil {
			return err
		}
		snapshot[relative] = entry
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot directory tree %q: %v", root, err)
	}
	return snapshot
}

func runMissingCheckoutRestoreWithHooks(
	t *testing.T,
	fixture *missingCheckoutFixture,
	hooks missingCheckoutRestoreHooks,
) error {
	t.Helper()
	ctx := context.Background()
	const operationID = "f94931a8-f793-49b3-8a08-f42b9228ea4c"
	request := fixture.request
	slot := &request.Slots[0]
	request.OperationID = operationID
	operationLock, err := fixture.manager.acquireMissingCheckoutOperationLock(ctx, *slot)
	if err != nil {
		t.Fatalf("acquire missing-checkout operation lock: %v", err)
	}
	defer func() { _ = operationLock.Close() }()
	claim, err := fixture.store.AcquireTaskEnvironmentRecoveryClaim(ctx, models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: fixture.environmentID, OwnerTaskID: fixture.taskID,
		OwnershipGeneration: request.OwnershipGeneration, SessionID: fixture.sessionID,
		OperationID: operationID, ExecutorType: request.ExecutorType,
	})
	if err != nil {
		t.Fatalf("acquire missing-checkout test claim: %v", err)
	}
	defer func() {
		if err := fixture.store.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
			t.Errorf("release missing-checkout test claim: %v", err)
		}
	}()
	return fixture.manager.restoreMissingCheckoutWithHooks(ctx, &request, slot, claim, hooks)
}

func (f *missingCheckoutFixture) readOperationRecord(t *testing.T) missingCheckoutRecoveryRecord {
	t.Helper()
	return f.readOperationRecordFor(t, f.request.Slots[0].Worktree)
}

func (f *missingCheckoutFixture) readOperationRecordFor(t *testing.T, worktree *Worktree) missingCheckoutRecoveryRecord {
	t.Helper()
	location, managed, err := f.manager.missingCheckoutLocation(worktree)
	if err != nil || !managed {
		t.Fatalf("resolve task root for operation record: managed=%v error=%v", managed, err)
	}
	parent, err := storageworkspaces.OpenDirectoryNoFollow(location.tasksBase, location.taskRoot)
	if err != nil {
		t.Fatalf("open task root for operation record: %v", err)
	}
	defer func() { _ = parent.Close() }()
	record, err := readMissingCheckoutRecord(parent, location.name+missingCheckoutRecordSuffix)
	if err != nil || record == nil {
		t.Fatalf("read missing-checkout operation record: record=%+v error=%v", record, err)
	}
	return *record
}

func assertMissingCheckoutUnchanged(t *testing.T, fixture *missingCheckoutFixture) {
	t.Helper()
	if _, err := os.Lstat(fixture.worktreePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing checkout path changed after refusal: %v", err)
	}
	registered, err := worktreeRegistrationExists(context.Background(), fixture.repositoryPath, fixture.worktreePath)
	if err != nil {
		t.Fatalf("inspect registration after refusal: %v", err)
	}
	if registered {
		t.Fatal("refused recovery created a Git registration")
	}
	worktree, err := fixture.store.GetWorktreeByID(context.Background(), fixture.worktreeID)
	if err != nil || worktree == nil || worktree.Path != fixture.worktreePath || worktree.Branch != fixture.branch {
		t.Fatalf("canonical row changed after refusal: worktree=%+v error=%v", worktree, err)
	}
}
