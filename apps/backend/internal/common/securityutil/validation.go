package securityutil

import (
	"fmt"
	"regexp"
	"strings"
)

// validBranchNameRegex matches safe git branch names.
// Allows alphanumeric, hyphens, underscores, slashes, and dots.
// Disallows: spaces, shell metacharacters, and control characters.
var validBranchNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

// IsValidBranchName validates that a branch name is safe to use in git commands.
// Returns true if the branch name:
// - Is not empty and under 256 characters
// - Matches the alphanumeric pattern with safe special chars
// - Does not contain ".." (path traversal prevention)
// - Does not end with ".lock" (git internal file)
func IsValidBranchName(branch string) bool {
	if branch == "" || len(branch) > 255 {
		return false
	}
	// Disallow ".." to prevent path traversal
	if strings.Contains(branch, "..") {
		return false
	}
	// Disallow ending with ".lock"
	if strings.HasSuffix(branch, ".lock") {
		return false
	}
	return validBranchNameRegex.MatchString(branch)
}

// IsKnownSafeGitFlag returns true if the argument is a known safe git flag used by this codebase.
// This prevents argument injection where user input could introduce malicious flags.
// Only flags actually used by the Kandev codebase are whitelisted.
func IsKnownSafeGitFlag(arg string) bool {
	// Whitelist of git flags actually used by our codebase
	safeFlags := []string{
		"-m", "-M", "--set-upstream", "--all", "--porcelain", "--short",
		"--abbrev-ref", "--symbolic-full-name", "--verify", "--no-patch",
		"--format", "--format=", "--stat", "--numstat", "-p", "-A",
		"--amend", "--allow-empty", "--soft", "--mixed", "--hard",
		"--cached", "--force", "--source=HEAD", "--staged", "--worktree",
		"--", // Path separator - everything after this is treated as paths, not flags
	}
	for _, safe := range safeFlags {
		if arg == safe || strings.HasPrefix(arg, safe) {
			return true
		}
	}
	return false
}

// IsKnownSafeGitLiteral returns true for git-specific literals that are safe to use in commands.
// This includes git refs, remotes, and special markers that don't need branch name validation.
func IsKnownSafeGitLiteral(arg string) bool {
	// Common safe git literals
	safeLiterals := []string{
		"HEAD", "ORIG_HEAD", "FETCH_HEAD", "MERGE_HEAD",
		"origin", "upstream", ".", "--",
	}
	for _, safe := range safeLiterals {
		if arg == safe {
			return true
		}
	}
	// HEAD~N, HEAD^N, HEAD@{} patterns are safe
	if strings.HasPrefix(arg, "HEAD~") || strings.HasPrefix(arg, "HEAD^") || strings.HasPrefix(arg, "HEAD@{") {
		return true
	}
	return false
}

// LooksLikeCommitSHA returns true if the string looks like a git commit SHA (hex chars only).
// This allows commit SHAs to bypass branch name validation.
// Valid commit SHAs are 7-64 hexadecimal characters (short or full SHA).
func LooksLikeCommitSHA(arg string) bool {
	if len(arg) < 7 || len(arg) > 64 {
		return false
	}
	for _, c := range arg {
		if !isHexChar(c) {
			return false
		}
	}
	return true
}

// ValidateBranchReference validates a branch reference like "origin/branch".
// Returns an error if the reference contains invalid branch or remote names.
func ValidateBranchReference(arg string) error {
	parts := strings.SplitN(arg, "/", 2)
	if len(parts) != 2 {
		return nil // Not a reference format
	}

	remote, branch := parts[0], parts[1]
	if !IsValidBranchName(remote) {
		return fmt.Errorf("invalid remote name in reference '%s'", arg)
	}
	if !IsValidBranchName(branch) {
		return fmt.Errorf("invalid branch name in reference '%s'", arg)
	}
	return nil
}

// isHexChar returns true if the rune is a hexadecimal character (0-9, a-f, A-F).
func isHexChar(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
