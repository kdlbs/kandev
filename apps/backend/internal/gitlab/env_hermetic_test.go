package gitlab

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// envMockGitLab mirrors the mock switch that factory.go and service_config.go
// read as a bare literal. TestAmbientGitLabEnvCoversEveryPackageEnvRead
// resolves literals as well as constants, so the two cannot drift apart
// unnoticed.
const envMockGitLab = "KANDEV_MOCK_GITLAB"

// ambientGitLabEnvVars lists every environment variable this package reads in
// non-test code. TestMain clears all of them before any test runs.
//
// Developer shells commonly set these for real: GITLAB_HOST is the standard
// variable for the glab CLI, and GITLAB_TOKEN is its companion PAT. Left in
// place they silently change host and credential resolution under test — a set
// GITLAB_HOST makes trustedEnvironmentTokenHost ignore the startup host, which
// fails TestEnvironmentTokenAllowsImmutableStartupHost with a trusted-origin
// rejection, and KANDEV_MOCK_GITLAB=true swaps the real client for the mock in
// a dozen controller tests.
//
// Clearing once here rather than per test also sidesteps t.Setenv, which
// cannot be called from a test that uses t.Parallel().
var ambientGitLabEnvVars = []string{
	envKandevGitLabHost,
	envGitLabHost,
	envMockGitLab,
	secretNameToken,
}

// clearAmbientGitLabEnv removes the inherited values so tests observe an
// unconfigured environment. Individual tests that need one of these variables
// still set it explicitly with t.Setenv.
func clearAmbientGitLabEnv() {
	for _, name := range ambientGitLabEnvVars {
		if err := os.Unsetenv(name); err != nil {
			panic("gitlab tests: unset " + name + ": " + err.Error())
		}
	}
}

func TestAmbientGitLabEnvIsClearedForTests(t *testing.T) {
	// Report only the name: one of these variables holds a real PAT on a
	// developer machine and test output ends up in CI logs.
	for _, name := range ambientGitLabEnvVars {
		if _, ok := os.LookupEnv(name); ok {
			t.Errorf("%s is set during tests; TestMain must clear it", name)
		}
	}
}

// TestTrustedEnvironmentTokenHostFallsBackToStartupHost pins the resolution
// TestEnvironmentTokenAllowsImmutableStartupHost depends on: with no host
// variable in the environment, the immutable startup host is the trusted
// origin. Production precedence is unchanged — an explicitly set variable
// still wins — so this fails only when the ambient environment leaks back in.
func TestTrustedEnvironmentTokenHostFallsBackToStartupHost(t *testing.T) {
	const startupHost = "https://gitlab.internal"
	if got := trustedEnvironmentTokenHost(startupHost); got != startupHost {
		t.Fatalf("trustedEnvironmentTokenHost(%q) = %q, want the startup host", startupHost, got)
	}
}

// TestClearAmbientGitLabEnvNeutralizesInheritedHosts reproduces the original
// failure in-process: a shell exporting GITLAB_HOST (or KANDEV_GITLAB_HOST)
// hijacks the trusted origin, and the scrub takes it back. Production
// precedence is deliberately left intact — an explicitly configured variable
// still outranks the startup host — so the guard asserts the scrub, not the
// precedence.
func TestClearAmbientGitLabEnvNeutralizesInheritedHosts(t *testing.T) {
	const startupHost = "https://gitlab.internal"
	t.Setenv(envGitLabHost, "https://attacker.invalid")
	t.Setenv(envKandevGitLabHost, "https://evil.invalid")
	if got := trustedEnvironmentTokenHost(startupHost); got == startupHost {
		t.Fatal("inherited host variables no longer affect the trusted origin; this guard is obsolete")
	}

	clearAmbientGitLabEnv()

	if got := trustedEnvironmentTokenHost(startupHost); got != startupHost {
		t.Fatalf("after scrub trustedEnvironmentTokenHost(%q) = %q, want the startup host", startupHost, got)
	}
}

// TestAmbientGitLabEnvCoversEveryPackageEnvRead fails when non-test code grows
// a new os.Getenv/os.LookupEnv call that ambientGitLabEnvVars does not cover,
// so the scrub cannot silently fall behind the code it protects.
func TestAmbientGitLabEnvCoversEveryPackageEnvRead(t *testing.T) {
	testutil.AssertEnvReadsCovered(t, ambientGitLabEnvVars, nil)
}
