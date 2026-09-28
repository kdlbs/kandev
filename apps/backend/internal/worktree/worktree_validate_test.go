package worktree

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestCreateRequest_Validate_FallsBackToFallbackBaseBranch pins the
// defence-in-depth path added with the add_branch_to_task fix: when the
// caller forgot to set BaseBranch but supplied a FallbackBaseBranch
// (typically the repository's persisted default_branch), Validate adopts
// the fallback instead of returning ErrInvalidBaseBranch.
func TestCreateRequest_Validate_FallsBackToFallbackBaseBranch(t *testing.T) {
	r := &CreateRequest{
		TaskID:             "task-1",
		RepositoryPath:     "/tmp/repo",
		BaseBranch:         "",
		FallbackBaseBranch: "main",
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate with fallback should succeed: %v", err)
	}
	if r.BaseBranch != "main" {
		t.Errorf("expected BaseBranch promoted from fallback, got %q", r.BaseBranch)
	}
}

func TestCreateRequest_ValidateRejectsQualifiedPRContributionHeadMismatch(t *testing.T) {
	base, binding := qualifiedContributionIdentityFixture()
	base.Target.HeadRepository.Path = "other-fork/widgets"
	base.Target.HeadRepository.RemoteURL = "https://github.com/other-fork/widgets.git"
	request := &CreateRequest{
		TaskID: "task-1", RepositoryPath: "/tmp/repo", BaseBranch: "release", CheckoutBranch: "feature",
		PRNumber: 42, QualifiedPRBase: &base, RemoteContribution: &binding,
	}
	if err := request.Validate(); err == nil {
		t.Fatal("Validate accepted a qualified PR head from another repository")
	}
}

func TestCreateRequest_ValidateAllowsContributionAttachedToPRBaseRepository(t *testing.T) {
	base, binding := qualifiedContributionIdentityFixture()
	request := &CreateRequest{
		TaskID: "task-1", RepositoryPath: "/tmp/repo", BaseBranch: "release", CheckoutBranch: "feature",
		PRNumber: 42, QualifiedPRBase: &base, RemoteContribution: &binding,
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate rejected valid contribution attachment: %v", err)
	}
}

func qualifiedContributionIdentityFixture() (models.PRBase, models.RemoteContribution) {
	binding := models.RemoteContribution{
		Version: models.RemoteContributionVersion, Provider: models.RemoteContributionProviderGitHub,
		Kind: models.RemoteContributionKindPullRequest, CanonicalURL: "https://github.com/upstream/widgets/pull/42",
		Number: 42, State: models.RemoteContributionStateOpen, BaseBranch: "release", HeadBranch: "feature",
		HeadSHA: "0123456789abcdef0123456789abcdef01234567", CollaborationAllowed: true,
		SourceRepository: models.RemoteContributionRepository{
			Host: "github.com", Path: "fork-owner/widgets", RemoteURL: "https://github.com/fork-owner/widgets.git",
		},
	}
	base := models.PRBase{Target: models.ComparisonTarget{
		Version: models.ComparisonTargetVersion, Provider: models.ComparisonTargetProviderGitHub,
		Kind: models.ComparisonTargetKindPullRequest, Number: 42, HeadBranch: "feature", TargetBranch: "release",
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "fork-owner/widgets", RemoteURL: binding.SourceRepository.RemoteURL,
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: "upstream/widgets", RemoteURL: "https://github.com/upstream/widgets.git",
		},
	}}
	return base, binding
}

func TestCreateRequest_Validate_NoFallbackStillRejects(t *testing.T) {
	r := &CreateRequest{
		TaskID:         "task-1",
		RepositoryPath: "/tmp/repo",
	}
	if err := r.Validate(); !errors.Is(err, ErrInvalidBaseBranch) {
		t.Errorf("expected ErrInvalidBaseBranch, got %v", err)
	}
}

func TestCreateRequest_Validate_ExplicitBaseWinsOverFallback(t *testing.T) {
	r := &CreateRequest{
		TaskID:             "task-1",
		RepositoryPath:     "/tmp/repo",
		BaseBranch:         "develop",
		FallbackBaseBranch: "main",
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if r.BaseBranch != "develop" {
		t.Errorf("expected explicit BaseBranch to be preserved, got %q", r.BaseBranch)
	}
}
