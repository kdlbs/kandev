package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const loopbackBrokerURL = "http://127.0.0.1:8080/api/git/credentials/resolve"

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

// Under the e2e profile an e2e-local repository needs no credential, so the
// launch issues no lease and adds no broker environment, whatever the broker
// URL; a github repository over a plain-http broker is still refused.
func TestConfigureGitCredentialBrokerSkipsLeaseForE2ELocalUnderE2EProfile(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, loopbackBrokerURL)
	req, info := e2eLocalLaunch(repoclone.ProviderE2ELocal)

	if err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info}); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories() error = %v", err)
	}
	if issuer.calls != 0 {
		t.Fatalf("Issue calls = %d, want 0", issuer.calls)
	}
	if len(req.Env) != 0 {
		t.Fatalf("e2e-local launch gained credential env: %v", req.Env)
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

// Outside the e2e profile the provider is not special: the launch does not skip
// it and the loopback remote is refused by the credentialed path.
func TestConfigureGitCredentialBrokerDoesNotSkipE2ELocalOutsideE2EProfile(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "")
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetGitHubCredentialBroker(issuer, loopbackBrokerURL)
	req, info := e2eLocalLaunch(repoclone.ProviderE2ELocal)

	err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info})
	if err == nil {
		t.Fatal("production profile skipped credential handling for an e2e-local repository")
	}
}

type pluginCloneCapabilities struct{ mockCapabilities }

func (*pluginCloneCapabilities) RequiresCloneURL(executorType string) bool {
	return executorType == string(models.ExecutorTypePluginRemote)
}

// Launch preparation for an e2e-local repository on a plugin executor: the
// clone URL is the repository's file:// remote and no credential lease or
// broker environment is added, so nothing between the stored repository and the
// launch request refuses or decorates it.
func TestPluginLaunchPreparationCarriesE2ELocalRemoteWithoutCredentials(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	const remote = "file:///tmp/repos/widgets.git"
	issuer := &fakeGitHubCredentialLeaseIssuer{lease: gitcredentials.Lease{Token: "opaque-lease"}}
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.capabilities = &pluginCloneCapabilities{}
	exec.SetGitHubCredentialBroker(issuer, loopbackBrokerURL)
	req, info := e2eLocalLaunch(repoclone.ProviderE2ELocal)
	info.Repository.RemoteURL = remote
	execConfig := executorConfig{ExecutorID: "exec-1", ExecutorType: string(models.ExecutorTypePluginRemote)}

	metadata, err := exec.applyRepositoryConfig(req, &v1.Task{ID: "task-1", WorkspaceID: "workspace-1"}, info, execConfig, nil)
	if err != nil {
		t.Fatalf("applyRepositoryConfig(): %v", err)
	}
	if err := exec.configureGitCredentialBrokerForRepositories(context.Background(), req, []*repoInfo{info}); err != nil {
		t.Fatalf("configureGitCredentialBrokerForRepositories(): %v", err)
	}
	if req.RepositoryURL != remote || metadata["repository_clone_url"] != remote {
		t.Fatalf("clone url = %q, metadata = %v, want %s", req.RepositoryURL, metadata["repository_clone_url"], remote)
	}
	if issuer.calls != 0 || len(req.Env) != 0 {
		t.Fatalf("lease issues = %d, env = %v; want none", issuer.calls, req.Env)
	}
}
