package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1
func TestGitHubComparisonRepositoryFromRepositoryPreservesEnterpriseHost(t *testing.T) {
	tests := []struct {
		providerHost string
		wantHost     string
		wantRemote   string
	}{
		{"https://git.example.com:8443", "git.example.com:8443", "https://git.example.com:8443/acme/widgets.git"},
		{"https://Tenant.ghe.com/", "tenant.ghe.com", "https://tenant.ghe.com/acme/widgets.git"},
		{"", "github.com", "https://github.com/acme/widgets.git"},
	}
	for _, tt := range tests {
		t.Run(tt.providerHost, func(t *testing.T) {
			got, ok := githubComparisonRepositoryFromRepository(&models.Repository{
				Provider: "github", ProviderHost: tt.providerHost, ProviderOwner: "acme", ProviderName: "widgets",
			})
			if !ok {
				t.Fatalf("githubComparisonRepositoryFromRepository() rejected provider host %q", tt.providerHost)
			}
			if got.Host != tt.wantHost || got.Path != "acme/widgets" || got.RemoteURL != tt.wantRemote {
				t.Fatalf("repository identity = %#v, want host %q remote %q", got, tt.wantHost, tt.wantRemote)
			}
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2
func TestNormalizeGitHubComparisonRepositoryKeepsLookalikeHostDistinct(t *testing.T) {
	lookalike, ok := normalizeGitHubComparisonRepository(models.ComparisonTargetRepository{
		Host: "https://github.com.evil", Path: "acme/widgets",
	})
	if !ok {
		t.Fatal("normalizeGitHubComparisonRepository() rejected a valid Enterprise authority")
	}
	public, _ := normalizeGitHubComparisonRepository(models.ComparisonTargetRepository{
		Host: "github.com", Path: "acme/widgets",
	})
	if lookalike.Host != "github.com.evil" || models.ComparisonTargetRepositoriesEqual(lookalike, public) {
		t.Fatalf("lookalike identity = %#v, must stay distinct from %#v", lookalike, public)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.4
func TestResolveTaskRepoInfo_EnterprisePRResolvesBase(t *testing.T) {
	target := enterprisePRComparisonTarget("git.example.com")
	resolver := &recordingPRBaseResolver{result: &models.PRBase{
		Target: target, OID: "0123456789abcdef0123456789abcdef01234567",
	}}
	info, err := resolveEnterprisePRTask(t, "https://git.example.com", resolver)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo() error: %v", err)
	}
	if info.BaseBranch != "release" || info.PRBase == nil ||
		info.PRBase.Target.TargetRepository.Host != "git.example.com" {
		t.Fatalf("resolved Enterprise PR base = %#v, branch = %q", info.PRBase, info.BaseBranch)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2
func TestResolveTaskRepoInfo_RejectsPRFromOtherHost(t *testing.T) {
	resolver := &recordingPRBaseResolver{result: &models.PRBase{Target: enterprisePRComparisonTarget("github.com")}}
	_, err := resolveEnterprisePRTask(t, "https://git.example.com", resolver)
	var resolutionErr *PRBaseResolutionError
	if !errors.As(err, &resolutionErr) || !resolutionErr.InvalidAssociation() {
		t.Fatalf("resolveTaskRepoInfo() error = %v, want invalid association for a PR on another host", err)
	}
}

func enterprisePRComparisonTarget(host string) models.ComparisonTarget {
	return models.ComparisonTarget{
		Version: models.ComparisonTargetVersion, Provider: models.ComparisonTargetProviderGitHub,
		Kind: models.ComparisonTargetKindPullRequest, Number: 42,
		HeadBranch: "feature", TargetBranch: "release",
		HeadRepository: models.ComparisonTargetRepository{
			Host: host, Path: "acme/widgets", RemoteURL: "https://" + host + "/acme/widgets.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host: host, Path: "acme/widgets", RemoteURL: "https://" + host + "/acme/widgets.git",
		},
	}
}

func resolveEnterprisePRTask(t *testing.T, providerHost string, resolver *recordingPRBaseResolver) (*repoInfo, error) {
	t.Helper()
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", WorkspaceID: "workspace-1", SourceType: sourceTypeLocal, LocalPath: t.TempDir(),
		Provider: "github", ProviderHost: providerHost, ProviderOwner: "acme", ProviderName: "widgets",
		DefaultBranch: "main",
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetPRBaseResolver(resolver)
	return exec.resolveTaskRepoInfo(context.Background(), &models.TaskRepository{
		ID: "task-repo-1", TaskID: "task-1", RepositoryID: "repo-1",
		BaseBranch: "main", CheckoutBranch: "feature",
		Metadata: map[string]interface{}{"pr_number": 42},
	})
}
