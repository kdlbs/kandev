package backendapp

import (
	"testing"

	githubpkg "github.com/kandev/kandev/internal/github"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1
// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.5
func TestPRBaseIdentityConversionUsesFetchedHost(t *testing.T) {
	tests := []struct {
		name       string
		githubHost string
		wantHost   string
	}{
		{name: "GHES with port", githubHost: "git.example.com:8443", wantHost: "git.example.com:8443"},
		{name: "GHE.com", githubHost: "tenant.ghe.com", wantHost: "tenant.ghe.com"},
		{name: "unstamped legacy", githubHost: "", wantHost: "github.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, err := githubPRBaseFromPR(enterprisePRBasePR(tt.githubHost), "upstream", "widgets", 42, "feature")
			if err != nil {
				t.Fatalf("githubPRBaseFromPR() error: %v", err)
			}
			wantTarget := "https://" + tt.wantHost + "/upstream/widgets.git"
			wantHead := "https://" + tt.wantHost + "/fork-owner/widgets.git"
			target, head := base.Target.TargetRepository, base.Target.HeadRepository
			if target.Host != tt.wantHost || target.RemoteURL != wantTarget ||
				head.Host != tt.wantHost || head.RemoteURL != wantHead {
				t.Fatalf("PR base repositories = target %#v, head %#v; want host %q", target, head, tt.wantHost)
			}
		})
	}
}

func TestPRBaseIdentityConversionRejectsInvalidFetchedHost(t *testing.T) {
	for _, host := range []string{"api.github.com", "user@git.example.com", "git.example.com/path"} {
		t.Run(host, func(t *testing.T) {
			if _, err := githubPRBaseFromPR(enterprisePRBasePR(host), "upstream", "widgets", 42, "feature"); err == nil {
				t.Fatalf("githubPRBaseFromPR() accepted invalid GitHub host %q", host)
			}
		})
	}
}

func enterprisePRBasePR(githubHost string) *githubpkg.PR {
	return &githubpkg.PR{
		Number: 42, RepoOwner: "upstream", RepoName: "widgets", GitHubHost: githubHost,
		HeadBranch: "feature", BaseBranch: "release",
		HeadRepoOwner: "fork-owner", HeadRepoName: "widgets",
		BaseRepoOwner: "upstream", BaseRepoName: "widgets",
	}
}

func TestPRBaseIdentityConversionPreservesLiveHeadAliases(t *testing.T) {
	pr := enterprisePRBasePR("git.example.com")
	pr.HeadRepoID, pr.HeadRepoNodeID, pr.BaseRepoID = 42, "R_verified", 99
	base, err := githubPRBaseFromPR(pr, "upstream", "widgets", 42, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if base.Target.HeadRepository.ProviderID != "42" || base.HeadRepositoryNodeID != "R_verified" || base.Target.TargetRepository.ProviderID != "99" {
		t.Fatalf("live repository identity = %#v", base)
	}
}
