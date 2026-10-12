package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManager_RecoverWorktreeSnapshotsAndRematerializesCheckout(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/recover", worktreePath, "main")
	uniqueFile := filepath.Join(worktreePath, "preserve-me.txt")
	if err := os.WriteFile(uniqueFile, []byte("preserved\n"), 0600); err != nil {
		t.Fatalf("write unique file: %v", err)
	}
	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove admin directory: %v", err)
	}

	store := newMockStore()
	original := &Worktree{ID: "wt-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1", BranchSlug: "feature-recover", RepositoryPath: repoPath,
		Path: worktreePath, Branch: "feature/recover", BaseBranch: "main", Status: StatusActive}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	mgr.worktrees[cacheKey(original.SessionID, original.RepositoryID, original.BranchSlug)] = original
	replacement, err := mgr.RecoverWorktree(ctx, original, CreateRequest{TaskID: original.TaskID, RepositoryID: original.RepositoryID, RepositoryPath: repoPath, BaseBranch: "main"})
	if err != nil {
		t.Fatalf("RecoverWorktree: %v", err)
	}
	if replacement.Path == original.Path || !mgr.IsValid(replacement.Path) {
		t.Fatalf("replacement = %+v, want a valid sibling checkout", replacement)
	}
	content, err := os.ReadFile(filepath.Join(replacement.Path, "preserve-me.txt"))
	if err != nil || string(content) != "preserved\n" {
		t.Fatalf("replacement content = %q, err=%v", content, err)
	}
	cached, err := mgr.GetBySessionAndRepo(ctx, original.SessionID, original.RepositoryID, original.BranchSlug)
	if err != nil || cached == nil || cached.Path != replacement.Path {
		t.Fatalf("cached worktree = %+v, err=%v, want replacement", cached, err)
	}
	if _, err := mgr.RecoverWorktree(ctx, original, CreateRequest{
		TaskID: original.TaskID, RepositoryID: original.RepositoryID,
		RepositoryPath: repoPath, BaseBranch: "main",
	}); err == nil || !strings.Contains(err.Error(), "already complete") {
		t.Fatalf("repeated recovery error = %v, want terminal complete state", err)
	}
	if _, err := os.Stat(worktreePath + ".kandev-recovery.json"); err != nil {
		t.Fatalf("durable recovery record missing: %v", err)
	}
}

func TestManager_RecoverWorktreeRebuildsPartialSnapshotAfterCrash(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree-partial-snapshot")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/recover-partial-snapshot", worktreePath, "main")
	uniqueFile := filepath.Join(worktreePath, "preserve-me.txt")
	if err := os.WriteFile(uniqueFile, []byte("preserved\n"), 0600); err != nil {
		t.Fatalf("write unique file: %v", err)
	}
	removeRecoveryAdminDirectory(t, worktreePath)

	store := newMockStore()
	original := &Worktree{ID: "wt-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1", BranchSlug: "feature-recover-partial-snapshot", RepositoryPath: repoPath,
		Path: worktreePath, Branch: "feature/recover-partial-snapshot", BaseBranch: "main", Status: StatusActive}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	snapshotPath := worktreePath + ".kandev-recovery-partial"
	if err := os.Mkdir(snapshotPath, 0700); err != nil {
		t.Fatalf("create partial snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshotPath, "partial-copy.txt"), []byte("incomplete\n"), 0600); err != nil {
		t.Fatalf("write partial snapshot: %v", err)
	}
	record := recoveryRecord{
		OperationID: "22222222-2222-2222-2222-222222222222", TaskID: original.TaskID, WorktreeID: original.ID,
		Original: original.Path, Snapshot: snapshotPath, State: RecoveryStateSnapshotting,
	}
	if err := createRecoveryRecord(worktreePath+".kandev-recovery.json", record); err != nil {
		t.Fatalf("create recovery record: %v", err)
	}

	replacement, err := mgr.RecoverWorktree(ctx, original, CreateRequest{TaskID: original.TaskID, RepositoryID: original.RepositoryID, RepositoryPath: repoPath, BaseBranch: "main"})
	if err != nil {
		t.Fatalf("RecoverWorktree: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(replacement.Path, "preserve-me.txt"))
	if err != nil || string(content) != "preserved\n" {
		t.Fatalf("recovered content = %q, err=%v; want preserved original checkout content", content, err)
	}
	if _, err := os.Lstat(filepath.Join(replacement.Path, "partial-copy.txt")); !os.IsNotExist(err) {
		t.Fatalf("partial snapshot entry survived recovery, stat error = %v", err)
	}
}

func TestManager_RecoverWorktreeResumesAfterReplacementWorktreeWasCreated(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree-resume-replacement")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/recover-resume-replacement", worktreePath, "main")
	uniqueFile := filepath.Join(worktreePath, "preserve-me.txt")
	if err := os.WriteFile(uniqueFile, []byte("preserved\n"), 0600); err != nil {
		t.Fatalf("write unique file: %v", err)
	}

	store := newMockStore()
	original := &Worktree{ID: "wt-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1", BranchSlug: "feature-recover-resume-replacement", RepositoryPath: repoPath,
		Path: worktreePath, Branch: "feature/recover-resume-replacement", BaseBranch: "main", Status: StatusActive}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	snapshotPath := worktreePath + ".kandev-recovery-resume001"
	if err := snapshotCheckout(worktreePath, snapshotPath); err != nil {
		t.Fatalf("snapshot checkout: %v", err)
	}
	manifest, err := checkoutManifest(snapshotPath)
	if err != nil {
		t.Fatalf("snapshot manifest: %v", err)
	}
	operationID := "33333333-3333-3333-3333-333333333333"
	replacementPath := worktreePath + ".recovered-" + operationID[:8]
	replacementBranch := original.Branch + "-recovered-" + operationID[:8]
	if _, err := mgr.gitAddWorktree(ctx, repoPath, replacementBranch, replacementPath, original.Branch); err != nil {
		t.Fatalf("create replacement before crash: %v", err)
	}
	removeRecoveryAdminDirectory(t, worktreePath)
	record := recoveryRecord{
		OperationID: operationID, TaskID: original.TaskID, WorktreeID: original.ID,
		Original: original.Path, Snapshot: snapshotPath, Manifest: manifest, State: RecoveryStateRematerializing,
	}
	if err := createRecoveryRecord(worktreePath+".kandev-recovery.json", record); err != nil {
		t.Fatalf("create recovery record: %v", err)
	}

	replacement, err := mgr.RecoverWorktree(ctx, original, CreateRequest{TaskID: original.TaskID, RepositoryID: original.RepositoryID, RepositoryPath: repoPath, BaseBranch: "main"})
	if err != nil {
		t.Fatalf("RecoverWorktree: %v", err)
	}
	if replacement.Path != replacementPath {
		t.Fatalf("replacement path = %q, want %q", replacement.Path, replacementPath)
	}
	content, err := os.ReadFile(filepath.Join(replacement.Path, "preserve-me.txt"))
	if err != nil || string(content) != "preserved\n" {
		t.Fatalf("recovered content = %q, err=%v", content, err)
	}
}

func TestManager_RecoverWorktreePreservesEmptyDirectories(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree-empty-directories")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/recover-empty-directories", worktreePath, "main")
	emptyDirectory := filepath.Join(worktreePath, "preserved", "nested", "empty")
	if err := os.MkdirAll(emptyDirectory, 0750); err != nil {
		t.Fatalf("create empty directory: %v", err)
	}
	if err := os.Chmod(emptyDirectory, 0700); err != nil {
		t.Fatalf("set empty directory mode: %v", err)
	}
	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove admin directory: %v", err)
	}

	store := newMockStore()
	original := &Worktree{ID: "wt-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1", BranchSlug: "feature-recover-empty-directories", RepositoryPath: repoPath,
		Path: worktreePath, Branch: "feature/recover-empty-directories", BaseBranch: "main", Status: StatusActive}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	replacement, err := mgr.RecoverWorktree(ctx, original, CreateRequest{TaskID: original.TaskID, RepositoryID: original.RepositoryID, RepositoryPath: repoPath, BaseBranch: "main"})
	if err != nil {
		t.Fatalf("RecoverWorktree: %v", err)
	}
	recoveredDirectory := filepath.Join(replacement.Path, "preserved", "nested", "empty")
	info, err := os.Stat(recoveredDirectory)
	if err != nil {
		t.Fatalf("stat recovered empty directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("recovered empty entry is not a directory: %s", info.Mode())
	}
	if got := info.Mode().Perm(); got != 0700 {
		t.Fatalf("recovered empty directory mode = %04o, want 0700", got)
	}
}

func TestManager_RecoverWorktreeRemovesDestinationDirectoriesAbsentFromSnapshot(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	deletedDirectory := "removed-from-checkout"
	deletedFile := filepath.Join(deletedDirectory, "tracked.txt")
	if err := os.Mkdir(filepath.Join(repoPath, deletedDirectory), 0755); err != nil {
		t.Fatalf("create tracked directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, deletedFile), []byte("tracked\n"), 0644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	runGit(t, repoPath, "add", deletedFile)
	runGit(t, repoPath, "commit", "-m", "add tracked directory")

	worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree-removed-directory")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/recover-removed-directory", worktreePath, "main")
	if err := os.RemoveAll(filepath.Join(worktreePath, deletedDirectory)); err != nil {
		t.Fatalf("remove tracked directory from damaged checkout: %v", err)
	}
	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove admin directory: %v", err)
	}

	store := newMockStore()
	original := &Worktree{ID: "wt-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1", BranchSlug: "feature-recover-removed-directory", RepositoryPath: repoPath,
		Path: worktreePath, Branch: "feature/recover-removed-directory", BaseBranch: "main", Status: StatusActive}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	replacement, err := mgr.RecoverWorktree(ctx, original, CreateRequest{TaskID: original.TaskID, RepositoryID: original.RepositoryID, RepositoryPath: repoPath, BaseBranch: "main"})
	if err != nil {
		t.Fatalf("RecoverWorktree: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(replacement.Path, deletedDirectory)); !os.IsNotExist(err) {
		t.Fatalf("recovered deleted directory exists, stat error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(replacement.Path, deletedFile)); !os.IsNotExist(err) {
		t.Fatalf("recovered deleted file exists, stat error = %v", err)
	}
}

func TestManager_RecoverWorktreeOverlaysAllEntryTypeTransitions(t *testing.T) {
	entryTypes := []string{"directory", "file", "symlink"}
	for _, destinationType := range entryTypes {
		for _, snapshotType := range entryTypes {
			if destinationType == snapshotType {
				continue
			}
			t.Run(destinationType+"-to-"+snapshotType, func(t *testing.T) {
				ctx := context.Background()
				cfg := newTestConfig(t)
				repoPath := initGitRepoForWorktreeTest(t)
				external := filepath.Join(t.TempDir(), "external")
				if err := os.Mkdir(external, 0750); err != nil {
					t.Fatalf("create external sentinel directory: %v", err)
				}
				if err := os.WriteFile(filepath.Join(external, "sentinel"), []byte("untouched\n"), 0600); err != nil {
					t.Fatalf("write external sentinel: %v", err)
				}
				writeRecoveryEntry(t, repoPath, "collision", destinationType, external)
				runGit(t, repoPath, "add", "collision")
				runGit(t, repoPath, "commit", "-m", "add recovery collision")

				worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree-"+destinationType+"-to-"+snapshotType)
				branch := "feature/recover-" + destinationType + "-to-" + snapshotType
				runGit(t, repoPath, "worktree", "add", "-b", branch, worktreePath, "main")
				if err := os.RemoveAll(filepath.Join(worktreePath, "collision")); err != nil {
					t.Fatalf("remove damaged checkout collision: %v", err)
				}
				writeRecoveryEntry(t, worktreePath, "collision", snapshotType, external)
				removeRecoveryAdminDirectory(t, worktreePath)

				store := newMockStore()
				original := &Worktree{ID: "wt-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1", BranchSlug: branch, RepositoryPath: repoPath,
					Path: worktreePath, Branch: branch, BaseBranch: "main", Status: StatusActive}
				store.worktrees[original.ID] = original
				mgr, err := NewManager(cfg, store, newTestLogger())
				if err != nil {
					t.Fatalf("NewManager failed: %v", err)
				}

				replacement, err := mgr.RecoverWorktree(ctx, original, CreateRequest{TaskID: original.TaskID, RepositoryID: original.RepositoryID, RepositoryPath: repoPath, BaseBranch: "main"})
				if err != nil {
					t.Fatalf("RecoverWorktree: %v", err)
				}
				assertRecoveryEntry(t, replacement.Path, "collision", snapshotType)
				if content, err := os.ReadFile(filepath.Join(external, "sentinel")); err != nil || string(content) != "untouched\n" {
					t.Fatalf("external symlink target changed: content=%q err=%v", content, err)
				}
			})
		}
	}
}

func writeRecoveryEntry(t *testing.T, root, name, entryType, external string) {
	t.Helper()
	path := filepath.Join(root, name)
	switch entryType {
	case "directory":
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatalf("create recovery directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "preserved"), []byte("directory content\n"), 0600); err != nil {
			t.Fatalf("write recovery directory content: %v", err)
		}
	case "file":
		if err := os.WriteFile(path, []byte("file content\n"), 0600); err != nil {
			t.Fatalf("write recovery file: %v", err)
		}
	case "symlink":
		if err := os.Symlink(external, path); err != nil {
			t.Skipf("create recovery symlink: %v", err)
		}
	default:
		t.Fatalf("unknown recovery entry type %q", entryType)
	}
}

func assertRecoveryEntry(t *testing.T, root, name, entryType string) {
	t.Helper()
	path := filepath.Join(root, name)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat recovered entry: %v", err)
	}
	switch entryType {
	case "directory":
		if !info.IsDir() || info.Mode().Perm() != 0700 {
			t.Fatalf("recovered entry mode = %s, want directory mode 0700", info.Mode())
		}
		if content, err := os.ReadFile(filepath.Join(path, "preserved")); err != nil || string(content) != "directory content\n" {
			t.Fatalf("recovered directory content = %q, err=%v", content, err)
		}
	case "file":
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			t.Fatalf("recovered entry mode = %s, want regular file mode 0600", info.Mode())
		}
		if content, err := os.ReadFile(path); err != nil || string(content) != "file content\n" {
			t.Fatalf("recovered file content = %q, err=%v", content, err)
		}
	case "symlink":
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("recovered entry mode = %s, want symlink", info.Mode())
		}
	default:
		t.Fatalf("unknown recovery entry type %q", entryType)
	}
}

func removeRecoveryAdminDirectory(t *testing.T, worktreePath string) {
	t.Helper()
	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove admin directory: %v", err)
	}
}

func TestCopySnapshotEntriesReplacesDestinationSymlinkWithDirectory(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "empty"), 0700); err != nil {
		t.Fatalf("create snapshot directory: %v", err)
	}
	destination := t.TempDir()
	external := filepath.Join(t.TempDir(), "external")
	if err := os.Mkdir(external, 0750); err != nil {
		t.Fatalf("create external directory: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(destination, "empty")); err != nil {
		t.Skipf("create destination symlink: %v", err)
	}

	if err := copySnapshotEntries(source, destination); err != nil {
		t.Fatalf("copySnapshotEntries: %v", err)
	}
	info, err := os.Lstat(filepath.Join(destination, "empty"))
	if err != nil {
		t.Fatalf("lstat restored directory: %v", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("restored entry mode = %s, want directory without symlink", info.Mode())
	}
	externalInfo, err := os.Stat(external)
	if err != nil {
		t.Fatalf("stat external directory: %v", err)
	}
	if got := externalInfo.Mode().Perm(); got != 0750 {
		t.Fatalf("external directory mode = %04o, want 0750", got)
	}
}

func TestCopySnapshotEntriesReplacesDestinationSymlinkWithFile(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("snapshot content\n"), 0600); err != nil {
		t.Fatalf("write snapshot file: %v", err)
	}
	destination := t.TempDir()
	external := filepath.Join(t.TempDir(), "external")
	if err := os.WriteFile(external, []byte("external content\n"), 0640); err != nil {
		t.Fatalf("write external file: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(destination, "file")); err != nil {
		t.Skipf("create destination symlink: %v", err)
	}

	if err := copySnapshotEntries(source, destination); err != nil {
		t.Fatalf("copySnapshotEntries: %v", err)
	}
	info, err := os.Lstat(filepath.Join(destination, "file"))
	if err != nil {
		t.Fatalf("lstat restored file: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("restored entry mode = %s, want regular file", info.Mode())
	}
	content, err := os.ReadFile(external)
	if err != nil {
		t.Fatalf("read external file: %v", err)
	}
	if string(content) != "external content\n" {
		t.Fatalf("external file content = %q, want unchanged content", content)
	}
}

func TestIsAdminDirectoryMissing(t *testing.T) {
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
	if isAdminDirectoryMissing(worktreePath) {
		t.Fatal("valid linked worktree reported a missing admin directory")
	}

	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove linked worktree admin directory: %v", err)
	}
	if !isAdminDirectoryMissing(worktreePath) {
		t.Fatal("missing linked worktree admin directory was not detected")
	}
}

func TestRecoveryOperationLockIsExclusiveAndReleasesForAdoption(t *testing.T) {
	claimPath := filepath.Join(t.TempDir(), "recovery.claim")
	first, err := acquireRecoveryOperation(claimPath)
	if err != nil {
		t.Fatalf("first recovery claim: %v", err)
	}
	second, err := acquireRecoveryOperation(claimPath)
	if !errors.Is(err, errRecoveryOperationClaimed) {
		t.Fatalf("second recovery claim error = %v, want errRecoveryOperationClaimed", err)
	}
	if second != nil {
		_ = second.Close()
		t.Fatal("second recovery claim unexpectedly acquired the lock")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("release first recovery claim: %v", err)
	}
	adopted, err := acquireRecoveryOperation(claimPath)
	if err != nil {
		t.Fatalf("recovery claim after owner exit: %v", err)
	}
	if err := adopted.Close(); err != nil {
		t.Fatalf("release adopted recovery claim: %v", err)
	}
}

func TestRecoveryRecordIsAdoptableAfterRestart(t *testing.T) {
	worktreePath := t.TempDir()
	jobPath := worktreePath + ".kandev-recovery.json"
	claimPath := worktreePath + ".kandev-recovery.claim"
	if err := os.WriteFile(claimPath, []byte("orphaned operation\n"), 0o600); err != nil {
		t.Fatalf("write orphaned recovery claim: %v", err)
	}
	original := recoveryRecord{
		OperationID: "44444444-4444-4444-4444-444444444444", TaskID: "task-1", WorktreeID: "worktree-1",
		Original: worktreePath, Snapshot: worktreePath + ".snapshot",
		State: RecoveryStateSnapshotting,
	}
	if err := createRecoveryRecord(jobPath, original); err != nil {
		t.Fatalf("create recovery record: %v", err)
	}
	adopted, snapshot, err := loadOrClaimRecovery(&Worktree{ID: original.WorktreeID, TaskID: original.TaskID, Path: worktreePath}, jobPath)
	if err != nil {
		t.Fatalf("adopt recovery record: %v", err)
	}
	if adopted.OperationID != original.OperationID || snapshot != original.Snapshot {
		t.Fatalf("adopted record = %+v, snapshot=%q; want %+v", adopted, snapshot, original)
	}
	claim, err := acquireRecoveryOperation(claimPath)
	if err != nil {
		t.Fatalf("adopt orphaned recovery claim: %v", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("release adopted orphaned recovery claim: %v", err)
	}
}

func TestBeginRecoveryAdoptsOrphanedClaimBeforeCreatingRecord(t *testing.T) {
	worktreePath := t.TempDir()
	claimPath := worktreePath + ".kandev-recovery.claim"
	jobPath := worktreePath + ".kandev-recovery.json"
	if err := os.WriteFile(claimPath, []byte("operation-from-crashed-process\n"), 0o600); err != nil {
		t.Fatalf("write orphaned claim: %v", err)
	}

	wt := &Worktree{ID: "worktree-1", TaskID: "task-1", Path: worktreePath}
	record, snapshot, claim, err := beginRecovery(wt, jobPath)
	if err != nil {
		t.Fatalf("begin recovery after crash: %v", err)
	}
	defer func() { _ = claim.Close() }()
	if record.OperationID == "" || snapshot != record.Snapshot {
		t.Fatalf("record = %+v, snapshot = %q, want one resumable operation", record, snapshot)
	}
	if _, err := os.Stat(jobPath); err != nil {
		t.Fatalf("recovery record was not created: %v", err)
	}
}

func TestBeginRecoveryCoalescesConcurrentCallers(t *testing.T) {
	worktreePath := t.TempDir()
	wt := &Worktree{ID: "worktree-1", TaskID: "task-1", Path: worktreePath}
	jobPath := worktreePath + ".kandev-recovery.json"
	type result struct {
		record recoveryRecord
		claim  *recoveryLock
		err    error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			record, _, claim, err := beginRecovery(wt, jobPath)
			results <- result{record: record, claim: claim, err: err}
		}()
	}
	first, second := <-results, <-results
	owner, blocked := first, second
	if owner.err != nil {
		owner, blocked = second, first
	}
	if owner.err != nil || owner.claim == nil {
		t.Fatalf("concurrent recovery owner = %+v", owner)
	}
	if blocked.err == nil || !strings.Contains(blocked.err.Error(), errRecoveryOperationClaimed.Error()) {
		t.Fatalf("concurrent recovery follower error = %v, want claimed", blocked.err)
	}
	if err := owner.claim.Close(); err != nil {
		t.Fatalf("release recovery owner claim: %v", err)
	}
	adopted, _, claim, err := beginRecovery(wt, jobPath)
	if err != nil {
		t.Fatalf("retry recovery after owner exit: %v", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("release adopted recovery claim: %v", err)
	}
	if owner.record.OperationID != adopted.OperationID || owner.record.Snapshot != adopted.Snapshot {
		t.Fatalf("coalesced records differ: owner=%+v adopted=%+v", owner.record, adopted)
	}
}

func TestBeginRecoveryRejectsIncompatibleClaimPath(t *testing.T) {
	worktreePath := t.TempDir()
	claimPath := worktreePath + ".kandev-recovery.claim"
	if err := os.Remove(worktreePath); err != nil {
		t.Fatalf("remove worktree directory: %v", err)
	}
	if err := os.Symlink(t.TempDir(), claimPath); err != nil {
		t.Fatalf("create symlink claim: %v", err)
	}
	_, _, claim, err := beginRecovery(&Worktree{ID: "worktree-1", TaskID: "task-1", Path: worktreePath}, worktreePath+".kandev-recovery.json")
	if err == nil || claim != nil {
		t.Fatalf("begin recovery with symlink claim = record/claim=%v/%v, want rejection", err, claim)
	}
}
