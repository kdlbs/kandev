package lifecycle

import "testing"

func TestLaunchRequest_RepoSpecs_ReturnsExplicitListWhenSet(t *testing.T) {
	req := &LaunchRequest{
		// Single-repo top-level fields are populated from Repositories[0] for
		// backwards compat by callers, but RepoSpecs() must always prefer the
		// explicit list when present.
		RepositoryID:   "legacy-repo",
		RepositoryPath: "/legacy",
		Repositories: []RepoLaunchSpec{
			{RepositoryID: "repo-a", RepositoryPath: "/a", BaseBranch: "main"},
			{RepositoryID: "repo-b", RepositoryPath: "/b", BaseBranch: "develop"},
		},
	}
	specs := req.RepoSpecs()
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].RepositoryID != "repo-a" || specs[1].RepositoryID != "repo-b" {
		t.Errorf("unexpected order: %+v", specs)
	}
}

func TestLaunchRequest_RepoSpecs_SynthesizesFromLegacyFields(t *testing.T) {
	req := &LaunchRequest{
		RepositoryID:         "repo-x",
		RepositoryPath:       "/x",
		BaseBranch:           "main",
		CheckoutBranch:       "feature/y",
		WorktreeID:           "wt-1",
		WorktreeBranchPrefix: "feat/",
		PullBeforeWorktree:   true,
		RepoName:             "x",
	}
	specs := req.RepoSpecs()
	if len(specs) != 1 {
		t.Fatalf("expected length-1 list synthesized, got %d", len(specs))
	}
	got := specs[0]
	if got.RepositoryID != "repo-x" || got.RepositoryPath != "/x" ||
		got.BaseBranch != "main" || got.CheckoutBranch != "feature/y" ||
		got.WorktreeID != "wt-1" || got.WorktreeBranchPrefix != "feat/" ||
		!got.PullBeforeWorktree || got.RepoName != "x" {
		t.Errorf("synthesized spec mismatch: %+v", got)
	}
}

func TestLaunchRequest_RepoSpecs_NilForRepoLessLaunch(t *testing.T) {
	req := &LaunchRequest{TaskID: "t1", SessionID: "s1"}
	if specs := req.RepoSpecs(); specs != nil {
		t.Errorf("expected nil for repo-less launch, got %v", specs)
	}
}

func TestEnvPrepareRequest_RepoSpecs_ExplicitWins(t *testing.T) {
	req := &EnvPrepareRequest{
		RepositoryID:   "legacy",
		RepositoryPath: "/legacy",
		Repositories: []RepoPrepareSpec{
			{RepositoryID: "a", RepositoryPath: "/a"},
		},
	}
	specs := req.RepoSpecs()
	if len(specs) != 1 || specs[0].RepositoryID != "a" {
		t.Fatalf("expected explicit list to win; got %+v", specs)
	}
}

func TestEnvPrepareRequest_RepoSpecs_SynthesizedCarriesRepoSetupScript(t *testing.T) {
	req := &EnvPrepareRequest{
		RepositoryID:    "r1",
		RepositoryPath:  "/r1",
		BaseBranch:      "main",
		RepoSetupScript: "make install",
	}
	specs := req.RepoSpecs()
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].RepoSetupScript != "make install" {
		t.Errorf("repo setup script not propagated: %+v", specs[0])
	}
}

func TestEnvPrepareRequest_RepoSpecs_NilForRepoLess(t *testing.T) {
	req := &EnvPrepareRequest{}
	if specs := req.RepoSpecs(); specs != nil {
		t.Errorf("expected nil, got %+v", specs)
	}
}
