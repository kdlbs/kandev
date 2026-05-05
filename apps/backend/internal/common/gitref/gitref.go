// Package gitref reads git ref state directly from the on-disk .git
// directory, without invoking the git binary. The functions here are kept
// minimal and stdlib-only so they can be imported from both the task service
// and the sqlite migration layer (where adding a service dependency would
// invert the existing layering).
package gitref

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const headFile = "HEAD"

// DefaultBranch returns the repository's *integration* branch — the branch
// that work is meant to merge back into. It is intentionally NOT the current
// HEAD: a developer who runs the dialog while checked out on a feature
// branch must still get "main" (or "master") back, otherwise downstream
// consumers (changes-panel merge-base, executor BaseBranch fallback) anchor
// to the wrong ref.
//
// Resolution order:
//  1. refs/remotes/origin/HEAD when set (the upstream's declared default)
//  2. The first ref that exists from
//     {origin/main, origin/master, main, master}
//  3. The current HEAD as a last resort, so brand-new repos with only a
//     feature branch still produce a value — callers that care about
//     correctness can override.
func DefaultBranch(repoPath string) (string, error) {
	gitDir, err := ResolveGitDir(repoPath)
	if err != nil {
		return "", err
	}
	commonDir := ResolveCommonGitDir(gitDir)
	if branch := readOriginHEAD(commonDir); branch != "" {
		return branch, nil
	}
	if branch := pickFirstExistingBranch(commonDir); branch != "" {
		return branch, nil
	}
	return readHEADBranchFallback(gitDir)
}

// ResolveGitDir returns the actual git directory for repoPath, following
// `.git` files (worktree pointers) when present.
func ResolveGitDir(repoPath string) (string, error) {
	gitPath := filepath.Join(repoPath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return gitPath, nil
	}
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir:") {
		return "", fmt.Errorf("invalid gitdir reference")
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if filepath.IsAbs(gitDir) {
		return gitDir, nil
	}
	return filepath.Clean(filepath.Join(repoPath, gitDir)), nil
}

// ResolveCommonGitDir returns the shared git dir for a worktree, or gitDir
// itself for a regular repo. Refs (refs/heads/*, refs/remotes/*, packed-refs)
// live under the common dir, not the worktree's gitDir.
func ResolveCommonGitDir(gitDir string) string {
	commonFile := filepath.Join(gitDir, "commondir")
	content, err := os.ReadFile(commonFile)
	if err != nil {
		return gitDir
	}
	commonDir := strings.TrimSpace(string(content))
	if commonDir == "" {
		return gitDir
	}
	if filepath.IsAbs(commonDir) {
		return filepath.Clean(commonDir)
	}
	return filepath.Clean(filepath.Join(gitDir, commonDir))
}

func readOriginHEAD(commonDir string) string {
	headPath := filepath.Join(commonDir, "refs", "remotes", "origin", headFile)
	content, err := os.ReadFile(headPath)
	if err != nil {
		// origin/HEAD only lives in packed-refs after a `git pack-refs --all`,
		// and the on-disk symref format there is gnarly to parse correctly.
		// We deliberately skip that case rather than maintain a broken parser:
		// pickFirstExistingBranch's origin/main → origin/master → main →
		// master fallbacks cover every realistic clone in practice.
		return ""
	}
	return parseSymbolicRefToBranch(strings.TrimSpace(string(content)))
}

func parseSymbolicRefToBranch(line string) string {
	ref, ok := strings.CutPrefix(line, "ref: ")
	if !ok {
		ref = line
	}
	switch {
	case strings.HasPrefix(ref, "refs/remotes/origin/"):
		return strings.TrimPrefix(ref, "refs/remotes/origin/")
	case strings.HasPrefix(ref, "refs/heads/"):
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	return ""
}

func pickFirstExistingBranch(commonDir string) string {
	for _, candidate := range []struct{ ref, branch string }{
		{"refs/remotes/origin/main", "main"},
		{"refs/remotes/origin/master", "master"},
		{"refs/heads/main", "main"},
		{"refs/heads/master", "master"},
	} {
		if refExists(commonDir, candidate.ref) {
			return candidate.branch
		}
	}
	return ""
}

func refExists(commonDir, ref string) bool {
	if _, err := os.Stat(filepath.Join(commonDir, ref)); err == nil {
		return true
	}
	content, err := os.ReadFile(filepath.Join(commonDir, "packed-refs"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		_, after, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		if after == ref {
			return true
		}
	}
	return false
}

func readHEADBranchFallback(gitDir string) (string, error) {
	headPath := filepath.Join(gitDir, headFile)
	content, err := os.ReadFile(headPath)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(string(content))
	if ref, ok := strings.CutPrefix(trimmed, "ref: "); ok {
		// Strip refs/heads/ as a prefix rather than splitting on "/" — branch
		// names legally contain slashes (e.g. "feature/my-feature"), so taking
		// the last path component would silently corrupt every nested branch.
		return strings.TrimPrefix(ref, "refs/heads/"), nil
	}
	if trimmed != "" {
		return headFile, nil
	}
	return "", fmt.Errorf("unable to determine branch")
}
