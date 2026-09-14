package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManager_IsValid(t *testing.T) {
	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test non-existent path
	if mgr.IsValid("/nonexistent/path") {
		t.Error("expected false for non-existent path")
	}

	// Create a mock worktree directory
	worktreePath := filepath.Join(cfg.TasksBasePath, "test-worktree")
	if err := os.MkdirAll(worktreePath, 0755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}

	// Without .git file - should be invalid
	if mgr.IsValid(worktreePath) {
		t.Error("expected false for directory without .git file")
	}

	// A pointer without a real linked-worktree admin entry is invalid.
	gitFile := filepath.Join(worktreePath, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /some/path/.git/worktrees/test"), 0644); err != nil {
		t.Fatalf("failed to create .git file: %v", err)
	}

	if mgr.IsValid(worktreePath) {
		t.Error("expected false for a pointer whose admin directory is missing")
	}
}

func TestManager_IsValid_RejectsMissingLinkedWorktreeAdminDirectory(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")

	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove linked worktree admin directory: %v", err)
	}

	if mgr.IsValid(worktreePath) {
		t.Fatal("IsValid accepted a checkout whose linked-worktree admin directory is missing")
	}
}

func TestManager_AdmitTaskRecoveryRejectsMissingAdminOutsideRepositoryCommonWorktrees(t *testing.T) {
	cfg, _, original := newBrokenRecoveryFixture(t, "foreign-admin")
	foreignAdminPath := filepath.Join(t.TempDir(), "worktrees", "missing")
	if err := os.WriteFile(filepath.Join(original.Path, ".git"), []byte("gitdir: "+foreignAdminPath+"\n"), 0600); err != nil {
		t.Fatalf("rewrite worktree pointer: %v", err)
	}

	store := newMockStore()
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	err = mgr.AdmitTaskRecovery(context.Background(), original.TaskID)
	var recoveryErr *WorktreeRecoveryError
	if !errors.As(err, &recoveryErr) || recoveryErr.State != string(linkedWorktreeAmbiguous) {
		t.Fatalf("AdmitTaskRecovery() error = %+v, want ambiguous recovery refusal", err)
	}
	if _, err := os.Stat(original.Path + ".kandev-recovery.json"); !os.IsNotExist(err) {
		t.Fatalf("recovery record exists after foreign admin refusal: %v", err)
	}
}

func TestManager_IsValid_RejectsSymlinkedLinkedWorktreeAdminDirectory(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/symlink-admin", worktreePath, "main")

	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	movedAdminPath := filepath.Join(t.TempDir(), "admin-target")
	if err := os.Rename(adminPath, movedAdminPath); err != nil {
		t.Fatalf("move admin directory: %v", err)
	}
	if err := os.Symlink(movedAdminPath, adminPath); err != nil {
		t.Fatalf("symlink admin directory: %v", err)
	}

	if mgr.IsValid(worktreePath) {
		t.Fatal("IsValid accepted a symlinked linked-worktree admin directory")
	}
}

func TestManager_AdmitTaskRecoveryRefusesPresentInvalidAdminTargetWithoutMutation(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		replace func(t *testing.T, adminPath string)
	}{
		{
			name: "symlink",
			replace: func(t *testing.T, adminPath string) {
				t.Helper()
				movedAdminPath := filepath.Join(t.TempDir(), "admin-target")
				if err := os.Rename(adminPath, movedAdminPath); err != nil {
					t.Fatalf("move admin directory: %v", err)
				}
				if err := os.Symlink(movedAdminPath, adminPath); err != nil {
					t.Fatalf("symlink admin directory: %v", err)
				}
			},
		},
		{
			name: "regular file",
			replace: func(t *testing.T, adminPath string) {
				t.Helper()
				if err := os.RemoveAll(adminPath); err != nil {
					t.Fatalf("remove admin directory: %v", err)
				}
				if err := os.WriteFile(adminPath, []byte("not an admin directory\n"), 0600); err != nil {
					t.Fatalf("write invalid admin target: %v", err)
				}
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := newTestConfig(t)
			repoPath := initGitRepoForWorktreeTest(t)
			worktreePath := filepath.Join(cfg.TasksBasePath, "present-invalid-admin")
			runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
			gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
			if err != nil {
				t.Fatalf("read linked worktree pointer: %v", err)
			}
			adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
			fixture.replace(t, adminPath)

			store := newMockStore()
			original := &Worktree{ID: "wt-1", TaskID: "task-1", RepositoryID: "repo-1", RepositoryPath: repoPath,
				Path: worktreePath, Branch: "feature/pr-branch", BaseBranch: "main", Status: StatusActive}
			store.worktrees[original.ID] = original
			mgr, err := NewManager(cfg, store, newTestLogger())
			if err != nil {
				t.Fatalf("NewManager failed: %v", err)
			}

			err = mgr.AdmitTaskRecovery(ctx, original.TaskID)
			if !errors.Is(err, ErrWorktreeCorrupted) {
				t.Fatalf("AdmitTaskRecovery() error = %v, want ErrWorktreeCorrupted", err)
			}
			var recoveryErr *WorktreeRecoveryError
			if !errors.As(err, &recoveryErr) || recoveryErr.State != string(linkedWorktreeAmbiguous) {
				t.Fatalf("AdmitTaskRecovery() error = %+v, want ambiguous recovery refusal", err)
			}
			if _, err := os.Stat(worktreePath + ".kandev-recovery.json"); !os.IsNotExist(err) {
				t.Fatalf("recovery record exists after invalid admin refusal: %v", err)
			}
			if matches, err := filepath.Glob(worktreePath + ".kandev-recovery-*"); err != nil || len(matches) != 0 {
				t.Fatalf("snapshot paths = %v, err = %v, want none", matches, err)
			}
			if matches, err := filepath.Glob(worktreePath + ".recovered-*"); err != nil || len(matches) != 0 {
				t.Fatalf("replacement paths = %v, err = %v, want none", matches, err)
			}
			if got := store.worktrees[original.ID]; got != original || got.Path != worktreePath {
				t.Fatalf("durable worktree changed after refusal: %+v", got)
			}
		})
	}
}

func TestManager_AdmitTaskRecoveryRejectsPresentInvalidCheckout(t *testing.T) {
	cfg := newTestConfig(t)
	path := filepath.Join(cfg.TasksBasePath, "invalid-checkout")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir checkout: %v", err)
	}
	store := newMockStore()
	store.worktrees["wt-1"] = &Worktree{ID: "wt-1", TaskID: "task-1", Path: path, Status: StatusActive}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if err := mgr.AdmitTaskRecovery(context.Background(), "task-1"); err == nil {
		t.Fatal("AdmitTaskRecovery accepted an invalid present checkout")
	} else if !errors.Is(err, ErrWorktreeCorrupted) {
		t.Fatalf("AdmitTaskRecovery error = %v, want ErrWorktreeCorrupted", err)
	}
}

func TestManager_IsValid_RejectsMismatchedLinkedWorktreeBacklink(t *testing.T) {
	cfg := newTestConfig(t)
	mgr, err := NewManager(cfg, newMockStore(), newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(t.TempDir(), "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	wrongBacklink := filepath.Join(t.TempDir(), "other-checkout", ".git")
	if err := os.WriteFile(filepath.Join(adminPath, "gitdir"), []byte(wrongBacklink+"\n"), 0644); err != nil {
		t.Fatalf("write mismatched backlink: %v", err)
	}

	if mgr.IsValid(worktreePath) {
		t.Fatal("IsValid accepted a checkout whose reciprocal gitdir backlink points elsewhere")
	}
}

func TestManager_AdmitTaskRecoveryRefusesBacklinkMismatchWithoutMutation(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "backlink-mismatch")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	wrongBacklink := filepath.Join(t.TempDir(), "foreign-checkout", ".git")
	if err := os.WriteFile(filepath.Join(adminPath, "gitdir"), []byte(wrongBacklink+"\n"), 0644); err != nil {
		t.Fatalf("write mismatched backlink: %v", err)
	}

	store := newMockStore()
	original := &Worktree{ID: "wt-1", TaskID: "task-1", RepositoryID: "repo-1", RepositoryPath: repoPath,
		Path: worktreePath, Branch: "feature/pr-branch", BaseBranch: "main", Status: StatusActive}
	store.worktrees[original.ID] = original
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	err = mgr.AdmitTaskRecovery(ctx, original.TaskID)
	if !errors.Is(err, ErrWorktreeCorrupted) {
		t.Fatalf("AdmitTaskRecovery() error = %v, want ErrWorktreeCorrupted", err)
	}
	if _, err := os.Stat(worktreePath + ".kandev-recovery.json"); !os.IsNotExist(err) {
		t.Fatalf("recovery record exists after ambiguous backlink refusal: %v", err)
	}
	if got := store.worktrees[original.ID]; got != original || got.Path != worktreePath {
		t.Fatalf("durable worktree changed after refusal: %+v", got)
	}
	if matches, err := filepath.Glob(worktreePath + ".recovered-*"); err != nil || len(matches) != 0 {
		t.Fatalf("replacement paths = %v, err = %v, want none", matches, err)
	}
	if got, err := os.ReadFile(filepath.Join(adminPath, "gitdir")); err != nil || string(got) != wrongBacklink+"\n" {
		t.Fatalf("admin backlink changed after refusal: %q, err=%v", got, err)
	}
}

func TestCheckoutManifestIncludesEmptyDirectoriesAndDirectoryModes(t *testing.T) {
	root := t.TempDir()
	before, err := checkoutManifest(root)
	if err != nil {
		t.Fatalf("checkoutManifest before: %v", err)
	}
	emptyDirectory := filepath.Join(root, "empty")
	if err := os.Mkdir(emptyDirectory, 0750); err != nil {
		t.Fatalf("mkdir empty directory: %v", err)
	}
	withDirectory, err := checkoutManifest(root)
	if err != nil {
		t.Fatalf("checkoutManifest with directory: %v", err)
	}
	if before == withDirectory {
		t.Fatal("checkout manifest did not include an empty directory")
	}
	if err := os.Chmod(emptyDirectory, 0700); err != nil {
		t.Fatalf("chmod empty directory: %v", err)
	}
	withMode, err := checkoutManifest(root)
	if err != nil {
		t.Fatalf("checkoutManifest with mode: %v", err)
	}
	if withDirectory == withMode {
		t.Fatal("checkout manifest did not include directory mode")
	}
}

func TestManager_RecoverWorktreeRefusesRequestRepositoryMismatchBeforeSnapshot(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "repository-mismatch")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read git pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove fixture admin: %v", err)
	}

	store := newMockStore()
	wt := &Worktree{ID: "wt-1", TaskID: "task-1", RepositoryID: "repo-1", RepositoryPath: repoPath,
		Path: worktreePath, Branch: "feature/pr-branch", BaseBranch: "main", Status: StatusActive}
	store.worktrees[wt.ID] = wt
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	_, err = mgr.RecoverWorktree(ctx, wt, CreateRequest{TaskID: wt.TaskID, RepositoryID: "other-repo", RepositoryPath: repoPath, BaseBranch: "main"})
	if !errors.Is(err, ErrWorktreeCorrupted) {
		t.Fatalf("RecoverWorktree() error = %v, want ErrWorktreeCorrupted", err)
	}
	if _, err := os.Stat(worktreePath + ".kandev-recovery.json"); !os.IsNotExist(err) {
		t.Fatalf("recovery record exists after identity refusal: %v", err)
	}
}

func TestManager_Create_RefusesInvalidNonEmptyCheckoutWithoutRemovingIt(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
	uniqueFile := filepath.Join(worktreePath, "untracked-preserve-me.txt")
	if err := os.WriteFile(uniqueFile, []byte("unique content\n"), 0600); err != nil {
		t.Fatalf("write unique file: %v", err)
	}

	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove linked worktree admin directory: %v", err)
	}

	store := newMockStore()
	store.worktrees["wt-1"] = &Worktree{
		ID: "wt-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1",
		RepositoryPath: repoPath, Path: worktreePath, Branch: "feature/pr-branch", Status: StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, err = mgr.Create(ctx, CreateRequest{
		TaskID: "task-1", SessionID: "session-1", RepositoryID: "repo-1", RepositoryPath: repoPath,
		BaseBranch: "main", WorktreeID: "wt-1", TaskDirName: "task-1", RepoName: "repo-1",
	})
	if !errors.Is(err, ErrWorktreeCorrupted) {
		t.Fatalf("Create() error = %v, want ErrWorktreeCorrupted", err)
	}
	var recoveryErr *WorktreeRecoveryError
	if !errors.As(err, &recoveryErr) {
		t.Fatalf("Create() error = %T, want WorktreeRecoveryError", err)
	}
	if recoveryErr.TaskID != "task-1" || recoveryErr.Checkout != worktreePath || !strings.Contains(recoveryErr.Reason, adminPath) {
		t.Fatalf("recovery error = %+v, want owning task, checkout, and missing admin target", recoveryErr)
	}
	if content, readErr := os.ReadFile(uniqueFile); readErr != nil || string(content) != "unique content\n" {
		t.Fatalf("unique checkout content changed after refusal: content=%q err=%v", content, readErr)
	}
}

func TestManager_Create_RefusesMissingAdminWhenRecordedBranchIsUnreachable(t *testing.T) {
	ctx := context.Background()
	cfg := newTestConfig(t)
	repoPath := initGitRepoForWorktreeTest(t)
	worktreePath := filepath.Join(cfg.TasksBasePath, "linked-worktree")
	runGit(t, repoPath, "worktree", "add", worktreePath, "feature/pr-branch")
	uniqueFile := filepath.Join(worktreePath, "untracked-preserve-me.txt")
	if err := os.WriteFile(uniqueFile, []byte("unique content\n"), 0600); err != nil {
		t.Fatalf("write unique file: %v", err)
	}

	gitPointer, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		t.Fatalf("read linked worktree pointer: %v", err)
	}
	adminPath := strings.TrimSpace(strings.TrimPrefix(string(gitPointer), "gitdir:"))
	runGit(t, repoPath, "update-ref", "-d", "refs/heads/feature/pr-branch")
	if err := os.RemoveAll(adminPath); err != nil {
		t.Fatalf("remove linked worktree admin directory: %v", err)
	}

	store := newMockStore()
	store.worktrees["wt-1"] = &Worktree{
		ID: "wt-1", SessionID: "session-1", TaskID: "task-1", RepositoryID: "repo-1",
		RepositoryPath: repoPath, Path: worktreePath, Branch: "feature/pr-branch", Status: StatusActive,
	}
	mgr, err := NewManager(cfg, store, newTestLogger())
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, err = mgr.Create(ctx, CreateRequest{
		TaskID: "task-1", SessionID: "session-1", RepositoryID: "repo-1", RepositoryPath: repoPath,
		BaseBranch: "main", WorktreeID: "wt-1", TaskDirName: "task-1", RepoName: "repo-1",
	})
	var recoveryErr *WorktreeRecoveryError
	if !errors.As(err, &recoveryErr) {
		t.Fatalf("Create() error = %T, want WorktreeRecoveryError", err)
	}
	if !errors.Is(err, ErrWorktreeCorrupted) {
		t.Fatalf("Create() error = %v, want ErrWorktreeCorrupted", err)
	}
	if !strings.Contains(recoveryErr.Reason, "content-only") || !strings.Contains(recoveryErr.Reason, "feature/pr-branch") {
		t.Fatalf("recovery reason = %q, want content-only unreachable branch refusal", recoveryErr.Reason)
	}
	if content, readErr := os.ReadFile(uniqueFile); readErr != nil || string(content) != "unique content\n" {
		t.Fatalf("unique checkout content changed after refusal: content=%q err=%v", content, readErr)
	}
}
