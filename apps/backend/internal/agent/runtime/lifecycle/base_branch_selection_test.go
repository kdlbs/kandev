package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
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

// TestBaseBranchSelection_SingleRepoRefreshErrorNamesRepository drives the
// single-repository preparation path: a missing-base refresh failure must name
// the repository rather than only the requested ref.
func TestBaseBranchSelection_SingleRepoRefreshErrorNamesRepository(t *testing.T) {
	preparer, _ := newPreparerForTest(t)
	repositoryPath := initBareGitRepo(t, "single-refresh")
	req := &EnvPrepareRequest{
		TaskID: "task-single-refresh", SessionID: "session-single-refresh",
		ExecutorType: "worktree", TaskDirName: "single-refresh_task", RepoName: "single-refresh",
		RepositoryID: "repo-single-refresh", TaskRepositoryID: "task-repo-single-refresh",
		RepositoryPath: repositoryPath, BaseBranch: "missing-base", DefaultBranch: "main",
		RefreshRepository: func(context.Context) error {
			return fmt.Errorf("refresh base %q: %w", "missing-base", worktree.ErrRemoteRefMissing)
		},
	}
	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare returned transport error: %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("Prepare result = %#v, want failure", result)
	}
	if !errors.Is(result.Error, worktree.ErrRemoteRefMissing) {
		t.Fatalf("result cause = %v, want ErrRemoteRefMissing", result.Error)
	}
	if !strings.Contains(result.ErrorMessage, "single-refresh") {
		t.Fatalf("error = %q, want repository identity", result.ErrorMessage)
	}
	if !strings.Contains(result.ErrorMessage, "missing-base") {
		t.Fatalf("error = %q, want requested ref", result.ErrorMessage)
	}
}

// TestBaseBranchSelection_SingleSpecOverridesTopLevelFields pins the
// single-repository preparation path: one explicit strict attachment must not
// be replaced by conflicting legacy top-level fields.
func TestBaseBranchSelection_SingleSpecOverridesTopLevelFields(t *testing.T) {
	preparer, _, store := newPreparerForTestWithStore(t)
	repositoryPath := initBareGitRepo(t, "single-spec")
	req := &EnvPrepareRequest{
		TaskID: "task-single-spec", SessionID: "session-single-spec",
		ExecutorType: "worktree", TaskDirName: "single-spec_task", RepoName: "single-spec",
		// Legacy fields disagree with the explicit spec below.
		RepositoryID: "repo-top-level", RepositoryPath: repositoryPath, BaseBranch: "main",
		BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyRepositoryDefault,
		DefaultBranch:            "main",
		Repositories: []RepoPrepareSpec{{
			RepositoryID: "repo-single-spec", RepositoryPath: repositoryPath, RepoName: "single-spec",
			BaseBranch: "missing-base", BaseBranchFallbackPolicy: models.BaseBranchFallbackPolicyStrict,
			DefaultBranch: "main",
		}},
	}
	result, err := preparer.Prepare(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Prepare returned transport error: %v", err)
	}
	if result == nil || result.Success {
		t.Fatalf("Prepare result = %#v, want failure from the strict spec", result)
	}
	if !errors.Is(result.Error, worktree.ErrInvalidBaseBranch) {
		t.Fatalf("result cause = %v, want ErrInvalidBaseBranch", result.Error)
	}
	if !strings.Contains(result.ErrorMessage, "single-spec") ||
		!strings.Contains(result.ErrorMessage, "missing-base") {
		t.Fatalf("error = %q, want repository and requested ref", result.ErrorMessage)
	}
	if len(store.worktrees) != 0 {
		t.Fatalf("strict spec created worktrees: %#v", store.worktrees)
	}
}
