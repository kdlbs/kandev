package worktree

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
)

const permissionOnlyRecoveryFailure = "recovery snapshot does not match original checkout"

type recoveryModeRetry struct {
	Version                int       `json:"version"`
	PreviousSnapshot       string    `json:"previous_snapshot"`
	PreviousError          string    `json:"previous_error"`
	PreviousUpdatedAt      time.Time `json:"previous_updated_at"`
	SourceManifest         string    `json:"source_manifest"`
	SourceIdentityManifest string    `json:"source_identity_manifest"`
}

func permissionRetrySnapshotPath(original, operationID string) string {
	return original + ".kandev-recovery-" + operationID + "-modes-v1"
}

func validRecoveryModeRetry(record recoveryRecord, original string) bool {
	retry := record.ModeRetry
	if retry == nil || retry.Version != 1 || retry.PreviousSnapshot == "" ||
		retry.PreviousSnapshot == record.Snapshot || retry.PreviousError != permissionOnlyRecoveryFailure ||
		retry.PreviousUpdatedAt.IsZero() || !validRecoveryDigest(retry.SourceManifest) ||
		!validRecoveryDigest(retry.SourceIdentityManifest) ||
		record.Snapshot != permissionRetrySnapshotPath(original, record.OperationID) {
		return false
	}
	if _, err := uuid.Parse(record.OperationID); err != nil {
		return false
	}
	if err := validateRecoverySnapshotPath(original, retry.PreviousSnapshot); err != nil {
		return false
	}
	info, err := os.Lstat(retry.PreviousSnapshot)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func validRecoveryDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func blockedPermissionRetryCandidate(
	req RecoveryAdmissionRequest,
	slot RecoverySlot,
	recovery recoveryRecord,
	relocation managedCloneRelocationRecord,
) error {
	wt := slot.Worktree
	if !isExplicitPermissionRetry(req, slot, wt) {
		return recoveryAdmissionError(req, "blocked recovery is not eligible for explicit relocation retry")
	}
	if !isPermissionRetryRecord(recovery, wt) {
		return recoveryAdmissionError(req, "blocked recovery does not match the permission-only retry format")
	}
	if !isPermissionRetryRelocation(relocation, recovery, wt) {
		return recoveryAdmissionError(req, "blocked recovery identity does not match the selected relocation")
	}
	if !isMaterializedPermissionRetryReplacement(relocation, recovery, wt) {
		return recoveryAdmissionError(req, "materialized relocation identity does not match the blocked recovery")
	}
	if err := validateRecoverySnapshotPath(wt.Path, recovery.Snapshot); err != nil {
		return recoveryAdmissionError(req, "historical recovery snapshot path is unsafe")
	}
	info, err := os.Lstat(recovery.Snapshot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return recoveryAdmissionError(req, "historical recovery snapshot is unavailable")
	}
	return nil
}

func isExplicitPermissionRetry(req RecoveryAdmissionRequest, slot RecoverySlot, wt *Worktree) bool {
	return req.RelocateDirty && slot.CloneRelocation != nil && wt != nil
}

func isPermissionRetryRecord(record recoveryRecord, wt *Worktree) bool {
	if record.State != RecoveryStateBlocked || record.Manifest != "" ||
		record.Error != permissionOnlyRecoveryFailure || record.ModeRetry != nil {
		return false
	}
	if _, err := uuid.Parse(record.OperationID); err != nil {
		return false
	}
	return record.OperationID != "" && record.TaskID == wt.TaskID &&
		record.WorktreeID == wt.ID && record.Original == wt.Path
}

func isPermissionRetryRelocation(record managedCloneRelocationRecord, recovery recoveryRecord, wt *Worktree) bool {
	return recovery.OperationID == record.OperationID && record.TaskID == wt.TaskID &&
		record.WorktreeID == wt.ID && record.Original == wt.Path
}

func isMaterializedPermissionRetryReplacement(
	record managedCloneRelocationRecord,
	recovery recoveryRecord,
	wt *Worktree,
) bool {
	return record.State == managedCloneRelocationStateMaterialized && record.TaskID == wt.TaskID &&
		record.EnvironmentID == wt.TaskEnvironmentID && record.WorktreeID == wt.ID &&
		record.Original == wt.Path && record.Replacement == wt.Path+".relocated-"+recovery.OperationID[:8] &&
		record.ReplacementID != ""
}

func (m *Manager) preflightPermissionOnlyBlockedRetries(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	indices []int,
) error {
	for _, index := range indices {
		if err := m.preflightPermissionOnlyBlockedRetry(ctx, req, index); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) preflightPermissionOnlyBlockedRetry(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	index int,
) error {
	slot := &req.Slots[index]
	if slot.Worktree == nil || slot.Worktree.Path == "" {
		return nil
	}
	recovery, relocation, eligible, err := readBlockedPermissionRetryRecords(*req, slot)
	if err != nil || !eligible {
		return err
	}
	if err := blockedPermissionRetryCandidate(*req, *slot, recovery, relocation); err != nil {
		return err
	}
	if err := validateDirtyCloneRelocationAuthorization(ctx); err != nil {
		return err
	}
	return m.verifyBlockedPermissionRetryUnderLock(ctx, req, slot, recovery, relocation)
}

func readBlockedPermissionRetryRecords(
	req RecoveryAdmissionRequest,
	slot *RecoverySlot,
) (recoveryRecord, managedCloneRelocationRecord, bool, error) {
	path := slot.Worktree.Path
	recovery, err := readRecoveryRecord(path + ".kandev-recovery.json")
	if errors.Is(err, os.ErrNotExist) || err == nil && recovery.State != RecoveryStateBlocked {
		return recoveryRecord{}, managedCloneRelocationRecord{}, false, nil
	}
	if err != nil {
		return recoveryRecord{}, managedCloneRelocationRecord{}, false,
			recoveryAdmissionError(req, "recovery record cannot be inspected for explicit retry")
	}
	relocation, err := readManagedCloneRelocationRecord(path + ".kandev-clone-relocation.json")
	if err != nil {
		return recoveryRecord{}, managedCloneRelocationRecord{}, false,
			recoveryAdmissionError(req, "managed-clone relocation record is unavailable for retry")
	}
	return recovery, relocation, true, nil
}

func (m *Manager) verifyBlockedPermissionRetryUnderLock(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	slot *RecoverySlot,
	recovery recoveryRecord,
	relocation managedCloneRelocationRecord,
) error {
	path := slot.Worktree.Path
	relocationLock, err := acquireRecoveryOperation(path + ".kandev-clone-relocation.claim")
	if err != nil {
		return recoveryAdmissionError(*req, "another clone relocation is active")
	}
	recoveryLock, err := acquireRecoveryOperation(path + ".kandev-recovery.claim")
	if err != nil {
		_ = relocationLock.Close()
		return recoveryAlreadyClaimedError(slot.Worktree, err.Error())
	}
	defer func() {
		_ = recoveryLock.Close()
		_ = relocationLock.Close()
	}()
	freshRecovery, recoveryErr := readRecoveryRecord(path + ".kandev-recovery.json")
	freshRelocation, relocationErr := readManagedCloneRelocationRecord(path + ".kandev-clone-relocation.json")
	if recoveryErr != nil || relocationErr != nil || freshRecovery != recovery || freshRelocation != relocation {
		return managedCloneRelocationError(slot.Worktree.TaskID, "historical snapshot is not a proven permission-only failure")
	}
	if err := blockedPermissionRetryCandidate(*req, *slot, freshRecovery, freshRelocation); err != nil {
		return err
	}
	if _, err := m.verifyPermissionOnlyRetry(ctx, slot.Worktree, slot.CloneRelocation, freshRecovery, freshRelocation); err != nil {
		return managedCloneRelocationError(slot.Worktree.TaskID, "historical snapshot is not a proven permission-only failure")
	}
	return nil
}

func (m *Manager) verifyPermissionOnlyRetry(
	ctx context.Context,
	wt *Worktree,
	proof *ManagedCloneRelocationProof,
	recovery recoveryRecord,
	relocation managedCloneRelocationRecord,
) (recoveryPermissionProof, error) {
	if err := validateDirtyCloneRelocationAuthorization(ctx); err != nil {
		return recoveryPermissionProof{}, err
	}
	if wt == nil || proof == nil || relocation.State != managedCloneRelocationStateMaterialized ||
		relocation.Original != wt.Path || relocation.TaskID != wt.TaskID || relocation.WorktreeID != wt.ID {
		return recoveryPermissionProof{}, errors.New("relocation identity changed")
	}
	if err := m.verifyPermissionRetryReplacement(ctx, relocation); err != nil {
		return recoveryPermissionProof{}, err
	}
	sourceProof, err := provePermissionOnlySnapshot(ctx, wt.Path, recovery.Snapshot)
	if err != nil {
		return recoveryPermissionProof{}, err
	}
	if err := validateDirtyCloneRelocationAuthorization(ctx); err != nil {
		return recoveryPermissionProof{}, err
	}
	return sourceProof, nil
}

func (m *Manager) verifyPermissionRetryReplacement(ctx context.Context, record managedCloneRelocationRecord) error {
	if !m.IsValid(record.Replacement) {
		return errors.New("replacement worktree failed integrity validation")
	}
	head, err := m.runBoundedGitInspect(ctx, record.Replacement, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(head) != record.Head {
		return errors.New("replacement commit changed")
	}
	branch, err := m.runBoundedGitInspect(ctx, record.Replacement, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) != record.Branch {
		return errors.New("replacement branch changed")
	}
	common, err := gitCommonDir(ctx, m, record.Replacement)
	if err != nil || filepath.Clean(common) != filepath.Clean(record.DestCommon) {
		return errors.New("replacement clone changed")
	}
	status, err := m.runBoundedGitInspect(ctx, record.Replacement, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional")
	if err != nil || strings.TrimSpace(status) != "" {
		return errors.New("replacement worktree is not clean")
	}
	return nil
}

func provePermissionOnlySnapshot(ctx context.Context, original, snapshot string) (recoveryPermissionProof, error) {
	if err := validateRecoverySnapshotPath(original, snapshot); err != nil {
		return recoveryPermissionProof{}, err
	}
	originalRoot, err := workspaces.OpenDirectoryNoFollow(filepath.Dir(original), original)
	if err != nil {
		return recoveryPermissionProof{}, err
	}
	defer func() { _ = originalRoot.Close() }()
	snapshotRoot, err := workspaces.OpenDirectoryNoFollow(filepath.Dir(snapshot), snapshot)
	if err != nil {
		return recoveryPermissionProof{}, err
	}
	defer func() { _ = snapshotRoot.Close() }()
	proof := recoveryPermissionProof{}
	if err := compareRecoveryDirectories(ctx, originalRoot, snapshotRoot, "", &proof); err != nil {
		return recoveryPermissionProof{}, err
	}
	if proof.differences == 0 {
		return recoveryPermissionProof{}, errors.New("historical snapshot has no permission differences")
	}
	if err := originalRoot.VerifyPath(original); err != nil {
		return recoveryPermissionProof{}, err
	}
	if err := snapshotRoot.VerifyPath(snapshot); err != nil {
		return recoveryPermissionProof{}, err
	}
	proof.manifest = recoveryEntriesManifest(proof.manifestEntries)
	proof.identityManifest = recoveryEntriesManifest(proof.identityEntries)
	return proof, nil
}

type recoveryPermissionProof struct {
	differences      int
	manifest         string
	identityManifest string
	manifestEntries  []string
	identityEntries  []string
}

func compareRecoveryDirectories(
	ctx context.Context,
	original, snapshot workspaces.DirectoryHandle,
	relative string,
	proof *recoveryPermissionProof,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	originalNames, err := recoveryDirectoryEntryNames(original, relative == "")
	if err != nil {
		return err
	}
	snapshotNames, err := recoveryDirectoryEntryNames(snapshot, relative == "")
	if err != nil {
		return err
	}
	if len(originalNames) != len(snapshotNames) {
		return errors.New("historical snapshot entry set changed")
	}
	for index, name := range originalNames {
		if name != snapshotNames[index] {
			return errors.New("historical snapshot entry set changed")
		}
		if err := compareRecoveryEntry(ctx, original, snapshot, name, filepath.Join(relative, name), proof); err != nil {
			return err
		}
	}
	return nil
}

func recoveryDirectoryEntryNames(handle workspaces.DirectoryHandle, root bool) ([]string, error) {
	entries, err := handle.ReadDir()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if root && entry.Name() == recoveryGitDirName {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func compareRecoveryEntry(
	ctx context.Context,
	original, snapshot workspaces.DirectoryHandle,
	name, relative string,
	proof *recoveryPermissionProof,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	originalMode, err := original.LstatEntry(name)
	if err != nil {
		return err
	}
	snapshotMode, err := snapshot.LstatEntry(name)
	if err != nil {
		return err
	}
	if originalMode.IsDir() {
		if !snapshotMode.IsDir() {
			return fmt.Errorf("historical snapshot entry type changed: %s", relative)
		}
		return compareRecoverySubdirectories(ctx, original, snapshot, name, relative, originalMode, snapshotMode, proof)
	}
	if originalMode&os.ModeSymlink != 0 {
		target, err := compareRecoverySymlinks(original, snapshot, name, relative, originalMode, snapshotMode)
		if err != nil {
			return err
		}
		proof.manifestEntries = append(proof.manifestEntries, relative+"|"+originalMode.String()+"|"+target)
		return nil
	}
	return compareRecoveryRegularEntry(ctx, original, snapshot, name, relative, originalMode, snapshotMode, proof)
}

func compareRecoverySymlinks(
	original, snapshot workspaces.DirectoryHandle,
	name, relative string,
	originalMode, snapshotMode os.FileMode,
) (string, error) {
	if snapshotMode&os.ModeSymlink == 0 || originalMode != snapshotMode {
		return "", fmt.Errorf("historical snapshot link mode changed: %s", relative)
	}
	originalTarget, err := original.ReadLink(name)
	if err != nil {
		return "", err
	}
	snapshotTarget, err := snapshot.ReadLink(name)
	if err != nil || originalTarget != snapshotTarget {
		return "", fmt.Errorf("historical snapshot link target changed: %s", relative)
	}
	return originalTarget, nil
}

func compareRecoveryRegularEntry(
	ctx context.Context,
	original, snapshot workspaces.DirectoryHandle,
	name, relative string,
	originalMode, snapshotMode os.FileMode,
	proof *recoveryPermissionProof,
) error {
	if originalMode.Type() != 0 || snapshotMode.Type() != 0 {
		return fmt.Errorf("historical snapshot contains an unsupported entry: %s", relative)
	}
	changed, err := permissionOnlyModeDifference(originalMode, snapshotMode, true)
	if err != nil {
		return err
	}
	if changed {
		proof.differences++
	}
	return compareRecoveryRegularFiles(ctx, original, snapshot, name, relative, originalMode, snapshotMode, proof)
}

func compareRecoverySubdirectories(
	ctx context.Context,
	original, snapshot workspaces.DirectoryHandle,
	name, relative string,
	originalMode, snapshotMode os.FileMode,
	proof *recoveryPermissionProof,
) error {
	changed, err := permissionOnlyModeDifference(originalMode, snapshotMode, false)
	if err != nil {
		return err
	}
	if changed {
		proof.differences++
	}
	originalDir, err := original.OpenSubdirectory(name)
	if err != nil {
		return err
	}
	defer func() { _ = originalDir.Close() }()
	snapshotDir, err := snapshot.OpenSubdirectory(name)
	if err != nil {
		return err
	}
	defer func() { _ = snapshotDir.Close() }()
	originalInfo, err := workspaces.PinnedDirectoryInfo(originalDir)
	if err != nil {
		return err
	}
	snapshotInfo, err := workspaces.PinnedDirectoryInfo(snapshotDir)
	if err != nil {
		return err
	}
	if originalInfo.Mode() != originalMode || snapshotInfo.Mode() != snapshotMode {
		return fmt.Errorf("historical snapshot directory changed during proof: %s", relative)
	}
	if err := verifyHistoricalRecoveryIdentity(originalInfo, snapshotInfo); err != nil {
		return fmt.Errorf("historical snapshot directory identity changed: %s", relative)
	}
	proof.manifestEntries = append(proof.manifestEntries, relative+"|"+originalInfo.Mode().String())
	if err := appendRecoveryIdentityEntry(relative, originalInfo, &proof.identityEntries); err != nil {
		return err
	}
	if err := compareRecoveryDirectories(ctx, originalDir, snapshotDir, relative, proof); err != nil {
		return err
	}
	originalAfter, err := workspaces.PinnedDirectoryInfo(originalDir)
	if err != nil {
		return err
	}
	return verifyRecoverySourceInfo(originalInfo, originalAfter)
}

func permissionOnlyModeDifference(original, snapshot os.FileMode, regular bool) (bool, error) {
	if original.Type() != snapshot.Type() {
		return false, errors.New("historical snapshot entry type changed")
	}
	originalSpecial := original & recoverySpecialMode
	snapshotSpecial := snapshot & recoverySpecialMode
	if snapshotSpecial&^originalSpecial != 0 {
		return false, errors.New("historical snapshot added special permission bits")
	}
	if !regular && original.Perm() != snapshot.Perm() {
		return false, errors.New("historical snapshot changed directory permissions")
	}
	if regular && snapshot.Perm()&^original.Perm() != 0 {
		return false, errors.New("historical snapshot added regular-file permissions")
	}
	return originalSpecial != snapshotSpecial || regular && original.Perm() != snapshot.Perm(), nil
}

func compareRecoveryRegularFiles(
	ctx context.Context,
	original, snapshot workspaces.DirectoryHandle,
	name, relative string,
	originalMode, snapshotMode os.FileMode,
	proof *recoveryPermissionProof,
) error {
	originalReader, err := original.OpenFile(name)
	if err != nil {
		return err
	}
	originalFile, ok := originalReader.(*os.File)
	if !ok {
		_ = originalReader.Close()
		return errors.New("pinned recovery file has no file descriptor")
	}
	defer func() { _ = originalFile.Close() }()
	snapshotReader, err := snapshot.OpenFile(name)
	if err != nil {
		return err
	}
	snapshotFile, ok := snapshotReader.(*os.File)
	if !ok {
		_ = snapshotReader.Close()
		return errors.New("pinned snapshot file has no file descriptor")
	}
	defer func() { _ = snapshotFile.Close() }()
	originalInfo, err := originalFile.Stat()
	if err != nil {
		return err
	}
	snapshotInfo, err := snapshotFile.Stat()
	if err != nil {
		return err
	}
	if originalInfo.Mode() != originalMode || snapshotInfo.Mode() != snapshotMode {
		return errors.New("historical snapshot file changed during proof")
	}
	if err := verifyHistoricalRecoveryIdentity(originalInfo, snapshotInfo); err != nil {
		return err
	}
	if originalInfo.Size() != snapshotInfo.Size() {
		return errors.New("historical snapshot file content changed")
	}
	contentHash, err := compareRecoveryFileBytes(ctx, originalFile, snapshotFile)
	if err != nil {
		return err
	}
	proof.manifestEntries = append(proof.manifestEntries, relative+"|"+originalInfo.Mode().String()+"|"+contentHash)
	if err := appendRecoveryIdentityEntry(relative, originalInfo, &proof.identityEntries); err != nil {
		return err
	}
	originalAfter, err := originalFile.Stat()
	if err != nil {
		return err
	}
	return verifyRecoverySourceInfo(originalInfo, originalAfter)
}

func verifyHistoricalRecoveryIdentity(source, snapshot os.FileInfo) error {
	wantUID, wantGID, requireUID, requireGID, err := recoveryRequiredIdentity(source)
	if err != nil {
		return err
	}
	checkUID := requireUID && snapshot.Mode()&os.ModeSetuid != 0
	checkGID := requireGID && snapshot.Mode()&os.ModeSetgid != 0
	gotUID, gotGID, _, _, err := recoveryRequiredIdentityForMode(snapshot, checkUID, checkGID)
	if err != nil {
		return err
	}
	if checkUID && wantUID != gotUID || checkGID && wantGID != gotGID {
		return errors.New("historical snapshot set-ID identity changed")
	}
	return nil
}

func compareRecoveryFileBytes(ctx context.Context, original, snapshot io.Reader) (string, error) {
	originalBuffer := make([]byte, 32*1024)
	snapshotBuffer := make([]byte, len(originalBuffer))
	hash := sha256.New()
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		originalCount, originalErr := io.ReadFull(original, originalBuffer)
		snapshotCount, snapshotErr := io.ReadFull(snapshot, snapshotBuffer)
		if originalCount != snapshotCount || !bytes.Equal(originalBuffer[:originalCount], snapshotBuffer[:snapshotCount]) {
			return "", errors.New("historical snapshot file content changed")
		}
		_, _ = hash.Write(originalBuffer[:originalCount])
		originalEOF := errors.Is(originalErr, io.EOF) || errors.Is(originalErr, io.ErrUnexpectedEOF)
		snapshotEOF := errors.Is(snapshotErr, io.EOF) || errors.Is(snapshotErr, io.ErrUnexpectedEOF)
		if originalEOF || snapshotEOF {
			if originalEOF && snapshotEOF {
				return hex.EncodeToString(hash.Sum(nil)), nil
			}
			return "", errors.New("historical snapshot file content changed")
		}
		if originalErr != nil {
			return "", originalErr
		}
		if snapshotErr != nil {
			return "", snapshotErr
		}
	}
}

func beginDirtyCloneRecovery(
	ctx context.Context,
	m *Manager,
	wt *Worktree,
	jobPath string,
	claim *models.TaskEnvironmentRecoveryClaim,
	proof *ManagedCloneRelocationProof,
	relocation managedCloneRelocationRecord,
) (recoveryRecord, string, *recoveryLock, error) {
	if wt == nil || claim == nil || claim.OperationID == "" {
		return recoveryRecord{}, "", nil, errors.New("dirty relocation recovery identity is incomplete")
	}
	lock, err := acquireRecoveryOperation(wt.Path + ".kandev-recovery.claim")
	if err != nil {
		return recoveryRecord{}, "", nil, recoveryAlreadyClaimedError(wt, err.Error())
	}
	existing, err := readRecoveryRecord(jobPath)
	if errors.Is(err, os.ErrNotExist) {
		return createDirtyCloneRecovery(wt, jobPath, claim.OperationID, lock)
	}
	if err != nil {
		_ = lock.Close()
		return recoveryRecord{}, "", nil, recoveryAlreadyClaimedError(wt, "recovery record is unreadable")
	}
	if existing.State != RecoveryStateBlocked {
		if existing.ModeRetry != nil {
			if err := verifyInterruptedPermissionRetry(ctx, wt.Path, existing); err != nil {
				_ = lock.Close()
				return recoveryRecord{}, "", nil,
					managedCloneRelocationError(wt.TaskID, "interrupted permission retry no longer matches original evidence")
			}
		}
		return adoptDirtyCloneRecovery(wt, claim, existing, lock)
	}
	return beginBlockedDirtyCloneRecovery(ctx, m, wt, jobPath, claim, proof, relocation, existing, lock)
}

func createDirtyCloneRecovery(
	wt *Worktree,
	jobPath, operationID string,
	lock *recoveryLock,
) (recoveryRecord, string, *recoveryLock, error) {
	record, snapshot, err := loadOrClaimRecoveryWithOperation(wt, jobPath, operationID)
	if err != nil {
		_ = lock.Close()
		return recoveryRecord{}, "", nil, err
	}
	return record, snapshot, lock, nil
}

func adoptDirtyCloneRecovery(
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	existing recoveryRecord,
	lock *recoveryLock,
) (recoveryRecord, string, *recoveryLock, error) {
	record, snapshot, err := adoptRecoveryRecord(wt, existing)
	if err == nil && existing.OperationID == claim.OperationID {
		return record, snapshot, lock, nil
	}
	_ = lock.Close()
	if err != nil {
		return recoveryRecord{}, "", nil, err
	}
	return recoveryRecord{}, "", nil,
		recoveryAlreadyClaimedError(wt, "recovery record operation ID does not match the durable claim")
}

func verifyInterruptedPermissionRetry(ctx context.Context, original string, record recoveryRecord) error {
	if !validRecoveryModeRetry(record, original) {
		return errors.New("interrupted permission retry record is invalid")
	}
	if err := validateDirtyCloneRelocationAuthorization(ctx); err != nil {
		return err
	}
	proof, err := provePermissionOnlySnapshot(ctx, original, record.ModeRetry.PreviousSnapshot)
	if err != nil {
		return err
	}
	if proof.manifest != record.ModeRetry.SourceManifest ||
		proof.identityManifest != record.ModeRetry.SourceIdentityManifest {
		return errors.New("original checkout changed since permission retry proof")
	}
	return validateDirtyCloneRelocationAuthorization(ctx)
}

func beginBlockedDirtyCloneRecovery(
	ctx context.Context,
	m *Manager,
	wt *Worktree,
	jobPath string,
	claim *models.TaskEnvironmentRecoveryClaim,
	proof *ManagedCloneRelocationProof,
	relocation managedCloneRelocationRecord,
	existing recoveryRecord,
	lock *recoveryLock,
) (recoveryRecord, string, *recoveryLock, error) {
	req := RecoveryAdmissionRequest{TaskID: wt.TaskID, RelocateDirty: true}
	if err := blockedPermissionRetryCandidate(req, RecoverySlot{Worktree: wt, CloneRelocation: proof}, existing, relocation); err != nil {
		_ = lock.Close()
		return recoveryRecord{}, "", nil, err
	}
	if existing.OperationID != claim.OperationID {
		_ = lock.Close()
		return recoveryRecord{}, "", nil, recoveryAlreadyClaimedError(wt, "blocked recovery operation ID does not match the durable claim")
	}
	sourceProof, err := m.verifyPermissionOnlyRetry(ctx, wt, proof, existing, relocation)
	if err != nil {
		_ = lock.Close()
		return recoveryRecord{}, "", nil, managedCloneRelocationError(wt.TaskID, "historical snapshot is not a proven permission-only failure")
	}
	newSnapshot := permissionRetrySnapshotPath(wt.Path, existing.OperationID)
	if _, err := os.Lstat(newSnapshot); err == nil || !errors.Is(err, os.ErrNotExist) {
		_ = lock.Close()
		return recoveryRecord{}, "", nil, recoveryAlreadyClaimedError(wt, "permission retry snapshot path is already occupied")
	}
	retry := &recoveryModeRetry{
		Version: 1, PreviousSnapshot: existing.Snapshot, PreviousError: existing.Error,
		PreviousUpdatedAt: existing.UpdatedAt, SourceManifest: sourceProof.manifest,
		SourceIdentityManifest: sourceProof.identityManifest,
	}
	existing.Snapshot, existing.Manifest, existing.State, existing.Error = newSnapshot, "", RecoveryStateSnapshotting, ""
	existing.ModeRetry, existing.UpdatedAt = retry, time.Now().UTC()
	if err := writeRecoveryRecord(jobPath, existing); err != nil {
		_ = lock.Close()
		return recoveryRecord{}, "", nil, err
	}
	return existing, newSnapshot, lock, nil
}
