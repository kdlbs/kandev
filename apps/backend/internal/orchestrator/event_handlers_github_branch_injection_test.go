package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/github"
)

// stubReviewResolver is a minimal RepositoryResolver that always resolves to a
// fixed repo/base-branch, so resolveReviewRepository reaches the head-branch
// validation branch under test.
type stubReviewResolver struct {
	repoID     string
	baseBranch string
}

func (s stubReviewResolver) ResolveForReview(
	_ context.Context, _, _, _, _, _ string,
) (string, string, error) {
	return s.repoID, s.baseBranch, nil
}

// TestResolveReviewRepository_RejectsUnsafeHeadBranch is the ingestion-layer
// (defense-in-depth) guard for the branch-name command-injection RCE. A fork PR
// head branch is attacker-controlled and flows into executor prepare scripts via
// {{worktree.branch}} / {{repository.branch}}. resolveReviewRepository must not
// propagate a head branch that fails securityutil.IsValidBranchName into
// CheckoutBranch — but it must still create the review task (without a checkout)
// so a hostile branch name cannot suppress review entirely.
func TestResolveReviewRepository_RejectsUnsafeHeadBranch(t *testing.T) {
	svc := &Service{
		logger:             testLogger(),
		repositoryResolver: stubReviewResolver{repoID: "repo1", baseBranch: "main"},
	}

	cases := []struct {
		name           string
		headBranch     string
		wantCheckout   string
		wantHasRepoRow bool
	}{
		{name: "safe branch is checked out", headBranch: "feature/login", wantCheckout: "feature/login", wantHasRepoRow: true},
		{name: "command substitution rejected", headBranch: "$(touch pwned)", wantCheckout: "", wantHasRepoRow: true},
		{name: "semicolon chaining rejected", headBranch: "main;touch pwned", wantCheckout: "", wantHasRepoRow: true},
		{name: "backtick rejected", headBranch: "`id`", wantCheckout: "", wantHasRepoRow: true},
		{name: "space rejected", headBranch: "a b", wantCheckout: "", wantHasRepoRow: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr := &github.PR{
				Number:     7,
				RepoOwner:  "myorg",
				RepoName:   "myrepo",
				BaseBranch: "main",
				HeadBranch: tc.headBranch,
			}
			got := svc.resolveReviewRepository(context.Background(), "ws1", pr)
			if len(got) != 1 {
				t.Fatalf("expected 1 repo row, got %d", len(got))
			}
			if got[0].RepositoryID != "repo1" {
				t.Fatalf("RepositoryID = %q, want repo1", got[0].RepositoryID)
			}
			if got[0].CheckoutBranch != tc.wantCheckout {
				t.Fatalf("CheckoutBranch = %q, want %q (head=%q)", got[0].CheckoutBranch, tc.wantCheckout, tc.headBranch)
			}
		})
	}
}
