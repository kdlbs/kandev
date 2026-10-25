package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type recordingPRBaseResolver struct {
	base       string
	headBranch string
	result     *models.PRBase
	err        error
	calls      []prBaseResolveCall
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
func TestResolveTaskRepoInfo_PRBaseLookupFailureKeepsStoredBase(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		WorkspaceID:   "workspace-1",
		SourceType:    sourceTypeLocal,
		LocalPath:     t.TempDir(),
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "widgets",
		DefaultBranch: "main",
	}
	resolver := &recordingPRBaseResolver{err: errors.New("GitHub unavailable")}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetPRBaseResolver(resolver)

	info, err := exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID:           "task-repo-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		BaseBranch:   "feature/stored-base",
		Metadata:     map[string]interface{}{"pr_number": 42},
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	if info.BaseBranch != "feature/stored-base" {
		t.Fatalf("BaseBranch = %q, want stored base", info.BaseBranch)
	}
}

func TestResolveTaskRepoInfo_PRBaseLookupCancellationAbortsLaunchResolution(t *testing.T) {
	target := forkPRComparisonTarget()
	metadata := map[string]interface{}{"pr_number": 42}
	if err := models.PutComparisonTarget(metadata, target); err != nil {
		t.Fatalf("PutComparisonTarget() error: %v", err)
	}
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: sourceTypeLocal,
		LocalPath: t.TempDir(), Provider: "github", ProviderOwner: "fork-owner",
		ProviderName: "widgets", DefaultBranch: "main",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetPRBaseResolver(&recordingPRBaseResolver{err: context.Canceled})

	info, err := exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1",
		BaseBranch: "main", CheckoutBranch: "feature", Metadata: metadata,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resolveTaskRepoInfo() error = %v, want context.Canceled", err)
	}
	if info != nil {
		t.Fatalf("resolveTaskRepoInfo() returned launch info after cancellation: %#v", info)
	}
}

func TestResolveTaskRepoInfo_FallsBackToTaskRepositoryBaseForIntegrationRef(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		WorkspaceID:   "workspace-1",
		SourceType:    sourceTypeLocal,
		LocalPath:     t.TempDir(),
		DefaultBranch: "main",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	info, err := exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID:           "task-repo-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		BaseBranch:   "main",
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	if info.IntegrationRef != "main" {
		t.Fatalf("IntegrationRef = %q, want task repository base", info.IntegrationRef)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
func TestResolveTaskRepoInfo_SkipsPRBaseLookupWithoutGitHubPR(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		metadata map[string]interface{}
	}{
		{name: "no PR number", provider: "github"},
		{name: "non GitHub provider", provider: "gitlab", metadata: map[string]interface{}{"pr_number": 42}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.repositories["repo-1"] = &models.Repository{
				ID:            "repo-1",
				WorkspaceID:   "workspace-1",
				SourceType:    sourceTypeLocal,
				LocalPath:     t.TempDir(),
				Provider:      tt.provider,
				ProviderOwner: "acme",
				ProviderName:  "widgets",
				DefaultBranch: "main",
			}
			resolver := &recordingPRBaseResolver{base: "main"}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			exec.SetPRBaseResolver(resolver)

			info, err := exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
				ID:           "task-repo-1",
				TaskID:       "task-1",
				RepositoryID: "repo-1",
				BaseBranch:   "feature/stored-base",
				Metadata:     tt.metadata,
			})
			if err != nil {
				t.Fatalf("resolveTaskRepoInfo() error: %v", err)
			}
			if info.BaseBranch != "feature/stored-base" {
				t.Fatalf("BaseBranch = %q, want stored base", info.BaseBranch)
			}
			if len(resolver.calls) != 0 {
				t.Fatalf("resolver calls = %#v, want none", resolver.calls)
			}
		})
	}
}

type prBaseResolveCall struct {
	workspaceID, owner, repo string
	number                   int
}

func (r *recordingPRBaseResolver) ResolvePRBase(
	_ context.Context, workspaceID string, lookup PRBaseLookup,
) (models.PRBase, error) {
	owner, repo := lookup.AttachedOwner, lookup.AttachedRepository
	if lookup.Target != nil {
		owner, repo, _ = strings.Cut(lookup.Target.TargetRepository.Path, "/")
	}
	r.calls = append(r.calls, prBaseResolveCall{workspaceID: workspaceID, owner: owner, repo: repo, number: lookup.Number})
	if r.err != nil {
		return models.PRBase{}, r.err
	}
	if r.result != nil {
		return *r.result, nil
	}
	headBranch := r.headBranch
	if headBranch == "" {
		headBranch = lookup.CheckoutBranch
	}
	if headBranch == "" {
		headBranch = "feature"
	}
	target, err := (models.ComparisonTargetCandidate{
		Provider:     models.ComparisonTargetProviderGitHub,
		Kind:         models.ComparisonTargetKindPullRequest,
		Number:       lookup.Number,
		HeadBranch:   headBranch,
		TargetBranch: r.base,
		HeadRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: owner + "/" + repo,
			RemoteURL: "https://github.com/" + owner + "/" + repo + ".git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: "github.com", Path: owner + "/" + repo,
			RemoteURL: "https://github.com/" + owner + "/" + repo + ".git",
		},
	}).Build()
	if err != nil {
		return models.PRBase{}, err
	}
	return models.PRBase{Target: target}, nil
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
func TestResolveTaskRepoInfo_UsesLivePRBase(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		WorkspaceID:   "workspace-1",
		SourceType:    sourceTypeLocal,
		LocalPath:     t.TempDir(),
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "widgets",
		DefaultBranch: "main",
	}
	resolver := &recordingPRBaseResolver{base: "main", headBranch: "feature/stacked-child"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetPRBaseResolver(resolver)

	info, err := exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID:             "task-repo-1",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		BaseBranch:     "feature/deleted-parent",
		CheckoutBranch: "feature/stacked-child",
		Metadata:       map[string]interface{}{"pr_number": float64(42)},
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	if info.BaseBranch != "main" {
		t.Fatalf("BaseBranch = %q, want live base main", info.BaseBranch)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != (prBaseResolveCall{workspaceID: "workspace-1", owner: "acme", repo: "widgets", number: 42}) {
		t.Fatalf("resolver calls = %#v, want acme/widgets#42", resolver.calls)
	}
}

// @covers AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.11
func TestResolveTaskRepoInfo_ReconcilesRetargetedRemoteContributionBase(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		WorkspaceID:   "workspace-1",
		SourceType:    sourceTypeLocal,
		LocalPath:     t.TempDir(),
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "widgets",
		DefaultBranch: "main",
	}
	binding := &models.RemoteContribution{
		Version:      models.RemoteContributionVersion,
		Provider:     models.RemoteContributionProviderGitHub,
		Kind:         models.RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widgets/pull/42",
		Number:       42,
		State:        models.RemoteContributionStateOpen,
		BaseBranch:   "feature/deleted-parent",
		HeadBranch:   "feature/stacked-child",
		HeadSHA:      strings.Repeat("a", 40),
		SourceRepository: models.RemoteContributionRepository{
			Host:      "github.com",
			Path:      "acme/widgets",
			RemoteURL: "https://github.com/acme/widgets.git",
		},
	}
	metadata := map[string]interface{}{}
	if err := models.PutRemoteContribution(metadata, binding); err != nil {
		t.Fatalf("PutRemoteContribution() error: %v", err)
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	info, err := exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID:             "task-repo-1",
		TaskID:         "task-1",
		RepositoryID:   "repo-1",
		BaseBranch:     "main",
		CheckoutBranch: "feature/stacked-child",
		Metadata:       metadata,
	})
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	if info.BaseBranch != "main" {
		t.Fatalf("BaseBranch = %q, want task repository base", info.BaseBranch)
	}
	if info.RemoteContribution == nil || info.RemoteContribution.BaseBranch != "main" {
		t.Fatalf("remote contribution base = %#v, want main", info.RemoteContribution)
	}
}
