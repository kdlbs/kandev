package service

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestNormalizeRepositoryBranchPolicyDefaultsPullRequestTarget(t *testing.T) {
	policy, err := normalizeRepositoryBranchPolicy(&models.RepositoryBranchPolicy{
		Name:           "Feature",
		BaseBranch:     "develop",
		BranchTemplate: "feature/{title}-{suffix}",
	})
	if err != nil {
		t.Fatalf("normalizeRepositoryBranchPolicy: %v", err)
	}
	if policy.PullRequestTarget != "develop" {
		t.Fatalf("pull request target = %q, want base branch", policy.PullRequestTarget)
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
	data, ok := eventBus.GetPublishedEvents()[0].Data.(map[string]interface{})
	if !ok || data["workspace_id"] != "ws-policy-service" {
		t.Fatalf("first policy event data = %#v, want workspace_id", eventBus.GetPublishedEvents()[0].Data)
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

func TestRepositoryBranchPolicyServiceRejectsImproveWorkspaceMutations(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedBranchPolicyRepository(t, repo)
	if err := repo.CreateRepositoryBranchPolicy(context.Background(), &models.RepositoryBranchPolicy{
		ID: "policy-improve", RepositoryID: "repo-policy-service", Name: "Existing",
		BaseBranch: "main", BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: "main",
	}); err != nil {
		t.Fatalf("seed branch policy: %v", err)
	}
	workspace, err := repo.GetWorkspace(context.Background(), "ws-policy-service")
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	workspace.Name = models.WorkspaceNameImproveKandev
	if err := repo.UpdateWorkspace(context.Background(), workspace); err != nil {
		t.Fatalf("update workspace: %v", err)
	}

	_, err = svc.CreateRepositoryBranchPolicy(context.Background(), &CreateRepositoryBranchPolicyRequest{
		RepositoryID: "repo-policy-service", Name: "Feature", BaseBranch: "main",
		BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: "main",
	})
	if !errors.Is(err, ErrRepositoryBranchPolicyReadOnly) {
		t.Fatalf("create read-only workspace error = %v", err)
	}

	_, err = svc.UpdateRepositoryBranchPolicy(context.Background(), "policy-improve", &UpdateRepositoryBranchPolicyRequest{
		Name: stringPointer("Renamed"),
	})
	if !errors.Is(err, ErrRepositoryBranchPolicyReadOnly) {
		t.Fatalf("update read-only workspace error = %v", err)
	}

	_, err = svc.CreateGitflowRepositoryBranchPolicies(context.Background(), &CreateGitflowRepositoryBranchPoliciesRequest{
		RepositoryID: "repo-policy-service", ProductionBranch: "main", DevelopmentBranch: "develop",
	})
	if !errors.Is(err, ErrRepositoryBranchPolicyReadOnly) {
		t.Fatalf("gitflow read-only workspace error = %v", err)
	}

	err = svc.DeleteRepositoryBranchPolicy(context.Background(), "policy-improve")
	if !errors.Is(err, ErrRepositoryBranchPolicyReadOnly) {
		t.Fatalf("delete read-only workspace error = %v", err)
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("read-only mutation published events = %d", len(eventBus.GetPublishedEvents()))
	}
}

func TestCreateTaskRejectsRemoteContributionPolicyBaseMismatch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workflowID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-policy-remote")
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-policy-remote", WorkspaceID: "ws-policy-remote", Name: "remote", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	policy, err := svc.CreateRepositoryBranchPolicy(ctx, &CreateRepositoryBranchPolicyRequest{
		RepositoryID: "repo-policy-remote", Name: "Feature", BaseBranch: "main",
		BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: "main",
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}

	_, err = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-policy-remote", WorkflowID: workflowID, Title: "Remote contribution",
		Repositories: []TaskRepositoryInput{{
			RepositoryID: "repo-policy-remote", BranchPolicyID: policy.ID, BaseBranch: "release",
			CheckoutBranch: "feature/remote", RemoteContribution: &models.RemoteContribution{
				Version: 1, Provider: "github", Kind: "pull_request", CanonicalURL: "https://github.com/acme/remote/pull/1",
				Number: 1, State: "open", BaseBranch: "release", HeadBranch: "feature/remote",
				HeadSHA: "0123456789abcdef0123456789abcdef01234567",
				SourceRepository: models.RemoteContributionRepository{
					Host: "github.com", Path: "acme/remote", RemoteURL: "https://github.com/acme/remote.git",
				},
			},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match branch policy base branch") {
		t.Fatalf("remote contribution mismatch error = %v", err)
	}
	_, total, err := repo.ListTasksByWorkspace(ctx, "ws-policy-remote", "", "", "", 1, 50, "", false, false, false, false)
	if err != nil {
		t.Fatalf("list tasks after rejected create: %v", err)
	}
	if total != 0 {
		t.Fatalf("rejected remote contribution mismatch persisted %d task(s)", total)
	}
}

func TestReplaceTaskRepositoriesPreservesFreshBranchWithPolicySnapshot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	workflowID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-policy-fresh")
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-policy-fresh", WorkspaceID: "ws-policy-fresh", Name: "fresh", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	policy, err := svc.CreateRepositoryBranchPolicy(ctx, &CreateRepositoryBranchPolicyRequest{
		RepositoryID: "repo-policy-fresh", Name: "Feature", BaseBranch: "main",
		BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: "main",
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-policy-fresh", WorkflowID: workflowID, Title: "Fresh branch",
		Repositories: []TaskRepositoryInput{{
			RepositoryID: "repo-policy-fresh", BranchPolicyID: policy.ID, BaseBranch: "main",
		}},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.UpdateRepositoryBranchPolicy(ctx, policy.ID, &UpdateRepositoryBranchPolicyRequest{
		BaseBranch: stringPointer("develop"),
	}); err != nil {
		t.Fatalf("update policy after task creation: %v", err)
	}

	if err := svc.ReplaceTaskRepositories(ctx, result.Task.ID, "ws-policy-fresh", []TaskRepositoryInput{{
		RepositoryID: "repo-policy-fresh", BranchPolicyID: policy.ID, BaseBranch: "feature/fresh-branch",
		PreserveBaseBranch: true,
	}}); err != nil {
		t.Fatalf("replace task repositories: %v", err)
	}

	linked, err := repo.ListTaskRepositories(ctx, result.Task.ID)
	if err != nil {
		t.Fatalf("list task repositories: %v", err)
	}
	if len(linked) != 1 {
		t.Fatalf("task repositories = %+v, want one association", linked)
	}
	if linked[0].BaseBranch != "feature/fresh-branch" {
		t.Fatalf("effective base branch = %q, want generated fresh branch", linked[0].BaseBranch)
	}
	if linked[0].BranchPolicyBaseBranch != "main" {
		t.Fatalf("policy snapshot base branch = %q, want main", linked[0].BranchPolicyBaseBranch)
	}
}

func stringPointer(value string) *string { return &value }
