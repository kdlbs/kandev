package skills

import (
	"testing"
)

// TestGitClone_MissingEndOfOptionsSeparator_PoC demonstrates the LOW-severity
// defense-in-depth gap in materializeGit: it invokes
//
//	runGit("", "clone", "--depth=1", skill.SourceLocator, repoDir)
//
// with NO `--` end-of-options separator before the locator (the sibling
// configloader/git.go correctly appends `"--", repoURL, wsPath`). Today this is
// unreachable because validateGitLocator requires an https/ssh/git scheme or a
// `git@host:` SCP prefix and rejects `..`, so a leading `-` can't get through.
// But if that validator were ever relaxed, a locator like `--upload-pack=...`
// would be handed to git as a FLAG rather than a positional URL.
//
// This test mirrors the exact argv materializeGit builds today and asserts the
// missing separator; the fix inserts `--` (see gitCloneArgs regression test).
func TestGitClone_MissingEndOfOptionsSeparator_PoC(t *testing.T) {
	// A hostile locator that only matters if validateGitLocator were relaxed.
	locator := "--upload-pack=touch /tmp/pwned;"
	repoDir := "/cache/git/deadbeef"

	// Exactly the args materializeGit passes to runGit today.
	args := []string{"clone", "--depth=1", locator, repoDir}

	sep := indexOf(args, "--")
	loc := indexOf(args, locator)

	if sep != -1 {
		t.Fatalf("PoC expected NO end-of-options separator in current argv, found -- at %d", sep)
	}
	if loc == -1 {
		t.Fatalf("PoC could not find locator in argv %v", args)
	}
	// The locator sits in option position (right after --depth=1) with nothing
	// telling git to stop parsing flags — so a `-`-prefixed locator is a flag.
	t.Logf("PoC: locator %q at index %d is parsed as a git FLAG (no -- guard)", locator, loc)
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}
