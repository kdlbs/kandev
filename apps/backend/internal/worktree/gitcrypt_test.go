package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHasGitCryptFilter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "file with git-crypt filter",
			content:  "secretfile filter=git-crypt diff=git-crypt\n*.key filter=git-crypt diff=git-crypt",
			expected: true,
		},
		{
			name:     "file without git-crypt filter",
			content:  "*.txt text\n*.bin binary",
			expected: false,
		},
		{
			name:     "empty file",
			content:  "",
			expected: false,
		},
		{
			name:     "filter with different name",
			content:  "*.lfs filter=lfs diff=lfs",
			expected: false,
		},
		{
			name:     "git-crypt in comment",
			content:  "# This uses filter=git-crypt for encryption\n*.txt text",
			expected: true, // We still detect it - conservative approach
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, ".gitattributes")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}

			result := hasGitCryptFilter(tmpFile)
			if result != tt.expected {
				t.Errorf("hasGitCryptFilter() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHasGitCryptFilter_NonexistentFile(t *testing.T) {
	result := hasGitCryptFilter("/nonexistent/path/.gitattributes")
	if result {
		t.Error("hasGitCryptFilter() should return false for nonexistent file")
	}
}

func TestIsGitCryptSmudgeError(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "smudge filter git-crypt failed",
			output:   "fatal: tools/secrets/config.yml: smudge filter git-crypt failed",
			expected: true,
		},
		{
			name:     "external filter git-crypt smudge failed",
			output:   "error: external filter '\"git-crypt\" smudge' failed 1",
			expected: true,
		},
		{
			name:     "external filter without quotes",
			output:   "error: external filter 'git-crypt smudge' failed",
			expected: true,
		},
		{
			name:     "portuguese locale error",
			output:   "error: filtro externo 'git-crypt smudge' falhou 1",
			expected: true,
		},
		{
			name:     "portuguese fatal with git-crypt smudge command",
			output:   "fatal: tools/config.yml: filtro de mancha git-crypt smudge falhou",
			expected: true,
		},
		{
			name:     "german locale error",
			output:   "Fehler: externer Filter 'git-crypt smudge' fehlgeschlagen",
			expected: true,
		},
		{
			name:     "unrelated git error",
			output:   "fatal: pathspec 'foo' did not match any files",
			expected: false,
		},
		{
			name:     "empty output",
			output:   "",
			expected: false,
		},
		{
			name:     "branch checkout error",
			output:   "fatal: 'main' is already checked out at '/path/to/worktree'",
			expected: false,
		},
		{
			name:     "only git-crypt without smudge",
			output:   "git-crypt: some other error",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGitCryptSmudgeError(tt.output)
			if result != tt.expected {
				t.Errorf("isGitCryptSmudgeError(%q) = %v, want %v", tt.output, result, tt.expected)
			}
		})
	}
}

func TestUsesGitCrypt(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake git repo structure
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "info"), 0755); err != nil {
		t.Fatalf("failed to create .git/info: %v", err)
	}

	log := newTestLogger()
	m := &Manager{logger: log}

	// Test 1: No .gitattributes
	if m.usesGitCrypt(tmpDir) {
		t.Error("usesGitCrypt() should return false when no .gitattributes exists")
	}

	// Test 2: .gitattributes without git-crypt
	gitattributes := filepath.Join(tmpDir, ".gitattributes")
	if err := os.WriteFile(gitattributes, []byte("*.txt text\n"), 0644); err != nil {
		t.Fatalf("failed to write .gitattributes: %v", err)
	}
	if m.usesGitCrypt(tmpDir) {
		t.Error("usesGitCrypt() should return false when .gitattributes has no git-crypt")
	}

	// Test 3: .gitattributes with git-crypt
	if err := os.WriteFile(gitattributes, []byte("secrets/* filter=git-crypt diff=git-crypt\n"), 0644); err != nil {
		t.Fatalf("failed to write .gitattributes: %v", err)
	}
	if !m.usesGitCrypt(tmpDir) {
		t.Error("usesGitCrypt() should return true when .gitattributes has git-crypt")
	}
}

func TestIsGitCryptUnlocked(t *testing.T) {
	t.Run("returns false when dir does not exist", func(t *testing.T) {
		if isGitCryptUnlocked("/nonexistent/git-crypt") {
			t.Error("expected false for nonexistent directory")
		}
	})

	t.Run("returns false when keys subdir is empty (locked)", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "keys"), 0755); err != nil {
			t.Fatal(err)
		}
		if isGitCryptUnlocked(dir) {
			t.Error("expected false for empty keys directory (locked repo)")
		}
	})

	t.Run("returns true when keys subdir has entries (unlocked)", func(t *testing.T) {
		dir := t.TempDir()
		defaultKey := filepath.Join(dir, "keys", "default")
		if err := os.MkdirAll(defaultKey, 0755); err != nil {
			t.Fatal(err)
		}
		if !isGitCryptUnlocked(dir) {
			t.Error("expected true when keys directory has entries (unlocked repo)")
		}
	})
}

// ---------- E2E tests requiring git-crypt binary ----------

// skipIfNoGitCrypt skips the test if the git-crypt binary is not available.
func skipIfNoGitCrypt(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git-crypt"); err != nil {
		t.Skip("git-crypt not installed, skipping e2e test")
	}
}

// initGitCryptRepo creates a temporary git repository with git-crypt
// initialised and a mix of encrypted and plain-text files committed.
// Returns the repo path. The caller owns cleanup via t.TempDir().
func initGitCryptRepo(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "-b", "main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")

	// Initialise git-crypt (generates symmetric key in .git/git-crypt/).
	cmd := exec.Command("git-crypt", "init")
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git-crypt init failed: %v\n%s", err, out)
	}

	// Set up .gitattributes: secret.txt is encrypted, public.txt is not.
	if err := os.WriteFile(
		filepath.Join(repoPath, ".gitattributes"),
		[]byte("secret.txt filter=git-crypt diff=git-crypt\n"),
		0644,
	); err != nil {
		t.Fatalf("write .gitattributes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "secret.txt"), []byte("top-secret-value\n"), 0644); err != nil {
		t.Fatalf("write secret.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "public.txt"), []byte("public-value\n"), 0644); err != nil {
		t.Fatalf("write public.txt: %v", err)
	}

	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "initial commit with git-crypt")

	// Also create a feature branch for the "existing branch" test.
	runGit(t, repoPath, "branch", "feature/encrypted-branch")

	return repoPath
}

func TestCreateWorktree_GitCryptNewBranch(t *testing.T) {
	skipIfNoGitCrypt(t)

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	repoPath := initGitCryptRepo(t)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "gc-task-1",
		SessionID:      "gc-session-1",
		TaskTitle:      "Git Crypt New Branch",
		RepositoryID:   "repo-gc",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
	})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// The worktree must exist and be on a new branch.
	if wt.Path == "" {
		t.Fatal("worktree path is empty")
	}
	gotBranch := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "--abbrev-ref", "HEAD"))
	if gotBranch == "main" {
		t.Fatal("expected a new branch, got main")
	}

	// Encrypted file must be decrypted in the worktree.
	secret, err := os.ReadFile(filepath.Join(wt.Path, "secret.txt"))
	if err != nil {
		t.Fatalf("read secret.txt: %v", err)
	}
	if strings.TrimSpace(string(secret)) != "top-secret-value" {
		t.Errorf("secret.txt = %q, want %q", string(secret), "top-secret-value\n")
	}

	// Plain-text file must also be present.
	pub, err := os.ReadFile(filepath.Join(wt.Path, "public.txt"))
	if err != nil {
		t.Fatalf("read public.txt: %v", err)
	}
	if strings.TrimSpace(string(pub)) != "public-value" {
		t.Errorf("public.txt = %q, want %q", string(pub), "public-value\n")
	}

	// Working tree must be clean.
	status := strings.TrimSpace(runGit(t, wt.Path, "status", "--porcelain"))
	if status != "" {
		t.Errorf("worktree is not clean: %s", status)
	}
}

func TestCreateWorktree_GitCryptExistingBranch(t *testing.T) {
	skipIfNoGitCrypt(t)

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	repoPath := initGitCryptRepo(t)

	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "gc-task-2",
		SessionID:      "gc-session-2",
		TaskTitle:      "Git Crypt Existing Branch",
		RepositoryID:   "repo-gc",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
		CheckoutBranch: "feature/encrypted-branch",
	})
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	gotBranch := strings.TrimSpace(runGit(t, wt.Path, "rev-parse", "--abbrev-ref", "HEAD"))
	if gotBranch != "feature/encrypted-branch" {
		t.Fatalf("branch = %q, want %q", gotBranch, "feature/encrypted-branch")
	}

	// Encrypted file must be decrypted.
	secret, err := os.ReadFile(filepath.Join(wt.Path, "secret.txt"))
	if err != nil {
		t.Fatalf("read secret.txt: %v", err)
	}
	if strings.TrimSpace(string(secret)) != "top-secret-value" {
		t.Errorf("secret.txt = %q, want %q", string(secret), "top-secret-value\n")
	}

	status := strings.TrimSpace(runGit(t, wt.Path, "status", "--porcelain"))
	if status != "" {
		t.Errorf("worktree is not clean: %s", status)
	}
}

func TestCreateWorktree_GitCryptDetection(t *testing.T) {
	skipIfNoGitCrypt(t)

	log := newTestLogger()
	m := &Manager{logger: log}

	repoPath := initGitCryptRepo(t)

	if !m.usesGitCrypt(repoPath) {
		t.Error("usesGitCrypt() should return true for a git-crypt initialised repo")
	}

	// Verify that the object store actually has encrypted content.
	raw := runGit(t, repoPath, "show", "HEAD:secret.txt")
	if strings.Contains(raw, "top-secret-value") {
		t.Error("expected encrypted blob in object store, got plaintext")
	}
}

func TestUnlockGitCryptAndCheckout_ManualWorktree(t *testing.T) {
	skipIfNoGitCrypt(t)

	log := newTestLogger()
	m := &Manager{logger: log}

	repoPath := initGitCryptRepo(t)

	// Manually create a worktree with --no-checkout (simulates what manager does).
	wtPath := filepath.Join(t.TempDir(), "manual-wt")
	runGit(t, repoPath, "worktree", "add", "-b", "manual-test", "--no-checkout", wtPath, "main")

	// The worktree should have no files checked out.
	entries, _ := os.ReadDir(wtPath)
	fileCount := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			fileCount++
		}
	}
	if fileCount != 0 {
		t.Fatalf("expected 0 non-dot files after --no-checkout, got %d", fileCount)
	}

	// Now run unlockGitCryptAndCheckout.
	if err := m.unlockGitCryptAndCheckout(context.Background(), wtPath); err != nil {
		t.Fatalf("unlockGitCryptAndCheckout() failed: %v", err)
	}

	// Verify files are checked out and decrypted.
	secret, err := os.ReadFile(filepath.Join(wtPath, "secret.txt"))
	if err != nil {
		t.Fatalf("read secret.txt after unlock: %v", err)
	}
	if strings.TrimSpace(string(secret)) != "top-secret-value" {
		t.Errorf("secret.txt = %q, want %q", string(secret), "top-secret-value\n")
	}

	pub, err := os.ReadFile(filepath.Join(wtPath, "public.txt"))
	if err != nil {
		t.Fatalf("read public.txt after unlock: %v", err)
	}
	if strings.TrimSpace(string(pub)) != "public-value" {
		t.Errorf("public.txt = %q, want %q", string(pub), "public-value\n")
	}

	status := strings.TrimSpace(runGit(t, wtPath, "status", "--porcelain"))
	if status != "" {
		t.Errorf("worktree not clean after unlock+checkout: %s", status)
	}
}


func TestCreateWorktree_GitCryptLockedRepo(t *testing.T) {
	skipIfNoGitCrypt(t)

	cfg := newTestConfig(t)
	log := newTestLogger()
	store := newMockStore()

	mgr, err := NewManager(cfg, store, log)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	repoPath := initGitCryptRepo(t)

	// Export the key before locking (git-crypt lock removes it from .git/).
	keyFile := filepath.Join(t.TempDir(), "exported.key")
	exportCmd := exec.Command("git-crypt", "export-key", keyFile)
	exportCmd.Dir = repoPath
	if out, err := exportCmd.CombinedOutput(); err != nil {
		t.Fatalf("git-crypt export-key failed: %v\n%s", err, out)
	}

	// Lock the repo — removes keys, re-encrypts working tree files.
	lockCmd := exec.Command("git-crypt", "lock")
	lockCmd.Dir = repoPath
	if out, err := lockCmd.CombinedOutput(); err != nil {
		t.Fatalf("git-crypt lock failed: %v\n%s", err, out)
	}

	// Verify the repo is locked (secret.txt should be encrypted binary).
	secretContent, _ := os.ReadFile(filepath.Join(repoPath, "secret.txt"))
	if strings.Contains(string(secretContent), "top-secret-value") {
		t.Fatal("expected secret.txt to be encrypted after lock")
	}

	// Create a worktree from the locked repo — should succeed.
	wt, err := mgr.Create(context.Background(), CreateRequest{
		TaskID:         "gc-locked-1",
		SessionID:      "gc-locked-session-1",
		TaskTitle:      "Git Crypt Locked Repo",
		RepositoryID:   "repo-gc-locked",
		RepositoryPath: repoPath,
		BaseBranch:     "main",
	})
	if err != nil {
		t.Fatalf("Create() with locked repo should succeed, got: %v", err)
	}

	// Public file must be present and readable.
	pub, err := os.ReadFile(filepath.Join(wt.Path, "public.txt"))
	if err != nil {
		t.Fatalf("read public.txt: %v", err)
	}
	if strings.TrimSpace(string(pub)) != "public-value" {
		t.Errorf("public.txt = %q, want %q", string(pub), "public-value")
	}

	// Encrypted file should exist but be binary (not decrypted).
	secret, err := os.ReadFile(filepath.Join(wt.Path, "secret.txt"))
	if err != nil {
		t.Fatalf("read secret.txt: %v", err)
	}
	if strings.Contains(string(secret), "top-secret-value") {
		t.Error("secret.txt should be encrypted in worktree of locked repo")
	}

	// Now unlock git-crypt in the worktree — this should work because
	// no broken filters are configured.
	unlockCmd := exec.Command("git-crypt", "unlock", keyFile)
	unlockCmd.Dir = wt.Path
	if out, unlockErr := unlockCmd.CombinedOutput(); unlockErr != nil {
		t.Fatalf("git-crypt unlock in worktree should work, got: %v\n%s", unlockErr, out)
	}

	// After unlock, secret.txt should be decrypted.
	secret, err = os.ReadFile(filepath.Join(wt.Path, "secret.txt"))
	if err != nil {
		t.Fatalf("read secret.txt after unlock: %v", err)
	}
	if strings.TrimSpace(string(secret)) != "top-secret-value" {
		t.Errorf("secret.txt after unlock = %q, want %q", string(secret), "top-secret-value")
	}
}
