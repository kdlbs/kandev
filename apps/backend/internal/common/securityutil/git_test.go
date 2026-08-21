package securityutil

import "testing"

// TestIsKnownSafeGitFlagAllowsDiffPrefixFlags pins the output-formatting flags
// GetCumulativeDiff and ShowCommit pass to force stable a/ and b/ path
// prefixes regardless of a user's diff.noprefix config. They only affect diff
// header formatting (never repository mutation), so any value form is safe.
func TestIsKnownSafeGitFlagAllowsDiffPrefixFlags(t *testing.T) {
	for _, flag := range []string{"--src-prefix=a/", "--dst-prefix=b/", "--src-prefix=old/", "--dst-prefix=new/"} {
		if !IsKnownSafeGitFlag(flag) {
			t.Fatalf("IsKnownSafeGitFlag(%q) = false, want true — the cumulative-diff parser needs it", flag)
		}
	}
}

func TestIsValidBaseBranchRef(t *testing.T) {
	cases := map[string]bool{
		"main":              true,
		"release/v2":        true,
		"feature/foo-bar":   true,
		"origin/main":       true,
		"origin/release/v2": true,
		"":                  false,
		"main/":             false, // trailing slash
		"origin/main/":      false, // trailing slash after origin strip
		"a//b":              false, // consecutive slashes
		"bad..ref":          false, // path traversal
		"feature.lock":      false, // .lock suffix
		"bad branch":        false, // space
		"bad;rm -rf":        false, // shell metacharacters
		"-flag":             false, // leading dash (regex requires alphanumeric first)
		"/leading":          false, // leading slash
	}
	for ref, want := range cases {
		if got := IsValidBaseBranchRef(ref); got != want {
			t.Errorf("IsValidBaseBranchRef(%q) = %v, want %v", ref, got, want)
		}
	}
}

func TestIsValidDefaultBranchName(t *testing.T) {
	cases := map[string]bool{
		"main":            true,
		"release/v2":      true,
		"feature/foo-bar": true,
		"":                false,
		"main/":           false, // trailing slash, via IsValidBaseBranchRef
		"a//b":            false, // consecutive slashes, via IsValidBaseBranchRef
		"HEAD":            false, // git symbolic ref
		"ORIG_HEAD":       false, // git symbolic ref
		"FETCH_HEAD":      false, // git symbolic ref
		"MERGE_HEAD":      false, // git symbolic ref
		"head":            true,  // lowercase is a real, distinct ref name
	}
	for branch, want := range cases {
		if got := IsValidDefaultBranchName(branch); got != want {
			t.Errorf("IsValidDefaultBranchName(%q) = %v, want %v", branch, got, want)
		}
	}
}

func TestIsGitSymbolicRef(t *testing.T) {
	for _, ref := range []string{"HEAD", "ORIG_HEAD", "FETCH_HEAD", "MERGE_HEAD"} {
		if !IsGitSymbolicRef(ref) {
			t.Errorf("IsGitSymbolicRef(%q) = false, want true", ref)
		}
	}
	for _, ref := range []string{"main", "head", "MERGE_HEAD_2"} {
		if IsGitSymbolicRef(ref) {
			t.Errorf("IsGitSymbolicRef(%q) = true, want false", ref)
		}
	}
}

func TestIsKnownSafeGitFlagAllowsRequiredGitOperationFlags(t *testing.T) {
	for _, flag := range []string{"--dry-run", "--first-parent", "--is-ancestor"} {
		if !IsKnownSafeGitFlag(flag) {
			t.Fatalf("%s must be allowed for the git operations that use it", flag)
		}
	}
}

func TestIsKnownSafeGitFlagRejectsPushOptionPrefixes(t *testing.T) {
	for _, flag := range []string{"--push", "--push-option=notify"} {
		if IsKnownSafeGitFlag(flag) {
			t.Fatalf("IsKnownSafeGitFlag(%q) = true, want false", flag)
		}
	}
}

// TestIsKnownSafeGitFlagAllowsRebaseAndAbort pins the two flags the git-pull
// rebase path needs. GitOperator.Pull builds "pull --rebase <remote> <branch>"
// and, when the rebase conflicts, recovers with "rebase --abort"; both were
// rejected here, so POST /api/v1/git/pull {"rebase":true} could never succeed
// and POST /api/v1/git/abort never worked at all.
func TestIsKnownSafeGitFlagAllowsRebaseAndAbort(t *testing.T) {
	for _, flag := range []string{"--rebase", "--abort"} {
		if !IsKnownSafeGitFlag(flag) {
			t.Fatalf("IsKnownSafeGitFlag(%q) = false, want true — the git pull --rebase path needs it", flag)
		}
	}
}

// TestIsKnownSafeGitFlagRejectsRebaseAndAbortVariants pins that --rebase and
// --abort are allowed as exact arguments only. The bulk of the allowlist
// matches by prefix, which for these two would also admit "--rebase-merges",
// "--rebase=interactive" (spawns an editor), and "--abort-on-*", none of which
// this codebase issues.
func TestIsKnownSafeGitFlagRejectsRebaseAndAbortVariants(t *testing.T) {
	for _, flag := range []string{
		"--rebase-merges",
		"--rebase=interactive",
		"--rebase=i",
		"--abort-on-error",
		"--aborted",
	} {
		if IsKnownSafeGitFlag(flag) {
			t.Fatalf("IsKnownSafeGitFlag(%q) = true, want false — only the exact flags are allowed", flag)
		}
	}
}
