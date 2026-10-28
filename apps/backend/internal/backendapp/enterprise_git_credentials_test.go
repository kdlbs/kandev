package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/repoclone"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"testing"
)

type enterpriseCloneCredentialRecorder struct {
	host  string
	calls int
}

func (r *enterpriseCloneCredentialRecorder) ResolveGitCredentialForHost(_ context.Context, _, _, host, _, _ string) (string, string, error) {
	r.host = host
	r.calls++
	return "user", "secret", nil
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1
func TestEnterpriseCloneForwardsHost(t *testing.T) {
	recorder := &enterpriseCloneCredentialRecorder{}
	provider := repositoryCloneCredentialProvider{github: recorder}
	request := repoclone.GitCredentialRequest{WorkspaceID: "workspace", Provider: "github", ProviderHost: "https://git.company.example", CloneURL: "https://git.company.example/acme/repo.git", Owner: "acme", Name: "repo"}
	if _, _, err := provider.ResolveGitCredential(context.Background(), request); err != nil || recorder.host != "git.company.example" {
		t.Fatalf("clone host = %q, err=%v", recorder.host, err)
	}
	recorder.calls = 0
	for _, cloneURL := range []string{"https://foreign.example/acme/repo.git", "https://git.company.example/other/repo.git"} {
		request.CloneURL = cloneURL
		if _, _, err := provider.ResolveGitCredential(context.Background(), request); err == nil || recorder.calls != 0 {
			t.Fatalf("out-of-scope clone reached credential resolver: %s", cloneURL)
		}
	}
}

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1
func TestEnterpriseContributionCredentialScope(t *testing.T) {
	host := "git.company.example"
	destination := taskmodels.ContributionDestination{Version: taskmodels.ContributionDestinationVersion, Provider: taskmodels.ContributionDestinationProviderGitHub,
		SourceRepository: taskmodels.ContributionDestinationRepository{Host: host, Path: "acme/repo", ProviderID: "100", RemoteURL: "https://" + host + "/acme/repo.git"},
		TargetRepository: taskmodels.ContributionDestinationRepository{Host: host, Path: "bot/repo", ProviderID: "200", RemoteURL: "https://" + host + "/bot/repo.git"}}
	metadata := map[string]interface{}{}
	if err := taskmodels.PutContributionDestination(metadata, &destination); err != nil {
		t.Fatal(err)
	}
	repository := &fakeGitHubBrokerTaskRepository{task: &taskmodels.Task{ID: "task", WorkspaceID: "workspace"}, session: &taskmodels.TaskSession{ID: "session", TaskID: "task", State: taskmodels.TaskSessionStateRunning}, repository: &taskmodels.Repository{ID: "repo", WorkspaceID: "workspace", Provider: "github", ProviderHost: "https://" + host, ProviderOwner: "acme", ProviderName: "repo", ProviderRepoID: "100"}, links: []*taskmodels.TaskRepository{{TaskID: "task", RepositoryID: "repo", Metadata: metadata}}}
	verifier := &fakeContributionDestinationVerifier{}
	authorizer := &githubBrokerScopeAuthorizer{repo: repository, provider: verifier}
	scope := gitcredentials.Scope{ProviderID: "github", WorkspaceID: "workspace", TaskID: "task", SessionID: "session", RepositoryID: "repo", Host: host, Path: "/bot/repo.git", IdentityProviderID: "200", ParentProviderID: "100"}
	if err := authorizer.AuthorizeGitCredential(context.Background(), scope); err != nil {
		t.Fatalf("Enterprise fork denied: %v", err)
	}
	if verifier.calls != 1 {
		t.Fatal("provider verification was skipped")
	}
	scope.Host = "foreign.company.example"
	if err := authorizer.AuthorizeGitCredential(context.Background(), scope); err == nil {
		t.Fatal("foreign host accepted")
	}
}
