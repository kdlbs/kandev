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

// IsValidBaseBranchRef validates a base-branch ref using the IsValidBranchName
// allowlist, transparently stripping an "origin/" prefix (the regex disallows
// "/" as the first character). Shared so service-tier base-branch rejection
// uses the same allowlist as task base-branch overrides and the agentctl-side
// sanitiser.
func IsValidBaseBranchRef(ref string) bool {
	rest := ref
	if stripped, ok := strings.CutPrefix(ref, "origin/"); ok {
		rest = stripped
	}
	// git check-ref-format also rejects a trailing "/" and consecutive "//",
	// which the IsValidBranchName regex (its char class includes "/") permits.
	// Reject them here so "main/" / "origin/main/" / "a//b" don't pass.
	if strings.HasSuffix(rest, "/") || strings.Contains(rest, "//") {
		return false
	}
	return IsValidBranchName(rest)
}

// gitSymbolicRefs are git's well-known symbolic refs: pure alnum/underscore
// with no leading dash and no "~"/"^", so validBranchNameRegex (and
// therefore IsValidBaseBranchRef) admits them even though none is ever a
// real branch name. A caller that persists one of these as a repository's
// default branch and later feeds it to a local rev-parse fallback (worktree
// FallbackBaseBranch, or internal/delivery's ancestry check) gets whatever
// commit that pseudo-ref currently resolves to in that checkout — silently
// the wrong commit, not a resolution failure.
var gitSymbolicRefs = map[string]struct{}{
	"HEAD":       {},
	"ORIG_HEAD":  {},
	"FETCH_HEAD": {},
	"MERGE_HEAD": {},
}

// IsGitSymbolicRef reports whether ref is one of git's reserved pseudo-refs
// above.
func IsGitSymbolicRef(ref string) bool {
	_, ok := gitSymbolicRefs[ref]
	return ok
}

// IsValidDefaultBranchName validates a value bound for
// repositories.default_branch: the IsValidBaseBranchRef allowlist (branch
// syntax, no trailing/double slash) plus a reject on git's symbolic
// pseudo-refs, which that allowlist alone admits. Every ingestion site for
// this field (repository create, update, and the FindOrCreateRepository
// backfill) must use this rather than IsValidBranchName or
// IsValidBaseBranchRef directly, since a default branch — unlike an
// ordinary base-branch override — is read back through a local-fallback
// rev-parse with no upstream ref to disambiguate it from a pseudo-ref.
func IsValidDefaultBranchName(branch string) bool {
	return IsValidBaseBranchRef(branch) && !IsGitSymbolicRef(branch)
}

// IsKnownSafeGitFlag returns true if the argument is a known safe git flag used by this codebase.
// This prevents argument injection where user input could introduce malicious flags.
// Only flags actually used by the Kandev codebase are whitelisted.
func IsKnownSafeGitFlag(arg string) bool {
	// Flags allowed only as an exact argument, because the prefix match below
	// would admit variants this codebase never issues: "--rebase" would let
	// through "--rebase-merges" and "--rebase=interactive" (which spawns an
	// editor), and "--abort" would let through "--abort-*".
	exactFlags := []string{
		"--", // Path separator - everything after this is treated as paths, not flags
		"--rebase",
		"--abort",
	}
	for _, safe := range exactFlags {
		if arg == safe {
			return true
		}
	}
	// Whitelist of git flags actually used by our codebase
	safeFlags := []string{
		"-m", "-M", "-n", "--set-upstream", "--all", "--porcelain", "--short",
		"--abbrev-ref", "--symbolic-full-name", "--verify", "--no-patch",
		"--format", "--format=", "--stat", "--shortstat", "--numstat", "-p", "-A",
		"--amend", "--allow-empty", "--soft", "--mixed", "--hard",
		"--cached", "--force", "--source=HEAD", "--staged", "--worktree",
		"--dry-run", "--get-all", "--first-parent", "--is-ancestor",
		"--refs",
		"--src-prefix=", "--dst-prefix=",
	}
	for _, safe := range safeFlags {
		if strings.HasPrefix(arg, safe) {
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
