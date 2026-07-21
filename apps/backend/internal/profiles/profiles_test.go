package profiles

import (
	"os"
	"strings"
	"testing"
)

// TestApplyProfile_DefaultsToProd pins the production-safety
// invariant: with no profile-selector env var set, ApplyProfile picks
// prod and Office stays off. A regression that flips the default would
// surface here as a failing test, not as an unintended release.
func TestApplyProfile_DefaultsToProd(t *testing.T) {
	clearProfileSelectors(t)
	clearProfilesYAMLVars(t)

	count, env, err := ApplyProfile()
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if env != EnvProd {
		t.Errorf("env = %q, want %q (no selector env vars set)", env, EnvProd)
	}
	// Prod writes the two experimental flags and the default-on status bar.
	if count != 3 {
		t.Errorf("ApplyProfile wrote %d vars in prod; want 3", count)
	}
	if v := os.Getenv("KANDEV_FEATURES_OFFICE"); v != "false" {
		t.Errorf("KANDEV_FEATURES_OFFICE = %q after prod ApplyProfile; want %q", v, "false")
	}
	if v := os.Getenv("KANDEV_FEATURES_PLUGINS"); v != "false" {
		t.Errorf("KANDEV_FEATURES_PLUGINS = %q after prod ApplyProfile; want %q", v, "false")
	}
	if v := os.Getenv("KANDEV_FEATURES_APP_STATUS_BAR"); v != "true" {
		t.Errorf("KANDEV_FEATURES_APP_STATUS_BAR = %q after prod ApplyProfile; want %q", v, "true")
	}
}

// TestApplyProfile_DevTurnsOnFeatures verifies that selecting the dev
// profile flips the in-progress features on. This is what `make dev`
// relies on once apps/cli/src/dev.ts stops setting these by hand.
func TestApplyProfile_DevTurnsOnFeatures(t *testing.T) {
	clearProfileSelectors(t)
	clearProfilesYAMLVars(t)
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "true")

	_, env, err := ApplyProfile()
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if env != EnvDev {
		t.Fatalf("env = %q, want %q", env, EnvDev)
	}
	if v := os.Getenv("KANDEV_FEATURES_OFFICE"); v != "true" {
		t.Errorf("KANDEV_FEATURES_OFFICE = %q in dev; want %q", v, "true")
	}
	if v := os.Getenv("KANDEV_FEATURES_PLUGINS"); v != "true" {
		t.Errorf("KANDEV_FEATURES_PLUGINS = %q in dev; want %q", v, "true")
	}
	if v := os.Getenv("KANDEV_FEATURES_APP_STATUS_BAR"); v != "true" {
		t.Errorf("KANDEV_FEATURES_APP_STATUS_BAR = %q in dev; want %q", v, "true")
	}
}

// TestApplyProfile_E2EWinsOverDev checks the documented detection
// precedence: when both selectors are set (e2e harness on a developer
// laptop), e2e takes priority because e2e fixtures are stricter (mock
// agent only, test harness routes, auto-approve permissions).
func TestApplyProfile_E2EWinsOverDev(t *testing.T) {
	clearProfileSelectors(t)
	clearProfilesYAMLVars(t)
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "true")
	t.Setenv("KANDEV_E2E_MOCK", "true")

	_, env, err := ApplyProfile()
	if err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if env != EnvE2E {
		t.Fatalf("env = %q with both selectors set; want %q", env, EnvE2E)
	}
	if v := os.Getenv("KANDEV_MOCK_AGENT"); v != "only" {
		t.Errorf("KANDEV_MOCK_AGENT = %q in e2e; want %q (e2e isolates to mock-agent)", v, "only")
	}
	if v := os.Getenv("AGENTCTL_AUTO_APPROVE_PERMISSIONS"); v != "true" {
		t.Errorf("AGENTCTL_AUTO_APPROVE_PERMISSIONS = %q in e2e; want %q", v, "true")
	}
}

// TestApplyProfile_ExistingEnvWins is the precedence guarantee that
// makes per-spec overrides work: if a launcher or spec already set a
// var, ApplyProfile must not overwrite it. Without this, an e2e spec
// like permission-approval.spec.ts couldn't set
// AGENTCTL_AUTO_APPROVE_PERMISSIONS=false to exercise the real flow.
func TestApplyProfile_ExistingEnvWins(t *testing.T) {
	clearProfileSelectors(t)
	clearProfilesYAMLVars(t)
	t.Setenv("KANDEV_E2E_MOCK", "true")
	// Spec opts out of auto-approve before the backend starts.
	t.Setenv("AGENTCTL_AUTO_APPROVE_PERMISSIONS", "false")

	if _, _, err := ApplyProfile(); err != nil {
		t.Fatalf("ApplyProfile: %v", err)
	}
	if v := os.Getenv("AGENTCTL_AUTO_APPROVE_PERMISSIONS"); v != "false" {
		t.Errorf("ApplyProfile clobbered an already-set var: AGENTCTL_AUTO_APPROVE_PERMISSIONS = %q; want %q", v, "false")
	}
}

// TestApplyProfile_Idempotent calls ApplyProfile twice and verifies
// the second call writes nothing. Documents that startup retries (or
// surprising double-init paths) are safe.
func TestApplyProfile_Idempotent(t *testing.T) {
	clearProfileSelectors(t)
	clearProfilesYAMLVars(t)
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "true")

	if _, _, err := ApplyProfile(); err != nil {
		t.Fatalf("first ApplyProfile: %v", err)
	}
	count, _, err := ApplyProfile()
	if err != nil {
		t.Fatalf("second ApplyProfile: %v", err)
	}
	if count != 0 {
		t.Errorf("second ApplyProfile wrote %d vars; want 0 (idempotency broken)", count)
	}
}

// TestFeatureFlagDefaults_LowercasesShortName verifies the mapping
// from env-var-style keys (KANDEV_FEATURES_OFFICE) to Viper-style
// short names (office). The config package depends on this exact
// transform for SetDefault to land on the right key.
func TestFeatureFlagDefaults_LowercasesShortName(t *testing.T) {
	clearProfileSelectors(t)
	clearProfilesYAMLVars(t)
	defaults, err := FeatureFlagDefaults()
	if err != nil {
		t.Fatalf("FeatureFlagDefaults: %v", err)
	}
	if _, ok := defaults["office"]; !ok {
		t.Errorf("FeatureFlagDefaults missing %q key; got %#v", "office", defaults)
	}
	if _, ok := defaults["plugins"]; !ok {
		t.Errorf("FeatureFlagDefaults missing %q key; got %#v", "plugins", defaults)
	}
	if _, ok := defaults["app_status_bar"]; !ok {
		t.Errorf("FeatureFlagDefaults missing %q key; got %#v", "app_status_bar", defaults)
	}
}

func TestMarkApplied_OnlyAllowsKnownDerivedEnvVars(t *testing.T) {
	clearAppliedEnvVars(t)

	MarkApplied("KANDEV_DEBUG_AGENT_MESSAGES")
	if !WasApplied("KANDEV_DEBUG_AGENT_MESSAGES") {
		t.Fatal("KANDEV_DEBUG_AGENT_MESSAGES was not marked applied")
	}
	MarkApplied("KANDEV_DEBUG_PPROF_ENABLED")
	if !WasApplied("KANDEV_DEBUG_PPROF_ENABLED") {
		t.Fatal("KANDEV_DEBUG_PPROF_ENABLED was not marked applied")
	}

	MarkApplied("KANDEV_UNRELATED")
	if WasApplied("KANDEV_UNRELATED") {
		t.Fatal("KANDEV_UNRELATED was marked applied")
	}
}

// TestProfilesYAML_ContainsRequiredSections is a smoke test that
// catches a zero-byte or truncated embed — an embed that silently
// picks up an empty file would make every var default to empty,
// which would look like "prod" semantics even in e2e and confuse the
// hell out of whoever debugs it.
func TestProfilesYAML_ContainsRequiredSections(t *testing.T) {
	yaml := string(ProfilesYAML())
	for _, section := range []string{"features:", "mocks:", "debug:", "KANDEV_FEATURES_OFFICE:", "KANDEV_FEATURES_PLUGINS:", "KANDEV_FEATURES_APP_STATUS_BAR:"} {
		if !strings.Contains(yaml, section) {
			t.Errorf("embedded profiles.yaml missing section/key %q; embed broken or file truncated", section)
		}
	}
}

// clearProfileSelectors unsets the env vars DetectEnvironment
// inspects, so a test runs against a clean slate regardless of the
// host shell's settings.
func clearProfileSelectors(t *testing.T) {
	t.Helper()
	for _, n := range []string{"KANDEV_E2E_MOCK", "KANDEV_DEBUG_DEV_MODE", "KANDEV_DEBUG_PPROF_ENABLED"} {
		_ = os.Unsetenv(n)
		t.Cleanup(func() { _ = os.Unsetenv(n) })
	}
}

// clearProfilesYAMLVars unsets every env var ApplyProfile might
// write, so each test starts from a clean baseline. Discovered from
// the YAML itself so it can't drift.
func clearProfilesYAMLVars(t *testing.T) {
	t.Helper()
	names, err := SortedEnvVars()
	if err != nil {
		t.Fatalf("SortedEnvVars: %v", err)
	}
	for _, n := range names {
		name := n
		_ = os.Unsetenv(name)
		t.Cleanup(func() { _ = os.Unsetenv(name) })
	}
}

func clearAppliedEnvVars(t *testing.T) {
	t.Helper()
	appliedEnvVars.Lock()
	previous := appliedEnvVars.names
	appliedEnvVars.names = map[string]bool{}
	appliedEnvVars.Unlock()
	t.Cleanup(func() {
		appliedEnvVars.Lock()
		appliedEnvVars.names = previous
		appliedEnvVars.Unlock()
	})
}
