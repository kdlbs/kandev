package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
)

const loopbackBrokerURL = "http://127.0.0.1:8080/api/git/credentials/resolve"

// Under the e2e profile a loopback http broker is accepted for a plugin
// executor whose repository is an e2e-local one.
func TestBrokerURLAcceptsLoopbackHTTPForE2ELocalRepositoryUnderE2EProfile(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	for _, executorType := range []models.ExecutorType{models.ExecutorTypePluginRemote, models.ExecutorTypeKubernetes} {
		if err := validateGitCredentialBrokerURLForProvider(loopbackBrokerURL, string(executorType), repoclone.ProviderE2ELocal); err != nil {
			t.Fatalf("%s: loopback broker refused for e2e-local: %v", executorType, err)
		}
	}
}

func TestBrokerURLExceptionStaysScopedToE2ELocalLoopbackUnderE2EProfile(t *testing.T) {
	t.Run("outside the e2e profile", func(t *testing.T) {
		t.Setenv("KANDEV_E2E_MOCK", "")
		err := validateGitCredentialBrokerURLForProvider(loopbackBrokerURL, string(models.ExecutorTypePluginRemote), repoclone.ProviderE2ELocal)
		if !errors.Is(err, ErrGitHubCredentialBrokerURL) {
			t.Fatalf("production profile accepted a plain-http broker: %v", err)
		}
	})
	t.Run("other providers", func(t *testing.T) {
		t.Setenv("KANDEV_E2E_MOCK", "true")
		err := validateGitCredentialBrokerURLForProvider(loopbackBrokerURL, string(models.ExecutorTypePluginRemote), "github")
		if !errors.Is(err, ErrGitHubCredentialBrokerURL) {
			t.Fatalf("a github repository accepted a plain-http broker: %v", err)
		}
	})
	t.Run("non-loopback hosts", func(t *testing.T) {
		t.Setenv("KANDEV_E2E_MOCK", "true")
		err := validateGitCredentialBrokerURLForProvider("http://broker.example/api/git/credentials/resolve", string(models.ExecutorTypePluginRemote), repoclone.ProviderE2ELocal)
		if !errors.Is(err, ErrGitHubCredentialBrokerURL) {
			t.Fatalf("a non-loopback plain-http broker was accepted: %v", err)
		}
	})
}

func e2eLocalLaunch(provider string) (*LaunchAgentRequest, *repoInfo) {
	req := &LaunchAgentRequest{
		TaskID: "task-1", WorkspaceID: "workspace-1", SessionID: "session-1",
		ExecutorType: string(models.ExecutorTypePluginRemote), Env: map[string]string{},
	}
	info := &repoInfo{RepositoryID: "repo-1", Repository: &models.Repository{
		SourceType: sourceTypeLocal, Provider: provider, ProviderOwner: "local", ProviderName: "widgets",
		RemoteURL: "http://127.0.0.1:9418/widgets.git",
	}}
	return req, info
}

// The launch path itself carries the exception: an e2e-local repository gets a
// lease through a loopback http broker, and a github one is still refused.
func TestConfigureGitCredentialBrokerIssuesLeaseForE2ELocalOverLoopbackHTTP(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, loopbackBrokerURL)
	req, info := e2eLocalLaunch(repoclone.ProviderE2ELocal)

	if err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info}); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
	}
	if issuer.calls != 1 {
		t.Fatalf("Issue calls = %d, want 1", issuer.calls)
	}

	refusedIssuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	refused := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	refused.SetGitHubCredentialBroker(refusedIssuer, loopbackBrokerURL)
	githubReq, githubInfo := e2eLocalLaunch("github")
	githubInfo.Repository.RemoteURL = "https://github.com/acme/widgets.git"
	githubInfo.Repository.ProviderOwner = "acme"
	err := refused.configureGitCredentialBrokerForRepositories(context.Background(), githubReq, []*repoInfo{githubInfo})
	if !errors.Is(err, ErrGitHubCredentialBrokerURL) || refusedIssuer.calls != 0 {
		t.Fatalf("github repository over a plain-http broker: err=%v, issue calls=%d", err, refusedIssuer.calls)
	}
}
