package skills

import (
	"slices"
	"testing"
)

// TestGitClone_MissingEndOfOptionsSeparator_PoC demonstrates the LOW-severity
// defense-in-depth gap that materializeGit used to have: it invoked
//
//	runGit("", "clone", "--depth=1", skill.SourceLocator, repoDir)
//
// with NO `--` end-of-options separator before the locator (the sibling
// configloader/git.go correctly appends `"--", repoURL, wsPath`). It was
// unreachable because validateGitLocator requires an https/ssh/git scheme or a
// `git@host:` SCP prefix and rejects `..`, so a leading `-` couldn't get
// through. But if that validator were ever relaxed, a locator like
// `--upload-pack=...` would be handed to git as a FLAG rather than a positional
// URL.
//
// This test contrasts the old argv (no separator) with the fixed builder
// (gitCloneArgs, which inserts `--`) so it genuinely FLIPS: the unsafe shape
// lacks `--`, the fixed shape has it before the locator.
func TestGitClone_MissingEndOfOptionsSeparator_PoC(t *testing.T) {
	// A hostile locator that only matters if validateGitLocator were relaxed.
	locator := "--upload-pack=touch /tmp/pwned;"
	repoDir := "/cache/git/deadbeef"

	// The argv materializeGit used to pass to runGit — no `--` guard.
	unsafe := []string{"clone", "--depth=1", locator, repoDir}
	if slices.Index(unsafe, "--") != -1 {
		t.Fatalf("PoC expected the pre-fix argv to omit the -- separator, got %v", unsafe)
	}
	// In the unsafe argv the locator sits in option position (right after
	// --depth=1) with nothing telling git to stop parsing flags.
	if got := slices.Index(unsafe, locator); got != 2 {
		t.Fatalf("PoC expected locator in flag position (index 2), got %d in %v", got, unsafe)
	}

	// The fixed builder places `--` immediately before the locator.
	fixed := gitCloneArgs(locator, repoDir)
	sep := slices.Index(fixed, "--")
	if sep == -1 || fixed[sep+1] != locator {
		t.Fatalf("PoC expected fixed argv to guard the locator with --, got %v", fixed)
	}
	t.Logf("PoC: pre-fix argv %v parses %q as a git flag; fixed argv %v does not", unsafe, locator, fixed)
}
