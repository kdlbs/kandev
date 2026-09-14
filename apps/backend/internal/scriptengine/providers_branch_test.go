package scriptengine

import (
	"maps"
	"testing"
)

// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.1
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.2
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.3
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.4
func TestRepositoryProvider_NormalizesOriginBranch(t *testing.T) {
	cases := []struct{ input, want string }{
		{"main", "main"},
		{"feature/login", "feature/login"},
		{"origin/main", "main"},
		{"origin/feature/login", "feature/login"},
		{"refs/remotes/origin/main", "main"},
		{"refs/remotes/origin/feature/login", "feature/login"},
		{"refs/heads/main", "main"},
		{"upstream/main", "upstream/main"},
		{"refs/heads/origin/topic", "origin/topic"},
		{"origin/origin/topic", "origin/topic"},
		{"", ""},
		{"origin/", ""},
		{"origin/x'$(printf injected)", "x'$(printf injected)"},
	}
	for _, tc := range cases {
		for _, key := range []string{"base_branch", "repository_branch"} {
			t.Run(key+"/"+tc.input, func(t *testing.T) {
				metadata := map[string]any{key: tc.input}
				before := maps.Clone(metadata)
				got := RepositoryProvider(metadata, nil, nil, nil)()["repository.branch"]
				if got != shellQuote(tc.want) {
					t.Fatalf("repository.branch = %q, want %q", got, shellQuote(tc.want))
				}
				if got := shellUnquote(t, got); got != tc.want {
					t.Fatalf("shell value = %q, want %q", got, tc.want)
				}
				if !maps.Equal(metadata, before) {
					t.Fatal("provider mutated metadata")
				}
			})
		}
	}
	t.Run("base branch takes precedence", func(t *testing.T) {
		metadata := map[string]any{"base_branch": "origin/main", "repository_branch": "other"}
		if got := RepositoryProvider(metadata, nil, nil, nil)()["repository.branch"]; got != "'main'" {
			t.Fatalf("repository.branch = %q", got)
		}
	})
	t.Run("empty base falls back", func(t *testing.T) {
		metadata := map[string]any{"base_branch": "", "repository_branch": "origin/topic"}
		if got := RepositoryProvider(metadata, nil, nil, nil)()["repository.branch"]; got != "'topic'" {
			t.Fatalf("repository.branch = %q", got)
		}
	})
}
