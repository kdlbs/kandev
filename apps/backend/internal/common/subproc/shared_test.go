package subproc

import "testing"

func TestResolveCap(t *testing.T) {
	const env = "KANDEV_SUBPROC_CAP_TEST"
	const def = 7

	cases := []struct {
		name string
		val  string
		want int
	}{
		{"empty", "", def},
		{"valid", "3", 3},
		{"zero falls back", "0", def},
		{"negative falls back", "-1", def},
		{"garbage falls back", "abc", def},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(env, tc.val)
			if got := resolveCap(env, def); got != tc.want {
				t.Errorf("resolveCap(%q, %d) = %d, want %d", tc.val, def, got, tc.want)
			}
		})
	}
}

// TestResolveGHMaxConcurrent and TestResolveGitMaxConcurrent guard the
// process-wide defaults: a typo'd env or a fall-through must land on
// the safe default rather than disable the throttle.
func TestResolveGHMaxConcurrent(t *testing.T) {
	t.Setenv(ghMaxConcurrentEnv, "")
	if got := resolveGHMaxConcurrent(); got != defaultGHMaxConcurrent {
		t.Errorf("resolveGHMaxConcurrent() = %d, want %d", got, defaultGHMaxConcurrent)
	}
	t.Setenv(ghMaxConcurrentEnv, "garbage")
	if got := resolveGHMaxConcurrent(); got != defaultGHMaxConcurrent {
		t.Errorf("garbage env: got %d, want %d", got, defaultGHMaxConcurrent)
	}
	t.Setenv(ghMaxConcurrentEnv, "5")
	if got := resolveGHMaxConcurrent(); got != 5 {
		t.Errorf("valid env: got %d, want 5", got)
	}
}

func TestResolveGitMaxConcurrent(t *testing.T) {
	t.Setenv(gitMaxConcurrentEnv, "")
	if got := resolveGitMaxConcurrent(); got != defaultGitMaxConcurrent {
		t.Errorf("resolveGitMaxConcurrent() = %d, want %d", got, defaultGitMaxConcurrent)
	}
	t.Setenv(gitMaxConcurrentEnv, "0")
	if got := resolveGitMaxConcurrent(); got != defaultGitMaxConcurrent {
		t.Errorf("zero env: got %d, want %d", got, defaultGitMaxConcurrent)
	}
	t.Setenv(gitMaxConcurrentEnv, "20")
	if got := resolveGitMaxConcurrent(); got != 20 {
		t.Errorf("valid env: got %d, want 20", got)
	}
}

// TestGHGitAreDistinctThrottles verifies the two singletons aren't
// accidentally aliased. A regression where both helpers return the same
// pool would let a gh storm starve git ops (or vice-versa) — exactly
// the cross-contention the per-binary split is meant to prevent.
func TestGHGitAreDistinctThrottles(t *testing.T) {
	if GH() == Git() {
		t.Fatal("GH() and Git() returned the same throttle instance")
	}
}
