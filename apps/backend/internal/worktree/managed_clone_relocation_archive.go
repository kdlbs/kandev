package worktree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type recoveryClaimReader interface {
	GetTaskEnvironmentRecoveryClaim(context.Context, string) (*models.TaskEnvironmentRecoveryClaim, error)
}

func writePublishedManagedCloneRelocationRecord(record *managedCloneRelocationRecord) error {
	if record == nil || record.Replacement == "" || record.State != managedCloneRelocationStateMaterialized {
		return errors.New("replacement relocation record is incomplete")
	}
	return writeManagedCloneRelocationRecord(record.Replacement+".kandev-clone-relocation.json", *record, true)
}

func (m *Manager) managedCloneRelocationArchivePath(record *managedCloneRelocationRecord) (string, error) {
	tasksBase, err := m.config.ExpandedTasksBasePath()
	if err != nil {
		return "", err
	}
	originalPath := record.OriginalWorkspacePath
	if originalPath == "" {
		originalPath = record.Original
	}
	identity := strings.Join([]string{record.TaskID, record.EnvironmentID, record.WorktreeID, record.OperationID}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return filepath.Join(tasksBase, ".kandev-recovery", hex.EncodeToString(digest[:]), filepath.Base(originalPath)), nil
}

func (m *Manager) retainManagedCloneOriginal(ctx context.Context, record *managedCloneRelocationRecord) (string, error) {
	archivePath, err := m.managedCloneRelocationArchivePath(record)
	if err != nil {
		return "", managedCloneRelocationError(record.TaskID, "cannot resolve retained checkout location")
	}
	originalPath := record.Original
	if filepath.Clean(originalPath) == filepath.Clean(archivePath) {
		return archivePath, nil
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o700); err != nil {
		return "", managedCloneRelocationError(record.TaskID, "cannot create retained checkout location")
	}
	if _, err := os.Lstat(archivePath); err == nil {
		if _, sourceErr := os.Lstat(originalPath); sourceErr == nil {
			return "", managedCloneRelocationError(record.TaskID, "retained checkout location is occupied")
		}
		return archivePath, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", managedCloneRelocationError(record.TaskID, "cannot inspect retained checkout location")
	}
	if _, err := os.Lstat(originalPath); err != nil {
		return "", managedCloneRelocationError(record.TaskID, "original checkout is unavailable for retention")
	}
	if _, err := os.Stat(record.SourcePath); err == nil {
		if _, err := m.runBoundedGitInspect(ctx, record.SourcePath, "worktree", "move", originalPath, archivePath); err != nil {
			return "", managedCloneRelocationError(record.TaskID, "original checkout could not be moved to retained storage")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(originalPath, archivePath); err != nil {
			return "", managedCloneRelocationError(record.TaskID, "original checkout could not be moved to retained storage")
		}
	} else {
		return "", managedCloneRelocationError(record.TaskID, "source clone could not be verified for retention")
	}
	if _, err := os.Lstat(archivePath); err != nil {
		return "", managedCloneRelocationError(record.TaskID, "retained checkout failed verification")
	}
	return archivePath, nil
}

func (m *Manager) reconcilePublishedManagedCloneRelocations(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	indices []int,
) (bool, error) {
	reconciled := false
	for _, index := range indices {
		completed, err := m.reconcilePublishedManagedCloneRelocation(ctx, req, &req.Slots[index])
		if err != nil {
			return false, err
		}
		reconciled = reconciled || completed
	}
	return reconciled, nil
}

func (m *Manager) reconcilePublishedManagedCloneRelocation(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slot *RecoverySlot,
) (bool, error) {
	if slot == nil || slot.Worktree == nil || slot.Worktree.Path == "" {
		return false, nil
	}
	wt := slot.Worktree
	recordPath := wt.Path + ".kandev-clone-relocation.json"
	record, err := readManagedCloneRelocationRecord(recordPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, recoveryAdmissionError(*req, "managed-clone relocation record is unreadable")
	}
	if !matchesPublishedManagedCloneRelocation(wt, req, record) {
		return false, nil
	}
	reader, ok := m.store.(recoveryClaimReader)
	if !ok {
		return false, recoveryAdmissionError(*req, "durable recovery claim state is unavailable")
	}
	claim, err := reader.GetTaskEnvironmentRecoveryClaim(ctx, req.TaskEnvironmentID)
	if err != nil {
		return false, recoveryAdmissionError(*req, "durable recovery claim could not be inspected")
	}
	if claim != nil && !publishedRelocationClaimMatches(claim, req, record) {
		return false, recoveryAdmissionError(*req, "published relocation is held by another recovery operation")
	}
	if record.State != string(RecoveryStateComplete) {
		if err := verifyPublishedManagedCloneRelocation(ctx, m, wt, record); err != nil {
			return false, err
		}
		if err := finishPublishedManagedCloneRelocation(ctx, m, req, recordPath, &record, claim); err != nil {
			return false, err
		}
		return true, nil
	}
	if !managedCloneRelocationProofComplete(wt, slot.CloneRelocation) || strings.TrimSpace(record.OperationID) == "" || record.Original == "" {
		return false, recoveryAdmissionError(*req, "completed relocation identity is incomplete")
	}
	if err := verifyCompletedManagedCloneRelocation(ctx, m, wt, slot.CloneRelocation, record); err != nil {
		return false, err
	}
	reconciled, err := reconcilePublishedDirtyRecovery(record)
	if err != nil {
		return false, recoveryAdmissionError(*req, "published relocation recovery journal could not be reconciled")
	}
	if claim != nil {
		if err := m.releaseRecoveryClaim(ctx, claim); err != nil {
			return false, recoveryAdmissionError(*req, "published relocation claim could not be released")
		}
		reconciled = true
	}
	return reconciled, nil
}

func matchesPublishedManagedCloneRelocation(
	wt *Worktree,
	req *RecoveryAdmissionRequest,
	record managedCloneRelocationRecord,
) bool {
	return record.Replacement == wt.Path && record.ReplacementID == wt.ID &&
		record.TaskID == req.OwnerTaskID && record.EnvironmentID == req.TaskEnvironmentID &&
		(record.State == managedCloneRelocationStateMaterialized || record.State == string(RecoveryStateComplete))
}

func finishPublishedManagedCloneRelocation(
	ctx context.Context,
	m *Manager,
	req *RecoveryAdmissionRequest,
	recordPath string,
	record *managedCloneRelocationRecord,
	claim *models.TaskEnvironmentRecoveryClaim,
) error {
	archivePath, err := m.retainManagedCloneOriginal(ctx, record)
	if err != nil {
		return err
	}
	record.Original = archivePath
	if err := markManagedCloneRelocationComplete(recordPath, record, req.TaskID); err != nil {
		return err
	}
	if _, err := reconcilePublishedDirtyRecovery(*record); err != nil {
		return recoveryAdmissionError(*req, "published relocation recovery journal could not be reconciled")
	}
	if claim != nil {
		if err := m.releaseRecoveryClaim(ctx, claim); err != nil {
			return recoveryAdmissionError(*req, "published relocation claim could not be released")
		}
	}
	return nil
}

func verifyPublishedManagedCloneRelocation(
	ctx context.Context,
	m *Manager,
	wt *Worktree,
	record managedCloneRelocationRecord,
) error {
	if !m.IsValid(wt.Path) {
		return managedCloneRelocationError(wt.TaskID, "published replacement worktree is unavailable")
	}
	head, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(head) != record.Head {
		return managedCloneRelocationError(wt.TaskID, "published replacement commit could not be verified")
	}
	common, err := gitCommonDir(ctx, m, wt.Path)
	if err != nil || filepath.Clean(common) != filepath.Clean(record.DestCommon) {
		return managedCloneRelocationError(wt.TaskID, "published replacement clone could not be verified")
	}
	return nil
}

func verifyCompletedManagedCloneRelocation(
	ctx context.Context,
	m *Manager,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
	record managedCloneRelocationRecord,
) error {
	if !m.IsValid(wt.Path) {
		return managedCloneRelocationError(wt.TaskID, "published replacement worktree is unavailable")
	}
	destination, err := verifyCompletedManagedCloneDestination(ctx, m, wt, proof, record)
	if err != nil {
		return err
	}
	return verifyCompletedManagedCloneBranch(ctx, m, wt, destination)
}

func verifyCompletedManagedCloneDestination(
	ctx context.Context,
	m *Manager,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
	record managedCloneRelocationRecord,
) (string, error) {
	_, destination, err := canonicalManagedCloneDestination(wt.TaskID, wt, proof)
	if err != nil || filepath.Clean(record.DestPath) != filepath.Clean(destination) {
		return "", managedCloneRelocationError(wt.TaskID, "completed replacement does not match the selected managed clone")
	}
	common, err := gitCommonDir(ctx, m, wt.Path)
	if err != nil || !sameDirectoryIdentity(common, filepath.Join(destination, ".git")) ||
		filepath.Clean(record.DestCommon) != filepath.Clean(filepath.Join(destination, ".git")) {
		return "", managedCloneRelocationError(wt.TaskID, "published replacement clone could not be verified")
	}
	if err := verifyManagedCloneOrigin(ctx, m, destination, proof.Identity); err != nil {
		if operationalErr := checkoutInspectionOperationalError(ctx, err); operationalErr != nil {
			return "", operationalErr
		}
		return "", managedCloneRelocationError(wt.TaskID, "published replacement provider identity could not be verified")
	}
	return destination, nil
}

func verifyCompletedManagedCloneBranch(ctx context.Context, m *Manager, wt *Worktree, destination string) error {
	branch := strings.TrimSpace(wt.Branch)
	if branch == "" || strings.HasPrefix(branch, "refs/") {
		return managedCloneRelocationError(wt.TaskID, "published replacement branch identity is incomplete")
	}
	if _, err := m.runBoundedGitInspect(ctx, destination, "check-ref-format", "--branch", branch); err != nil {
		return managedCloneRelocationError(wt.TaskID, "published replacement branch name is invalid")
	}
	head, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || !relocationCommitPattern.MatchString(strings.TrimSpace(head)) {
		return managedCloneRelocationError(wt.TaskID, "published replacement commit could not be verified")
	}
	registeredHead, registered, err := registeredWorktreeHead(ctx, m, destination, wt.Path, branch)
	if err != nil || !registered || strings.TrimSpace(registeredHead) != strings.TrimSpace(head) {
		return managedCloneRelocationError(wt.TaskID, "published replacement worktree registration could not be verified")
	}
	branchHead, err := m.runBoundedGitInspect(ctx, destination, "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil || strings.TrimSpace(branchHead) != strings.TrimSpace(head) {
		return managedCloneRelocationError(wt.TaskID, "published replacement branch no longer identifies its checkout")
	}
	return nil
}

func publishedRelocationClaimMatches(
	claim *models.TaskEnvironmentRecoveryClaim,
	req *RecoveryAdmissionRequest,
	record managedCloneRelocationRecord,
) bool {
	return claim != nil && claim.OperationID == record.OperationID && claim.TaskEnvironmentID == req.TaskEnvironmentID &&
		claim.OwnerTaskID == req.OwnerTaskID && claim.OwnershipGeneration == req.OwnershipGeneration &&
		claim.SessionID == req.SessionID && claim.ExecutorType == req.ExecutorType
}

func reconcilePublishedDirtyRecovery(record managedCloneRelocationRecord) (bool, error) {
	originalPath := record.OriginalWorkspacePath
	if originalPath == "" {
		return false, nil
	}
	primaryPath := originalPath + ".kandev-recovery.json"
	recovery, err := readRecoveryRecord(primaryPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || recovery.OperationID != record.OperationID ||
		(recovery.State != RecoveryStateRematerializing && recovery.State != RecoveryStateComplete) {
		return false, fmt.Errorf("dirty recovery record is not reconciliable")
	}
	paths := []string{primaryPath, record.Original + ".kandev-recovery.json"}
	needsWrite, uniquePaths, err := inspectPublishedDirtyRecoveryRecords(paths, record)
	if err != nil {
		return false, err
	}
	if !needsWrite {
		return false, nil
	}
	recovery.Original = record.Original
	recovery.Replacement = record.Replacement
	recovery.State = RecoveryStateComplete
	recovery.UpdatedAt = time.Now().UTC()
	for _, path := range uniquePaths {
		if err := writeRecoveryRecord(path, recovery); err != nil {
			return false, err
		}
	}
	return true, nil
}

func inspectPublishedDirtyRecoveryRecords(
	paths []string,
	record managedCloneRelocationRecord,
) (bool, []string, error) {
	needsWrite := false
	seen := make(map[string]struct{}, len(paths))
	uniquePaths := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		uniquePaths = append(uniquePaths, path)
		companion, readErr := readRecoveryRecord(path)
		if errors.Is(readErr, os.ErrNotExist) {
			needsWrite = true
			continue
		}
		if readErr != nil || companion.OperationID != record.OperationID ||
			(companion.State != RecoveryStateRematerializing && companion.State != RecoveryStateComplete) {
			return false, nil, fmt.Errorf("dirty recovery record is not reconciliable")
		}
		if companion.State != RecoveryStateComplete || companion.Original != record.Original ||
			companion.Replacement != record.Replacement {
			needsWrite = true
		}
	}
	return needsWrite, uniquePaths, nil
}
