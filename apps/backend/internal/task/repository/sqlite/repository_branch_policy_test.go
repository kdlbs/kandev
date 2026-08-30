package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestRepositoryBranchPolicySchemaExistsWithCaseInsensitiveNameIndex(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()

	var tableName string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'repository_branch_policies'
	`).Scan(&tableName); err != nil {
		t.Fatalf("repository_branch_policies table is missing: %v", err)
	}
	if tableName != "repository_branch_policies" {
		t.Fatalf("table name = %q", tableName)
	}

	var indexName string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type = 'index' AND name = 'uniq_repository_branch_policies_repository_lower_name'
	`).Scan(&indexName); err != nil {
		t.Fatalf("case-insensitive policy-name index is missing: %v", err)
	}
	if indexName != "uniq_repository_branch_policies_repository_lower_name" {
		t.Fatalf("index name = %q", indexName)
	}
}

func TestRepositoryBranchPolicyCRUDAndCaseInsensitiveUniqueness(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-policy")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-policy", WorkspaceID: "ws-policy", Name: "repo-policy"}); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	policy := &models.RepositoryBranchPolicy{
		RepositoryID:      "repo-policy",
		Name:              "Feature",
		Description:       "Fresh features",
		BaseBranch:        "develop",
		BranchTemplate:    "feature/{title}-{suffix}",
		PullRequestTarget: "develop",
	}
	if err := repo.CreateRepositoryBranchPolicy(ctx, policy); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	loaded, err := repo.GetRepositoryBranchPolicy(ctx, policy.ID)
	if err != nil {
		t.Fatalf("get policy: %v", err)
	}
	if loaded.Name != "Feature" || loaded.BaseBranch != "develop" {
		t.Fatalf("loaded policy = %+v", loaded)
	}
	byName, err := repo.GetRepositoryBranchPolicyByName(ctx, "repo-policy", "FEATURE")
	if err != nil || byName == nil || byName.ID != policy.ID {
		t.Fatalf("lookup by name = %+v, err=%v", byName, err)
	}

	duplicate := *policy
	duplicate.ID = "duplicate-policy"
	duplicate.Name = "feature"
	if err := repo.CreateRepositoryBranchPolicy(ctx, &duplicate); err == nil {
		t.Fatal("case-insensitive duplicate policy name was accepted")
	}

	policy.Description = "Updated"
	if err := repo.UpdateRepositoryBranchPolicy(ctx, policy); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	policies, err := repo.ListRepositoryBranchPolicies(ctx, "repo-policy")
	if err != nil || len(policies) != 1 || policies[0].Description != "Updated" {
		t.Fatalf("list policies = %+v, err=%v", policies, err)
	}
	deleted, err := repo.DeleteRepositoryBranchPolicy(ctx, policy.ID)
	if err != nil || !deleted {
		t.Fatalf("delete policy = %t, err=%v", deleted, err)
	}
	if _, err := repo.GetRepositoryBranchPolicy(ctx, policy.ID); !errors.Is(err, repoerrors.ErrRepositoryBranchPolicyNotFound) {
		t.Fatalf("deleted policy error = %v", err)
	}
}

func TestRepositoryDeletePrunesBranchPolicies(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-policy-prune")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-policy-prune", WorkspaceID: "ws-policy-prune", Name: "repo-policy-prune"}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateRepositoryBranchPolicy(ctx, &models.RepositoryBranchPolicy{ID: "policy-prune", RepositoryID: "repo-policy-prune", Name: "Feature"}); err != nil {
		t.Fatalf("create policy: %v", err)
	}
	if err := repo.DeleteRepository(ctx, "repo-policy-prune"); err != nil {
		t.Fatalf("delete repository: %v", err)
	}
	policies, err := repo.ListRepositoryBranchPolicies(ctx, "repo-policy-prune")
	if err != nil {
		t.Fatalf("list policies: %v", err)
	}
	if len(policies) != 0 {
		t.Fatalf("policies after repository deletion = %+v", policies)
	}
}

func TestListRepositoryBranchPoliciesByWorkspaceGroupsOnlyWorkspaceRepositories(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-policy-batch")
	seedWorkspace(t, repo, "ws-policy-other")
	for _, item := range []struct {
		id          string
		workspaceID string
		policyID    string
		name        string
	}{
		{id: "repo-batch-a", workspaceID: "ws-policy-batch", policyID: "policy-batch-a", name: "Feature"},
		{id: "repo-batch-b", workspaceID: "ws-policy-batch", policyID: "policy-batch-b", name: "Hotfix"},
		{id: "repo-other", workspaceID: "ws-policy-other", policyID: "policy-other", name: "Other"},
	} {
		if err := repo.CreateRepository(ctx, &models.Repository{ID: item.id, WorkspaceID: item.workspaceID, Name: item.id}); err != nil {
			t.Fatalf("create repository %s: %v", item.id, err)
		}
		if err := repo.CreateRepositoryBranchPolicy(ctx, &models.RepositoryBranchPolicy{
			ID: item.policyID, RepositoryID: item.id, Name: item.name,
		}); err != nil {
			t.Fatalf("create policy %s: %v", item.policyID, err)
		}
	}

	policies, err := repo.ListRepositoryBranchPoliciesByWorkspace(ctx, "ws-policy-batch")
	if err != nil {
		t.Fatalf("list workspace policies: %v", err)
	}
	if len(policies) != 2 || policies[0].RepositoryID != "repo-batch-a" || policies[1].RepositoryID != "repo-batch-b" {
		t.Fatalf("workspace policies = %+v, want the two ordered workspace repositories", policies)
	}
}
