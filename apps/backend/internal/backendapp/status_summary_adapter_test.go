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
		{
			name: "repository only",
			pr:   &github.TaskPR{ID: "association-3", RepositoryID: "repo-a"},
			want: "repo-a",
		},
		{
			name: "number only",
			pr:   &github.TaskPR{ID: "association-4", PRNumber: 42},
			want: "#42",
		},
		{
			name: "association fallback",
			pr:   &github.TaskPR{ID: "association-5"},
			want: "association-5",
		},
		{
			name: "nil pull request",
			want: "",
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

func TestTaskStatusRuntimeProvidersTreatNilOrchestratorAsAbsent(t *testing.T) {
	activityProvider, countQueuedPrompts := taskStatusRuntimeProviders(nil)
	if activityProvider != nil {
		t.Fatalf("activity provider = %#v, want nil", activityProvider)
	}
	if countQueuedPrompts != nil {
		t.Fatal("queued prompt counter should be nil without an orchestrator")
	}
}
