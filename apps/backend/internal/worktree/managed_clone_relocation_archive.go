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

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryartifact"
)

const (
	managedCloneRecoveryDirectory          = ".kandev-recovery"
	managedCloneRecoveryRecordsDirectory   = "records"
	managedCloneRecoverySnapshotsDirectory = "snapshots"
	managedCloneRelocationRecordFilename   = "relocation.json"
	managedCloneRecoveryRecordFilename     = "recovery.json"
)

type recoveryClaimReader interface {
	GetTaskEnvironmentRecoveryClaim(context.Context, string) (*models.TaskEnvironmentRecoveryClaim, error)
}

type recoveryArtifactRegistry interface {
	RegisterTaskEnvironmentRecoveryArtifacts(context.Context, recoveryartifact.Registration) error
	ListTaskEnvironmentRecoveryArtifacts(context.Context, string) ([]recoveryartifact.Registered, error)
}

type managedCloneRecoveryArtifactPaths struct {
	LayoutVersion    int
	Bucket           string
	RelocationRecord string
	RelocationClaim  string
	RecoveryRecord   string
	RecoveryClaim    string
	Snapshot         string
	ReplacementID    string
}

func readManagedClonePrivateRecord(path string) ([]byte, bool, error) {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(path) || clean != path {
		return nil, false, nil
	}
	directory := filepath.Dir(clean)
	bucket := filepath.Dir(directory)
	privateRoot := filepath.Dir(bucket)
	if filepath.Base(directory) != managedCloneRecoveryRecordsDirectory || filepath.Base(privateRoot) != managedCloneRecoveryDirectory ||
		filepath.Base(clean) != managedCloneRelocationRecordFilename && filepath.Base(clean) != managedCloneRecoveryRecordFilename {
		return nil, false, nil
	}
	digest := filepath.Base(bucket)
	if len(digest) != sha256.Size*2 {
		return nil, true, errors.New("private recovery bucket identity is invalid")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return nil, true, errors.New("private recovery bucket identity is invalid")
	}
	root := filepath.Dir(privateRoot)
	handle, err := workspaces.OpenDirectoryNoFollow(root, directory)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = handle.Close() }()
	if err := handle.VerifyPath(directory); err != nil {
		return nil, true, err
	}
	data, err := handle.ReadFile(filepath.Base(clean))
	return data, true, err
}

func isManagedClonePrivateRecordPath(path string) bool {
	clean := filepath.Clean(path)
	directory := filepath.Dir(clean)
	bucket := filepath.Dir(directory)
	return filepath.IsAbs(path) && clean == path && filepath.Base(directory) == managedCloneRecoveryRecordsDirectory &&
		filepath.Base(filepath.Dir(bucket)) == managedCloneRecoveryDirectory &&
		(filepath.Base(clean) == managedCloneRelocationRecordFilename || filepath.Base(clean) == managedCloneRecoveryRecordFilename)
}

func verifyManagedClonePrivateRecordDirectory(path string) error {
	return verifyManagedClonePrivateArtifactDirectory(path, managedCloneRecoveryRecordsDirectory, "private recovery record path is invalid")
}

func verifyManagedClonePrivateSnapshotDirectory(snapshot string) error {
	return verifyManagedClonePrivateArtifactDirectory(snapshot, managedCloneRecoverySnapshotsDirectory, "private recovery snapshot path is invalid")
}

func verifyManagedClonePrivateArtifactDirectory(path, expectedDirectory, invalidPathMessage string) error {
	clean := filepath.Clean(path)
	directory := filepath.Dir(clean)
	bucket := filepath.Dir(directory)
	privateRoot := filepath.Dir(bucket)
	if !validManagedClonePrivateArtifactDirectory(path, clean, directory, privateRoot, filepath.Base(bucket), expectedDirectory) {
		return errors.New(invalidPathMessage)
	}
	root := filepath.Dir(privateRoot)
	handle, err := workspaces.OpenDirectoryNoFollow(root, directory)
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()
	return handle.VerifyPath(directory)
}

func validManagedClonePrivateArtifactDirectory(path, clean, directory, privateRoot, bucketDigest, expectedDirectory string) bool {
	if !filepath.IsAbs(path) || clean != path || filepath.Base(directory) != expectedDirectory ||
		filepath.Base(privateRoot) != managedCloneRecoveryDirectory || len(bucketDigest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(bucketDigest)
	return err == nil
}

func (m *Manager) managedCloneRecoveryArtifactPaths(record managedCloneRelocationRecord) (managedCloneRecoveryArtifactPaths, error) {
	if record.LayoutVersion == 1 {
		recoveryPath := record.OriginalWorkspacePath + ".kandev-recovery.json"
		snapshot := record.OriginalWorkspacePath + ".kandev-recovery-" + record.OperationID
		if existing, err := readRecoveryRecord(recoveryPath); err == nil {
			snapshot = existing.Snapshot
		} else if !errors.Is(err, os.ErrNotExist) {
			return managedCloneRecoveryArtifactPaths{}, err
		}
		archive, err := m.managedCloneRelocationArchivePath(&record)
		if err != nil {
			return managedCloneRecoveryArtifactPaths{}, err
		}
		return managedCloneRecoveryArtifactPaths{
			LayoutVersion: 1, Bucket: filepath.Dir(archive),
			RelocationRecord: record.OriginalWorkspacePath + ".kandev-clone-relocation.json",
			RelocationClaim:  record.OriginalWorkspacePath + ".kandev-clone-relocation.claim",
			RecoveryRecord:   recoveryPath,
			RecoveryClaim:    record.OriginalWorkspacePath + ".kandev-recovery.claim",
			Snapshot:         snapshot, ReplacementID: record.ReplacementID,
		}, nil
	}
	archive, err := m.managedCloneRelocationArchivePath(&record)
	if err != nil {
		return managedCloneRecoveryArtifactPaths{}, err
	}
	bucket := filepath.Dir(archive)
	return managedCloneRecoveryArtifactPaths{
		LayoutVersion: 2, Bucket: bucket,
		RelocationRecord: filepath.Join(bucket, managedCloneRecoveryRecordsDirectory, managedCloneRelocationRecordFilename),
		RelocationClaim:  filepath.Join(bucket, "claims", "relocation.claim"),
		RecoveryRecord:   filepath.Join(bucket, managedCloneRecoveryRecordsDirectory, managedCloneRecoveryRecordFilename),
		RecoveryClaim:    filepath.Join(bucket, "claims", "recovery.claim"),
		Snapshot:         filepath.Join(bucket, managedCloneRecoverySnapshotsDirectory, record.OperationID),
		ReplacementID:    record.ReplacementID,
	}, nil
}

func (m *Manager) prepareManagedCloneArtifactLayout(
	ctx context.Context,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	dirty bool,
) (managedCloneRecoveryArtifactPaths, error) {
	if err := validateManagedCloneArtifactIdentity(wt, claim); err != nil {
		return managedCloneRecoveryArtifactPaths{}, err
	}
	paths, found, err := m.selectedLegacyManagedCloneArtifactPaths(wt, claim, dirty)
	if err != nil {
		return managedCloneRecoveryArtifactPaths{}, err
	}
	if found {
		return paths, nil
	}
	return m.createManagedCloneArtifactLayout(ctx, wt, claim)
}

func validateManagedCloneArtifactIdentity(wt *Worktree, claim *models.TaskEnvironmentRecoveryClaim) error {
	if wt == nil || claim == nil || claim.OperationID == "" || wt.TaskEnvironmentID == "" || wt.RepositoryID == "" {
		return managedCloneRelocationError(wtTaskID(wt), "recovery artifact identity is incomplete")
	}
	if _, err := uuid.Parse(claim.OperationID); err != nil {
		return managedCloneRelocationError(wt.TaskID, "recovery artifact operation identity is invalid")
	}
	return nil
}

func (m *Manager) selectedLegacyManagedCloneArtifactPaths(
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	dirty bool,
) (managedCloneRecoveryArtifactPaths, bool, error) {
	legacy, err := readManagedCloneRelocationRecord(wt.Path + ".kandev-clone-relocation.json")
	if errors.Is(err, os.ErrNotExist) {
		return managedCloneRecoveryArtifactPaths{}, false, nil
	}
	if err != nil {
		return managedCloneRecoveryArtifactPaths{}, false, managedCloneRelocationError(wt.TaskID, "legacy relocation record is unreadable")
	}
	if !matchesSelectedLegacyManagedClone(wt, claim, legacy) {
		return managedCloneRecoveryArtifactPaths{}, false, managedCloneRelocationError(wt.TaskID, "legacy relocation identity does not match the selected operation")
	}
	paths, err := m.managedCloneRecoveryArtifactPaths(legacy)
	if err != nil {
		return managedCloneRecoveryArtifactPaths{}, false, managedCloneRelocationError(wt.TaskID, "legacy recovery artifact paths are unreadable")
	}
	if dirty {
		if err := validateSelectedLegacyRecoveryRecord(wt, legacy, paths); err != nil {
			return managedCloneRecoveryArtifactPaths{}, false, err
		}
	}
	return paths, true, nil
}

func matchesSelectedLegacyManagedClone(wt *Worktree, claim *models.TaskEnvironmentRecoveryClaim, record managedCloneRelocationRecord) bool {
	return record.LayoutVersion == 1 && record.OperationID == claim.OperationID && record.TaskID == wt.TaskID &&
		record.EnvironmentID == wt.TaskEnvironmentID && record.WorktreeID == wt.ID && record.OriginalWorkspacePath == wt.Path
}

func validateSelectedLegacyRecoveryRecord(
	wt *Worktree,
	legacy managedCloneRelocationRecord,
	paths managedCloneRecoveryArtifactPaths,
) error {
	recovery, err := readRecoveryRecord(paths.RecoveryRecord)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return managedCloneRelocationError(wt.TaskID, "legacy recovery record is unreadable")
	}
	if !matchesSelectedLegacyRecovery(wt, legacy, recovery) {
		return managedCloneRelocationError(wt.TaskID, "legacy recovery record identity is ambiguous")
	}
	if recovery.ModeRetry != nil && !validRecoveryModeRetry(recovery, wt.Path) {
		return managedCloneRelocationError(wt.TaskID, "legacy permission retry proof is invalid")
	}
	return nil
}

func matchesSelectedLegacyRecovery(wt *Worktree, legacy managedCloneRelocationRecord, recovery recoveryRecord) bool {
	return recovery.LayoutVersion == 1 && recovery.OperationID == legacy.OperationID && recovery.TaskID == legacy.TaskID &&
		recovery.WorktreeID == legacy.WorktreeID && recovery.Original == legacy.OriginalWorkspacePath && wt.Path == legacy.OriginalWorkspacePath
}

func (m *Manager) createManagedCloneArtifactLayout(
	ctx context.Context,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
) (managedCloneRecoveryArtifactPaths, error) {
	record := managedCloneRelocationRecord{
		LayoutVersion: 2,
		OperationID:   claim.OperationID, TaskID: wt.TaskID, EnvironmentID: wt.TaskEnvironmentID,
		WorktreeID: wt.ID, Original: wt.Path, OriginalWorkspacePath: wt.Path,
		Replacement:   wt.Path + ".relocated-" + claim.OperationID[:8],
		ReplacementID: managedCloneReplacementID(wt, claim.OperationID),
	}
	paths, err := m.managedCloneRecoveryArtifactPaths(record)
	if err != nil {
		return managedCloneRecoveryArtifactPaths{}, managedCloneRelocationError(wt.TaskID, "cannot resolve private recovery storage")
	}
	registration := managedCloneArtifactRegistration(wt, claim, record, paths, recoveryartifact.ProvenanceV2Operation)
	if err := m.registerManagedCloneArtifactRegistration(ctx, registration); err != nil {
		return managedCloneRecoveryArtifactPaths{}, managedCloneRelocationError(wt.TaskID, "private recovery artifact registry is unavailable")
	}
	if err := ensureManagedCloneRecoveryDirectories(paths); err != nil {
		return managedCloneRecoveryArtifactPaths{}, managedCloneRelocationError(wt.TaskID, "private recovery storage is unavailable")
	}
	return paths, nil
}

func managedCloneReplacementID(wt *Worktree, operationID string) string {
	identity := strings.Join([]string{wt.TaskEnvironmentID, wt.ID, operationID, "replacement"}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(hex.EncodeToString(digest[:]))).String()
}

func (m *Manager) registerManagedCloneArtifactPaths(
	ctx context.Context,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	record managedCloneRelocationRecord,
	additional []string,
) error {
	if _, ok := m.store.(recoveryArtifactRegistry); !ok && record.LayoutVersion == 1 {
		return nil
	}
	paths, err := m.managedCloneRecoveryArtifactPaths(record)
	if err != nil {
		return err
	}
	provenance := recoveryartifact.ProvenanceV2Operation
	if paths.LayoutVersion == 1 {
		provenance = recoveryartifact.ProvenanceLegacyAdmission
	}
	registration := managedCloneArtifactRegistration(wt, claim, record, paths, provenance)
	registration.ArtifactPaths = append(registration.ArtifactPaths, additional...)
	return m.registerManagedCloneArtifactRegistration(ctx, registration)
}

func (m *Manager) registerManagedCloneArtifactRegistration(ctx context.Context, registration recoveryartifact.Registration) error {
	registry, ok := m.store.(recoveryArtifactRegistry)
	if !ok {
		return errors.New("managed clone recovery artifact registry is unavailable")
	}
	return registry.RegisterTaskEnvironmentRecoveryArtifacts(ctx, registration)
}

func managedCloneArtifactRegistration(
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	record managedCloneRelocationRecord,
	paths managedCloneRecoveryArtifactPaths,
	provenance string,
) recoveryartifact.Registration {
	return recoveryartifact.Registration{
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnerTaskID: claim.OwnerTaskID,
		OwnershipGeneration: claim.OwnershipGeneration, SessionID: claim.SessionID,
		OperationID: claim.OperationID, ExecutorType: claim.ExecutorType,
		WorktreeID: record.WorktreeID, RepositoryID: wt.RepositoryID,
		OriginalPath: record.OriginalWorkspacePath, ReplacementID: record.ReplacementID,
		ReplacementPath: record.Replacement, LayoutVersion: record.LayoutVersion,
		Provenance:    provenance,
		ArtifactPaths: []string{paths.RelocationRecord, paths.RelocationClaim},
	}
}

func ensureManagedCloneRecoveryDirectories(paths managedCloneRecoveryArtifactPaths) error {
	for _, target := range []string{
		paths.Bucket,
		filepath.Join(paths.Bucket, managedCloneRecoveryRecordsDirectory),
		filepath.Join(paths.Bucket, "claims"),
		filepath.Join(paths.Bucket, managedCloneRecoverySnapshotsDirectory),
	} {
		root := filepath.Dir(target)
		if target == paths.Bucket {
			root = filepath.Dir(target)
		}
		handle, err := workspaces.CreateDirectoryNoFollow(root, target, 0o700)
		if err != nil {
			// The target may already exist. Opening it with no-follow semantics
			// pins the directory and rejects a substituted symlink.
			handle, err = workspaces.OpenDirectoryNoFollow(root, target)
		}
		if err != nil {
			return err
		}
		if err := handle.VerifyPath(target); err != nil {
			_ = handle.Close()
			return err
		}
		if err := handle.Close(); err != nil {
			return err
		}
	}
	return nil
}

func writePublishedManagedCloneRelocationRecord(record *managedCloneRelocationRecord) error {
	if record == nil || record.Replacement == "" || record.State != managedCloneRelocationStateMaterialized {
		return errors.New("replacement relocation record is incomplete")
	}
	if record.LayoutVersion == 2 {
		return nil
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
	return filepath.Join(tasksBase, managedCloneRecoveryDirectory, hex.EncodeToString(digest[:]), filepath.Base(originalPath)), nil
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
	recordPath, record, found, err := m.readRegisteredPublishedRelocation(ctx, req, wt)
	if err != nil {
		return false, recoveryAdmissionError(*req, "registered managed-clone relocation record is unreadable")
	}
	if !found {
		recordPath = wt.Path + ".kandev-clone-relocation.json"
		record, err = readManagedCloneRelocationRecord(recordPath)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, recoveryAdmissionError(*req, "managed-clone relocation record is unreadable")
		}
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
	if err := finishPublishedManagedCloneRelocation(ctx, m, req, recordPath, &record); err != nil {
		return false, err
	}
	if !found && claim != nil && record.LayoutVersion == 1 {
		if err := m.registerVerifiedLegacyRelocation(ctx, req, wt, claim, record); err != nil {
			return false, recoveryAdmissionError(*req, "legacy relocation artifact ownership could not be registered")
		}
	}
	if claim != nil {
		if err := m.releaseRecoveryClaim(ctx, claim); err != nil {
			return false, recoveryAdmissionError(*req, "published relocation claim could not be released")
		}
	}
	return true, nil
}

func (m *Manager) readRegisteredPublishedRelocation(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	wt *Worktree,
) (string, managedCloneRelocationRecord, bool, error) {
	registry, ok := m.store.(recoveryArtifactRegistry)
	if !ok {
		return "", managedCloneRelocationRecord{}, false, nil
	}
	registered, err := registry.ListTaskEnvironmentRecoveryArtifacts(ctx, req.TaskEnvironmentID)
	if err != nil {
		return "", managedCloneRelocationRecord{}, false, err
	}
	for _, item := range registered {
		if !registeredRelocationMatchesPublishedWorktree(item, req, wt) {
			continue
		}
		path, found := registeredRelocationRecordPath(item.ArtifactPaths)
		if !found {
			return "", managedCloneRelocationRecord{}, true, errors.New("registered relocation path is missing")
		}
		record, err := readRegisteredRelocationRecord(item, path)
		if err != nil {
			return "", managedCloneRelocationRecord{}, true, err
		}
		return path, record, true, nil
	}
	return "", managedCloneRelocationRecord{}, false, nil
}

func registeredRelocationMatchesPublishedWorktree(
	item recoveryartifact.Registered,
	req *RecoveryAdmissionRequest,
	wt *Worktree,
) bool {
	return item.LayoutVersion == 2 && item.OwnerTaskID == req.OwnerTaskID &&
		item.OwnershipGeneration == req.OwnershipGeneration && item.ReplacementID == wt.ID &&
		item.ReplacementPath == wt.Path && item.RepositoryID == wt.RepositoryID
}

func registeredRelocationRecordPath(paths []string) (string, bool) {
	for _, path := range paths {
		if filepath.Base(path) == managedCloneRelocationRecordFilename &&
			filepath.Base(filepath.Dir(path)) == managedCloneRecoveryRecordsDirectory {
			return path, true
		}
	}
	return "", false
}

func readRegisteredRelocationRecord(
	item recoveryartifact.Registered,
	path string,
) (managedCloneRelocationRecord, error) {
	record, err := readManagedCloneRelocationRecord(path)
	if err != nil || !registeredRelocationRecordMatches(item, record) {
		return managedCloneRelocationRecord{}, errors.New("registered relocation record differs from its ownership proof")
	}
	return record, nil
}

func registeredRelocationRecordMatches(item recoveryartifact.Registered, record managedCloneRelocationRecord) bool {
	return record.LayoutVersion == 2 && record.OperationID == item.OperationID &&
		record.TaskID == item.OwnerTaskID && record.EnvironmentID == item.TaskEnvironmentID &&
		record.WorktreeID == item.WorktreeID && record.OriginalWorkspacePath == item.OriginalPath &&
		record.ReplacementID == item.ReplacementID && record.Replacement == item.ReplacementPath
}

func (m *Manager) registerVerifiedLegacyRelocation(
	ctx context.Context,
	req *RecoveryAdmissionRequest,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	record managedCloneRelocationRecord,
) error {
	if req == nil || wt == nil || claim == nil || !m.validLegacyRelocationArtifactJournal(req, wt, claim, record) {
		return errors.New("legacy relocation proof does not match the published slot")
	}
	recordPath := wt.Path + ".kandev-clone-relocation.json"
	if err := verifyCurrentLegacyRelocationJournal(recordPath, record); err != nil {
		return err
	}
	paths := verifiedLegacyRelocationJournalPaths(record)
	paths = append(paths, verifiedLegacyRecoveryArtifactPaths(record)...)
	registration := recoveryartifact.Registration{
		TaskEnvironmentID: claim.TaskEnvironmentID, OwnerTaskID: claim.OwnerTaskID,
		OwnershipGeneration: claim.OwnershipGeneration, SessionID: claim.SessionID,
		OperationID: claim.OperationID, ExecutorType: claim.ExecutorType,
		WorktreeID: record.WorktreeID, RepositoryID: wt.RepositoryID,
		OriginalPath: record.OriginalWorkspacePath, ReplacementID: record.ReplacementID,
		ReplacementPath: record.Replacement, LayoutVersion: 1,
		Provenance: recoveryartifact.ProvenanceLegacyPublished, ArtifactPaths: paths,
	}
	return m.registerManagedCloneArtifactRegistration(ctx, registration)
}

func verifyCurrentLegacyRelocationJournal(path string, expected managedCloneRelocationRecord) error {
	if !isRegularNoFollow(path) {
		return errors.New("legacy relocation journal is not a regular file")
	}
	current, err := readManagedCloneRelocationRecord(path)
	if err != nil || !sameLegacyRelocationJournal(current, expected) {
		return errors.New("legacy relocation journal does not match the published slot")
	}
	return nil
}

func verifiedLegacyRelocationJournalPaths(record managedCloneRelocationRecord) []string {
	paths := make([]string, 0, 2)
	for _, path := range []string{
		record.OriginalWorkspacePath + ".kandev-clone-relocation.json",
		record.Replacement + ".kandev-clone-relocation.json",
	} {
		if isCurrentLegacyRelocationJournal(path, record) {
			paths = append(paths, path)
		}
	}
	return paths
}

func isCurrentLegacyRelocationJournal(path string, expected managedCloneRelocationRecord) bool {
	if !isRegularNoFollow(path) {
		return false
	}
	current, err := readManagedCloneRelocationRecord(path)
	return err == nil && sameLegacyRelocationJournal(current, expected)
}

func verifiedLegacyRecoveryArtifactPaths(record managedCloneRelocationRecord) []string {
	path := record.OriginalWorkspacePath + ".kandev-recovery.json"
	if !isRegularNoFollow(path) {
		return nil
	}
	recovery, err := readRecoveryRecord(path)
	if err != nil || !validLegacyRecoveryArtifactJournal(recovery, record) {
		return nil
	}
	paths := []string{path}
	if legacyRecoverySnapshotIsCurrentlyProven(record, recovery) {
		paths = append(paths, recovery.Snapshot)
	}
	if legacyRecoveryPreviousSnapshotIsCurrentlyProven(record, recovery) {
		paths = append(paths, recovery.ModeRetry.PreviousSnapshot)
	}
	return paths
}

func legacyRecoverySnapshotIsCurrentlyProven(record managedCloneRelocationRecord, recovery recoveryRecord) bool {
	return legacyRecoverySnapshotPathAllowed(record.OriginalWorkspacePath, record.OperationID, recovery.Snapshot) &&
		validateRecoverySnapshotPath(record.OriginalWorkspacePath, recovery.Snapshot) == nil
}

func legacyRecoveryPreviousSnapshotIsCurrentlyProven(record managedCloneRelocationRecord, recovery recoveryRecord) bool {
	return recovery.ModeRetry != nil && validLegacyRecoveryModeRetry(recovery, record)
}

func isRegularNoFollow(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func (m *Manager) validLegacyRelocationArtifactJournal(
	req *RecoveryAdmissionRequest,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	record managedCloneRelocationRecord,
) bool {
	if req == nil || wt == nil || claim == nil || !validLegacyClaimOperation(claim) {
		return false
	}
	if !legacyClaimMatchesPublishedSlot(req, wt, claim) || !legacyRelocationMatchesPublishedSlot(req, wt, claim, record) {
		return false
	}
	if filepath.Clean(record.Original) == filepath.Clean(record.OriginalWorkspacePath) {
		return true
	}
	if record.State != string(RecoveryStateComplete) {
		return false
	}
	archivePath, err := m.managedCloneRelocationArchivePath(&record)
	return err == nil && filepath.Clean(record.Original) == filepath.Clean(archivePath)
}

func validLegacyClaimOperation(claim *models.TaskEnvironmentRecoveryClaim) bool {
	if claim.OperationID == "" || len(claim.OperationID) < 8 {
		return false
	}
	_, err := uuid.Parse(claim.OperationID)
	return err == nil
}

func legacyClaimMatchesPublishedSlot(
	req *RecoveryAdmissionRequest,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
) bool {
	return claim.TaskEnvironmentID == req.TaskEnvironmentID && claim.OwnerTaskID == req.OwnerTaskID &&
		claim.OwnershipGeneration == req.OwnershipGeneration && claim.SessionID == req.SessionID &&
		claim.ExecutorType == req.ExecutorType && wt.TaskID == req.OwnerTaskID &&
		wt.TaskEnvironmentID == req.TaskEnvironmentID
}

func legacyRelocationMatchesPublishedSlot(
	req *RecoveryAdmissionRequest,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	record managedCloneRelocationRecord,
) bool {
	return validLegacyRelocationIdentity(req, wt, claim, record) &&
		validLegacyRelocationRepositories(wt, record) && validLegacyRelocationSource(record) &&
		validLegacyRelocationDestination(record) && validLegacyRelocationState(record)
}

func validLegacyRelocationIdentity(
	req *RecoveryAdmissionRequest,
	wt *Worktree,
	claim *models.TaskEnvironmentRecoveryClaim,
	record managedCloneRelocationRecord,
) bool {
	original := record.OriginalWorkspacePath
	return record.LayoutVersion == 1 && record.OperationID == claim.OperationID &&
		record.TaskID == req.OwnerTaskID && record.EnvironmentID == req.TaskEnvironmentID &&
		record.WorktreeID != "" && record.ReplacementID == wt.ID && record.Replacement == wt.Path &&
		record.Original != "" && filepath.IsAbs(original) && filepath.Clean(original) == original &&
		record.Replacement == original+".relocated-"+record.OperationID[:8]
}

func validLegacyRelocationRepositories(wt *Worktree, record managedCloneRelocationRecord) bool {
	if record.DestPath == "" || wt.RepositoryPath == "" || record.SourcePath == "" {
		return false
	}
	return filepath.Clean(record.DestPath) == filepath.Clean(wt.RepositoryPath) &&
		filepath.Clean(record.DestCommon) == filepath.Join(filepath.Clean(record.DestPath), ".git") &&
		filepath.IsAbs(record.SourcePath) && filepath.Clean(record.SourcePath) == record.SourcePath &&
		filepath.Clean(record.SourceCommon) == filepath.Join(filepath.Clean(record.SourcePath), ".git")
}

func validLegacyRelocationSource(record managedCloneRelocationRecord) bool {
	return relocationCommitPattern.MatchString(record.Head) && strings.TrimSpace(record.Branch) != ""
}

func validLegacyRelocationDestination(record managedCloneRelocationRecord) bool {
	return record.DestPath != "" && record.DestCommon != ""
}

func validLegacyRelocationState(record managedCloneRelocationRecord) bool {
	return record.State == managedCloneRelocationStateMaterialized || record.State == string(RecoveryStateComplete)
}

func sameLegacyRelocationJournal(current, expected managedCloneRelocationRecord) bool {
	return managedCloneRelocationRecordMatches(current, expected) &&
		current.OriginalWorkspacePath == expected.OriginalWorkspacePath &&
		current.ReplacementID == expected.ReplacementID && current.EnvironmentID == expected.EnvironmentID &&
		current.State == expected.State
}

func validLegacyRecoveryArtifactJournal(recovery recoveryRecord, relocation managedCloneRelocationRecord) bool {
	if !legacyRecoveryIdentityMatches(recovery, relocation) || !validLegacyRecoveryJournalState(recovery) {
		return false
	}
	return recovery.ModeRetry == nil || validLegacyRecoveryModeRetry(recovery, relocation)
}

func legacyRecoveryIdentityMatches(recovery recoveryRecord, relocation managedCloneRelocationRecord) bool {
	return recovery.LayoutVersion == 1 && recovery.OperationID == relocation.OperationID &&
		recovery.TaskID == relocation.TaskID && recovery.WorktreeID == relocation.WorktreeID &&
		(recovery.Original == relocation.OriginalWorkspacePath || recovery.Original == relocation.Original) &&
		(recovery.Replacement == "" || recovery.Replacement == relocation.Replacement)
}

func validLegacyRecoveryJournalState(recovery recoveryRecord) bool {
	switch recovery.State {
	case RecoveryStateSnapshotting, RecoveryStateRematerializing, RecoveryStateBlocked, RecoveryStateComplete:
	default:
		return false
	}
	if recovery.State == RecoveryStateRematerializing || recovery.State == RecoveryStateComplete {
		return validRecoveryDigest(recovery.Manifest)
	}
	return true
}

func validLegacyRecoveryModeRetry(recovery recoveryRecord, relocation managedCloneRelocationRecord) bool {
	if !validRecoveryModeRetry(recovery, relocation.OriginalWorkspacePath) {
		return false
	}
	return recovery.ModeRetry.PreviousSnapshot == relocation.OriginalWorkspacePath+".kandev-recovery-"+relocation.OperationID
}

func legacyRecoverySnapshotPathAllowed(original, operationID, snapshot string) bool {
	return filepath.Clean(snapshot) == filepath.Clean(original+".kandev-recovery-"+operationID) ||
		filepath.Clean(snapshot) == filepath.Clean(permissionRetrySnapshotPath(original, operationID))
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
) error {
	archivePath, err := m.retainManagedCloneOriginal(ctx, record)
	if err != nil {
		return err
	}
	record.Original = archivePath
	if err := markManagedCloneRelocationComplete(recordPath, record, req.TaskID); err != nil {
		return err
	}
	recoveryPath := record.OriginalWorkspacePath + ".kandev-recovery.json"
	if record.LayoutVersion == 2 {
		recoveryPath = filepath.Join(filepath.Dir(recordPath), "recovery.json")
	}
	if err := reconcilePublishedDirtyRecovery(recoveryPath, *record); err != nil {
		return recoveryAdmissionError(*req, "published relocation recovery journal could not be reconciled")
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

func reconcilePublishedDirtyRecovery(path string, record managedCloneRelocationRecord) error {
	originalPath := record.OriginalWorkspacePath
	if originalPath == "" {
		return nil
	}
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
