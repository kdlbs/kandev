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
// If the main repository is unlocked (has keys), it replicates what git-crypt
// unlock does so the worktree gets decrypted files:
//  1. Symlink the git-crypt key directory from the main repo into the worktree.
//  2. Configure the smudge/clean/diff filters in the worktree's local git config.
//  3. Run git checkout to populate the working tree with decrypted files.
//
// If the main repository is locked (no keys), it skips the git-crypt setup and
// checks out without decryption. Encrypted files will remain as binary blobs but
// the worktree is still usable for non-encrypted files.
func (m *Manager) unlockGitCryptAndCheckout(ctx context.Context, worktreePath string) error {
	m.logger.Debug("setting up git-crypt filters in worktree",
		zap.String("worktree_path", worktreePath))

	// Resolve the worktree's git dir and the common dir (main repo .git).
	commonDir, err := resolveGitDir(ctx, worktreePath, "--git-common-dir")
	if err != nil {
		return &GitCryptError{Op: "resolve-common-dir", Path: worktreePath, Output: "", Err: err}
	}
	gitDir, err := resolveGitDir(ctx, worktreePath, "--git-dir")
	if err != nil {
		return &GitCryptError{Op: "resolve-git-dir", Path: worktreePath, Output: "", Err: err}
	}

	// Check if the main repo has git-crypt unlocked (keys present).
	src := filepath.Join(commonDir, "git-crypt")
	unlocked := isGitCryptUnlocked(src)

	if unlocked {
		// Symlink the git-crypt key directory into the worktree git dir.
		dst := filepath.Join(gitDir, "git-crypt")
		if err := symlinkGitCryptDir(src, dst); err != nil {
			return &GitCryptError{Op: "symlink", Path: worktreePath, Output: "", Err: err}
		}

		// Configure the smudge/clean/diff filters.
		if err := configureGitCryptFilters(ctx, worktreePath); err != nil {
			return &GitCryptError{Op: "config", Path: worktreePath, Output: "", Err: err}
		}
	} else {
		m.logger.Warn("git-crypt is locked in main repository, checking out without decryption",
			zap.String("common_dir", commonDir),
			zap.String("worktree_path", worktreePath))
	}

	// Checkout HEAD to populate the working tree.
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

	if unlocked {
		m.logger.Info("successfully set up git-crypt and checked out worktree",
			zap.String("worktree_path", worktreePath))
	} else {
		m.logger.Info("checked out worktree without git-crypt decryption (repo is locked)",
			zap.String("worktree_path", worktreePath))
	}
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
// Caller must verify src is valid (via isGitCryptUnlocked) before calling.
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

// isGitCryptUnlocked checks whether a git-crypt directory exists and contains
// at least one key file (e.g. keys/default). Returns false if the directory
// doesn't exist, has no keys/ subdir, or the keys/ subdir is empty (locked).
func isGitCryptUnlocked(gitCryptDir string) bool {
	keysDir := filepath.Join(gitCryptDir, "keys")
	entries, err := os.ReadDir(keysDir)
	if err != nil {
		return false
	}
	return len(entries) > 0
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
// The detection is language-agnostic: git translates its own messages but the
// filter name ("git-crypt smudge") always appears verbatim in the output.
func isGitCryptSmudgeError(output string) bool {
	lower := strings.ToLower(output)
	// Language-agnostic: look for the filter name which is never translated.
	if strings.Contains(lower, "git-crypt") && strings.Contains(lower, "smudge") {
		return true
	}
	return false
}
