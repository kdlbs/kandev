package lifecycle

import "testing"

// A Windows prepare script receives the same upstream branch name as a POSIX
// one, so an origin-qualified base branch clones the upstream branch.
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.1
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.2
// @covers AC-EXECUTORS-REPOSITORY-BRANCH-001.4
func TestSSHWindowsRepositoryProviderNormalizesOriginBranch(t *testing.T) {
	cases := []struct{ input, want string }{
		{"main", "main"},
		{"feature/login", "feature/login"},
		{"origin/main", "main"},
		{"origin/feature/login", "feature/login"},
		{"refs/remotes/origin/main", "main"},
		{"refs/heads/main", "main"},
		{"upstream/main", "upstream/main"},
		{"refs/heads/origin/topic", "origin/topic"},
		{"origin/origin/topic", "origin/topic"},
		{"origin/O’Brien", "O’Brien"},
	}
	for _, tc := range cases {
		for _, key := range []string{MetadataKeyBaseBranch, "repository_branch"} {
			t.Run(key+"/"+tc.input, func(t *testing.T) {
				got := windowsRepositoryProvider(map[string]interface{}{key: tc.input})()["repository.branch"]
				if want := powerShellQuote(tc.want); got != want {
					t.Fatalf("repository.branch = %q, want %q", got, want)
				}
			})
		}
	}
}
