package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func TestGitHubApprovalsWithoutReviews(t *testing.T) {
	for state, want := range map[string]mergestate.Tri{
		"clean": mergestate.Met, "has_hooks": mergestate.Met, "behind": mergestate.Met,
		"blocked": mergestate.Unmet, "": mergestate.Unavailable, "unknown": mergestate.Unavailable,
	} {
		if got := githubApprovalsOf("", state); got != want {
			t.Errorf("no reviews, state %q: got %v want %v", state, got, want)
		}
	}
}

func TestGitHubBlockedWaitsOnItsExplainedCondition(t *testing.T) {
	got := githubMergeableOf("blocked", mergestate.Unmet, 0)
	if got.Mergeable != mergestate.Unmet || got.Explains != mergestate.CondApprovals {
		t.Fatalf("blocked with unmet approvals: %+v", got)
	}
	got = githubMergeableOf("blocked", mergestate.Met, 2)
	if got.Explains != mergestate.CondThreads {
		t.Fatalf("blocked with threads: %+v", got)
	}
	if got = githubMergeableOf("blocked", mergestate.Met, 0); got.Explains != "" || got.Mergeable != mergestate.Unmet {
		t.Fatalf("unexplained blocked must stay a refusal: %+v", got)
	}
	if got = githubMergeableOf("behind", mergestate.Met, 0); got.Mergeable != mergestate.Met {
		t.Fatalf("behind is mergeable: %+v", got)
	}
}
