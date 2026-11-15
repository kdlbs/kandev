package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestBaseBranchSelection_Propagation pins the policy carried alongside each
// attachment. A permissive request-level value must never replace a strict
// per-repository spec.
func TestBaseBranchSelection_Propagation(t *testing.T) {
	infos := []*repoInfo{
		{
			TaskRepositoryID: "task-repo-strict", RepositoryID: "repo-strict",
			RepositoryPath: "/tmp/strict", BaseBranch: "feature/strict",
			BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyStrict,
			Repository:               &models.Repository{ID: "repo-strict", Name: "strict", DefaultBranch: "main"},
		},
		{
			TaskRepositoryID: "task-repo-permitted", RepositoryID: "repo-permitted",
			RepositoryPath: "/tmp/permitted", BaseBranch: "feature/permitted",
			BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault,
			Repository:               &models.Repository{ID: "repo-permitted", Name: "permitted", DefaultBranch: "main"},
		},
	}
	specs := buildRepoSpecs(infos)
	if len(specs) != 2 {
		t.Fatalf("specs = %d, want two", len(specs))
	}
	if specs[0].BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyStrict {
		t.Fatalf("strict spec policy = %q", specs[0].BaseBranchFallbackPolicy)
	}
	if specs[1].BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyRepositoryDefault {
		t.Fatalf("permitted spec policy = %q", specs[1].BaseBranchFallbackPolicy)
	}

	req := &LaunchAgentRequest{
		RepositoryID: "repo-strict", RepositoryPath: "/tmp/strict", BaseBranch: "feature/strict",
		BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyStrict,
	}
	spec, ok := topLevelLaunchRepoSpec(req)
	if !ok {
		t.Fatal("topLevelLaunchRepoSpec() did not resolve the legacy single-repo request")
	}
	if spec.BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyStrict {
		t.Fatalf("top-level spec policy = %q, want strict", spec.BaseBranchFallbackPolicy)
	}
}
