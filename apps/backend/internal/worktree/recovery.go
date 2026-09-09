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
)

const recoveryGitDirName = ".git"

// RecoveryState is persisted beside a damaged checkout so a restart observes
// the same operation rather than beginning a second destructive transition.
type RecoveryState string

const (
	RecoveryStateSnapshotting    RecoveryState = "snapshotting"
	RecoveryStateRematerializing RecoveryState = "rematerializing"
	RecoveryStateBlocked         RecoveryState = "blocked"
	RecoveryStateComplete        RecoveryState = "complete"
)

type recoveryRecord struct {
	OperationID string        `json:"operation_id"`
	TaskID      string        `json:"task_id"`
	WorktreeID  string        `json:"worktree_id"`
	Original    string        `json:"original"`
	Snapshot    string        `json:"snapshot"`
	Replacement string        `json:"replacement,omitempty"`
	Manifest    string        `json:"manifest"`
	State       RecoveryState `json:"state"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Error       string        `json:"error,omitempty"`
}

// CompareAndSwapWorktree replaces one exact durable worktree identity. The
// optional interface keeps legacy test stores usable while production stores
// can make the path change atomic with their row update.
type CompareAndSwapWorktreeStore interface {
	CompareAndSwapWorktree(ctx context.Context, expected, replacement *Worktree) (bool, error)
}

var errRecoveryOperationClaimed = errors.New("recovery operation is currently claimed")

// RecoverWorktree snapshots a damaged checkout and rematerializes it beside
// the original. The original and snapshot are retained; callers can validate
// and explicitly clean them up later.
//
//nolint:cyclop,gocognit,nestif,funlen // Recovery is one stateful transaction boundary.
func (m *Manager) RecoverWorktree(ctx context.Context, wt *Worktree, req CreateRequest) (*Worktree, error) {
	if wt == nil || wt.TaskID == "" || wt.TaskID != req.TaskID || wt.Path == "" || req.RepositoryPath == "" {
		return nil, fmt.Errorf("%w: recovery identity is incomplete", ErrWorktreeCorrupted)
	}
	if wt.RepositoryID == "" || wt.RepositoryID != req.RepositoryID ||
		(wt.TaskEnvironmentID != "" && req.TaskEnvironmentID != "" && wt.TaskEnvironmentID != req.TaskEnvironmentID) {
		return nil, fmt.Errorf("%w: recovery request does not match durable repository identity", ErrWorktreeCorrupted)
	}
	repositoryPath, err := filepath.Abs(req.RepositoryPath)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve requested repository path: %v", ErrWorktreeCorrupted, err)
	}
	durableRepositoryPath, err := filepath.Abs(wt.RepositoryPath)
	if err != nil || filepath.Clean(repositoryPath) != filepath.Clean(durableRepositoryPath) {
		return nil, fmt.Errorf("%w: recovery request does not match durable repository path", ErrWorktreeCorrupted)
	}
	inspection := inspectLinkedWorktree(wt.Path)
	if inspection.class != linkedWorktreeMissingAdmin {
		return nil, &WorktreeRecoveryError{
			TaskID: wt.TaskID, Checkout: wt.Path, PointerTarget: inspection.adminPath,
			ExpectedBacklink: inspection.expectedBacklink, ActualBacklink: inspection.actualBacklink,
			State: string(inspection.class), Reason: inspection.reason,
		}
	}
	if _, err := os.Lstat(wt.Path); err != nil {
		return nil, fmt.Errorf("%w: original checkout is unavailable: %v", ErrWorktreeCorrupted, err)
	}
	if err := m.validateExistingWorktreePathOwner(wt.Path, wt); err != nil {
		return nil, fmt.Errorf("%w: recovery ownership validation failed: %w", ErrWorktreeCorrupted, err)
	}
	jobPath := wt.Path + ".kandev-recovery.json"
	record, snapshotPath, claim, err := beginRecovery(wt, jobPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = claim.Close() }()
	manifest, err := prepareRecoverySnapshot(wt.Path, snapshotPath, jobPath, record)
	if err != nil {
		return nil, err
	}
	record.Manifest = manifest
	record.State = RecoveryStateRematerializing
	record.UpdatedAt = time.Now().UTC()
	if err := writeRecoveryRecord(jobPath, record); err != nil {
		return nil, err
	}
	branch := wt.Branch
	if branch == "" {
		branch = wt.BaseBranch
	}
	if branch == "" {
		return nil, blockRecovery(jobPath, record, fmt.Errorf("cannot rematerialize without a validated branch"))
	}
	replacementPath := wt.Path + ".recovered-" + record.OperationID[:8]
	replacementBranch := branch + "-recovered-" + record.OperationID[:8]
	if err := m.ensureRecoveryReplacementWorktree(ctx, req.RepositoryPath, replacementBranch, replacementPath, branch); err != nil {
		return nil, blockRecovery(jobPath, record, err)
	}
	if err := restoreSnapshot(snapshotPath, replacementPath, manifest); err != nil {
		return nil, blockRecovery(jobPath, record, err)
	}
	replacement := *wt
	replacement.ID = uuid.NewString()
	replacement.Path = replacementPath
	replacement.Branch = replacementBranch
	replacement.UpdatedAt = time.Now().UTC()
	if cas, ok := m.store.(CompareAndSwapWorktreeStore); ok {
		swapped, casErr := cas.CompareAndSwapWorktree(ctx, wt, &replacement)
		if casErr != nil {
			return nil, fmt.Errorf("%w: recovery compare-and-swap failed: %w", ErrWorktreeCorrupted, casErr)
		}
		if !swapped {
			persisted, lookupErr := m.persistedRecoveryReplacement(ctx, wt, &replacement)
			if lookupErr != nil {
				return nil, fmt.Errorf("%w: inspect recovery compare-and-swap result: %w", ErrWorktreeCorrupted, lookupErr)
			}
			if persisted == nil {
				return nil, blockRecovery(jobPath, record, fmt.Errorf("recovery compare-and-swap rejected"))
			}
			return m.completeRecovery(jobPath, record, wt, persisted)
		}
	} else if err := m.store.UpdateWorktree(ctx, &replacement); err != nil {
		return nil, blockRecovery(jobPath, record, err)
	}
	return m.completeRecovery(jobPath, record, wt, &replacement)
}

func (m *Manager) persistedRecoveryReplacement(ctx context.Context, expected, replacement *Worktree) (*Worktree, error) {
	worktrees, err := m.store.GetWorktreesByTaskID(ctx, expected.TaskID)
	if err != nil {
		return nil, err
	}
	for _, current := range worktrees {
		if current == nil || current.Status != StatusActive ||
			current.TaskEnvironmentID != expected.TaskEnvironmentID ||
			current.RepositoryID != expected.RepositoryID || current.BranchSlug != expected.BranchSlug ||
			current.Path != replacement.Path || current.Branch != replacement.Branch {
			continue
		}
		if !m.IsValid(current.Path) {
			return nil, fmt.Errorf("persisted recovery replacement failed integrity validation")
		}
		return current, nil
	}
	return nil, nil
}

func (m *Manager) completeRecovery(jobPath string, record recoveryRecord, expected, replacement *Worktree) (*Worktree, error) {
	m.refreshRecoveredWorktreeCache(expected, replacement)
	record.Replacement, record.State, record.UpdatedAt = replacement.Path, RecoveryStateComplete, time.Now().UTC()
	if err := writeRecoveryRecord(jobPath, record); err != nil {
		return nil, err
	}
	return replacement, nil
}

// ensureRecoveryReplacementWorktree resumes the deterministic replacement
// created before a crash, or creates it when the prior attempt did not reach Git.
func (m *Manager) ensureRecoveryReplacementWorktree(ctx context.Context, repositoryPath, branch, replacementPath, baseRef string) error {
	if m.IsValid(replacementPath) {
		currentBranch, err := m.runBoundedGitInspect(ctx, replacementPath, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil || strings.TrimSpace(currentBranch) != branch {
			return fmt.Errorf("recovery replacement has an unexpected branch")
		}
		return nil
	}
	exists, err := m.branchExists(ctx, repositoryPath, "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("inspect recovery replacement branch: %w", err)
	}
	if exists {
		_, err = m.gitAddWorktreeExisting(ctx, repositoryPath, branch, replacementPath)
	} else {
		_, err = m.gitAddWorktree(ctx, repositoryPath, branch, replacementPath, baseRef)
	}
	return err
}

func beginRecovery(wt *Worktree, jobPath string) (recoveryRecord, string, *recoveryLock, error) {
	// The advisory lock is the first durable boundary. Its inode may survive a
	// crash before the record is created, so taking the lock must never depend
	// on the claim path being absent.
	claim, err := acquireRecoveryOperation(wt.Path + ".kandev-recovery.claim")
	if err != nil {
		return recoveryRecord{}, "", nil, recoveryAlreadyClaimedError(wt, err.Error())
	}
	record, snapshotPath, err := loadOrClaimRecovery(wt, jobPath)
	if err != nil {
		_ = claim.Close()
		return recoveryRecord{}, "", nil, err
	}
	return record, snapshotPath, claim, nil
}

func loadOrClaimRecovery(wt *Worktree, jobPath string) (recoveryRecord, string, error) {
	snapshotPath := wt.Path + ".kandev-recovery-" + uuid.NewString()
	record := recoveryRecord{
		OperationID: uuid.NewString(), TaskID: wt.TaskID, WorktreeID: wt.ID,
		Original: wt.Path, Snapshot: snapshotPath, State: RecoveryStateSnapshotting,
		UpdatedAt: time.Now().UTC(),
	}
	if existing, err := readRecoveryRecord(jobPath); err == nil {
		return adoptRecoveryRecord(wt, existing)
	} else if !os.IsNotExist(err) {
		return recoveryRecord{}, "", recoveryAlreadyClaimedError(wt, "recovery record is unreadable")
	}
	if err := createRecoveryRecord(jobPath, record); err != nil {
		if existing, readErr := readRecoveryRecord(jobPath); readErr == nil && existing.TaskID == wt.TaskID && existing.WorktreeID == wt.ID {
			return adoptRecoveryRecord(wt, existing)
		}
		return recoveryRecord{}, "", err
	}
	return record, snapshotPath, nil
}

func adoptRecoveryRecord(wt *Worktree, existing recoveryRecord) (recoveryRecord, string, error) {
	if existing.TaskID != wt.TaskID || existing.WorktreeID != wt.ID {
		return recoveryRecord{}, "", recoveryAlreadyClaimedError(wt, "recovery record ownership is ambiguous")
	}
	if existing.Original != wt.Path || existing.Snapshot == "" {
		return recoveryRecord{}, "", recoveryAlreadyClaimedError(wt, "recovery record identity is incomplete")
	}
	if _, err := uuid.Parse(existing.OperationID); err != nil {
		return recoveryRecord{}, "", recoveryAlreadyClaimedError(wt, "recovery record operation ID is invalid")
	}
	if existing.State == RecoveryStateBlocked || existing.State == RecoveryStateComplete {
		return recoveryRecord{}, "", recoveryStateError(wt, existing.State)
	}
	return existing, existing.Snapshot, nil
}

func createRecoveryRecord(path string, record recoveryRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".kandev-recovery-*")
	if err != nil {
		return fmt.Errorf("create recovery record: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmpPath, path); err != nil {
		return err
	}
	return syncRecoveryDirectory(filepath.Dir(path))
}

func recoveryStateError(wt *Worktree, state RecoveryState) error {
	return &WorktreeRecoveryError{
		TaskID: wt.TaskID, Checkout: wt.Path,
		Reason: fmt.Sprintf("recovery operation is already %s", state),
	}
}

func recoveryAlreadyClaimedError(wt *Worktree, reason string) error {
	return &WorktreeRecoveryError{
		TaskID: wt.TaskID, Checkout: wt.Path,
		Reason: "recovery operation is already claimed: " + reason,
	}
}

func prepareRecoverySnapshot(source, snapshot, recordPath string, record recoveryRecord) (string, error) {
	switch record.State {
	case RecoveryStateSnapshotting:
		return rebuildRecoverySnapshot(source, snapshot, recordPath, record)
	case RecoveryStateRematerializing:
		if err := validateRecoverySnapshotPath(source, snapshot); err != nil {
			return "", blockRecovery(recordPath, record, err)
		}
		manifest, err := checkoutManifest(snapshot)
		if err != nil {
			return "", blockRecovery(recordPath, record, err)
		}
		if record.Manifest == "" || manifest != record.Manifest {
			return "", blockRecovery(recordPath, record, fmt.Errorf("recovery snapshot does not match completed snapshot record"))
		}
		return manifest, nil
	default:
		return "", blockRecovery(recordPath, record, fmt.Errorf("recovery record has invalid snapshot state %q", record.State))
	}
}

// rebuildRecoverySnapshot discards a snapshot only while its record has not
// committed a manifest. A crash in snapshotting can leave a partial copy.
func rebuildRecoverySnapshot(source, snapshot, recordPath string, record recoveryRecord) (string, error) {
	if err := validateRecoverySnapshotPath(source, snapshot); err != nil {
		return "", blockRecovery(recordPath, record, err)
	}
	if err := os.RemoveAll(snapshot); err != nil {
		return "", blockRecovery(recordPath, record, fmt.Errorf("discard incomplete recovery snapshot: %w", err))
	}
	sourceBefore, err := checkoutManifest(source)
	if err != nil {
		return "", blockRecovery(recordPath, record, err)
	}
	if err := snapshotCheckout(source, snapshot); err != nil {
		record.State, record.Error, record.UpdatedAt = RecoveryStateBlocked, err.Error(), time.Now().UTC()
		_ = writeRecoveryRecord(recordPath, record)
		return "", err
	}
	sourceAfter, err := checkoutManifest(source)
	if err != nil || sourceBefore != sourceAfter {
		if err == nil {
			err = fmt.Errorf("original checkout changed during snapshot")
		}
		return "", blockRecovery(recordPath, record, err)
	}
	manifest, err := checkoutManifest(snapshot)
	if err != nil {
		return "", blockRecovery(recordPath, record, err)
	}
	if sourceBefore != manifest {
		return "", blockRecovery(recordPath, record, fmt.Errorf("recovery snapshot does not match original checkout"))
	}
	return manifest, nil
}

func validateRecoverySnapshotPath(source, snapshot string) error {
	cleanSource := filepath.Clean(source)
	cleanSnapshot := filepath.Clean(snapshot)
	prefix := cleanSource + ".kandev-recovery-"
	if !strings.HasPrefix(cleanSnapshot, prefix) {
		return fmt.Errorf("recovery snapshot path is outside the task-owned snapshot namespace")
	}
	info, err := os.Lstat(cleanSnapshot)
	if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("recovery snapshot path is not a task-owned directory")
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect recovery snapshot path: %w", err)
	}
	return nil
}

func blockRecovery(path string, record recoveryRecord, err error) error {
	record.State, record.Error, record.UpdatedAt = RecoveryStateBlocked, err.Error(), time.Now().UTC()
	_ = writeRecoveryRecord(path, record)
	return fmt.Errorf("%w: %s", ErrWorktreeCorrupted, err)
}

func readRecoveryRecord(path string) (recoveryRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return recoveryRecord{}, err
	}
	var record recoveryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return recoveryRecord{}, err
	}
	return record, nil
}

func writeRecoveryRecord(path string, record recoveryRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write recovery state: %w", err)
	}
	if err := syncRecoveryFile(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit recovery state: %w", err)
	}
	return syncRecoveryDirectory(filepath.Dir(path))
}

func (m *Manager) refreshRecoveredWorktreeCache(expected, replacement *Worktree) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, cached := range m.worktrees {
		if cached == nil || cached.ID != expected.ID {
			continue
		}
		updated := *replacement
		updated.SessionID = cached.SessionID
		m.worktrees[key] = &updated
	}
	if replacement.SessionID != "" {
		m.worktrees[cacheKey(replacement.SessionID, replacement.RepositoryID, replacement.BranchSlug)] = replacement
	}
}
