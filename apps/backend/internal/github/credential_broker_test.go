package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/gitcredentials"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeBrokerAuthorizer struct {
	err                 error
	calls               int
	sessionIDs          []string
	identityProviderIDs []string
}

type fakeBrokerInstallationProvider struct {
	resolved *ResolvedCredential
}

type blockingBrokerInstallationProvider struct {
	started  chan struct{}
	release  chan struct{}
	resolved *ResolvedCredential
}

func (p *blockingBrokerInstallationProvider) AppCredentialGeneration(
	registrationID string,
) (int64, bool) {
	return p.resolved.AppCredentialGeneration,
		p.resolved.AppRegistrationID == registrationID && p.resolved.AppCredentialGeneration > 0
}

func (p *blockingBrokerInstallationProvider) ResolveInstallation(
	ctx context.Context,
	_ *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	select {
	case p.started <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-p.release:
		return p.resolved, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f fakeBrokerInstallationProvider) AppCredentialGeneration(registrationID string) (int64, bool) {
	return f.resolved.AppCredentialGeneration,
		f.resolved.AppRegistrationID == registrationID && f.resolved.AppCredentialGeneration > 0
}

func (f fakeBrokerInstallationProvider) ResolveInstallation(
	_ context.Context,
	_ *WorkspaceConnection,
	_ ResolveCredentialRequest,
) (*ResolvedCredential, error) {
	return f.resolved, nil
}

func (a *fakeBrokerAuthorizer) AuthorizeGitHubRepository(
	_ context.Context,
	_, _, sessionID, _, _, _ string,
) error {
	a.calls++
	a.sessionIDs = append(a.sessionIDs, sessionID)
	return a.err
}

func (a *fakeBrokerAuthorizer) AuthorizeGitHubRepositoryWithIdentity(
	ctx context.Context,
	workspaceID, taskID, sessionID, repositoryID, owner, repo, providerID, parentProviderID string,
) error {
	a.identityProviderIDs = append(a.identityProviderIDs, providerID+"/"+parentProviderID)
	return a.AuthorizeGitHubRepository(ctx, workspaceID, taskID, sessionID, repositoryID, owner, repo)
}

func newPATCredentialBroker(t *testing.T) (*CredentialBroker, *WorkspaceConnection, *fakeBrokerAuthorizer) {
	t.Helper()
	connection := &WorkspaceConnection{
		WorkspaceID:          "workspace-1",
		Source:               ConnectionSourcePAT,
		Login:                "octocat",
		Status:               ConnectionStatusActive,
		CredentialGeneration: 7,
	}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		connection.WorkspaceID: connection,
	}}
	resolver := NewCredentialResolver(connections, fakeAuthSecrets{
		WorkspacePATSecretKey(connection.WorkspaceID): "workspace-secret",
	})
	authorizer := &fakeBrokerAuthorizer{}
	return NewCredentialBroker(connections, resolver, authorizer), connection, authorizer
}

func brokerLeaseRequest() CredentialLeaseRequest {
	return CredentialLeaseRequest{
		WorkspaceID:  "workspace-1",
		TaskID:       "task-1",
		SessionID:    "session-1",
		RepositoryID: "repository-1",
		Owner:        "kdlbs",
		Repo:         "kandev",
		Host:         "github.com",
	}
}

func brokerCredentialRequest(lease string) BrokerCredentialRequest {
	return BrokerCredentialRequest{
		Lease: lease, TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Owner: "kdlbs", Repo: "kandev", Host: "github.com",
	}
}

func TestCredentialBrokerStoresHashAndRenewsCredentialOnRedemption(t *testing.T) {
	broker, _, authorizer := newPATCredentialBroker(t)
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if lease.Token == "" {
		t.Fatal("empty lease")
	}
	if broker.ActiveLeaseCount() != 1 {
		t.Fatalf("active leases = %d, want 1", broker.ActiveLeaseCount())
	}
	credential, err := broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if credential.Password != "workspace-secret" || credential.Username != "x-access-token" {
		t.Fatalf("credential = %+v", credential)
	}
	if authorizer.calls != 2 {
		t.Fatalf("scope checks = %d, want 2", authorizer.calls)
	}
	if got := authorizer.sessionIDs; len(got) != 2 || got[0] != "session-1" || got[1] != "session-1" {
		t.Fatalf("authorized sessions = %v, want session-1 twice", got)
	}
}

func TestCredentialBrokerLimitsActiveLeasesPerWorkspace(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	const maxExpectedLeases = 10_000
	request := brokerLeaseRequest()
	for i := range maxExpectedLeases {
		request.TaskID = fmt.Sprintf("task-%d", i)
		request.SessionID = fmt.Sprintf("session-%d", i)
		if _, err := broker.Issue(context.Background(), request); err != nil {
			t.Fatalf("Issue(%d): %v", i, err)
		}
	}

	request.TaskID = "task-over-limit"
	request.SessionID = "session-over-limit"
	if _, err := broker.Issue(context.Background(), request); err == nil {
		t.Fatal("Issue() succeeded after the workspace lease limit")
	}
}

func TestCredentialBrokerRejectsScopeMismatch(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := brokerCredentialRequest(lease.Token)
	request.TaskID = "other-task"
	_, err = broker.Resolve(context.Background(), request)
	if !errors.Is(err, ErrCredentialScopeDenied) {
		t.Fatalf("error = %v, want scope denied", err)
	}
}

func TestCredentialBrokerGenerationChangeRevokesLease(t *testing.T) {
	broker, connection, _ := newPATCredentialBroker(t)
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatal(err)
	}
	connection.CredentialGeneration++
	_, err = broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token))
	if !errors.Is(err, ErrCredentialLeaseRevoked) {
		t.Fatalf("error = %v, want revoked", err)
	}
}

func TestCredentialBrokerRejectsDestinationLeaseForChangedWorkspaceConnection(t *testing.T) {
	broker, connection, authorizer := newPATCredentialBroker(t)
	binding := &taskmodels.ContributionDestinationCredentialBinding{
		Source:               string(ConnectionSourcePAT),
		Login:                "octocat",
		CredentialGeneration: 7,
	}
	req := brokerLeaseRequest()
	req.ProviderID = "200"
	req.ParentProviderID = "100"
	req.DestinationBinding = binding

	lease, err := broker.Issue(context.Background(), req)
	if err != nil {
		t.Fatalf("Issue with matching binding: %v", err)
	}
	if got := authorizer.identityProviderIDs; len(got) != 1 || got[0] != "200/100" {
		t.Fatalf("issued identity checks = %v, want [200/100]", got)
	}
	connection.Login = "different-user"
	connection.CredentialGeneration++

	if _, err := broker.Issue(context.Background(), req); !errors.Is(err, ErrCredentialScopeDenied) {
		t.Fatalf("Issue with changed connection error = %v, want scope denial", err)
	}
	redemption := brokerCredentialRequest(lease.Token)
	redemption.ProviderID = "200"
	redemption.ParentProviderID = "100"
	if _, err := broker.Resolve(context.Background(), redemption); !errors.Is(err, ErrCredentialLeaseRevoked) {
		t.Fatalf("Resolve with changed connection error = %v, want lease revoked", err)
	}
}

func TestCredentialBrokerReissueRefreshesDestinationBindingGeneration(t *testing.T) {
	broker, connection, _ := newPATCredentialBroker(t)
	signer, err := gitcredentials.NewReissueCapabilitySigner("stable-test-key")
	if err != nil {
		t.Fatalf("NewReissueCapabilitySigner() error = %v", err)
	}
	broker.broker.SetReissueCapabilitySigner(signer)
	binding, err := json.Marshal(taskmodels.ContributionDestinationCredentialBinding{
		Source: string(ConnectionSourcePAT), Login: "octocat", CredentialGeneration: connection.CredentialGeneration,
	})
	if err != nil {
		t.Fatalf("marshal destination binding: %v", err)
	}
	scope := gitcredentials.Scope{
		ProviderID: "github", WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		RepositoryID: "repository-1", Host: "github.com", Path: "/kdlbs/kandev.git",
		CredentialBinding: string(binding),
	}
	_, capability, err := broker.broker.IssueWithReissueCapability(t.Context(), scope)
	if err != nil {
		t.Fatalf("IssueWithReissueCapability() error = %v", err)
	}
	connection.CredentialGeneration++
	reissued, err := broker.broker.Reissue(t.Context(), gitcredentials.ReissueRequest{
		Capability: capability.Token, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1",
		Host: "github.com", Path: "/kdlbs/kandev.git",
	})
	if err != nil {
		t.Fatalf("Reissue() after destination connection rotation error = %v", err)
	}
	if _, err := broker.broker.Redeem(t.Context(), gitcredentials.Redemption{
		Lease: reissued.Token, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1",
		Host: "github.com", Path: "/kdlbs/kandev.git",
	}); err != nil {
		t.Fatalf("Redeem() reissued destination lease error = %v", err)
	}
}

func TestCredentialBrokerAppRegistrationChangeRevokesLease(t *testing.T) {
	installationID := int64(42)
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, AppRegistrationID: "registration-a",
		Status: ConnectionStatusActive, CredentialGeneration: 7,
	}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		connection.WorkspaceID: connection,
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetInstallationProvider(fakeBrokerInstallationProvider{resolved: &ResolvedCredential{
		Client: &MockClient{}, Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			AppRegistrationID: "registration-a", AppCredentialGeneration: 3,
		},
		Capabilities:      map[GitHubAppCapability]bool{CapabilityGitRead: true},
		AppRegistrationID: "registration-a", AppCredentialGeneration: 3,
		credential: "installation-token",
	}})
	broker := NewCredentialBroker(connections, resolver, &fakeBrokerAuthorizer{})
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatal(err)
	}

	connection.AppRegistrationID = "registration-b"
	_, err = broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token))
	if !errors.Is(err, ErrCredentialLeaseRevoked) {
		t.Fatalf("error = %v, want revoked", err)
	}
}

func TestCredentialBrokerDoesNotReturnCredentialAfterConcurrentAppSwitch(t *testing.T) {
	installationID := int64(42)
	connection := WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		InstallationID: &installationID, AppRegistrationID: "registration-a",
		Status: ConnectionStatusActive, CredentialGeneration: 7,
	}
	connections := &synchronizedConnectionReader{connection: connection}
	provider := &blockingBrokerInstallationProvider{
		started: make(chan struct{}), release: make(chan struct{}),
		resolved: &ResolvedCredential{
			Client: &MockClient{}, Principal: AuthPrincipal{
				Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
				AppRegistrationID: "registration-a", AppCredentialGeneration: 3,
			},
			Capabilities:         map[GitHubAppCapability]bool{CapabilityGitRead: true},
			CredentialGeneration: 7, AppRegistrationID: "registration-a",
			AppCredentialGeneration: 3, credential: "old-installation-token",
		},
	}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetInstallationProvider(provider)
	broker := NewCredentialBroker(connections, resolver, &fakeBrokerAuthorizer{})
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan *BrokerCredential, 1)
	errs := make(chan error, 1)
	go func() {
		credential, resolveErr := broker.Resolve(
			context.Background(), brokerCredentialRequest(lease.Token),
		)
		result <- credential
		errs <- resolveErr
	}()
	<-provider.started
	replacement := connection
	replacement.AppRegistrationID = "registration-b"
	replacement.CredentialGeneration++
	connections.replace(replacement)
	close(provider.release)

	if credential, resolveErr := <-result, <-errs; credential != nil ||
		!errors.Is(resolveErr, ErrCredentialLeaseRevoked) {
		t.Fatalf("Resolve() = %+v, %v; want nil, ErrCredentialLeaseRevoked", credential, resolveErr)
	}
}

func TestCredentialBrokerExpiryAndExplicitRevocation(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	req := brokerLeaseRequest()
	lease, err := broker.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	broker.RevokeTask("task-1")
	_, err = broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token))
	if !errors.Is(err, ErrCredentialLeaseInvalid) {
		t.Fatalf("error = %v, want invalid after revoke", err)
	}
}

func TestCredentialBrokerNeverReturnsPersonalCredential(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatal(err)
	}
	credential, err := broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token))
	if err != nil {
		t.Fatal(err)
	}
	if credential.Principal.UserID != "" || credential.Principal.Source != ConnectionSourcePAT {
		t.Fatalf("principal = %+v", credential.Principal)
	}
}

func TestCredentialBrokerAllowsReadOnlyAppForGitTransport(t *testing.T) {
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		AppRegistrationID: "registration-a",
		Status:            ConnectionStatusActive, CredentialGeneration: 7,
	}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		connection.WorkspaceID: connection,
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetInstallationProvider(fakeBrokerInstallationProvider{resolved: &ResolvedCredential{
		Client: &MockClient{}, Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			WorkspaceID: connection.WorkspaceID, AppRegistrationID: "registration-a",
			AppCredentialGeneration: 3,
		},
		Capabilities: map[GitHubAppCapability]bool{
			CapabilityGitRead: true,
		},
		CredentialGeneration:    7,
		AppRegistrationID:       "registration-a",
		AppCredentialGeneration: 3,
		credential:              "installation-token",
	}})
	broker := NewCredentialBroker(connections, resolver, &fakeBrokerAuthorizer{})
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatal(err)
	}

	credential, err := broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if credential.Password != "installation-token" {
		t.Fatalf("credential password = %q", credential.Password)
	}
}

func TestCredentialBrokerRejectsAppWithoutGitRead(t *testing.T) {
	connection := &WorkspaceConnection{
		WorkspaceID: "workspace-1", Source: ConnectionSourceGitHubAppInstallation,
		AppRegistrationID: "registration-a",
		Status:            ConnectionStatusActive, CredentialGeneration: 7,
	}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		connection.WorkspaceID: connection,
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetInstallationProvider(fakeBrokerInstallationProvider{resolved: &ResolvedCredential{
		Client: &MockClient{}, Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			WorkspaceID: connection.WorkspaceID, AppRegistrationID: "registration-a",
			AppCredentialGeneration: 3,
		},
		Capabilities:            map[GitHubAppCapability]bool{},
		CredentialGeneration:    7,
		AppRegistrationID:       "registration-a",
		AppCredentialGeneration: 3,
		credential:              "installation-token",
	}})
	broker := NewCredentialBroker(connections, resolver, &fakeBrokerAuthorizer{})
	lease, err := broker.Issue(context.Background(), brokerLeaseRequest())
	if err != nil {
		t.Fatal(err)
	}

	_, err = broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token))
	if !errors.Is(err, ErrGitHubCapabilityDenied) {
		t.Fatalf("Resolve() error = %v, want capability denied", err)
	}
}
