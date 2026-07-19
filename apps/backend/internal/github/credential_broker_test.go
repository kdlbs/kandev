package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeBrokerAuthorizer struct {
	err        error
	calls      int
	sessionIDs []string
}

type fakeBrokerInstallationProvider struct {
	resolved *ResolvedCredential
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
	for _, record := range broker.leases {
		if record.TaskID != "task-1" {
			t.Fatalf("record = %+v", record)
		}
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

func TestCredentialBrokerClampsRequestedLeaseTTL(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	now := time.Now().UTC()
	broker.now = func() time.Time { return now }
	req := brokerLeaseRequest()
	req.TTL = 30 * 24 * time.Hour

	lease, err := broker.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if want := now.Add(defaultCredentialLeaseTTL); !lease.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want capped expiry %v", lease.ExpiresAt, want)
	}
}

func TestCredentialBrokerRenewsLeaseOnSuccessfulRedemption(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	issuedAt := time.Now().UTC()
	now := issuedAt
	broker.now = func() time.Time { return now }
	req := brokerLeaseRequest()
	req.TTL = time.Minute

	lease, err := broker.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	now = issuedAt.Add(30 * time.Second)
	if _, err := broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token)); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}

	// This is after the original expiry, but still inside the renewed window.
	now = issuedAt.Add(75 * time.Second)
	if _, err := broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token)); err != nil {
		t.Fatalf("Resolve after original expiry: %v", err)
	}
}

func TestCredentialBrokerSweepsExpiredLeasesWhenIssuing(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	issuedAt := time.Now().UTC()
	now := issuedAt
	broker.now = func() time.Time { return now }
	req := brokerLeaseRequest()
	req.TTL = time.Minute

	if _, err := broker.Issue(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	now = issuedAt.Add(2 * time.Minute)
	req.TaskID = "task-2"
	req.SessionID = "session-2"
	if _, err := broker.Issue(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	if got := len(broker.leases); got != 1 {
		t.Fatalf("lease records = %d, want only the active lease", got)
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

func TestCredentialBrokerExpiryAndExplicitRevocation(t *testing.T) {
	broker, _, _ := newPATCredentialBroker(t)
	now := time.Now().UTC()
	broker.now = func() time.Time { return now }
	req := brokerLeaseRequest()
	req.TTL = time.Minute
	lease, err := broker.Issue(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	broker.now = func() time.Time { return now.Add(2 * time.Minute) }
	_, err = broker.Resolve(context.Background(), brokerCredentialRequest(lease.Token))
	if !errors.Is(err, ErrCredentialLeaseExpired) {
		t.Fatalf("error = %v, want expired", err)
	}

	broker.now = func() time.Time { return now }
	lease, err = broker.Issue(context.Background(), req)
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
		Status: ConnectionStatusActive, CredentialGeneration: 7,
	}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		connection.WorkspaceID: connection,
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetInstallationProvider(fakeBrokerInstallationProvider{resolved: &ResolvedCredential{
		Client: &MockClient{}, Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			WorkspaceID: connection.WorkspaceID,
		},
		Capabilities: map[GitHubAppCapability]bool{
			CapabilityGitRead: true,
		},
		CredentialGeneration: 7,
		credential:           "installation-token",
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
		Status: ConnectionStatusActive, CredentialGeneration: 7,
	}
	connections := &fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{
		connection.WorkspaceID: connection,
	}}
	resolver := NewCredentialResolver(connections, nil)
	resolver.SetInstallationProvider(fakeBrokerInstallationProvider{resolved: &ResolvedCredential{
		Client: &MockClient{}, Principal: AuthPrincipal{
			Kind: AuthPrincipalApp, Source: ConnectionSourceGitHubAppInstallation,
			WorkspaceID: connection.WorkspaceID,
		},
		Capabilities:         map[GitHubAppCapability]bool{},
		CredentialGeneration: 7,
		credential:           "installation-token",
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
