package worktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.9
func TestMissingCheckoutRecoveryUsesTheSurvivingBranchIdentity(t *testing.T) {
	t.Run("origin branch without tracking ref", func(t *testing.T) {
		fixture := newMissingCheckoutFixture(t)
		fixture.removeCheckout(t, false)
		runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
		runGit(t, fixture.repositoryPath, "update-ref", "-d", "refs/remotes/origin/"+fixture.branch)

		assertMissingCheckoutRecovered(t, fixture)
	})

	t.Run("recorded remote branch", func(t *testing.T) {
		fixture := newMissingCheckoutFixture(t)
		fixture.removeCheckout(t, false)
		runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)

		assertMissingCheckoutRecovered(t, fixture)
	})

	t.Run("compaction recovery head", func(t *testing.T) {
		fixture := newMissingCheckoutFixture(t)
		fixture.removeCheckout(t, false)
		compactedAt := time.Now().UTC()
		worktree, err := fixture.store.GetWorktreeByID(context.Background(), fixture.worktreeID)
		if err != nil || worktree == nil {
			t.Fatalf("load canonical worktree for compaction metadata: worktree=%+v error=%v", worktree, err)
		}
		worktree.BranchOwner = BranchOwnerManaged
		worktree.RecoveryHeadSHA = fixture.branchHead
		worktree.BranchCompactedAt = &compactedAt
		if err := fixture.store.UpdateWorktree(context.Background(), worktree); err != nil {
			t.Fatalf("persist compaction recovery identity: %v", err)
		}
		fixture.refreshSelectionSnapshot(t)
		runGit(t, fixture.repositoryPath, "update-ref", recoveryRefName(fixture.worktreeID), fixture.branchHead)
		runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
		runGit(t, fixture.repositoryPath, "update-ref", "-d", "refs/remotes/origin/"+fixture.branch)

		assertMissingCheckoutRecovered(t, fixture)
	})
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.9
func TestMissingCheckoutRecoveryPreservesConfirmedBranchLossContract(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	runGit(t, fixture.repositoryPath, "push", "origin", "--delete", fixture.branch)
	runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
	runGit(t, fixture.repositoryPath, "fetch", "--prune", "origin")

	_, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	if !errors.Is(err, ErrBranchUnrecoverable) {
		t.Fatalf("AdmitRecovery error = %v, want ErrBranchUnrecoverable", err)
	}
	var branchErr *BranchUnrecoverableError
	if !errors.As(err, &branchErr) || branchErr.BranchName() != fixture.branch {
		t.Fatalf("AdmitRecovery error = %v, want branch-specific loss for %q", err, fixture.branch)
	}
	assertMissingCheckoutUnchanged(t, fixture)
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.8
func TestMissingCheckoutRecoveryLeavesCompletedMatchingClaimForRestartReconciliation(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	second := fixture.addWorktreeSlot(t, "wt-missing-checkout-secondary", "repo-missing-checkout-secondary", "secondary", "feature/secondary", true)
	fixture.request.Slots = append(fixture.request.Slots, RecoverySlot{
		WorktreeID: second.ID, RepositoryID: second.RepositoryID, BranchSlug: second.BranchSlug,
		RepositoryPath: fixture.repositoryPath, Worktree: second,
	})
	operationID := "94b7f766-95db-4f45-9752-1f4dcbf62e2e"
	fixture.writeOperationRecord(t, fixture.request.Slots[0].Worktree, operationID, missingCheckoutRecordComplete)
	fixture.writeOperationRecord(t, second, operationID, missingCheckoutRecordComplete)
	claim, err := fixture.store.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: fixture.environmentID, OwnerTaskID: fixture.taskID,
		OwnershipGeneration: fixture.request.OwnershipGeneration, SessionID: fixture.sessionID,
		OperationID: operationID, ExecutorType: fixture.request.ExecutorType,
	})
	if err != nil {
		t.Fatalf("seed claim left by completed recovery: %v", err)
	}

	manager, err := NewManager(fixture.manager.config, fixture.store, newTestLogger())
	if err != nil {
		t.Fatalf("create restarted recovery manager: %v", err)
	}
	admission, err := manager.AdmitRecovery(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("reconcile completed recovery claim: %v", err)
	}
	if admission != nil {
		if err := admission.Release(context.Background()); err != nil {
			t.Fatalf("release unexpected admission: %v", err)
		}
		t.Fatal("completed operation reconciliation unexpectedly returned an admission")
	}
	remaining, err := fixture.store.GetTaskEnvironmentRecoveryClaim(context.Background(), fixture.environmentID)
	if err != nil {
		t.Fatalf("read claim after reconciliation: %v", err)
	}
	if remaining != nil {
		t.Fatalf("completed operation claim remains: %+v (original %+v)", remaining, claim)
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.8
func TestMissingCheckoutRecoveryContinuesMultiSlotClaimFromCompletedRecord(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	second := fixture.addWorktreeSlot(t, "wt-missing-checkout-next", "repo-missing-checkout-next", "next", "feature/next", true)
	runGit(t, fixture.repositoryPath, "worktree", "remove", "--force", second.Path)
	operationID := "5507569d-4a1e-4e78-82bf-8f33c11c91ac"
	fixture.writeOperationRecord(t, fixture.request.Slots[0].Worktree, operationID, missingCheckoutRecordComplete)
	fixture.request.Slots = append(fixture.request.Slots, RecoverySlot{
		WorktreeID: second.ID, RepositoryID: second.RepositoryID, BranchSlug: second.BranchSlug,
		RepositoryPath: fixture.repositoryPath, Worktree: second,
	})
	if _, err := fixture.store.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: fixture.environmentID, OwnerTaskID: fixture.taskID,
		OwnershipGeneration: fixture.request.OwnershipGeneration, SessionID: fixture.sessionID,
		OperationID: operationID, ExecutorType: fixture.request.ExecutorType,
	}); err != nil {
		t.Fatalf("seed interrupted multi-slot claim: %v", err)
	}

	restarted, err := NewManager(fixture.manager.config, fixture.store, newTestLogger())
	if err != nil {
		t.Fatalf("create restarted multi-slot manager: %v", err)
	}
	admission, err := restarted.AdmitRecovery(context.Background(), fixture.request)
	if err != nil || admission == nil || admission.Claim().OperationID != operationID {
		t.Fatalf("continue interrupted multi-slot operation: admission=%+v err=%v", admission, err)
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release continued multi-slot admission: %v", err)
		}
	}()
	if head := strings.TrimSpace(runGit(t, second.Path, "rev-parse", "HEAD")); head != strings.TrimSpace(runGit(t, fixture.repositoryPath, "rev-parse", "refs/heads/feature/next")) {
		t.Fatalf("continued slot HEAD = %q, want surviving branch head", head)
	}
	if record := fixture.readOperationRecordFor(t, second); record.OperationID != operationID || record.State != missingCheckoutRecordComplete {
		t.Fatalf("continued slot operation record = %+v, want completed operation %q", record, operationID)
	}
}

func TestMissingCheckoutRecoveryIgnoresUnrelatedClaimForHealthyCheckout(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	operationID := "c3af05d8-3017-47c9-8944-69c7799ce10f"
	claim, err := fixture.store.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: fixture.environmentID, OwnerTaskID: fixture.taskID,
		OwnershipGeneration: fixture.request.OwnershipGeneration, SessionID: fixture.sessionID,
		OperationID: operationID, ExecutorType: fixture.request.ExecutorType,
	})
	if err != nil {
		t.Fatalf("seed unrelated environment claim: %v", err)
	}
	defer func() {
		if err := fixture.store.ReleaseTaskEnvironmentRecoveryClaim(context.Background(), claim); err != nil {
			t.Errorf("release unrelated environment claim: %v", err)
		}
	}()

	admission, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	if err != nil || admission != nil {
		if admission != nil {
			_ = admission.Release(context.Background())
		}
		t.Fatalf("healthy checkout admission = %v, %v; want no recovery and no error", admission, err)
	}
	current, err := fixture.store.GetTaskEnvironmentRecoveryClaim(context.Background(), fixture.environmentID)
	if err != nil || current == nil || current.OperationID != operationID {
		t.Fatalf("unrelated claim after ordinary admission = %+v, err=%v", current, err)
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.8
func TestMissingCheckoutRecoverySameOperationReplayIsExcludedAcrossManagers(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	operationID := "7b4b8b18-536b-41a6-92fe-f624252bbd07"
	fixture.writePendingOperationRecord(t, operationID)

	first, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	if err != nil || first == nil {
		t.Fatalf("first operation admission = %v, %v", first, err)
	}
	defer func() {
		if err := first.Release(context.Background()); err != nil {
			t.Errorf("release first operation admission: %v", err)
		}
	}()
	if record := fixture.readOperationRecord(t); record.State != missingCheckoutRecordComplete || record.OperationID != operationID {
		t.Fatalf("first operation record = %+v, want completed operation %q", record, operationID)
	}

	secondManager, err := NewManager(fixture.manager.config, fixture.store, newTestLogger())
	if err != nil {
		t.Fatalf("create independent recovery manager: %v", err)
	}
	second, err := secondManager.AdmitRecovery(context.Background(), fixture.request)
	if err == nil {
		if second != nil {
			_ = second.Release(context.Background())
		}
		t.Fatal("same-operation replay acquired authority while the first admission retained its OS lock")
	}
	claim, claimErr := fixture.store.GetTaskEnvironmentRecoveryClaim(context.Background(), fixture.environmentID)
	if claimErr != nil || claim == nil || claim.OperationID != operationID {
		t.Fatalf("active operation claim after replay refusal = %+v, err=%v", claim, claimErr)
	}
}

func TestMissingCheckoutRecoveryRefreshesRemoteOnlyBranchHeadOnReplay(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	newHead := advanceOriginBranch(t, fixture.repositoryPath, fixture.branch)
	runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
	operationID := "8b51ed95-6078-402d-9391-813c72ca2bf0"
	fixture.writeRemoteOperationRecord(t, fixture.request.Slots[0].Worktree, operationID, missingCheckoutRecordInProgress, fixture.branchHead)

	admission, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	if err != nil || admission == nil {
		t.Fatalf("replay remote-only operation after origin advanced: admission=%+v err=%v", admission, err)
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release refreshed remote operation admission: %v", err)
		}
	}()
	if head := strings.TrimSpace(runGit(t, fixture.worktreePath, "rev-parse", "HEAD")); head != newHead {
		t.Fatalf("replayed checkout HEAD = %q, want refreshed origin head %q", head, newHead)
	}
	if record := fixture.readOperationRecord(t); record.Head != newHead || record.State != missingCheckoutRecordComplete {
		t.Fatalf("replayed operation record = %+v, want completed refreshed head %q", record, newHead)
	}
}

func TestMissingCheckoutRecoveryRetriesWhenOriginAdvancesBetweenProbeAndFetch(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
	runGit(t, fixture.repositoryPath, "update-ref", "-d", "refs/remotes/origin/"+fixture.branch)
	var advancedHead string
	err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
		beforeRemoteFetch: func() error {
			advancedHead = advanceOriginBranch(t, fixture.repositoryPath, fixture.branch)
			return nil
		},
	})
	if err == nil || advancedHead == "" {
		t.Fatalf("restore between remote probe and fetch = %v, advanced head=%q; want safe interruption", err, advancedHead)
	}
	if record := fixture.readOperationRecord(t); record.State != missingCheckoutRecordInProgress || record.Head != fixture.branchHead {
		t.Fatalf("interrupted operation record = %+v, want old advertised head %q before retry", record, fixture.branchHead)
	}
	if _, err := os.Lstat(fixture.worktreePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkout path changed after fetch detected an origin advance: %v", err)
	}

	admission, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	if err != nil || admission == nil {
		t.Fatalf("retry after origin advancement: admission=%+v err=%v", admission, err)
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release retried origin advancement admission: %v", err)
		}
	}()
	if head := strings.TrimSpace(runGit(t, fixture.worktreePath, "rev-parse", "HEAD")); head != advancedHead {
		t.Fatalf("retried checkout HEAD = %q, want latest origin head %q", head, advancedHead)
	}
	if record := fixture.readOperationRecord(t); record.State != missingCheckoutRecordComplete || record.Head != advancedHead {
		t.Fatalf("retried operation record = %+v, want completed head %q", record, advancedHead)
	}
}

func TestMissingCheckoutRemoteRefCleanupRequiresRecordedHeadAndAcceptsAbsence(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	otherHead := advanceOriginBranch(t, fixture.repositoryPath, fixture.branch)
	runGit(t, fixture.repositoryPath, "fetch", "origin", fixture.branch)
	const operationID = "aefc046a-c7c9-4bbb-9799-3779f1b55b0a"
	ref := "refs/kandev/missing-checkout/" + operationID
	runGit(t, fixture.repositoryPath, "update-ref", ref, otherHead)
	if err := fixture.manager.deleteMissingCheckoutRemoteRef(context.Background(), fixture.repositoryPath, operationID, fixture.branchHead); err == nil {
		t.Fatal("cleanup deleted an operation ref that did not match the recorded head")
	}
	if got := strings.TrimSpace(runGit(t, fixture.repositoryPath, "rev-parse", ref)); got != otherHead {
		t.Fatalf("mismatched operation ref after refused cleanup = %q, want preserved %q", got, otherHead)
	}
	if err := fixture.manager.deleteMissingCheckoutRemoteRef(context.Background(), fixture.repositoryPath, operationID, otherHead); err != nil {
		t.Fatalf("delete exact operation ref: %v", err)
	}
	if err := fixture.manager.deleteMissingCheckoutRemoteRef(context.Background(), fixture.repositoryPath, operationID, otherHead); err != nil {
		t.Fatalf("treat already-absent operation ref as successful cleanup: %v", err)
	}
}

func TestMissingCheckoutRecoveryCleansRemoteOperationRefAfterInterruptedCleanup(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
	runGit(t, fixture.repositoryPath, "update-ref", "-d", "refs/remotes/origin/"+fixture.branch)
	var injected bool
	err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
		beforeRemoteRefCleanup: func() error {
			injected = true
			return errors.New("injected temporary-ref cleanup failure")
		},
	})
	if err == nil || !injected {
		t.Fatalf("first restoration = %v, cleanup hook ran=%v; want injected cleanup failure", err, injected)
	}
	operationRef := "refs/kandev/missing-checkout/" + "f94931a8-f793-49b3-8a08-f42b9228ea4c"
	if got := strings.TrimSpace(runGit(t, fixture.repositoryPath, "rev-parse", operationRef)); got != fixture.branchHead {
		t.Fatalf("orphaned operation ref = %q, want %q", got, fixture.branchHead)
	}

	admission, err := fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	if err != nil || admission == nil {
		t.Fatalf("replay interrupted checkout cleanup: admission=%+v err=%v", admission, err)
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release cleanup replay admission: %v", err)
		}
	}()
	output, err := exec.Command("git", "-C", fixture.repositoryPath, "show-ref", "--verify", "--quiet", operationRef).CombinedOutput()
	if err == nil {
		t.Fatalf("operation temporary ref %q survived successful replay cleanup", operationRef)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("temporary ref lookup failed unexpectedly: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if record := fixture.readOperationRecord(t); record.State != missingCheckoutRecordComplete {
		t.Fatalf("operation record after cleanup replay = %+v, want complete", record)
	}
}

func advanceOriginBranch(t *testing.T, repositoryPath, branch string) string {
	t.Helper()
	origin := strings.TrimSpace(runGit(t, repositoryPath, "remote", "get-url", "origin"))
	clonePath := filepath.Join(t.TempDir(), "origin-clone")
	output, err := exec.Command("git", "clone", origin, clonePath).CombinedOutput()
	if err != nil {
		t.Fatalf("clone origin to advance branch: %v: %s", err, output)
	}
	runGit(t, clonePath, "config", "user.email", "missing-checkout-test@example.invalid")
	runGit(t, clonePath, "config", "user.name", "Missing Checkout Test")
	runGit(t, clonePath, "checkout", branch)
	if err := os.WriteFile(filepath.Join(clonePath, "origin-advance.txt"), []byte("advanced "+branch+"\n"), 0o600); err != nil {
		t.Fatalf("write origin advancement: %v", err)
	}
	runGit(t, clonePath, "add", "origin-advance.txt")
	runGit(t, clonePath, "commit", "-m", "Advance origin branch for replay test")
	runGit(t, clonePath, "push", "origin", branch)
	return strings.TrimSpace(runGit(t, clonePath, "rev-parse", "HEAD"))
}

func TestMissingCheckoutRecoveryDoesNotRecordEmptyPlanWhenBranchDisappearsAtClaimedInspection(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
		beforeClaimedInspection: func() error {
			runGit(t, fixture.repositoryPath, "push", "origin", "--delete", fixture.branch)
			runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
			runGit(t, fixture.repositoryPath, "fetch", "--prune", "origin")
			return nil
		},
	})
	if !errors.Is(err, ErrBranchUnrecoverable) {
		t.Fatalf("claimed inspection error = %v, want confirmed branch loss", err)
	}
	location, managed, err := fixture.manager.missingCheckoutLocation(fixture.request.Slots[0].Worktree)
	if err != nil || !managed {
		t.Fatalf("resolve operation record location: managed=%v err=%v", managed, err)
	}
	parent, err := storageworkspaces.OpenDirectoryNoFollow(location.tasksBase, location.taskRoot)
	if err != nil {
		t.Fatalf("open managed task root: %v", err)
	}
	defer func() { _ = parent.Close() }()
	if record, err := readMissingCheckoutRecord(parent, location.name+missingCheckoutRecordSuffix); err != nil || record != nil {
		t.Fatalf("operation record after branch-loss refusal = %+v, err=%v; want no record", record, err)
	}
	if _, err := os.Lstat(fixture.worktreePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("branch-loss refusal changed checkout path: %v", err)
	}
}

func TestMissingCheckoutRecoveryCleansOnlyEmptyReservedTargetOnEarlyFailure(t *testing.T) {
	t.Run("empty target removed", func(t *testing.T) {
		fixture := newMissingCheckoutFixture(t)
		fixture.removeCheckout(t, false)
		err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
			beforeWorktreeAdd: func() error { return errors.New("stop before git add") },
		})
		if err == nil {
			t.Fatal("restore succeeded after the pre-add barrier failed")
		}
		if _, err := os.Lstat(fixture.worktreePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("empty reserved target remained after early failure: %v", err)
		}
	})

	t.Run("non-empty target preserved", func(t *testing.T) {
		fixture := newMissingCheckoutFixture(t)
		fixture.removeCheckout(t, false)
		err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
			beforeWorktreeAdd: func() error {
				return os.WriteFile(filepath.Join(fixture.worktreePath, "external-content"), []byte("preserve"), 0o600)
			},
		})
		if err == nil {
			t.Fatal("restore succeeded after external content appeared in the reserved target")
		}
		content, readErr := os.ReadFile(filepath.Join(fixture.worktreePath, "external-content"))
		if readErr != nil || string(content) != "preserve" {
			t.Fatalf("non-empty appeared target content = %q, err=%v", content, readErr)
		}
	})
}

func TestMissingCheckoutGitAddCanUsePinnedWorkingDirectoryStrategy(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	location, managed, err := fixture.manager.missingCheckoutLocation(fixture.request.Slots[0].Worktree)
	if err != nil || !managed {
		t.Fatalf("resolve managed checkout location: managed=%v err=%v", managed, err)
	}
	parent, err := storageworkspaces.OpenDirectoryNoFollow(location.tasksBase, location.taskRoot)
	if err != nil {
		t.Fatalf("open managed task root: %v", err)
	}
	defer func() { _ = parent.Close() }()
	target, err := parent.CreateSubdirectory(location.name, 0o700)
	if err != nil {
		t.Fatalf("reserve pinned target: %v", err)
	}
	defer func() { _ = target.Close() }()
	processPath, targetFile, err := target.ProcessPath(missingCheckoutChildFD)
	if err != nil {
		t.Fatalf("create pinned process path: %v", err)
	}
	defer func() { _ = targetFile.Close() }()
	plan := missingCheckoutBranchPlan{branch: fixture.branch, head: fixture.branchHead}
	if err := fixture.manager.addMissingCheckoutGitWorktreeWithStrategy(
		context.Background(), fixture.repositoryPath, processPath, targetFile, plan, true,
	); err != nil {
		t.Fatalf("git worktree add through pinned working directory: %v", err)
	}
	if head := strings.TrimSpace(runGit(t, fixture.worktreePath, "rev-parse", "HEAD")); head != fixture.branchHead {
		t.Fatalf("pinned working-directory checkout HEAD = %q, want %q", head, fixture.branchHead)
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.5
func TestMissingCheckoutRecoveryDoesNotDeleteCheckoutThatAppearsBeforeCleanup(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, true)
	otherRepository := initGitRepoWithRemote(t)
	var original map[string]directorySnapshotEntry
	err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
		beforeRegistrationCleanup: func() error {
			if _, err := exec.LookPath("git"); err != nil {
				return err
			}
			if out, err := exec.Command("git", "-C", otherRepository, "worktree", "add", "--detach", fixture.worktreePath, "main").CombinedOutput(); err != nil {
				return errors.New(strings.TrimSpace(string(out)) + ": " + err.Error())
			}
			original = snapshotDirectoryTree(t, fixture.worktreePath)
			return nil
		},
	})
	if err == nil {
		t.Fatal("restore succeeded after another checkout appeared before stale-registration cleanup")
	}
	if got := snapshotDirectoryTree(t, fixture.worktreePath); !reflect.DeepEqual(got, original) {
		t.Fatalf("appeared checkout changed during stale-registration cleanup: before=%v after=%v", original, got)
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.5
func TestMissingCheckoutRecoveryPinsGitAddAcrossPathReplacement(t *testing.T) {
	t.Run("task-root symlink replacement", func(t *testing.T) {
		fixture := newMissingCheckoutFixture(t)
		fixture.removeCheckout(t, false)
		root := filepath.Dir(fixture.worktreePath)
		movedRoot := root + ".moved"
		unrelatedRoot := filepath.Join(t.TempDir(), "unrelated")
		if err := os.MkdirAll(unrelatedRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		otherRepository := initGitRepoWithRemote(t)
		otherCheckout := filepath.Join(unrelatedRoot, filepath.Base(fixture.worktreePath))
		if out, err := exec.Command("git", "-C", otherRepository, "worktree", "add", "--detach", otherCheckout, "main").CombinedOutput(); err != nil {
			t.Fatalf("create unrelated checkout: %v: %s", err, out)
		}
		before := snapshotDirectoryTree(t, unrelatedRoot)
		err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
			beforeWorktreeAdd: func() error {
				if err := os.Rename(root, movedRoot); err != nil {
					return err
				}
				return os.Symlink(unrelatedRoot, root)
			},
		})
		if err == nil {
			t.Fatal("restore succeeded after the managed task root was replaced")
		}
		if got := snapshotDirectoryTree(t, unrelatedRoot); !reflect.DeepEqual(got, before) {
			t.Fatalf("Git changed the unrelated tree reached through the replacement symlink: before=%v after=%v", before, got)
		}
	})

	t.Run("target replacement before add", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the pinned Windows target denies rename while Git runs")
		}
		fixture := newMissingCheckoutFixture(t)
		fixture.removeCheckout(t, false)
		otherRepository := initGitRepoWithRemote(t)
		var before map[string]directorySnapshotEntry
		appearedCheckout := fixture.worktreePath
		err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
			beforeWorktreeAdd: func() error {
				reserved := fixture.worktreePath + ".reserved"
				if err := os.Rename(fixture.worktreePath, reserved); err != nil {
					return err
				}
				if out, err := exec.Command("git", "-C", otherRepository, "worktree", "add", "--detach", appearedCheckout, "main").CombinedOutput(); err != nil {
					return errors.New(strings.TrimSpace(string(out)) + ": " + err.Error())
				}
				before = snapshotDirectoryTree(t, appearedCheckout)
				return nil
			},
		})
		if err == nil {
			t.Fatal("restore succeeded after an unrelated checkout replaced the reserved target")
		}
		if got := snapshotDirectoryTree(t, appearedCheckout); !reflect.DeepEqual(got, before) {
			t.Fatalf("Git changed the appeared checkout: before=%v after=%v", before, got)
		}
	})
	t.Run("checkout appears before target reservation", func(t *testing.T) {
		fixture := newMissingCheckoutFixture(t)
		fixture.removeCheckout(t, false)
		otherRepository := initGitRepoWithRemote(t)
		var before map[string]directorySnapshotEntry
		err := runMissingCheckoutRestoreWithHooks(t, fixture, missingCheckoutRestoreHooks{
			beforeTargetReservation: func() error {
				if out, err := exec.Command("git", "-C", otherRepository, "worktree", "add", "--detach", fixture.worktreePath, "main").CombinedOutput(); err != nil {
					return errors.New(strings.TrimSpace(string(out)) + ": " + err.Error())
				}
				before = snapshotDirectoryTree(t, fixture.worktreePath)
				return nil
			},
		})
		if err == nil {
			t.Fatal("restore succeeded after another checkout appeared before target reservation")
		}
		if got := snapshotDirectoryTree(t, fixture.worktreePath); !reflect.DeepEqual(got, before) {
			t.Fatalf("checkout that appeared before target reservation changed: before=%v after=%v", before, got)
		}
	})
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.9
func TestMissingCheckoutRecoveryDistinguishesOriginProbeFailure(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
	runGit(t, fixture.repositoryPath, "update-ref", "-d", "refs/remotes/origin/"+fixture.branch)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	scriptDir := writeFakeGitScript(t, `
case "${1:-}" in
  ls-remote)
    echo "fatal: Authentication failed" >&2
    exit 128
    ;;
  *)
    exec `+realGit+` "$@"
    ;;
esac`)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err = fixture.manager.AdmitRecovery(context.Background(), fixture.request)
	if errors.Is(err, ErrBranchUnrecoverable) {
		t.Fatalf("origin probe failure was reported as confirmed branch loss: %v", err)
	}
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("origin probe error = %v, want ErrAuthFailed", err)
	}
	assertMissingCheckoutUnchanged(t, fixture)
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-004.9
func TestMissingCheckoutRecoveryExplicitNewBranchUsesClaimedPreflight(t *testing.T) {
	fixture := newMissingCheckoutFixture(t)
	fixture.removeCheckout(t, false)
	runGit(t, fixture.repositoryPath, "push", "origin", "--delete", fixture.branch)
	runGit(t, fixture.repositoryPath, "branch", "-D", fixture.branch)
	runGit(t, fixture.repositoryPath, "fetch", "--prune", "origin")
	request := fixture.request
	request.AllowBranchReplacement = true
	admission, err := fixture.manager.AdmitRecovery(context.Background(), request)
	if err != nil || admission == nil {
		t.Fatalf("explicit branch replacement preflight = %v, %v", admission, err)
	}
	defer func() {
		if err := admission.Release(context.Background()); err != nil {
			t.Errorf("release explicit branch replacement admission: %v", err)
		}
	}()
	if _, err := os.Lstat(fixture.worktreePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight automatically materialized a replacement checkout: %v", err)
	}
	created, err := fixture.manager.Create(WithRecoveryAdmission(context.Background(), admission), CreateRequest{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, TaskEnvironmentID: fixture.environmentID,
		RepositoryID: fixture.repositoryID, RepositoryPath: fixture.repositoryPath, BaseBranch: "main",
		WorktreeID: fixture.worktreeID, TaskDirName: fixture.taskDirName, RepoName: "repository",
		BranchSlug: fixture.branchSlug, AllowBranchReplacement: true,
	})
	if err != nil {
		t.Fatalf("authorized branch replacement after preflight: %v", err)
	}
	if created == nil || created.Branch == fixture.branch {
		t.Fatalf("explicit branch replacement result = %+v, want a different branch", created)
	}
	if _, err := os.Stat(created.Path); err != nil {
		t.Fatalf("authorized replacement checkout is unavailable at %q: %v", created.Path, err)
	}
}
