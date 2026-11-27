package worktree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
)

func (m *Manager) missingCheckoutOperationLockPath(ctx context.Context, slot RecoverySlot) (string, error) {
	if slot.Worktree == nil || strings.TrimSpace(slot.Worktree.ID) == "" {
		return "", fmt.Errorf("missing-checkout worktree identity is unavailable")
	}
	commonDir, err := gitCommonDir(ctx, m, recoveryRepositoryPath(slot))
	if err != nil {
		return "", fmt.Errorf("resolve repository common directory: %w", err)
	}
	identity := sha256.Sum256([]byte(slot.Worktree.ID))
	return filepath.Join(commonDir, "kandev-missing-checkout-"+hex.EncodeToString(identity[:])+".claim"), nil
}

func (m *Manager) acquireMissingCheckoutOperationLock(ctx context.Context, slot RecoverySlot) (*recoveryLock, error) {
	path, err := m.missingCheckoutOperationLockPath(ctx, slot)
	if err != nil {
		return nil, err
	}
	lock, err := acquireRecoveryOperation(path)
	if errors.Is(err, errRecoveryOperationClaimed) {
		return nil, fmt.Errorf("missing-checkout operation is active in another backend process")
	}
	return lock, err
}

func releaseMissingCheckoutOperationLocks(locks []*recoveryLock) {
	for index := len(locks) - 1; index >= 0; index-- {
		_ = locks[index].Close()
	}
}

func (m *Manager) acquireMissingCheckoutOperationLocks(
	ctx context.Context,
	slots []RecoverySlot,
) ([]*recoveryLock, error) {
	ordered := append([]RecoverySlot(nil), slots...)
	sort.Slice(ordered, func(i, j int) bool {
		return recoverySlotKey(ordered[i]) < recoverySlotKey(ordered[j])
	})
	locks := make([]*recoveryLock, 0, len(ordered))
	seen := make(map[string]struct{}, len(ordered))
	for _, slot := range ordered {
		key := recoverySlotKey(slot)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		lock, err := m.acquireMissingCheckoutOperationLock(ctx, slot)
		if err != nil {
			releaseMissingCheckoutOperationLocks(locks)
			return nil, err
		}
		locks = append(locks, lock)
	}
	return locks, nil
}

func (m *Manager) reconcileCompletedMissingCheckoutClaim(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	indices []int,
) (bool, error) {
	claimReader, ok := m.store.(recoveryClaimReader)
	if !ok {
		if selectedSlotsHaveCompletedMissingCheckoutRecord(req, indices) {
			return false, recoveryAdmissionError(*req, "completed missing-checkout claim reader is unavailable")
		}
		return false, nil
	}
	claim, err := claimReader.GetTaskEnvironmentRecoveryClaim(ctx, req.TaskEnvironmentID)
	if err != nil {
		return false, recoveryAdmissionError(*req, fmt.Sprintf("read completed missing-checkout claim: %v", err))
	}
	if claim == nil {
		return false, nil
	}
	if !selectedSlotsHaveCompletedMissingCheckoutOperation(req, indices, claim.OperationID) {
		return false, nil
	}
	completedSlots, err := matchingCompletedMissingCheckoutSlots(req, indices, claim)
	if err != nil {
		return false, err
	}
	locks, err := m.acquireMissingCheckoutOperationLocks(ctx, completedSlots)
	if err != nil {
		return false, recoveryAdmissionError(*req, err.Error())
	}
	defer releaseMissingCheckoutOperationLocks(locks)
	if err := m.revalidateCompletedMissingCheckoutSlots(ctx, req, completedSlots, claim); err != nil {
		return false, err
	}
	current, err := claimReader.GetTaskEnvironmentRecoveryClaim(ctx, req.TaskEnvironmentID)
	if err != nil {
		return false, recoveryAdmissionError(*req, fmt.Sprintf("recheck completed missing-checkout claim: %v", err))
	}
	if err := m.releaseReconciledMissingCheckoutClaim(ctx, req, claim, current); err != nil {
		return false, err
	}
	return true, nil
}

func selectedSlotsHaveCompletedMissingCheckoutOperation(
	req *RecoveryAdmissionRequest,
	indices []int,
	operationID string,
) bool {
	for _, index := range indices {
		missing := req.Slots[index].missingCheckout
		if matchingCompletedMissingCheckoutRecord(missing, operationID) {
			return true
		}
	}
	return false
}

func selectedSlotsHaveCompletedMissingCheckoutRecord(req *RecoveryAdmissionRequest, indices []int) bool {
	for _, index := range indices {
		missing := req.Slots[index].missingCheckout
		if missing != nil && missing.record != nil && missing.record.State == missingCheckoutRecordComplete {
			return true
		}
	}
	return false
}

func matchingCompletedMissingCheckoutSlots(
	req *RecoveryAdmissionRequest,
	indices []int,
	claim *models.TaskEnvironmentRecoveryClaim,
) ([]RecoverySlot, error) {
	if !missingCheckoutClaimMatchesRequest(claim, *req) {
		return nil, recoveryAdmissionError(*req, "completed missing-checkout claim identity does not match the selected environment")
	}
	completed := make([]RecoverySlot, 0, len(indices))
	for _, index := range indices {
		slot := req.Slots[index]
		if !matchingCompletedMissingCheckoutRecord(slot.missingCheckout, claim.OperationID) {
			continue
		}
		if err := validateCompletedMissingCheckoutRecord(*req, slot); err != nil {
			return nil, err
		}
		completed = append(completed, slot)
	}
	if len(completed) == 0 {
		return nil, recoveryAdmissionError(*req, "durable recovery claim has no matching completed missing-checkout record")
	}
	return completed, nil
}

func matchingCompletedMissingCheckoutRecord(missing *missingCheckoutInspection, operationID string) bool {
	return missing != nil && missing.record != nil && missing.record.State == missingCheckoutRecordComplete &&
		missing.record.OperationID == operationID
}

func validateCompletedMissingCheckoutRecord(req RecoveryAdmissionRequest, slot RecoverySlot) error {
	record := slot.missingCheckout.record
	if err := validateMissingCheckoutRecordIdentity(*record, slot.Worktree, recoveryRepositoryPath(slot), req.OwnershipGeneration, true); err != nil ||
		record.TaskID != req.OwnerTaskID || record.TaskEnvironmentID != req.TaskEnvironmentID {
		return recoveryAdmissionError(req, "completed missing-checkout record does not match the current owner and generation")
	}
	return nil
}

func (m *Manager) revalidateCompletedMissingCheckoutSlots(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slots []RecoverySlot,
	claim *models.TaskEnvironmentRecoveryClaim,
) error {
	for _, slot := range slots {
		if err := m.revalidateCompletedMissingCheckoutSlot(ctx, req, slot, claim); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) revalidateCompletedMissingCheckoutSlot(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slot RecoverySlot,
	claim *models.TaskEnvironmentRecoveryClaim,
) error {
	info, err := os.Lstat(slot.Worktree.Path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return recoveryAdmissionError(*req, "completed missing-checkout path changed before claim reconciliation")
	}
	fresh, err := m.inspectMissingCheckout(ctx, req.OwnerTaskID, req.OwnershipGeneration, &slot, true, false)
	if err != nil || !matchingCompletedMissingCheckoutRecord(&fresh, claim.OperationID) {
		return recoveryAdmissionError(*req, "completed missing-checkout record or checkout changed before claim reconciliation")
	}
	return validateCompletedMissingCheckoutRecord(*req, RecoverySlot{
		Worktree: slot.Worktree, RepositoryPath: slot.RepositoryPath, missingCheckout: &fresh,
	})
}

func (m *Manager) releaseReconciledMissingCheckoutClaim(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	expected, current *models.TaskEnvironmentRecoveryClaim,
) error {
	if !sameRecoveryClaim(expected, current) {
		return recoveryAdmissionError(*req, "completed missing-checkout claim changed before reconciliation")
	}
	claimStore, ok := m.store.(recoveryClaimStore)
	if !ok {
		return recoveryAdmissionError(*req, "completed missing-checkout claim release is unavailable")
	}
	if err := claimStore.ReleaseTaskEnvironmentRecoveryClaim(ctx, current); err != nil {
		return recoveryAdmissionError(*req, fmt.Sprintf("release completed missing-checkout claim: %v", err))
	}
	return nil
}

func missingCheckoutClaimMatchesRequest(claim *models.TaskEnvironmentRecoveryClaim, req RecoveryAdmissionRequest) bool {
	return claim != nil && claim.TaskEnvironmentID == req.TaskEnvironmentID && claim.OwnerTaskID == req.OwnerTaskID &&
		claim.OwnershipGeneration == req.OwnershipGeneration && claim.SessionID == req.SessionID &&
		claim.ExecutorType == req.ExecutorType
}

func sameRecoveryClaim(left, right *models.TaskEnvironmentRecoveryClaim) bool {
	return left != nil && right != nil && left.TaskEnvironmentID == right.TaskEnvironmentID &&
		left.OwnerTaskID == right.OwnerTaskID && left.OwnershipGeneration == right.OwnershipGeneration &&
		left.SessionID == right.SessionID && left.OperationID == right.OperationID &&
		left.ExecutorType == right.ExecutorType
}

func (m *Manager) removeMissingCheckoutStaleRegistration(
	ctx context.Context,
	repositoryPath, worktreePath string,
	plan missingCheckoutBranchPlan,
) error {
	output, err := m.runBoundedGitInspect(ctx, repositoryPath, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return fmt.Errorf("recheck Git worktree registrations: %w", err)
	}
	wantPath, err := normalizedWorktreeTargetPath(worktreePath)
	if err != nil {
		return err
	}
	wantBranch := "refs/heads/" + plan.branch
	if err := validateStaleMissingCheckoutRegistrationList(output, wantPath, wantBranch, plan.head); err != nil {
		return err
	}
	commonDir, err := gitCommonDir(ctx, m, repositoryPath)
	if err != nil {
		return fmt.Errorf("resolve repository common directory: %w", err)
	}
	registrationsPath := filepath.Join(commonDir, "worktrees")
	registrations, err := storageworkspaces.OpenDirectoryNoFollow(commonDir, registrationsPath)
	if err != nil {
		return fmt.Errorf("open Git worktree metadata: %w", err)
	}
	defer func() { _ = registrations.Close() }()
	wantGitDir, err := normalizedWorktreeTargetPath(worktreePath + string(filepath.Separator) + ".git")
	if err != nil {
		return err
	}
	exact, exactPath, err := findExactStaleMissingCheckoutMetadata(registrations, registrationsPath, commonDir, wantGitDir, wantBranch)
	if err != nil {
		return err
	}
	defer func() { _ = exact.Close() }()
	if err := exact.VerifyPath(exactPath); err != nil {
		return fmt.Errorf("git metadata entry changed before cleanup: %w", err)
	}
	if err := exact.RemoveDirectory(ctx); err != nil {
		return fmt.Errorf("remove exact stale Git metadata entry: %w", err)
	}
	return nil
}

func validateStaleMissingCheckoutRegistrationList(output, wantPath, wantBranch, head string) error {
	matched := 0
	for _, registration := range parseWorktreeRegistrations(output) {
		registeredPath, err := normalizedWorktreeTargetPath(registration.path)
		if err != nil {
			return fmt.Errorf("git registration path is invalid: %w", err)
		}
		if registeredPath == wantPath {
			if err := validateExactStaleRegistration(registration, wantBranch, head); err != nil {
				return err
			}
			matched++
			continue
		}
		if registration.branch == wantBranch {
			return fmt.Errorf("recorded branch is checked out in another worktree")
		}
	}
	if matched != 1 {
		return fmt.Errorf("exact stale Git registration is no longer present")
	}
	return nil
}

func validateExactStaleRegistration(registration worktreeRegistration, wantBranch, head string) error {
	if registration.locked || !registration.prunable || registration.branch != wantBranch ||
		!strings.EqualFold(registration.head, head) {
		return fmt.Errorf("git registration is no longer the exact stale entry")
	}
	return nil
}

func findExactStaleMissingCheckoutMetadata(
	registrations storageworkspaces.DirectoryHandle,
	registrationsPath, commonDir, wantGitDir, wantBranch string,
) (storageworkspaces.DirectoryHandle, string, error) {
	entries, err := registrations.ReadDir()
	if err != nil {
		return nil, "", fmt.Errorf("read Git worktree metadata: %w", err)
	}
	var exact storageworkspaces.DirectoryHandle
	exactPath := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		admin, path, found, inspectErr := inspectStaleMissingCheckoutMetadata(
			registrations, registrationsPath, commonDir, entry.Name(), wantGitDir, wantBranch,
		)
		if inspectErr != nil {
			_ = closeMissingCheckoutDirectories(exact, admin)
			return nil, "", inspectErr
		}
		if !found {
			continue
		}
		if exact != nil {
			_ = closeMissingCheckoutDirectories(exact, admin)
			return nil, "", fmt.Errorf("more than one Git metadata entry names the absent checkout")
		}
		exact, exactPath = admin, path
	}
	if exact == nil {
		return nil, "", fmt.Errorf("cannot identify the exact stale Git metadata entry")
	}
	return exact, exactPath, nil
}

func inspectStaleMissingCheckoutMetadata(
	registrations storageworkspaces.DirectoryHandle,
	registrationsPath, commonDir, entryName, wantGitDir, wantBranch string,
) (storageworkspaces.DirectoryHandle, string, bool, error) {
	admin, err := registrations.OpenSubdirectory(entryName)
	if err != nil {
		return nil, "", false, nil
	}
	gitDirData, err := admin.ReadFile("gitdir")
	if err != nil {
		_ = admin.Close()
		return nil, "", false, nil
	}
	registeredGitDir, err := normalizedWorktreeTargetPath(strings.TrimSpace(string(gitDirData)))
	if err != nil || registeredGitDir != wantGitDir {
		_ = admin.Close()
		return nil, "", false, nil
	}
	if err := validateStaleMissingCheckoutMetadata(admin, registrationsPath, commonDir, entryName, wantBranch); err != nil {
		_ = admin.Close()
		return nil, "", false, err
	}
	return admin, filepath.Join(registrationsPath, entryName), true, nil
}

func validateStaleMissingCheckoutMetadata(
	admin storageworkspaces.DirectoryHandle,
	registrationsPath, commonDir, entryName, wantBranch string,
) error {
	head, headErr := admin.ReadFile("HEAD")
	common, commonErr := admin.ReadFile("commondir")
	_, lockedErr := admin.LstatEntry("locked")
	registeredCommon := filepath.Clean(filepath.Join(registrationsPath, entryName, strings.TrimSpace(string(common))))
	if headErr != nil || string(head) != "ref: "+wantBranch+"\n" || commonErr != nil || registeredCommon != commonDir ||
		(lockedErr == nil || !errors.Is(lockedErr, os.ErrNotExist)) {
		return fmt.Errorf("git metadata entry does not prove the exact unlocked registration")
	}
	return nil
}

func closeMissingCheckoutDirectories(handles ...storageworkspaces.DirectoryHandle) error {
	var closeErr error
	for _, handle := range handles {
		if handle != nil {
			if err := handle.Close(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	}
	return closeErr
}
