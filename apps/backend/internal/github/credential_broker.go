package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/gitcredentials"
)

const gitHubTokenUsername = "x-access-token"

var (
	ErrCredentialLeaseInvalid = gitcredentials.ErrLeaseInvalid
	ErrCredentialLeaseExpired = gitcredentials.ErrLeaseExpired
	ErrCredentialLeaseRevoked = gitcredentials.ErrLeaseRevoked
	ErrCredentialLeaseLimit   = gitcredentials.ErrLeaseLimit
	ErrCredentialScopeDenied  = gitcredentials.ErrScopeDenied
)

// BrokerScopeAuthorizer verifies task/workspace/repository ownership. It is
// called both when a lease is issued and each time it is redeemed.
type BrokerScopeAuthorizer interface {
	AuthorizeGitHubRepository(
		ctx context.Context,
		workspaceID, taskID, sessionID, repositoryID, owner, repo string,
	) error
}

type CredentialLeaseRequest struct {
	WorkspaceID  string
	TaskID       string
	SessionID    string
	RepositoryID string
	Owner        string
	Repo         string
	Host         string
	TTL          time.Duration
}

type CredentialLease struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BrokerCredentialRequest struct {
	Lease        string
	TaskID       string
	SessionID    string
	RepositoryID string
	Owner        string
	Repo         string
	Host         string
	Path         string
}

type BrokerCredential struct {
	Username  string        `json:"username"`
	Password  string        `json:"password"`
	ExpiresAt time.Time     `json:"expires_at,omitempty"`
	Principal AuthPrincipal `json:"principal"`
}

// CredentialBroker is GitHub's compatibility adapter over the generic
// broker. GitHub-specific connection and App-generation checks stay in its
// resolver; lease mechanics live in internal/gitcredentials.
type CredentialBroker struct {
	broker *gitcredentials.Broker
}

func NewCredentialBroker(
	connections workspaceConnectionReader,
	resolver *CredentialResolver,
	authorizer BrokerScopeAuthorizer,
) *CredentialBroker {
	provider := &gitHubCredentialProvider{connections: connections, resolver: resolver}
	return &CredentialBroker{broker: gitcredentials.NewBroker(provider, gitHubScopeAuthorizer{authorizer: authorizer})}
}

// NewCredentialBrokerFromBroker binds GitHub's existing HTTP controller to a
// provider-neutral broker composed by backendapp.
func NewCredentialBrokerFromBroker(broker *gitcredentials.Broker) *CredentialBroker {
	return &CredentialBroker{broker: broker}
}

// GitCredentialResolver exposes GitHub's live connection and generation
// checks as one provider-neutral resolver. Returned credentials retain their
// GitHub-specific formatting while generic lease state stays outside this pkg.
func (s *Service) GitCredentialResolver() gitcredentials.Resolver {
	if s == nil {
		return nil
	}
	return &gitHubCredentialProvider{connections: s.store, resolver: s.resolver}
}

func (b *CredentialBroker) Issue(ctx context.Context, req CredentialLeaseRequest) (*CredentialLease, error) {
	if b == nil || b.broker == nil {
		return nil, ErrGitHubNotConfigured
	}
	lease, err := b.broker.Issue(ctx, gitcredentials.Scope{
		ProviderID: githubProviderName, WorkspaceID: req.WorkspaceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: req.RepositoryID, Host: req.Host, Path: githubCredentialPath(req.Owner, req.Repo), TTL: req.TTL,
	})
	if err != nil {
		return nil, err
	}
	return &CredentialLease{Token: lease.Token, ExpiresAt: lease.ExpiresAt}, nil
}

func (b *CredentialBroker) Resolve(ctx context.Context, req BrokerCredentialRequest) (*BrokerCredential, error) {
	if b == nil || b.broker == nil {
		return nil, ErrGitHubNotConfigured
	}
	path := req.Path
	if strings.TrimSpace(path) == "" {
		path = githubCredentialPath(req.Owner, req.Repo)
	}
	credential, err := b.broker.Redeem(ctx, gitcredentials.Redemption{
		Lease: req.Lease, TaskID: req.TaskID, SessionID: req.SessionID, RepositoryID: req.RepositoryID,
		Host: req.Host, Path: path,
	})
	if err != nil {
		return nil, err
	}
	principal, _ := credential.Metadata.(AuthPrincipal)
	return &BrokerCredential{
		Username: credential.Username, Password: credential.Password, ExpiresAt: credential.ExpiresAt, Principal: principal,
	}, nil
}

func (b *CredentialBroker) RevokeTask(taskID string) {
	if b != nil && b.broker != nil {
		b.broker.RevokeTask(taskID)
	}
}

func (b *CredentialBroker) RevokeSession(sessionID string) {
	if b != nil && b.broker != nil {
		b.broker.RevokeSession(sessionID)
	}
}

func (b *CredentialBroker) RevokeWorkspace(workspaceID string) {
	if b != nil && b.broker != nil {
		b.broker.RevokeWorkspace(workspaceID)
	}
}

func (b *CredentialBroker) ActiveLeaseCount() int {
	if b == nil || b.broker == nil {
		return 0
	}
	return b.broker.ActiveLeaseCount()
}

type gitHubScopeAuthorizer struct{ authorizer BrokerScopeAuthorizer }

func (a gitHubScopeAuthorizer) AuthorizeGitCredential(ctx context.Context, scope gitcredentials.Scope) error {
	if a.authorizer == nil {
		return ErrGitHubNotConfigured
	}
	owner, repo, err := githubOwnerRepo(scope.Path)
	if err != nil {
		return err
	}
	return a.authorizer.AuthorizeGitHubRepository(
		ctx, scope.WorkspaceID, scope.TaskID, scope.SessionID, scope.RepositoryID, owner, repo,
	)
}

type gitHubCredentialProvider struct {
	connections workspaceConnectionReader
	resolver    *CredentialResolver
}

func (*gitHubCredentialProvider) Supports(providerID string) bool {
	return strings.EqualFold(strings.TrimSpace(providerID), githubProviderName)
}

func (p *gitHubCredentialProvider) Binding(ctx context.Context, scope gitcredentials.Scope) (string, error) {
	if !strings.EqualFold(scope.Host, defaultGitHubHost) {
		return "", fmt.Errorf("%w: unsupported host", ErrCredentialScopeDenied)
	}
	connection, appGeneration, err := p.issueConnection(ctx, scope.WorkspaceID)
	if err != nil {
		return "", err
	}
	return githubCredentialBindingFor(connection, appGeneration)
}

func (p *gitHubCredentialProvider) Resolve(ctx context.Context, scope gitcredentials.Scope) (gitcredentials.Credential, error) {
	if p == nil || p.resolver == nil {
		return gitcredentials.Credential{}, ErrGitHubNotConfigured
	}
	owner, repo, err := githubOwnerRepo(scope.Path)
	if err != nil {
		return gitcredentials.Credential{}, err
	}
	resolved, err := p.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: scope.WorkspaceID, Purpose: CredentialPurposeGitTransport, RepoOwner: owner, RepoName: repo,
	})
	if err != nil {
		return gitcredentials.Credential{}, err
	}
	if resolved == nil || strings.TrimSpace(resolved.credential) == "" {
		return gitcredentials.Credential{}, ErrCredentialLeaseRevoked
	}
	if resolved.Principal.Kind == AuthPrincipalApp && !resolved.Capabilities[CapabilityGitRead] {
		return gitcredentials.Credential{}, fmt.Errorf("%w: %s", ErrGitHubCapabilityDenied, CapabilityGitRead)
	}
	return gitcredentials.Credential{
		Username: gitHubTokenUsername, Password: resolved.credential, ExpiresAt: resolved.ExpiresAt, Metadata: resolved.Principal,
	}, nil
}

func (p *gitHubCredentialProvider) issueConnection(ctx context.Context, workspaceID string) (*WorkspaceConnection, int64, error) {
	if p == nil || p.connections == nil || p.resolver == nil {
		return nil, 0, ErrGitHubNotConfigured
	}
	connection, err := p.connections.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return nil, 0, fmt.Errorf("load GitHub workspace connection: %w", err)
	}
	if connection == nil {
		return nil, 0, ErrGitHubNotConfigured
	}
	if connection.Status != ConnectionStatusActive {
		return nil, 0, ErrGitHubConnectionInvalid
	}
	if connection.Source != ConnectionSourceGitHubAppInstallation {
		return connection, 0, nil
	}
	if strings.TrimSpace(connection.AppRegistrationID) == "" {
		return nil, 0, ErrGitHubNotConfigured
	}
	generation, err := p.resolver.appCredentialGeneration(connection.AppRegistrationID)
	if err != nil {
		return nil, 0, err
	}
	return connection, generation, nil
}

type githubCredentialBinding struct {
	CredentialGeneration    int64  `json:"credential_generation"`
	AppRegistrationID       string `json:"app_registration_id"`
	AppCredentialGeneration int64  `json:"app_credential_generation"`
}

func githubCredentialBindingFor(connection *WorkspaceConnection, appGeneration int64) (string, error) {
	if connection == nil {
		return "", ErrGitHubNotConfigured
	}
	encoded, err := json.Marshal(githubCredentialBinding{
		CredentialGeneration: connection.CredentialGeneration, AppRegistrationID: connection.AppRegistrationID,
		AppCredentialGeneration: appGeneration,
	})
	if err != nil {
		return "", fmt.Errorf("encode GitHub credential binding: %w", err)
	}
	return string(encoded), nil
}

func githubCredentialPath(owner, repo string) string {
	return "/" + strings.Trim(strings.TrimSpace(owner), "/") + "/" + strings.Trim(strings.TrimSpace(repo), "/") + ".git"
}

func githubOwnerRepo(path string) (string, string, error) {
	trimmed := strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, repo, found := strings.Cut(trimmed, "/")
	if !found || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("%w: GitHub repository path is invalid", ErrCredentialScopeDenied)
	}
	return owner, repo, nil
}
