package process

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// ambientGitLabEnvVars lists every environment variable this package reads in
// non-test code for GitLab host/token resolution. TestMain clears all of them
// before any test runs; TestMainScrubsAmbientGitLabEnvironment guards that the
// scrub actually ran.
var ambientGitLabEnvVars = []string{
	gitLabHostEnv,
	legacyGitLabHostEnv,
	gitLabTokenEnv,
}

// ambientEnvNotScrubbed lists environment variables this package legitimately
// reads in non-test code that TestMain must NOT clear.
var ambientEnvNotScrubbed = []string{
	// SHELL selects the interactive shell in shell_unix.go; shell_test.go sets
	// it deliberately to assert that selection, so clearing it here would
	// break that test rather than protect it.
	"SHELL",
	// COMSPEC is the Windows analogue of SHELL, read in shell_windows.go. Both
	// files are parsed by the guard below regardless of build tags (only one
	// compiles per platform), so both names must stay exempt.
	"COMSPEC",
}

// clearAmbientGitLabEnv removes the inherited GitLab host/token values so
// tests observe an unconfigured environment. Individual tests that need one
// of these variables still set it explicitly with t.Setenv.
func clearAmbientGitLabEnv() {
	for _, name := range ambientGitLabEnvVars {
		_ = os.Unsetenv(name)
	}
}

// TestAmbientEnvCoverageIncludesEveryPackageEnvRead fails when non-test code
// grows a new os.Getenv/os.LookupEnv call that is neither in
// ambientGitLabEnvVars nor ambientEnvNotScrubbed, so the TestMain scrub cannot
// silently fall behind the code it protects.
func TestAmbientEnvCoverageIncludesEveryPackageEnvRead(t *testing.T) {
	testutil.AssertEnvReadsCovered(t, ambientGitLabEnvVars, ambientEnvNotScrubbed)
}
