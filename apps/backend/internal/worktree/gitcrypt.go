package worktree

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// usesGitCrypt checks if a repository uses git-crypt by looking for
// the git-crypt filter in .gitattributes files.
func (m *Manager) usesGitCrypt(repoPath string) bool {
	// Check root .gitattributes
	if hasGitCryptFilter(filepath.Join(repoPath, ".gitattributes")) {
		return true
	}

	// Check .git/info/attributes (local gitattributes)
	gitDir := filepath.Join(repoPath, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		if hasGitCryptFilter(filepath.Join(gitDir, "info", "attributes")) {
			return true
		}
	}

	return false
}

// hasGitCryptFilter checks if a gitattributes file contains the git-crypt filter.
func hasGitCryptFilter(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// git-crypt uses "filter=git-crypt" in gitattributes
		if strings.Contains(line, "filter=git-crypt") {
			return true
		}
	}
	return false
}

// unlockGitCryptAndCheckout sets up git-crypt decryption in a worktree created
// with --no-checkout and then checks out the files.
//
// git-crypt unlock cannot be used directly because it refuses to run when the
// working directory is not clean (which is always the case after --no-checkout).
// Instead we manually replicate what git-crypt unlock does:
//  1. Symlink the git-crypt key directory from the main repo (GIT_COMMON_DIR)
//     into the worktree's own git directory so the smudge filter can find it.
//  2. Configure the smudge/clean/diff filters in the worktree's local git config.
//  3. Run git checkout to populate the working tree with decrypted files.
func (m *Manager) unlockGitCryptAndCheckout(ctx context.Context, worktreePath string) error {
	m.logger.Debug("setting up git-crypt filters in worktree",
		zap.String("worktree_path", worktreePath))

	// Step 1: Resolve the worktree's git dir and the common dir (main repo .git).
	commonDir, err := resolveGitDir(ctx, worktreePath, "--git-common-dir")
	if err != nil {
		return &GitCryptError{Op: "resolve-common-dir", Path: worktreePath, Output: "", Err: err}
	}
	gitDir, err := resolveGitDir(ctx, worktreePath, "--git-dir")
	if err != nil {
		return &GitCryptError{Op: "resolve-git-dir", Path: worktreePath, Output: "", Err: err}
	}

	// Step 2: Symlink the git-crypt key directory into the worktree git dir.
	src := filepath.Join(commonDir, "git-crypt")
	dst := filepath.Join(gitDir, "git-crypt")
	if err := symlinkGitCryptDir(src, dst); err != nil {
		return &GitCryptError{Op: "symlink", Path: worktreePath, Output: "", Err: err}
	}

	// Step 3: Configure the smudge/clean/diff filters.
	if err := configureGitCryptFilters(ctx, worktreePath); err != nil {
		return &GitCryptError{Op: "config", Path: worktreePath, Output: "", Err: err}
	}

	// Step 4: Checkout HEAD to populate the working tree with decrypted files.
	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", "HEAD", "--", ".")
	checkoutCmd.Dir = worktreePath
	if output, err := checkoutCmd.CombinedOutput(); err != nil {
		m.logger.Error("git checkout failed after git-crypt setup",
			zap.String("worktree_path", worktreePath),
			zap.String("output", string(output)),
			zap.Error(err))
		return &GitCryptError{
			Op:     "checkout",
			Path:   worktreePath,
			Output: string(output),
			Err:    err,
		}
	}

	m.logger.Info("successfully set up git-crypt and checked out worktree",
		zap.String("worktree_path", worktreePath))
	return nil
}

// resolveGitDir runs git rev-parse with the given flag (e.g. --git-dir,
// --git-common-dir) and returns the resolved absolute path.
func resolveGitDir(ctx context.Context, worktreePath, flag string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", flag)
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", flag, err)
	}
	resolved := strings.TrimSpace(string(out))
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(worktreePath, resolved)
	}
	return resolved, nil
}

// symlinkGitCryptDir creates a symlink from src (main repo's .git/git-crypt)
// to dst (worktree's git dir git-crypt). Skips if dst already exists.
func symlinkGitCryptDir(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("git-crypt key dir not found at %s: %w", src, err)
	}
	// Already set up (e.g. retry path).
	if _, err := os.Lstat(dst); err == nil {
		return nil
	}
	return os.Symlink(src, dst)
}

// configureGitCryptFilters sets the smudge/clean/diff filters in the
// worktree's local git config so that git checkout can decrypt files.
func configureGitCryptFilters(ctx context.Context, worktreePath string) error {
	configs := [][2]string{
		{"filter.git-crypt.smudge", "git-crypt smudge"},
		{"filter.git-crypt.clean", "git-crypt clean"},
		{"filter.git-crypt.required", "true"},
		{"diff.git-crypt.textconv", "git-crypt diff"},
	}
	for _, kv := range configs {
		cmd := exec.CommandContext(ctx, "git", "config", kv[0], kv[1])
		cmd.Dir = worktreePath
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git config %s: %s: %w", kv[0], string(out), err)
		}
	}
	return nil
}

// GitCryptError represents an error during git-crypt operations in a worktree.
type GitCryptError struct {
	Op     string // "unlock" or "checkout"
	Path   string // worktree path
	Output string // command output
	Err    error  // underlying error
}

func (e *GitCryptError) Error() string {
	return "git-crypt " + e.Op + " failed in worktree " + e.Path + ": " + e.Output
}

func (e *GitCryptError) Unwrap() error {
	return e.Err
}

// isGitCryptSmudgeError checks if a git error is caused by git-crypt smudge filter failure.
func isGitCryptSmudgeError(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "smudge filter git-crypt failed") ||
		strings.Contains(lower, "filter '\"git-crypt\" smudge' failed") ||
		strings.Contains(lower, "filter 'git-crypt smudge' failed")
}
