package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
)

func TestConfigureGitHubCredentialBrokerFallsBackToLeaseOnlyWhenReissueUnavailable(t *testing.T) {
	issuer := &fakeGitCredentialLeaseReissuer{
		fakeGitHubCredentialLeaseIssuer: fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "lease-only"}},
		reissueErr:                      gitcredentials.ErrReissueUnavailable,
	}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/github/credentials/resolve")
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypeRemoteDocker), Env: map[string]string{},
	}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{Provider: "github", ProviderOwner: "acme", ProviderName: "widgets"}}
	if err := exec.configureGitHubCredentialBroker(context.Background(), req, info); err != nil {
		t.Fatalf("configureGitHubCredentialBroker() error = %v", err)
	}
	if issuer.calls != 1 {
		t.Fatalf("lease issue calls = %d, want 1", issuer.calls)
	}
	if got := req.Env[envGitHubCredentialLease]; got != "lease-only" {
		t.Fatalf("lease env = %q, want lease-only", got)
	}
	if got := strings.TrimSpace(req.Env[githubauth.CredentialReissueCapabilityEnv]); got != "" {
		t.Fatalf("reissue capability env = %q, want empty", got)
	}
	if got := req.Env[envGitHubCredentialScopes]; strings.Contains(got, `"reissue_capability"`) {
		t.Fatalf("credential scopes contain a reissue capability: %s", got)
	}
}
