package github

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// ambientGitHubEnvVars lists every environment variable this package reads in
// non-test code. TestMain clears all of them before any test runs.
//
// Developer and CI shells commonly export GH_TOKEN/GITHUB_TOKEN so the gh CLI
// and other tooling work. Left in place they silently change auth-method
// resolution under test: legacyEnvironmentToken picks up the ambient token
// and newLegacyCredential returns a PAT client instead of the "no auth
// configured" NoopClient, which fails TestNewClient_NoAuth_ReturnsNoop and
// TestClearToken. KANDEV_MOCK_GITHUB is included for the same reason gitlab's
// mock switch is: an ambient "true" would silently swap in the mock client.
//
// Clearing once here rather than per test also sidesteps t.Setenv, which
// cannot be called from a test that uses t.Parallel().
var ambientGitHubEnvVars = []string{
	"GH_TOKEN",
	"GITHUB_TOKEN",
	"KANDEV_MOCK_GITHUB",
}

// clearAmbientGitHubEnv removes the inherited values so tests observe an
// unconfigured environment. Individual tests that need one of these variables
// still set it explicitly with t.Setenv.
func clearAmbientGitHubEnv() {
	for _, name := range ambientGitHubEnvVars {
		if err := os.Unsetenv(name); err != nil {
			panic("github tests: unset " + name + ": " + err.Error())
		}
	}
}

func TestAmbientGitHubEnvIsClearedForTests(t *testing.T) {
	// Report only the name: one of these variables holds a real token on a
	// developer machine and test output ends up in CI logs.
	for _, name := range ambientGitHubEnvVars {
		if _, ok := os.LookupEnv(name); ok {
			t.Errorf("%s is set during tests; TestMain must clear it", name)
		}
	}
}

// TestAmbientGitHubEnvCoversEveryPackageEnvRead fails when non-test code grows
// a new os.Getenv/os.LookupEnv call that ambientGitHubEnvVars does not cover,
// so the scrub cannot silently fall behind the code it protects.
func TestAmbientGitHubEnvCoversEveryPackageEnvRead(t *testing.T) {
	testutil.AssertEnvReadsCovered(t, ambientGitHubEnvVars, nil)
}
