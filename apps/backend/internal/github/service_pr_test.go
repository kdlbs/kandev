package github

import (
	"fmt"
	"testing"
)

func TestCompletePRCommitHistoryRequiresACompleteProviderSnapshot(t *testing.T) {
	complete := make([]PRCommitInfo, githubPRCommitHistoryLimit-1)
	for index := range complete {
		complete[index] = PRCommitInfo{SHA: fmt.Sprintf("commit-%d", index)}
	}
	complete[len(complete)-1] = PRCommitInfo{SHA: "provider-head"}

	truncated := make([]PRCommitInfo, githubPRCommitHistoryLimit)
	for index := range truncated {
		truncated[index] = PRCommitInfo{SHA: fmt.Sprintf("commit-%d", index)}
	}
	truncated[len(truncated)-1] = PRCommitInfo{SHA: "provider-head"}

	tests := []struct {
		name    string
		head    string
		commits []PRCommitInfo
		want    bool
	}{
		{name: "head terminates a paginated history", head: "provider-head", commits: complete, want: true},
		{name: "head is missing from the returned history", head: "provider-head", commits: []PRCommitInfo{{SHA: "older"}}, want: false},
		{name: "head does not terminate the returned history", head: "provider-head", commits: []PRCommitInfo{{SHA: "provider-head"}, {SHA: "newer"}}, want: false},
		{name: "empty history is incomplete", head: "provider-head", commits: nil, want: false},
		{name: "provider head is empty", head: "", commits: complete, want: false},
		{name: "provider endpoint cap is incomplete", head: "provider-head", commits: truncated, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := completePRCommitHistory(test.head, test.commits); got != test.want {
				t.Fatalf("completePRCommitHistory() = %v, want %v", got, test.want)
			}
		})
	}
}
