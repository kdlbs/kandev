package worktree

import (
	"bufio"
	"context"
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
	defer file.Close()

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

// unlockGitCryptAndCheckout unlocks git-crypt in a worktree and performs
// the checkout that was skipped during worktree creation.
//
// When a repository uses git-crypt and we create a worktree with --no-checkout,
// we need to:
// 1. Unlock git-crypt in the worktree (using the key from the main repo)
// 2. Checkout the files (which will now be properly decrypted)
func (m *Manager) unlockGitCryptAndCheckout(ctx context.Context, worktreePath string) error {
	m.logger.Debug("unlocking git-crypt in worktree",
		zap.String("worktree_path", worktreePath))

	// Step 1: Unlock git-crypt
	// git-crypt unlock will find the key from GIT_COMMON_DIR (the main repo's .git)
	unlockCmd := exec.CommandContext(ctx, "git-crypt", "unlock")
	unlockCmd.Dir = worktreePath
	if output, err := unlockCmd.CombinedOutput(); err != nil {
		m.logger.Error("git-crypt unlock failed in worktree",
			zap.String("worktree_path", worktreePath),
			zap.String("output", string(output)),
			zap.Error(err))
		return &GitCryptError{
			Op:     "unlock",
			Path:   worktreePath,
			Output: string(output),
			Err:    err,
		}
	}

	// Step 2: Checkout HEAD to populate the working tree
	checkoutCmd := exec.CommandContext(ctx, "git", "checkout", "HEAD", "--", ".")
	checkoutCmd.Dir = worktreePath
	if output, err := checkoutCmd.CombinedOutput(); err != nil {
		m.logger.Error("git checkout failed after git-crypt unlock",
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

	m.logger.Info("successfully unlocked git-crypt and checked out worktree",
		zap.String("worktree_path", worktreePath))
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
