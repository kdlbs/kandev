package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/github"
)

func TestInterpolateReviewPrompt(t *testing.T) {
	pr := &github.PR{
		Number:      42,
		Title:       "Add feature X",
		HTMLURL:     "https://github.com/myorg/myrepo/pull/42",
		AuthorLogin: "alice",
		RepoOwner:   "myorg",
		RepoName:    "myrepo",
		HeadBranch:  "feature-x",
		BaseBranch:  "main",
	}

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{
			"empty template uses default",
			"",
			"Pull Request ready for review: https://github.com/myorg/myrepo/pull/42",
		},
		{
			"all placeholders",
			"Review {{pr.link}} (#{{pr.number}}) by {{pr.author}} in {{pr.repo}} on {{pr.branch}} -> {{pr.base_branch}}: {{pr.title}}",
			"Review https://github.com/myorg/myrepo/pull/42 (#42) by alice in myorg/myrepo on feature-x -> main: Add feature X",
		},
		{
			"no placeholders",
			"Please review this PR",
			"Please review this PR",
		},
		{
			"partial placeholders",
			"Check {{pr.link}}",
			"Check https://github.com/myorg/myrepo/pull/42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interpolateReviewPrompt(tt.template, pr)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
