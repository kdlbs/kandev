package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestBaseBranchSelection_Propagation pins the typed policy through the
// launch-to-prepare copies. The per-repository spec is authoritative; a
// permissive request-level value must not authorize a strict attachment.
func TestBaseBranchSelection_Propagation(t *testing.T) {
	req := &LaunchRequest{
		TaskID: "task-1", SessionID: "session-1", ExecutorType: "worktree",
		BaseBranch:               "feature/top",
		BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault,
		Repositories: []RepoLaunchSpec{
			{
				TaskRepositoryID: "task-repo-strict", RepositoryID: "repo-strict",
				BaseBranch: "feature/strict", BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyStrict,
			},
			{
				TaskRepositoryID: "task-repo-permitted", RepositoryID: "repo-permitted",
				BaseBranch:               "feature/permitted",
				BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault,
			},
		},
	}
	specs := req.RepoSpecs()
	if len(specs) != 2 {
		t.Fatalf("launch specs = %d, want two", len(specs))
	}
	if specs[0].BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyStrict {
		t.Fatalf("strict launch spec policy = %q", specs[0].BaseBranchFallbackPolicy)
	}
	if specs[1].BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyRepositoryDefault {
		t.Fatalf("permitted launch spec policy = %q", specs[1].BaseBranchFallbackPolicy)
	}

	prep := buildEnvPrepareRequest(req, "/tmp/workspace", "worktree")
	if prep.BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyRepositoryDefault {
		t.Fatalf("prepare top-level policy = %q", prep.BaseBranchFallbackPolicy)
	}
	if len(prep.Repositories) != 2 {
		t.Fatalf("prepare specs = %d, want two", len(prep.Repositories))
	}
	if prep.Repositories[0].BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyStrict {
		t.Fatalf("strict prepare spec policy = %q", prep.Repositories[0].BaseBranchFallbackPolicy)
	}
	if prep.Repositories[1].BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyRepositoryDefault {
		t.Fatalf("permitted prepare spec policy = %q", prep.Repositories[1].BaseBranchFallbackPolicy)
	}

	singlePrep := &EnvPrepareRequest{
		TaskID: "task-2", RepositoryID: "repo-single", RepositoryPath: "/tmp/single",
		BaseBranch: "feature/single", BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyStrict,
	}
	singleSpecs := singlePrep.RepoSpecs()
	if len(singleSpecs) != 1 {
		t.Fatalf("single prepare specs = %d, want one", len(singleSpecs))
	}
	if singleSpecs[0].BaseBranchFallbackPolicy != models.BaseBranchFallbackPolicyStrict {
		t.Fatalf("single prepare spec policy = %q, want strict", singleSpecs[0].BaseBranchFallbackPolicy)
	}
}
