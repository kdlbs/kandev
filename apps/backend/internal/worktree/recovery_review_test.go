package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type recoveryCASStore struct {
	*mockStore
	mu     sync.Mutex
	err    error
	reject bool
}

func (s *recoveryCASStore) CompareAndSwapWorktree(_ context.Context, expected, replacement *Worktree) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return false, s.err
	}
	if s.reject {
		return false, nil
	}
	current, ok := s.worktrees[expected.ID]
	if !ok || current.Path != expected.Path || current.Branch != expected.Branch {
		return false, nil
	}
	delete(s.worktrees, expected.ID)
	s.worktrees[replacement.ID] = replacement
	return true, nil
}

func (s *recoveryCASStore) GetWorktreesByTaskID(_ context.Context, taskID string) ([]*Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*Worktree
	for _, wt := range s.worktrees {
		if wt.TaskID == taskID {
			copy := *wt
			result = append(result, &copy)
		}
	}
	return result, nil
}

type concurrentRecoveryStore struct {
	*recoveryCASStore
	original *Worktree
	listed   chan struct{}
	release  chan struct{}
	calls    atomic.Int32
}

func (s *concurrentRecoveryStore) GetWorktreesByTaskID(ctx context.Context, taskID string) ([]*Worktree, error) {
	if s.calls.Add(1) <= 2 {
		copy := *s.original
		s.listed <- struct{}{}
		<-s.release
		return []*Worktree{&copy}, nil
	}
	return s.recoveryCASStore.GetWorktreesByTaskID(ctx, taskID)
}

func newBrokenRecoveryFixture(t *testing.T, suffix string) (Config, string, *Worktree) {
	t.Helper()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree-"+suffix)
	branch := "feature/recover-" + suffix
	runGit(t, repoPath, "worktree", "add", "-b", branch, worktreePath, "main")
	removeRecoveryAdminDirectory(t, worktreePath)
	return cfg, repoPath, &Worktree{
		ID: "wt-1", SessionID: "session-1", TaskID: "task-1", TaskEnvironmentID: "env-1",
		RepositoryID: "repo-1", BranchSlug: branch, RepositoryPath: repoPath,
		Path: worktreePath, Branch: branch, BaseBranch: "main", Status: StatusActive,
	}
}

func TestManager_IsValidAcceptsCanonicalBacklinkThroughSymlinkedAncestor(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	realParent := t.TempDir()
	linkedParent := filepath.Join(t.TempDir(), "tasks-link")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Skipf("create tasks symlink: %v", err)
	}
	worktreePath := filepath.Join(linkedParent, "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
	runGit(t, worktreePath, "status", "--porcelain")
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !mgr.IsValid(worktreePath) {
		t.Fatal("healthy worktree through a symlinked ancestor was rejected")
	}
}

func TestManager_IsValidAcceptsAbsoluteCommonDir(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
	pointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	commonDir, err := linkedWorktreeCommonDir(adminPath)
	if err != nil {
		t.Fatalf("resolve common dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(adminPath, "commondir"), []byte(commonDir+"\n"), 0o644); err != nil {
		t.Fatalf("write absolute commondir: %v", err)
	}
	runGit(t, worktreePath, "status", "--porcelain")
	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !mgr.IsValid(worktreePath) {
		t.Fatal("healthy worktree with an absolute commondir was rejected")
	}
}

func TestManager_IsValidAcceptsRelativeGitDirPointer(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")

	pointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	relativeAdminPath, err := filepath.Rel(worktreePath, adminPath)
	if err != nil {
		t.Fatalf("make relative admin path: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte("gitdir: "+relativeAdminPath+"\n"), 0600); err != nil {
		t.Fatalf("write relative worktree pointer: %v", err)
	}

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if !mgr.IsValid(worktreePath) {
		t.Fatal("healthy worktree with a relative gitdir pointer was rejected")
	}
}

func TestManager_IsValidRejectsAdminDirectoryOutsideCommonWorktrees(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")

	pointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	commonDir, err := linkedWorktreeCommonDir(adminPath)
	if err != nil {
		t.Fatalf("resolve common dir: %v", err)
	}
	foreignAdminPath := filepath.Join(t.TempDir(), "foreign-admin")
	if err := os.Rename(adminPath, foreignAdminPath); err != nil {
		t.Fatalf("move admin directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(foreignAdminPath, "commondir"), []byte(commonDir+"\n"), 0600); err != nil {
		t.Fatalf("rewrite common dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(foreignAdminPath, "gitdir"), []byte(filepath.Join(worktreePath, ".git")+"\n"), 0600); err != nil {
		t.Fatalf("rewrite backlink: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreePath, ".git"), []byte("gitdir: "+foreignAdminPath+"\n"), 0600); err != nil {
		t.Fatalf("rewrite worktree pointer: %v", err)
	}

	mgr, err := NewManager(newTestConfig(t), newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr.IsValid(worktreePath) {
		t.Fatal("worktree with admin metadata outside common worktrees was accepted")
	}
}

func TestManager_AdmitTaskRecoveryFailsClosedOnWorktreeStatError(t *testing.T) {
	cfg := newTestConfig(t)
	parent := filepath.Join(cfg.TasksBasePath, "not-a-directory")
	if err := os.WriteFile(parent, []byte("file\n"), 0o600); err != nil {
		t.Fatalf("write path parent: %v", err)
	}
	store := newMockStore()
	store.worktrees["wt-1"] = &Worktree{
		ID: "wt-1", TaskID: "task-1", RepositoryID: "repo-1",
		Path: filepath.Join(parent, "checkout"), Status: StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	var recoveryErr *WorktreeRecoveryError
	if err := mgr.AdmitTaskRecovery(context.Background(), "task-1"); !errors.As(err, &recoveryErr) {
		t.Fatalf("AdmitTaskRecovery() error = %v, want WorktreeRecoveryError", err)
	}
}

func TestAdoptRecoveryRecordRejectsInvalidOperationID(t *testing.T) {
	worktreePath := t.TempDir()
	record := recoveryRecord{
		OperationID: "short", TaskID: "task-1", WorktreeID: "wt-1",
		Original: worktreePath, Snapshot: worktreePath + ".kandev-recovery-snapshot",
		State: RecoveryStateSnapshotting,
	}
	_, _, err := adoptRecoveryRecord(&Worktree{ID: "wt-1", TaskID: "task-1", Path: worktreePath}, record)
	if err == nil {
		t.Fatal("adoptRecoveryRecord accepted an invalid operation ID")
	}
}

func TestWriteRecoveryRecordDoesNotFollowPredictableTempSymlink(t *testing.T) {
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "state.json")
	sentinelPath := filepath.Join(dir, "sentinel")
	const sentinel = "keep this file"
	if err := os.WriteFile(sentinelPath, []byte(sentinel), 0600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := os.Symlink(sentinelPath, recordPath+".tmp"); err != nil {
		t.Skipf("create symlink: %v", err)
	}

	record := recoveryRecord{OperationID: "11111111-1111-1111-1111-111111111111", TaskID: "task-1", WorktreeID: "wt-1"}
	if err := writeRecoveryRecord(recordPath, record); err != nil {
		t.Fatalf("write recovery record: %v", err)
	}
	if got, err := os.ReadFile(sentinelPath); err != nil || string(got) != sentinel {
		t.Fatalf("sentinel changed through temporary symlink: %q, err=%v", got, err)
	}
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("recovery record was not committed: %v", err)
	}
}

func TestPrepareRecoverySnapshotRejectsRematerializingPathOutsideNamespace(t *testing.T) {
	source := t.TempDir()
	snapshot := t.TempDir()
	manifest, err := checkoutManifest(snapshot)
	if err != nil {
		t.Fatalf("snapshot manifest: %v", err)
	}
	recordPath := source + ".kandev-recovery.json"
	record := recoveryRecord{
		OperationID: "11111111-1111-1111-1111-111111111111", TaskID: "task-1", WorktreeID: "wt-1",
		Original: source, Snapshot: snapshot, Manifest: manifest, State: RecoveryStateRematerializing,
	}
	if _, err := prepareRecoverySnapshot(source, snapshot, recordPath, record); err == nil {
		t.Fatal("prepareRecoverySnapshot accepted a rematerializing snapshot outside the task namespace")
	}
}

func TestPrepareRecoverySnapshotRejectsSymlinkedRematerializingSnapshot(t *testing.T) {
	source := t.TempDir()
	foreign := t.TempDir()
	snapshot := source + ".kandev-recovery-snapshot"
	if err := os.Symlink(foreign, snapshot); err != nil {
		t.Skipf("create snapshot symlink: %v", err)
	}
	manifest, err := checkoutManifest(foreign)
	if err != nil {
		t.Fatalf("foreign manifest: %v", err)
	}
	recordPath := source + ".kandev-recovery.json"
	record := recoveryRecord{
		OperationID: "11111111-1111-1111-1111-111111111111", TaskID: "task-1", WorktreeID: "wt-1",
		Original: source, Snapshot: snapshot, Manifest: manifest, State: RecoveryStateRematerializing,
	}
	if _, err := prepareRecoverySnapshot(source, snapshot, recordPath, record); err == nil {
		t.Fatal("prepareRecoverySnapshot accepted a symlinked rematerializing snapshot")
	}
}

func TestSnapshotAndRestoreWriteChildrenBeforeApplyingReadOnlyDirectoryMode(t *testing.T) {
	source := t.TempDir()
	readOnly := filepath.Join(source, "readonly")
	if err := os.Mkdir(readOnly, 0o700); err != nil {
		t.Fatalf("create read-only source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(readOnly, "preserved.txt"), []byte("preserved\n"), 0o600); err != nil {
		t.Fatalf("write source child: %v", err)
	}
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatalf("chmod source directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })

	snapshot := filepath.Join(t.TempDir(), "snapshot")
	if err := snapshotCheckout(source, snapshot); err != nil {
		t.Fatalf("snapshotCheckout: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(snapshot, "readonly"), 0o700) })
	destination := t.TempDir()
	if err := copySnapshotEntries(snapshot, destination); err != nil {
		t.Fatalf("copySnapshotEntries: %v", err)
	}
	info, err := os.Stat(filepath.Join(destination, "readonly"))
	if err != nil {
		t.Fatalf("stat restored directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(destination, "readonly"), 0o700) })
	if info.Mode().Perm() != 0o500 {
		t.Fatalf("restored directory mode = %04o, want 0500", info.Mode().Perm())
	}
	if content, err := os.ReadFile(filepath.Join(destination, "readonly", "preserved.txt")); err != nil || string(content) != "preserved\n" {
		t.Fatalf("restored child = %q, err=%v", content, err)
	}
}

func TestRestoreSnapshotPreservesManifestReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	err := restoreSnapshot(missing, t.TempDir(), "manifest")
	if err == nil || !strings.Contains(err.Error(), "no such file or directory") {
		t.Fatalf("restoreSnapshot() error = %v, want underlying manifest read error", err)
	}
}

func TestManager_RecoverWorktreeDoesNotBlockOnTransientCompareAndSwapError(t *testing.T) {
	cfg, repoPath, original := newBrokenRecoveryFixture(t, "transient-cas")
	casErr := errors.New("database temporarily unavailable")
	store := &recoveryCASStore{mockStore: newMockStore(), err: casErr}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	_, err = mgr.RecoverWorktree(context.Background(), original, CreateRequest{
		TaskID: original.TaskID, RepositoryID: original.RepositoryID,
		RepositoryPath: repoPath, BaseBranch: "main",
	})
	if !errors.Is(err, casErr) {
		t.Fatalf("RecoverWorktree() error = %v, want transient store error", err)
	}
	record, readErr := readRecoveryRecord(original.Path + ".kandev-recovery.json")
	if readErr != nil {
		t.Fatalf("read recovery record: %v", readErr)
	}
	if record.State != RecoveryStateRematerializing {
		t.Fatalf("recovery state = %s, want rematerializing", record.State)
	}
}

func TestManager_RecoverWorktreeRequiresRecordedBranch(t *testing.T) {
	cfg, repoPath, original := newBrokenRecoveryFixture(t, "missing-recorded-branch")
	original.Branch = ""
	store := &recoveryCASStore{mockStore: newMockStore()}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	_, err = mgr.RecoverWorktree(context.Background(), original, CreateRequest{
		TaskID: original.TaskID, RepositoryID: original.RepositoryID,
		RepositoryPath: repoPath, BaseBranch: "main",
	})
	if err == nil || !strings.Contains(err.Error(), "recorded branch") {
		t.Fatalf("RecoverWorktree() error = %v, want missing recorded branch refusal", err)
	}
}

func TestManager_RecoverWorktreeInvalidatesEveryMatchingSessionCacheEntry(t *testing.T) {
	cfg, repoPath, original := newBrokenRecoveryFixture(t, "cache-invalidation")
	store := &recoveryCASStore{mockStore: newMockStore()}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	first := *original
	second := *original
	second.SessionID = "session-2"
	firstKey := cacheKey(first.SessionID, first.RepositoryID, first.BranchSlug)
	secondKey := cacheKey(second.SessionID, second.RepositoryID, second.BranchSlug)
	mgr.worktrees[firstKey] = &first
	mgr.worktrees[secondKey] = &second

	replacement, err := mgr.RecoverWorktree(context.Background(), original, CreateRequest{
		TaskID: original.TaskID, RepositoryID: original.RepositoryID,
		RepositoryPath: repoPath, BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("RecoverWorktree: %v", err)
	}
	for _, key := range []string{firstKey, secondKey} {
		if cached := mgr.worktrees[key]; cached != nil && cached.Path != replacement.Path {
			t.Fatalf("cache entry %q retained stale path %q", key, cached.Path)
		}
	}
}

func TestManager_RecoverWorktreeCompletesAfterDurableRowWasAlreadyRetargeted(t *testing.T) {
	cfg, repoPath, original := newBrokenRecoveryFixture(t, "post-cas-crash")
	store := &recoveryCASStore{mockStore: newMockStore(), reject: true}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	operationID := "11111111-1111-1111-1111-111111111111"
	snapshotPath := original.Path + ".kandev-recovery-" + operationID
	if err := snapshotCheckout(original.Path, snapshotPath); err != nil {
		t.Fatalf("snapshot checkout: %v", err)
	}
	manifest, err := checkoutManifest(snapshotPath)
	if err != nil {
		t.Fatalf("snapshot manifest: %v", err)
	}
	replacementPath := original.Path + ".recovered-" + operationID[:8]
	replacementBranch := original.Branch + "-recovered-" + operationID[:8]
	if _, err := mgr.gitAddWorktree(context.Background(), repoPath, replacementBranch, replacementPath, original.Branch); err != nil {
		t.Fatalf("create replacement: %v", err)
	}
	current := *original
	current.ID = "wt-replacement"
	current.Path = replacementPath
	current.Branch = replacementBranch
	store.worktrees[current.ID] = &current
	record := recoveryRecord{
		OperationID: operationID, TaskID: original.TaskID, WorktreeID: original.ID,
		Original: original.Path, Snapshot: snapshotPath, Manifest: manifest, State: RecoveryStateRematerializing,
	}
	if err := createRecoveryRecord(original.Path+".kandev-recovery.json", record); err != nil {
		t.Fatalf("create recovery record: %v", err)
	}

	recovered, err := mgr.RecoverWorktree(context.Background(), original, CreateRequest{
		TaskID: original.TaskID, RepositoryID: original.RepositoryID,
		RepositoryPath: repoPath, BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("RecoverWorktree after durable CAS: %v", err)
	}
	if recovered.ID != current.ID || recovered.Path != current.Path {
		t.Fatalf("recovered worktree = %+v, want durable replacement %+v", recovered, current)
	}
	completed, err := readRecoveryRecord(original.Path + ".kandev-recovery.json")
	if err != nil {
		t.Fatalf("read completed recovery record: %v", err)
	}
	if completed.State != RecoveryStateComplete {
		t.Fatalf("recovery state = %s, want complete", completed.State)
	}
}

func TestManager_AdmitTaskRecoveryCoalescesFollowerAfterSuccessfulRecovery(t *testing.T) {
	cfg, _, original := newBrokenRecoveryFixture(t, "concurrent-admission")
	base := &recoveryCASStore{mockStore: newMockStore()}
	base.worktrees[original.ID] = original
	store := &concurrentRecoveryStore{
		recoveryCASStore: base, original: original,
		listed: make(chan struct{}, 2), release: make(chan struct{}),
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- mgr.AdmitTaskRecovery(context.Background(), original.TaskID) }()
	}
	<-store.listed
	<-store.listed
	close(store.release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent admission failed after successful recovery: %v", err)
		}
	}
}
