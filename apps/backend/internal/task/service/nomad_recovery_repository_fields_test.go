package service

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

func TestApplyNomadRecoveryRepositoryFieldsUsesInjectedResolverAndIdentity(t *testing.T) {
	svc := &Service{}
	repo := &models.Repository{ID: "repo-1"}
	svc.SetRepoCloneURLResolver(func(r *models.Repository) string { return "https://example.test/" + r.ID + ".git" })

	spec := lifecycle.WorkspaceRepositorySpec{RepositoryID: "repo-1"}
	svc.applyNomadRecoveryRepositoryFields(&spec, repo, "main")
	if spec.RepositoryURL != "https://example.test/repo-1.git" || spec.BranchIdentitySlug != "main" {
		t.Fatalf("spec = %+v", spec)
	}

	selected := lifecycle.WorkspaceRepositorySpec{RepositoryID: "repo-1", BranchIdentitySlug: "kept"}
	svc.applyNomadRecoveryRepositoryFields(&selected, repo, "main")
	if selected.BranchIdentitySlug != "kept" {
		t.Fatalf("a worktree-selected identity must not be overwritten: %+v", selected)
	}
}

func TestApplyNomadRecoveryRepositoryFieldsWithoutResolverLeavesLocatorEmpty(t *testing.T) {
	spec := lifecycle.WorkspaceRepositorySpec{RepositoryID: "repo-1"}
	(&Service{}).applyNomadRecoveryRepositoryFields(&spec, &models.Repository{}, "main")
	if spec.RepositoryURL != "" {
		t.Fatalf("locator must stay empty so recovery fails closed: %q", spec.RepositoryURL)
	}
}
