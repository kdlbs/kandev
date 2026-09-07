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
	// KANDEV_DESKTOP_RUNTIME describes the launch policy inherited by the
	// agentctl child. Keep it intact so diagnostics reflect the real runtime.
	"KANDEV_DESKTOP_RUNTIME",
}

// clearAmbientGitLabEnv removes the inherited GitLab host/token values so
// tests observe an unconfigured environment. Individual tests that need one
// of these variables still set it explicitly with t.Setenv.
func clearAmbientGitLabEnv() {
	for _, name := range ambientGitLabEnvVars {
		if err := os.Unsetenv(name); err != nil {
			panic("process tests: unset " + name + ": " + err.Error())
		}
	}
}

// TestAmbientEnvCoverageIncludesEveryPackageEnvRead fails when non-test code
// grows a new environment read that is neither in ambientGitLabEnvVars nor
// ambientEnvNotScrubbed, so the TestMain scrub cannot silently fall behind the
// code it protects.
//
// environmentValue is declared as an extra reader because it, not os.Getenv,
// is how this package reads the environment almost everywhere: six of the
// eight GitLab host/token reads go through (*GitOperator).environmentValue,
// the other two being the package-level os.Getenv fallbacks in
// git_pr_providers.go. environmentValue falls back to
// os.Environ() when the operator was built with a nil provider (git.go:119).
// That is the exact path the reported failure came in on, so a guard watching
// only os.Getenv would have watched the wrong two call sites.
func TestAmbientEnvCoverageIncludesEveryPackageEnvRead(t *testing.T) {
	testutil.AssertEnvReadsCovered(t, ambientGitLabEnvVars, ambientEnvNotScrubbed, "environmentValue")
}
