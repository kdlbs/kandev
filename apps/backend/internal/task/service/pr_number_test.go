package service

import "testing"

// TestResolvePRNumber covers every branch of the helper that surfaces a PR
// number for downstream worktree creation. Explicit PRNumber always wins;
// GitHubURL is parsed only when it carries a /pull/<N> path.
func TestResolvePRNumber(t *testing.T) {
	cases := []struct {
		name  string
		input TaskRepositoryInput
		want  int
	}{
		{
			name:  "explicit pr number wins",
			input: TaskRepositoryInput{PRNumber: 42, GitHubURL: "https://github.com/owner/repo/pull/99"},
			want:  42,
		},
		{
			name:  "parses pr number from pull URL",
			input: TaskRepositoryInput{GitHubURL: "https://github.com/kdlbs/kandev/pull/974"},
			want:  974,
		},
		{
			name:  "parses with trailing slash",
			input: TaskRepositoryInput{GitHubURL: "https://github.com/owner/repo/pull/123/"},
			want:  123,
		},
		{
			name:  "parses with query string",
			input: TaskRepositoryInput{GitHubURL: "https://github.com/owner/repo/pull/7?diff=unified"},
			want:  7,
		},
		{
			name:  "parses with fragment",
			input: TaskRepositoryInput{GitHubURL: "https://github.com/owner/repo/pull/8#discussion_r1"},
			want:  8,
		},
		{
			name:  "non-pr github URL returns zero",
			input: TaskRepositoryInput{GitHubURL: "https://github.com/owner/repo"},
			want:  0,
		},
		{
			name:  "empty input returns zero",
			input: TaskRepositoryInput{},
			want:  0,
		},
		{
			name:  "malformed pull segment returns zero",
			input: TaskRepositoryInput{GitHubURL: "https://github.com/owner/repo/pull/abc"},
			want:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolvePRNumber(tc.input); got != tc.want {
				t.Fatalf("resolvePRNumber(%+v) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}
