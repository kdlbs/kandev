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
	if err := verifyPublishedManagedCloneRelocation(ctx, m, wt, record); err != nil {
		return false, err
	}
	claim, err := reader.GetTaskEnvironmentRecoveryClaim(ctx, req.TaskEnvironmentID)
	if err != nil {
		return false, recoveryAdmissionError(*req, "durable recovery claim could not be inspected")
	}
	if claim != nil && !publishedRelocationClaimMatches(claim, req, record) {
		return false, recoveryAdmissionError(*req, "published relocation is held by another recovery operation")
	}
	if err := finishPublishedManagedCloneRelocation(ctx, m, req, recordPath, &record, claim); err != nil {
		return false, err
	}
	return true, nil
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
	if err := reconcilePublishedDirtyRecovery(*record); err != nil {
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

func publishedRelocationClaimMatches(
	claim *models.TaskEnvironmentRecoveryClaim,
	req *RecoveryAdmissionRequest,
	record managedCloneRelocationRecord,
) bool {
	return claim != nil && claim.OperationID == record.OperationID && claim.TaskEnvironmentID == req.TaskEnvironmentID &&
		claim.OwnerTaskID == req.OwnerTaskID && claim.OwnershipGeneration == req.OwnershipGeneration &&
		claim.SessionID == req.SessionID && claim.ExecutorType == req.ExecutorType
}

func reconcilePublishedDirtyRecovery(record managedCloneRelocationRecord) error {
	originalPath := record.OriginalWorkspacePath
	if originalPath == "" {
		return nil
	}
	path := originalPath + ".kandev-recovery.json"
	recovery, err := readRecoveryRecord(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || recovery.OperationID != record.OperationID ||
		(recovery.State != RecoveryStateRematerializing && recovery.State != RecoveryStateComplete) {
		return fmt.Errorf("dirty recovery record is not reconciliable")
	}
	recovery.Original = record.Original
	recovery.Replacement = record.Replacement
	recovery.State = RecoveryStateComplete
	recovery.UpdatedAt = time.Now().UTC()
	if err := writeRecoveryRecord(path, recovery); err != nil {
		return err
	}
	return writeRecoveryRecord(record.Original+".kandev-recovery.json", recovery)
}
