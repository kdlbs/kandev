package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/gitcredentials"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const gitHubTokenUsername = "x-access-token"

var (
	ErrCredentialLeaseInvalid             = gitcredentials.ErrLeaseInvalid
	ErrCredentialLeaseExpired             = gitcredentials.ErrLeaseExpired
	ErrCredentialLeaseRevoked             = gitcredentials.ErrLeaseRevoked
	ErrCredentialLeaseLimit               = gitcredentials.ErrLeaseLimit
	ErrCredentialScopeDenied              = gitcredentials.ErrScopeDenied
	ErrCredentialReissueCapabilityInvalid = gitcredentials.ErrReissueCapabilityInvalid
	ErrCredentialReissueCapabilityExpired = gitcredentials.ErrReissueCapabilityExpired
)

// BrokerScopeAuthorizer verifies task/workspace/repository ownership. It is
// called both when a lease is issued and each time it is redeemed.
type BrokerScopeAuthorizer interface {
	AuthorizeGitHubRepository(
		ctx context.Context,
		workspaceID, taskID, sessionID, repositoryID, owner, repo string,
	) error
}

// BrokerScopeAuthorizerWithIdentity extends the task scope check with the
// provider identity that was used to issue the lease. The optional interface
// keeps legacy test and compatibility authorizers source-compatible while the
// production authorizer enforces stable repository IDs.
type BrokerScopeAuthorizerWithIdentity interface {
	AuthorizeGitHubRepositoryWithIdentity(
		ctx context.Context,
		workspaceID, taskID, sessionID, repositoryID, owner, repo, providerID, parentProviderID string,
	) error
}

type CredentialLeaseRequest struct {
	WorkspaceID        string
	TaskID             string
	SessionID          string
	RepositoryID       string
	Owner              string
	Repo               string
	Host               string
	ProviderID         string
	ParentProviderID   string
	DestinationBinding *taskmodels.ContributionDestinationCredentialBinding
	TTL                time.Duration
}

type CredentialLease struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type BrokerCredentialRequest struct {
	Lease            string
	TaskID           string
	SessionID        string
	RepositoryID     string
	Owner            string
	Repo             string
	Host             string
	Path             string
	ProviderID       string
	ParentProviderID string
}

// CredentialLeaseReissueRequest is authorized only by its opaque execution
// capability; it deliberately has no lease field.
type CredentialLeaseReissueRequest struct {
	Capability       string
	TaskID           string
	SessionID        string
	RepositoryID     string
	Owner            string
	Repo             string
	Host             string
	Path             string
	ProviderID       string
	ParentProviderID string
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
	scope := gitcredentials.Scope{
		ProviderID: githubProviderName, WorkspaceID: req.WorkspaceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: req.RepositoryID, Host: req.Host, Path: githubCredentialPath(req.Owner, req.Repo), TTL: req.TTL,
		IdentityProviderID: req.ProviderID, ParentProviderID: req.ParentProviderID,
	}
	if req.DestinationBinding != nil {
		encodedBinding, err := json.Marshal(req.DestinationBinding)
		if err != nil {
			return nil, fmt.Errorf("encode contribution destination credential binding: %w", err)
		}
		scope.CredentialBinding = string(encodedBinding)
	}
	lease, err := b.broker.Issue(ctx, scope)
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
		Host: req.Host, Path: path, IdentityProviderID: req.ProviderID, ParentProviderID: req.ParentProviderID,
	})
	if err != nil {
		return nil, err
	}
	principal, _ := credential.Metadata.(AuthPrincipal)
	return &BrokerCredential{
		Username: credential.Username, Password: credential.Password, ExpiresAt: credential.ExpiresAt, Principal: principal,
	}, nil
}

func (b *CredentialBroker) Reissue(ctx context.Context, req CredentialLeaseReissueRequest) (*CredentialLease, error) {
	if b == nil || b.broker == nil {
		return nil, ErrGitHubNotConfigured
	}
	path := req.Path
	if strings.TrimSpace(path) == "" {
		path = githubCredentialPath(req.Owner, req.Repo)
	}
	lease, err := b.broker.Reissue(ctx, gitcredentials.ReissueRequest{
		Capability: req.Capability, TaskID: req.TaskID, SessionID: req.SessionID,
		RepositoryID: req.RepositoryID, Host: req.Host, Path: path,
		IdentityProviderID: req.ProviderID, ParentProviderID: req.ParentProviderID,
	})
	if err != nil {
		return nil, err
	}
	return &CredentialLease{Token: lease.Token, ExpiresAt: lease.ExpiresAt}, nil
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
	if identityAuthorizer, ok := a.authorizer.(BrokerScopeAuthorizerWithIdentity); ok {
		return identityAuthorizer.AuthorizeGitHubRepositoryWithIdentity(
			ctx, scope.WorkspaceID, scope.TaskID, scope.SessionID, scope.RepositoryID, owner, repo,
			scope.IdentityProviderID, scope.ParentProviderID,
		)
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
	if rawBinding := strings.TrimSpace(scope.CredentialBinding); rawBinding != "" {
		var binding taskmodels.ContributionDestinationCredentialBinding
		if err := json.Unmarshal([]byte(rawBinding), &binding); err != nil {
			return "", fmt.Errorf("%w: invalid destination credential binding", ErrCredentialScopeDenied)
		}
		if err := binding.Validate(); err != nil || !destinationBindingMatchesConnection(&binding, connection, appGeneration) {
			return "", fmt.Errorf("%w: destination credential binding does not match the active workspace connection", ErrCredentialScopeDenied)
		}
	}
	return githubCredentialBindingFor(connection, appGeneration)
}

func (p *gitHubCredentialProvider) RefreshScope(ctx context.Context, scope gitcredentials.Scope) (gitcredentials.Scope, error) {
	rawBinding := strings.TrimSpace(scope.CredentialBinding)
	if rawBinding == "" {
		return scope, nil
	}
	var binding taskmodels.ContributionDestinationCredentialBinding
	if err := json.Unmarshal([]byte(rawBinding), &binding); err != nil {
		return gitcredentials.Scope{}, fmt.Errorf("%w: invalid destination credential binding", ErrCredentialScopeDenied)
	}
	if err := binding.Validate(); err != nil {
		return gitcredentials.Scope{}, fmt.Errorf("%w: invalid destination credential binding", ErrCredentialScopeDenied)
	}
	connection, appGeneration, err := p.issueConnection(ctx, scope.WorkspaceID)
	if err != nil {
		return gitcredentials.Scope{}, err
	}
	if !destinationBindingIdentityMatchesConnection(&binding, connection) {
		return gitcredentials.Scope{}, fmt.Errorf("%w: destination credential binding does not match the active workspace connection", ErrCredentialScopeDenied)
	}
	binding.CredentialGeneration = connection.CredentialGeneration
	if connection.Source == ConnectionSourceGitHubAppInstallation {
		binding.AppCredentialGeneration = appGeneration
	} else {
		binding.AppCredentialGeneration = 0
		binding.InstallationID = 0
	}
	encoded, err := json.Marshal(binding)
	if err != nil {
		return gitcredentials.Scope{}, fmt.Errorf("encode contribution destination credential binding: %w", err)
	}
	scope.CredentialBinding = string(encoded)
	return scope, nil
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

func destinationBindingMatchesConnection(
	binding *taskmodels.ContributionDestinationCredentialBinding,
	connection *WorkspaceConnection,
	appCredentialGeneration int64,
) bool {
	if !destinationBindingIdentityMatchesConnection(binding, connection) ||
		binding.CredentialGeneration != connection.CredentialGeneration {
		return false
	}
	if connection.Source == ConnectionSourceGitHubAppInstallation {
		return connection.InstallationID != nil && binding.InstallationID == *connection.InstallationID &&
			binding.AppCredentialGeneration == appCredentialGeneration
	}
	if binding.Login != "" && !strings.EqualFold(binding.Login, connection.Login) {
		return false
	}
	return binding.AppCredentialGeneration == 0 && binding.InstallationID == 0
}

func destinationBindingIdentityMatchesConnection(
	binding *taskmodels.ContributionDestinationCredentialBinding,
	connection *WorkspaceConnection,
) bool {
	if binding == nil || connection == nil || binding.Source != string(connection.Source) ||
		binding.AppRegistrationID != connection.AppRegistrationID {
		return false
	}
	if connection.Source == ConnectionSourceGitHubAppInstallation {
		return connection.InstallationID != nil && binding.InstallationID == *connection.InstallationID
	}
	return binding.Login == "" || strings.EqualFold(binding.Login, connection.Login)
}
