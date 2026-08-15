package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/github"
)

func TestTaskStatusSummaryPRKeyMatchesLiveEventIdentity(t *testing.T) {
	tests := []struct {
		name string
		pr   *github.TaskPR
		want string
	}{
		{
			name: "repository and number",
			pr:   &github.TaskPR{ID: "association-1", RepositoryID: "repo-a", PRNumber: 42, PRURL: "https://example.test/42"},
			want: "repo-a#42",
		},
		{
			name: "legacy URL",
			pr:   &github.TaskPR{ID: "association-2", PRNumber: 42, PRURL: "https://example.test/42"},
			want: "https://example.test/42",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := taskStatusSummaryPRKey(test.pr); got != test.want {
				t.Fatalf("taskStatusSummaryPRKey() = %q, want %q", got, test.want)
			}
		})
	}
}
