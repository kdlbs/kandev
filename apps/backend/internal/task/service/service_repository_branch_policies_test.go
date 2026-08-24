package service

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func seedBranchPolicyRepository(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateRepository(context.Context, *models.Repository) error
}) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-policy-service", Name: "Policy workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-policy-service", WorkspaceID: "ws-policy-service", Name: "Policy repo", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
}

func TestRepositoryBranchPolicyServiceNormalizesAndRejectsInvalidUpdates(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedBranchPolicyRepository(t, repo)
	ctx := context.Background()

	policy, err := svc.CreateRepositoryBranchPolicy(ctx, &CreateRepositoryBranchPolicyRequest{
		RepositoryID: "repo-policy-service", Name: "  Feature  ", Description: "  branch flow ",
		BaseBranch: " develop ", BranchTemplate: " feature/{title}-{suffix} ", PullRequestTarget: " develop ",
	})
	if err != nil {
		t.Fatalf("CreateRepositoryBranchPolicy: %v", err)
	}
	if policy.Name != "Feature" || policy.Description != "branch flow" || policy.BaseBranch != "develop" {
		t.Fatalf("normalized policy = %+v", policy)
	}

	_, err = svc.CreateRepositoryBranchPolicy(ctx, &CreateRepositoryBranchPolicyRequest{
		RepositoryID: "repo-policy-service", Name: "feature", BaseBranch: "main",
		BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: "main",
	})
	if !errors.Is(err, ErrRepositoryBranchPolicyNameConflict) {
		t.Fatalf("duplicate name error = %v", err)
	}

	tooLong := make([]rune, repositoryBranchPolicyDescriptionMaxLength+1)
	for i := range tooLong {
		tooLong[i] = 'x'
	}
	_, err = svc.UpdateRepositoryBranchPolicy(ctx, policy.ID, &UpdateRepositoryBranchPolicyRequest{
		Description: stringPointer(string(tooLong)),
	})
	if !errors.Is(err, ErrInvalidRepositoryBranchPolicy) {
		t.Fatalf("long description error = %v", err)
	}

	if _, err := svc.GetRepositoryBranchPolicy(ctx, "missing"); !errors.Is(err, repoerrors.ErrRepositoryBranchPolicyNotFound) {
		t.Fatalf("missing policy error = %v", err)
	}
}

func TestRepositoryBranchPolicyServiceGitflowStarterIsAtomicAndOneTime(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, eventBus, repo := createTestService(t)
	seedBranchPolicyRepository(t, repo)
	repository, err := repo.GetRepository(context.Background(), "repo-policy-service")
	if err != nil {
		t.Fatalf("get repository: %v", err)
	}
	repositoryPath := filepath.Join(t.TempDir(), "policy-repo")
	initRealGitRepo(t, repositoryPath)
	cmd := exec.Command("git", "branch", "develop")
	cmd.Dir = repositoryPath
	cmd.Env = isolatedGitEnv()
	if output, branchErr := cmd.CombinedOutput(); branchErr != nil {
		t.Fatalf("create develop branch: %v (%s)", branchErr, output)
	}
	repository.LocalPath = repositoryPath
	repository.SourceType = sourceTypeLocal
	if err := repo.UpdateRepository(context.Background(), repository); err != nil {
		t.Fatalf("update repository path: %v", err)
	}
	ctx := context.Background()

	policies, err := svc.CreateGitflowRepositoryBranchPolicies(ctx, &CreateGitflowRepositoryBranchPoliciesRequest{
		RepositoryID: "repo-policy-service", ProductionBranch: "main", DevelopmentBranch: "develop",
	})
	if err != nil {
		t.Fatalf("CreateGitflowRepositoryBranchPolicies: %v", err)
	}
	if len(policies) != 4 {
		t.Fatalf("seeded %d policies, want 4", len(policies))
	}
	if policies[0].BaseBranch != "develop" || policies[2].BaseBranch != "main" || policies[3].PullRequestTarget != "main" {
		t.Fatalf("gitflow policies = %+v", policies)
	}
	if len(eventBus.GetPublishedEvents()) != 4 {
		t.Fatalf("published events = %d, want 4", len(eventBus.GetPublishedEvents()))
	}

	_, err = svc.CreateGitflowRepositoryBranchPolicies(ctx, &CreateGitflowRepositoryBranchPoliciesRequest{
		RepositoryID: "repo-policy-service", ProductionBranch: "main", DevelopmentBranch: "develop",
	})
	if !errors.Is(err, ErrRepositoryBranchPolicyAlreadySeeded) {
		t.Fatalf("second starter error = %v", err)
	}
	stored, err := repo.ListRepositoryBranchPolicies(ctx, "repo-policy-service")
	if err != nil || len(stored) != 4 {
		t.Fatalf("stored policies after rejected starter = %d, err=%v", len(stored), err)
	}
}

func stringPointer(value string) *string { return &value }
