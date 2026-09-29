package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
)

const missingCheckoutChildFD = 3

func (m *Manager) fetchMissingCheckoutRemoteCommit(
	ctx context.Context,
	repositoryPath string,
	plan *missingCheckoutBranchPlan,
	operationID string,
) error {
	if plan == nil || plan.source != missingCheckoutSourceRemote || !plan.fetchRemote {
		return nil
	}
	if _, err := uuid.Parse(operationID); err != nil {
		return fmt.Errorf("missing-checkout operation identity is invalid")
	}
	remoteRef := "refs/heads/" + normalizeOriginBranchName(plan.branch)
	temporaryRef := "refs/kandev/missing-checkout/" + operationID
	output, runErr, execCtxErr := m.runGitCombinedAfterAcquire(
		ctx, m.fetchTimeout, repositoryPath, "fetch", "--no-tags", "origin", "+"+remoteRef+":"+temporaryRef,
	)
	if runErr != nil {
		if ctxErr := firstContextError(execCtxErr, runErr); ctxErr != nil {
			return ctxErr
		}
		return ClassifyGitError(string(output), runErr)
	}
	head, err := m.resolveCommit(ctx, repositoryPath, temporaryRef)
	if err != nil {
		return fmt.Errorf("resolve fetched recorded branch: %w", err)
	}
	if !strings.EqualFold(head, plan.head) {
		return fmt.Errorf("origin branch changed after preflight")
	}
	plan.head = strings.ToLower(head)
	return nil
}

func (m *Manager) deleteMissingCheckoutRemoteRef(ctx context.Context, repositoryPath, operationID, head string) error {
	if _, err := uuid.Parse(operationID); err != nil {
		return fmt.Errorf("missing-checkout operation identity is invalid")
	}
	cmd := m.newNonInteractiveGitCmd(ctx, repositoryPath, "update-ref", "-d", "refs/kandev/missing-checkout/"+operationID, head)
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("delete temporary origin reference %q: %s: %w", operationID, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) restoreMissingCheckout(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slot *RecoverySlot,
	claim *models.TaskEnvironmentRecoveryClaim,
) error {
	return m.restoreMissingCheckoutWithHooks(ctx, req, slot, claim, missingCheckoutRestoreHooks{})
}

func (m *Manager) restoreMissingCheckoutWithHooks(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slot *RecoverySlot,
	claim *models.TaskEnvironmentRecoveryClaim,
	hooks missingCheckoutRestoreHooks,
) error {
	wt := slot.Worktree
	repositoryPath := recoveryRepositoryPath(*slot)
	repoLock := m.getRepoLock(repositoryPath)
	repoLock.Lock()
	defer func() {
		repoLock.Unlock()
		m.releaseRepoLock(repositoryPath)
	}()
	targetRelease, err := acquireWorktreeTargetPath(ctx, wt.Path)
	if err != nil {
		return recoveryAdmissionError(*req, fmt.Sprintf("cannot reserve canonical checkout path: %v", err))
	}
	defer targetRelease()

	_, pathErr := os.Lstat(wt.Path)
	pathPresent := pathErr == nil
	if pathErr != nil && !errors.Is(pathErr, os.ErrNotExist) {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "cannot inspect checkout path during recovery")
	}
	inspection, err := m.inspectMissingCheckout(
		ctx, req.OwnerTaskID, req.OwnershipGeneration, slot, pathPresent, req.AllowBranchReplacement,
	)
	if err != nil {
		return err
	}
	if inspection.pathPresent {
		return m.completeInterruptedMissingCheckout(ctx, req, wt, claim, inspection)
	}
	return m.restoreMissingCheckoutOperation(ctx, req, slot, claim, inspection, repositoryPath, hooks)
}

func (m *Manager) restoreMissingCheckoutOperation(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slot *RecoverySlot,
	claim *models.TaskEnvironmentRecoveryClaim,
	inspection missingCheckoutInspection,
	repositoryPath string,
	hooks missingCheckoutRestoreHooks,
) error {
	wt := slot.Worktree
	location, managed, err := m.missingCheckoutLocation(wt)
	if err != nil || !managed {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "persisted managed task-root identity is unavailable")
	}
	parent, err := openOrCreateMissingCheckoutTaskRoot(location, wt)
	if err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, err.Error())
	}
	defer func() { _ = parent.Close() }()
	if err := parent.VerifyPath(location.taskRoot); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "managed task root changed during recovery")
	}
	if err := validateMissingCheckoutTaskRoot(parent, wt); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, err.Error())
	}

	plan, record, err := m.prepareMissingCheckoutOperation(ctx, req, slot, claim, inspection, parent, location.name)
	if err != nil {
		return err
	}
	if err := m.cleanupMissingCheckoutRegistration(ctx, req, wt, repositoryPath, plan, inspection.pruneStale, hooks); err != nil {
		return err
	}
	if err := parent.VerifyPath(location.taskRoot); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "managed task root changed before materialization")
	}
	if err := m.fetchAndRecordMissingCheckoutRemoteCommit(ctx, req, repositoryPath, parent, location.name, &plan, &record); err != nil {
		return err
	}
	if hooks.beforeTargetReservation != nil {
		if err := hooks.beforeTargetReservation(); err != nil {
			return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("target-reservation barrier: %v", err))
		}
	}
	if err := m.materializeMissingCheckoutTarget(ctx, req, wt, repositoryPath, parent, location, plan, record, hooks); err != nil {
		return err
	}
	if plan.fetchRemote {
		if err := m.deleteMissingCheckoutRemoteRef(ctx, repositoryPath, record.OperationID, record.Head); err != nil {
			return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("cannot remove temporary origin reference: %v", err))
		}
	}
	return markMissingCheckoutOperationComplete(parent, location.name, record)
}

func (m *Manager) cleanupMissingCheckoutRegistration(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	wt *Worktree,
	repositoryPath string,
	plan missingCheckoutBranchPlan,
	pruneStale bool,
	hooks missingCheckoutRestoreHooks,
) error {
	if !pruneStale {
		return nil
	}
	if hooks.beforeRegistrationCleanup != nil {
		if err := hooks.beforeRegistrationCleanup(); err != nil {
			return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("stale-registration barrier: %v", err))
		}
	}
	if err := m.removeMissingCheckoutStaleRegistration(ctx, repositoryPath, wt.Path, plan); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("cannot remove the proven stale Git metadata: %v", err))
	}
	return nil
}

func (m *Manager) fetchAndRecordMissingCheckoutRemoteCommit(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	repositoryPath string,
	parent storageworkspaces.DirectoryHandle,
	targetName string,
	plan *missingCheckoutBranchPlan,
	record *missingCheckoutRecoveryRecord,
) error {
	if !plan.fetchRemote {
		return nil
	}
	if err := m.fetchMissingCheckoutRemoteCommit(ctx, repositoryPath, plan, record.OperationID); err != nil {
		return missingCheckoutError(req.OwnerTaskID, record.Path, fmt.Sprintf("cannot fetch the recorded origin branch: %v", err))
	}
	record.Head = plan.head
	record.UpdatedAt = time.Now().UTC()
	if err := writeMissingCheckoutRecord(parent, targetName+missingCheckoutRecordSuffix, *record); err != nil {
		return missingCheckoutError(req.OwnerTaskID, record.Path, "cannot persist fetched branch head")
	}
	return nil
}

func (m *Manager) completeInterruptedMissingCheckout(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	inspection missingCheckoutInspection,
) error {
	record := inspection.record
	if record == nil || record.State != missingCheckoutRecordInProgress || record.OperationID != claim.OperationID {
		return fmt.Errorf("%w: checkout path appeared during recovery", ErrWorktreeCorrupted)
	}
	location, managed, err := m.missingCheckoutLocation(wt)
	if err != nil || !managed {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "persisted managed task-root identity is unavailable")
	}
	parent, rootMissing, err := openMissingCheckoutTaskRoot(location, false)
	if err != nil || rootMissing || parent == nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "cannot open interrupted checkout task root")
	}
	defer func() { _ = parent.Close() }()
	if err := validateMissingCheckoutTaskRoot(parent, wt); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, err.Error())
	}
	if err := m.verifyMissingCheckoutResult(ctx, wt, *record); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("interrupted checkout failed identity validation: %v", err))
	}
	return markMissingCheckoutOperationComplete(parent, location.name, *record)
}

func openOrCreateMissingCheckoutTaskRoot(
	location missingCheckoutLocation,
	wt *Worktree,
) (storageworkspaces.DirectoryHandle, error) {
	parent, rootMissing, err := openMissingCheckoutTaskRoot(location, false)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("cannot open managed task root: %w", err)
	}
	if rootMissing || errors.Is(err, os.ErrNotExist) {
		parent, _, err = openMissingCheckoutTaskRoot(location, true)
		if err != nil {
			return nil, fmt.Errorf("cannot restore managed task root: %w", err)
		}
		if err := markNewMissingCheckoutTaskRoot(parent, wt); err != nil {
			_ = parent.Close()
			return nil, err
		}
	}
	return parent, nil
}

func markNewMissingCheckoutTaskRoot(parent storageworkspaces.DirectoryHandle, wt *Worktree) error {
	entries, err := parent.ReadDir()
	if err != nil {
		return missingCheckoutError(wt.TaskID, wt.Path, "cannot inspect newly created managed task root")
	}
	if len(entries) != 0 {
		if err := validateMissingCheckoutTaskRoot(parent, wt); err != nil {
			return missingCheckoutError(wt.TaskID, wt.Path, "managed task root appeared with unexpected content")
		}
		return nil
	}
	if err := storageworkspaces.WriteOwnershipMarkerNoFollow(parent, storageworkspaces.OwnershipMarker{
		TaskID: wt.TaskID, TaskDirName: wt.TaskDirName, LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		return missingCheckoutError(wt.TaskID, wt.Path, fmt.Sprintf("cannot mark restored task root: %v", err))
	}
	return nil
}

func (m *Manager) prepareMissingCheckoutOperation(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slot *RecoverySlot,
	claim *models.TaskEnvironmentRecoveryClaim,
	inspection missingCheckoutInspection,
	parent storageworkspaces.DirectoryHandle,
	targetName string,
) (missingCheckoutBranchPlan, missingCheckoutRecoveryRecord, error) {
	var plan missingCheckoutBranchPlan
	if inspection.record != nil && inspection.record.State == missingCheckoutRecordInProgress {
		var err error
		plan, err = m.missingCheckoutPlanFromRecord(ctx, recoveryRepositoryPath(*slot), slot.Worktree, *inspection.record)
		if err != nil {
			return plan, missingCheckoutRecoveryRecord{}, missingCheckoutError(req.OwnerTaskID, slot.Worktree.Path, err.Error())
		}
		return plan, *inspection.record, nil
	}
	plan = inspection.plan
	record := missingCheckoutRecoveryRecord{
		Version: 1, OperationID: claim.OperationID, State: missingCheckoutRecordInProgress,
		TaskID: slot.Worktree.TaskID, TaskEnvironmentID: req.TaskEnvironmentID, OwnershipGeneration: req.OwnershipGeneration,
		TaskDirName: slot.Worktree.TaskDirName, WorktreeID: slot.Worktree.ID, RepositoryID: slot.Worktree.RepositoryID,
		RepositoryPath: filepath.Clean(recoveryRepositoryPath(*slot)), BranchSlug: slot.Worktree.BranchSlug,
		Path: filepath.Clean(slot.Worktree.Path), Branch: plan.branch, Head: plan.head, Source: plan.source,
		UpdatedAt: time.Now().UTC(),
	}
	if err := writeMissingCheckoutRecord(parent, targetName+missingCheckoutRecordSuffix, record); err != nil {
		return plan, missingCheckoutRecoveryRecord{}, missingCheckoutError(req.OwnerTaskID, slot.Worktree.Path, "cannot record missing-checkout operation")
	}
	return plan, record, nil
}

func (m *Manager) materializeMissingCheckoutTarget(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	wt *Worktree,
	repositoryPath string,
	parent storageworkspaces.DirectoryHandle,
	location missingCheckoutLocation,
	plan missingCheckoutBranchPlan,
	record missingCheckoutRecoveryRecord,
	hooks missingCheckoutRestoreHooks,
) error {
	target, err := parent.CreateSubdirectory(location.name, 0o700)
	if err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "checkout path appeared before materialization")
	}
	defer func() { _ = target.Close() }()
	if err := target.VerifyPath(wt.Path); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "reserved checkout path changed before materialization")
	}
	processPath, targetFile, err := target.ProcessPath(missingCheckoutChildFD)
	if err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("cannot pin checkout path for Git: %v", err))
	}
	if targetFile != nil {
		defer func() { _ = targetFile.Close() }()
	}
	if hooks.beforeWorktreeAdd != nil {
		if err := hooks.beforeWorktreeAdd(); err != nil {
			return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("worktree-add barrier: %v", err))
		}
	}
	if err := m.addMissingCheckoutGitWorktree(ctx, repositoryPath, processPath, targetFile, plan); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("cannot restore recorded checkout: %v", err))
	}
	if err := target.VerifyPath(wt.Path); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "reserved checkout path changed during materialization")
	}
	if err := parent.VerifyPath(location.taskRoot); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, "managed task root changed during materialization")
	}
	if err := m.verifyMissingCheckoutResult(ctx, wt, record); err != nil {
		return missingCheckoutError(req.OwnerTaskID, wt.Path, fmt.Sprintf("restored checkout failed identity validation: %v", err))
	}
	return nil
}

func markMissingCheckoutOperationComplete(
	parent storageworkspaces.DirectoryHandle,
	targetName string,
	record missingCheckoutRecoveryRecord,
) error {
	record.State = missingCheckoutRecordComplete
	record.UpdatedAt = time.Now().UTC()
	if err := writeMissingCheckoutRecord(parent, targetName+missingCheckoutRecordSuffix, record); err != nil {
		return missingCheckoutError(record.TaskID, record.Path, "cannot complete missing-checkout operation record")
	}
	return nil
}

func (m *Manager) addMissingCheckoutGitWorktree(
	ctx context.Context,
	repositoryPath, processPath string,
	targetFile *os.File,
	plan missingCheckoutBranchPlan,
) error {
	args := []string{"worktree", "add"}
	if plan.createLocal {
		args = append(args, "-b", plan.branch)
	}
	args = append(args, "--", processPath)
	if plan.createLocal {
		args = append(args, plan.head)
	} else {
		args = append(args, plan.branch)
	}
	cmd := newGitCommand(ctx, args...)
	cmd.Dir = repositoryPath
	if targetFile != nil {
		cmd.ExtraFiles = []*os.File{targetFile}
	}
	output, err := runGitCmdCombinedOutput(ctx, cmd)
	if err != nil {
		return ClassifyGitError(string(output), err)
	}
	return nil
}

func (m *Manager) verifyMissingCheckoutResult(
	ctx context.Context,
	wt *Worktree,
	record missingCheckoutRecoveryRecord,
) error {
	if inspectLinkedWorktree(wt.Path).class != linkedWorktreeHealthy {
		return fmt.Errorf("checkout is not a healthy linked worktree")
	}
	if err := m.validateExistingWorktreePathOwner(wt.Path, wt); err != nil {
		return fmt.Errorf("checkout task-root ownership changed: %w", err)
	}
	if err := m.verifyMissingCheckoutBranchHead(ctx, wt, record); err != nil {
		return err
	}
	if err := m.verifyMissingCheckoutCommonDir(ctx, wt); err != nil {
		return err
	}
	return m.verifyMissingCheckoutRegistration(ctx, wt, record)
}

func (m *Manager) verifyMissingCheckoutBranchHead(
	ctx context.Context,
	wt *Worktree,
	record missingCheckoutRecoveryRecord,
) error {
	branchOutput, err := m.runBoundedGitInspect(ctx, wt.Path, "branch", "--show-current")
	if err != nil || strings.TrimSpace(branchOutput) != record.Branch {
		return fmt.Errorf("checkout branch does not match the recorded branch")
	}
	head, err := m.resolveCommit(ctx, wt.Path, "HEAD")
	if err != nil || head != strings.ToLower(record.Head) {
		return fmt.Errorf("checkout head does not match the recorded commit")
	}
	return nil
}

func (m *Manager) verifyMissingCheckoutCommonDir(ctx context.Context, wt *Worktree) error {
	wantCommon, err := gitCommonDir(ctx, m, wt.RepositoryPath)
	if err != nil {
		return fmt.Errorf("cannot inspect canonical repository identity")
	}
	gotCommon, err := gitCommonDir(ctx, m, wt.Path)
	if err != nil || filepath.Clean(gotCommon) != filepath.Clean(wantCommon) {
		return fmt.Errorf("checkout belongs to a different Git repository")
	}
	return nil
}

func (m *Manager) verifyMissingCheckoutRegistration(
	ctx context.Context,
	wt *Worktree,
	record missingCheckoutRecoveryRecord,
) error {
	output, err := m.runBoundedGitInspect(ctx, wt.RepositoryPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return fmt.Errorf("cannot verify restored Git registration")
	}
	wantPath, err := normalizedWorktreeTargetPath(wt.Path)
	if err != nil {
		return err
	}
	wantBranch := "refs/heads/" + record.Branch
	for _, registration := range parseWorktreeRegistrations(output) {
		registeredPath, err := normalizedWorktreeTargetPath(registration.path)
		if err != nil {
			return err
		}
		if registeredPath == wantPath && registration.branch == wantBranch &&
			strings.EqualFold(registration.head, record.Head) && !registration.locked && !registration.prunable {
			return nil
		}
	}
	return fmt.Errorf("restored Git registration does not match the canonical checkout")
}

func writeMissingCheckoutRecord(
	parent storageworkspaces.DirectoryHandle,
	name string,
	record missingCheckoutRecoveryRecord,
) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return parent.WriteFile(name, data, 0o600)
}

func missingCheckoutError(taskID, checkout, reason string) error {
	return &WorktreeRecoveryError{TaskID: taskID, Checkout: checkout, State: "missing_checkout", Reason: reason}
}
