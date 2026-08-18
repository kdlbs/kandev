package environment

import "testing"

// TestIsRepositoryOrigin pins the normative repository-origin rule and its
// worked boundary table from the spec: the first whitespace-delimited field
// must be exactly "repository".
func TestIsRepositoryOrigin(t *testing.T) {
	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "bare repository (blank name case)", origin: "repository", want: true},
		{name: "repository with name", origin: "repository app", want: true},
		{name: "repository with two spaces", origin: "repository  app", want: true},
		{name: "repository with tab", origin: "repository\tapp", want: true},
		{name: "no separator", origin: "repositoryapp", want: false},
		{name: "empty string", origin: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsRepositoryOrigin(tc.origin); got != tc.want {
				t.Fatalf("IsRepositoryOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

// TestRepositoryOrigin pins the shared repository-origin construction used by
// both launch assembly paths, including its blank-name fallback.
func TestRepositoryOrigin(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "named repository", input: "app", want: "repository app"},
		{name: "trims repository name", input: "  app  ", want: "repository app"},
		{name: "empty repository name", input: "", want: "repository"},
		{name: "whitespace-only repository name", input: " \t ", want: "repository"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RepositoryOrigin(tc.input); got != tc.want {
				t.Fatalf("RepositoryOrigin(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestTierForOrigin_KnownOrigins pins tier membership for every named origin.
func TestTierForOrigin_KnownOrigins(t *testing.T) {
	cases := []struct {
		origin string
		want   Tier
	}{
		{OriginManagedRuntime, TierAuthoritative},
		{OriginManagedCredentials, TierAuthoritative},
		{OriginExecutorProfile, TierAuthoritative},
		{"repository", TierAuthoritative},
		{"repository app", TierAuthoritative},
		{OriginAgentProfile, TierProfileDefault},
		{OriginManagedAgentDefaults, TierAgentRuntimeDefault},
	}
	for _, tc := range cases {
		t.Run(tc.origin, func(t *testing.T) {
			if got := TierForOrigin(tc.origin); got != tc.want {
				t.Fatalf("TierForOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

// TestTierForOrigin_UnrecognisedIsAuthoritative pins AC-35: an origin string
// the tier table does not name, and which is not a repository origin,
// classifies as TierAuthoritative — fail closed rather than a silent loser.
func TestTierForOrigin_UnrecognisedIsAuthoritative(t *testing.T) {
	for _, origin := range []string{"", "repositoryapp", "some-future-origin"} {
		if got := TierForOrigin(origin); got != TierAuthoritative {
			t.Fatalf("TierForOrigin(%q) = %v, want TierAuthoritative", origin, got)
		}
	}
}

func TestNormalizeOriginLabel(t *testing.T) {
	cases := []struct {
		origin string
		want   string
	}{
		{"repository app", "repository"},
		{"repository web", "repository"},
		{"repository", "repository"},
		{OriginManagedRuntime, OriginManagedRuntime},
		{OriginAgentProfile, OriginAgentProfile},
		{"some-future-origin", "some-future-origin"},
	}
	for _, tc := range cases {
		t.Run(tc.origin, func(t *testing.T) {
			if got := NormalizeOriginLabel(tc.origin); got != tc.want {
				t.Fatalf("NormalizeOriginLabel(%q) = %q, want %q", tc.origin, got, tc.want)
			}
		})
	}
}

// TestJoinOriginLabels_NilAndEmpty pins F34: an unreachable-today but
// declared boundary — nil/empty input returns "" rather than panicking or
// erroring.
func TestJoinOriginLabels_NilAndEmpty(t *testing.T) {
	if got := JoinOriginLabels(nil); got != "" {
		t.Fatalf("JoinOriginLabels(nil) = %q, want \"\"", got)
	}
	if got := JoinOriginLabels([]string{}); got != "" {
		t.Fatalf("JoinOriginLabels([]string{}) = %q, want \"\"", got)
	}
}

func TestJoinOriginLabels_SortsAndJoins(t *testing.T) {
	got := JoinOriginLabels([]string{OriginManagedRuntime, OriginExecutorProfile})
	want := "executor profile+managed runtime"
	if got != want {
		t.Fatalf("JoinOriginLabels = %q, want %q", got, want)
	}
}

// TestJoinOriginLabels_DedupesAfterNormalization covers AC-25's join
// de-duplication: two distinct repository origins both normalise to
// "repository" and must collapse to one label rather than "repository+repository".
func TestJoinOriginLabels_DedupesAfterNormalization(t *testing.T) {
	got := JoinOriginLabels([]string{"repository app", "repository web"})
	if got != "repository" {
		t.Fatalf("JoinOriginLabels = %q, want %q", got, "repository")
	}
}
