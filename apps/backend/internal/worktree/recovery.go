package worktree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
//nolint:cyclop,gocognit,nestif // Recovery is one stateful transaction boundary.
func (m *Manager) RecoverWorktree(ctx context.Context, wt *Worktree, req CreateRequest) (*Worktree, error) {
	if wt == nil || wt.TaskID == "" || wt.TaskID != req.TaskID || wt.Path == "" || req.RepositoryPath == "" {
		return nil, fmt.Errorf("%w: recovery identity is incomplete", ErrWorktreeCorrupted)
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
	if _, err := m.gitAddWorktree(ctx, req.RepositoryPath, replacementBranch, replacementPath, branch); err != nil {
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
		ok, err := cas.CompareAndSwapWorktree(ctx, wt, &replacement)
		if err != nil || !ok {
			return nil, blockRecovery(jobPath, record, fmt.Errorf("recovery compare-and-swap rejected"))
		}
	} else if err := m.store.UpdateWorktree(ctx, &replacement); err != nil {
		return nil, blockRecovery(jobPath, record, err)
	}
	if replacement.SessionID != "" {
		m.mu.Lock()
		m.worktrees[cacheKey(replacement.SessionID, replacement.RepositoryID, replacement.BranchSlug)] = &replacement
		m.mu.Unlock()
	}
	record.Replacement, record.State, record.UpdatedAt = replacementPath, RecoveryStateComplete, time.Now().UTC()
	if err := writeRecoveryRecord(jobPath, record); err != nil {
		return nil, err
	}
	return &replacement, nil
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
	if existing.Original != wt.Path || existing.Snapshot == "" || existing.OperationID == "" {
		return recoveryRecord{}, "", recoveryAlreadyClaimedError(wt, "recovery record identity is incomplete")
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
	return syncFile(filepath.Dir(path))
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

//nolint:nestif // Snapshot preparation must fail closed across each validation stage.
func prepareRecoverySnapshot(source, snapshot, recordPath string, record recoveryRecord) (string, error) {
	var sourceBefore string
	if _, err := os.Stat(snapshot); os.IsNotExist(err) {
		var manifestErr error
		sourceBefore, manifestErr = checkoutManifest(source)
		if manifestErr != nil {
			return "", blockRecovery(recordPath, record, manifestErr)
		}
		if snapshotErr := snapshotCheckout(source, snapshot); snapshotErr != nil {
			record.State, record.Error, record.UpdatedAt = RecoveryStateBlocked, snapshotErr.Error(), time.Now().UTC()
			_ = writeRecoveryRecord(recordPath, record)
			return "", snapshotErr
		}
		sourceAfter, manifestErr := checkoutManifest(source)
		if manifestErr != nil || sourceBefore != sourceAfter {
			if manifestErr == nil {
				manifestErr = fmt.Errorf("original checkout changed during snapshot")
			}
			return "", blockRecovery(recordPath, record, manifestErr)
		}
	}
	manifest, err := checkoutManifest(snapshot)
	if err != nil {
		return "", blockRecovery(recordPath, record, err)
	}
	if sourceBefore != "" && sourceBefore != manifest {
		return "", blockRecovery(recordPath, record, fmt.Errorf("recovery snapshot does not match original checkout"))
	}
	return manifest, nil
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
	if err := syncFile(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("commit recovery state: %w", err)
	}
	return syncFile(filepath.Dir(path))
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open recovery state for sync: %w", err)
	}
	defer func() { _ = file.Close() }()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync recovery state: %w", err)
	}
	return nil
}

//nolint:cyclop,gocognit // Filesystem entry handling must remain fail-closed in one walk.
func snapshotCheckout(source, destination string) error {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return fmt.Errorf("create recovery snapshot: %w", err)
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return nil
		}
		if rel == recoveryGitDirName || strings.HasPrefix(rel, recoveryGitDirName+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeNamedPipe != 0 || info.Mode()&os.ModeSocket != 0 || info.Mode()&os.ModeDevice != 0 {
			return fmt.Errorf("unsupported recovery entry %q", rel)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		_ = in.Close()
		syncErr := out.Sync()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	})
}

func checkoutManifest(root string) (string, error) {
	var entries []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == recoveryGitDirName || strings.HasPrefix(rel, recoveryGitDirName+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		entryValue := rel + "|" + info.Mode().String()
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entryValue += "|" + target
		} else {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			hash := sha256.New()
			_, copyErr := io.Copy(hash, file)
			_ = file.Close()
			if copyErr != nil {
				return copyErr
			}
			entryValue += "|" + hex.EncodeToString(hash.Sum(nil))
		}
		entries = append(entries, entryValue)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(entries)
	hash := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(hash[:]), nil
}

func restoreSnapshot(source, destination, expectedManifest string) error {
	if current, err := checkoutManifest(source); err != nil || current != expectedManifest {
		return fmt.Errorf("recovery snapshot changed during rematerialization")
	}
	return copySnapshotEntries(source, destination)
}

//nolint:cyclop // The copy walk handles each filesystem type explicitly.
func copySnapshotEntries(source, destination string) error {
	if err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == destination || entry.IsDir() || entry.Name() == recoveryGitDirName {
			return nil
		}
		rel, err := filepath.Rel(destination, path)
		if err != nil {
			return err
		}
		if _, statErr := os.Lstat(filepath.Join(source, rel)); os.IsNotExist(statErr) {
			return os.Remove(path)
		}
		return nil
	}); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == source || entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chmod(target, info.Mode().Perm())
	})
}
