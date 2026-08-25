package backendapp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/repoclone"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	gitCredentialGitHubProviderID = "github"
	gitCredentialGitHubHost       = "github.com"
)

type gitHubCredentialSource interface {
	GitCredentialResolver() gitcredentials.Resolver
	VerifyContributionDestinationForWorkspace(
		context.Context, string, string, string, string, string, string, string,
	) error
}

type gitCredentialTaskRepository interface {
	GetTask(context.Context, string) (*taskmodels.Task, error)
	GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
	GetRepository(context.Context, string) (*taskmodels.Repository, error)
	ListTaskRepositories(context.Context, string) ([]*taskmodels.TaskRepository, error)
}

func newGitCredentialBroker(
	githubSvc gitHubCredentialSource,
	pluginSvc *plugins.Service,
	repo gitCredentialTaskRepository,
	reissueSigningKey string,
) *gitcredentials.Broker {
	resolvers := make([]gitcredentials.Resolver, 0, 2)
	if githubSvc != nil {
		resolvers = append(resolvers, githubSvc.GitCredentialResolver())
	}
	if pluginSvc != nil {
		resolvers = append(resolvers, pluginGitCredentialResolver{service: pluginCredentialServiceAdapter{service: pluginSvc}})
	}
	broker := gitcredentials.NewBroker(
		gitcredentials.NewCompositeResolver(resolvers...),
		&githubBrokerScopeAuthorizer{repo: repo, provider: githubSvc},
	)
	if signer, err := gitcredentials.NewReissueCapabilitySigner(reissueSigningKey); err == nil {
		broker.SetReissueCapabilitySigner(signer)
	}
	return broker
}

// pluginGitCredentialResolver resolves only live, active manifest-declared
// repository providers. It never caches a returned secret, so each helper
// redemption observes plugin-side OAuth rotation.
type pluginGitCredentialResolver struct {
	service pluginCredentialService
}

type pluginCredentialRemote interface {
	ResolveGitCredential(context.Context, *pluginsdk.ResolveGitCredentialRequest) (*pluginsdk.ResolveGitCredentialResponse, error)
}

type pluginCredentialBindingRemote interface {
	GetGitCredentialBinding(context.Context, *pluginsdk.GitCredentialBindingRequest) (*pluginsdk.GitCredentialBindingResponse, error)
}

type pluginCredentialService interface {
	Provider(string) (id, version string, remote pluginCredentialRemote, found bool)
}

type githubCloneCredentialService interface {
	ResolveGitCredential(
		context.Context, string, string, string, string,
	) (username, password string, err error)
}

type repositoryCloneCredentialProvider struct {
	github  githubCloneCredentialService
	plugins pluginCredentialService
}

func newRepositoryCloneCredentialProvider(
	github githubCloneCredentialService, pluginService *plugins.Service,
) repoclone.GitCredentialProvider {
	var pluginAdapter pluginCredentialService
	if pluginService != nil {
		pluginAdapter = pluginCredentialServiceAdapter{service: pluginService}
	}
	return repositoryCloneCredentialProvider{github: github, plugins: pluginAdapter}
}

func (p repositoryCloneCredentialProvider) ResolveGitCredential(
	ctx context.Context, request repoclone.GitCredentialRequest,
) (string, string, error) {
	providerID := strings.ToLower(strings.TrimSpace(request.Provider))
	if providerID == "" {
		providerID = gitCredentialGitHubProviderID
	}
	if providerID == gitCredentialGitHubProviderID {
		if p.github == nil {
			return "", "", repoclone.ErrWorkspaceCredentialUnavailable
		}
		return p.github.ResolveGitCredential(
			ctx, request.WorkspaceID, providerID, request.Owner, request.Name,
		)
	}
	if p.plugins == nil {
		return "", "", repoclone.ErrWorkspaceCredentialUnavailable
	}
	if err := repoclone.ValidateHTTPSCloneOrigin(request.CloneURL, request.ProviderHost); err != nil {
		return "", "", fmt.Errorf("plugin clone credential origin: %w", err)
	}
	parsed, err := url.Parse(strings.TrimSpace(request.CloneURL))
	if err != nil || parsed.Host == "" || parsed.Path == "" {
		return "", "", fmt.Errorf("plugin clone credential URL is invalid")
	}
	credential, err := (pluginGitCredentialResolver{service: p.plugins}).Resolve(ctx, gitcredentials.Scope{
		ProviderID: providerID, WorkspaceID: request.WorkspaceID,
		TaskID: request.TaskID, SessionID: request.SessionID, RepositoryID: request.RepositoryID,
		Host: strings.ToLower(parsed.Host), Path: parsed.Path,
	})
	if err != nil {
		return "", "", err
	}
	return credential.Username, credential.Password, nil
}

type pluginCredentialServiceAdapter struct{ service *plugins.Service }

func (a pluginCredentialServiceAdapter) Provider(providerID string) (string, string, pluginCredentialRemote, bool) {
	if a.service == nil || a.service.Registry() == nil || a.service.Runtime() == nil {
		return "", "", nil, false
	}
	for _, record := range a.service.Registry().List() {
		if record.Status != plugins.StatusActive || !declaresProvider(record.RepositoryProviders, providerID) {
			continue
		}
		remote, running := a.service.Runtime().Get(record.ID)
		if !running || remote == nil {
			return "", "", nil, false
		}
		return record.ID, record.Version, remote, true
	}
	return "", "", nil, false
}

func (r pluginGitCredentialResolver) Supports(providerID string) bool {
	_, _, _, found := r.provider(providerID)
	return found
}

func (r pluginGitCredentialResolver) Binding(ctx context.Context, scope gitcredentials.Scope) (string, error) {
	_, _, remote, found := r.provider(scope.ProviderID)
	if !found {
		return "", gitcredentials.ErrUnsupported
	}
	binder, ok := remote.(pluginCredentialBindingRemote)
	if !ok {
		// A legacy plugin can still be active and resolve credentials, but its
		// generation cannot prove a lease remained valid after issuance.
		return "", gitcredentials.ErrUnsupported
	}
	response, err := binder.GetGitCredentialBinding(ctx, &pluginsdk.GitCredentialBindingRequest{
		ProviderID: scope.ProviderID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID,
		SessionID: scope.SessionID, RepositoryID: scope.RepositoryID, Host: scope.Host, Path: scope.Path,
	})
	if err != nil {
		return "", fmt.Errorf("resolve plugin Git credential binding: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Binding) == "" {
		return "", gitcredentials.ErrLeaseRevoked
	}
	return response.Binding, nil
}

func (r pluginGitCredentialResolver) Resolve(ctx context.Context, scope gitcredentials.Scope) (gitcredentials.Credential, error) {
	_, _, remote, found := r.provider(scope.ProviderID)
	if !found {
		return gitcredentials.Credential{}, gitcredentials.ErrUnsupported
	}
	response, err := remote.ResolveGitCredential(ctx, &pluginsdk.ResolveGitCredentialRequest{
		ProviderID: scope.ProviderID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID,
		SessionID: scope.SessionID, RepositoryID: scope.RepositoryID, Host: scope.Host, Path: scope.Path,
	})
	if err != nil {
		return gitcredentials.Credential{}, fmt.Errorf("resolve plugin Git credential: %w", err)
	}
	if response == nil || strings.TrimSpace(response.Username) == "" || strings.TrimSpace(response.Secret) == "" {
		return gitcredentials.Credential{}, gitcredentials.ErrLeaseRevoked
	}
	expiresAt, err := pluginCredentialExpiry(response.ExpiresAt)
	if err != nil {
		return gitcredentials.Credential{}, err
	}
	return gitcredentials.Credential{Username: response.Username, Password: response.Secret, ExpiresAt: expiresAt}, nil
}

func (r pluginGitCredentialResolver) provider(providerID string) (string, string, pluginCredentialRemote, bool) {
	if r.service == nil {
		return "", "", nil, false
	}
	return r.service.Provider(providerID)
}

func declaresProvider(providers []string, want string) bool {
	for _, provider := range providers {
		if strings.EqualFold(strings.TrimSpace(provider), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func pluginCredentialExpiry(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, raw)
	if err != nil || !expiresAt.After(time.Now()) {
		return time.Time{}, fmt.Errorf("plugin returned invalid Git credential expiry")
	}
	return expiresAt, nil
}

func (a *githubBrokerScopeAuthorizer) AuthorizeGitCredential(ctx context.Context, scope gitcredentials.Scope) error {
	if a == nil || a.repo == nil {
		return fmt.Errorf("task repository is unavailable")
	}
	if err := a.authorizeTaskSession(ctx, scope.WorkspaceID, scope.TaskID, scope.SessionID); err != nil {
		return err
	}
	if _, err := a.authorizeTaskRepository(ctx, scope.TaskID, scope.RepositoryID); err != nil {
		return err
	}
	if !strings.EqualFold(scope.ProviderID, gitCredentialGitHubProviderID) ||
		!strings.EqualFold(scope.Host, gitCredentialGitHubHost) {
		return a.authorizeRepositoryIdentity(ctx, scope)
	}
	owner, repo, err := gitCredentialScopeOwnerRepo(scope.Path)
	if err != nil {
		return err
	}
	if err := a.AuthorizeGitHubRepositoryWithIdentity(
		ctx, scope.WorkspaceID, scope.TaskID, scope.SessionID, scope.RepositoryID,
		owner, repo, scope.IdentityProviderID, scope.ParentProviderID,
	); err == nil {
		return nil
	}
	// Legacy GitHub rows may omit ProviderHost while their persisted clone URL
	// still proves github.com. This fallback is upstream-only. Preserve any
	// provider identity carried by the lease before using that legacy proof;
	// fork credentials must pass the destination check above.
	return a.authorizeLegacyGitHubRepositoryIdentity(ctx, scope)
}

func gitCredentialScopeOwnerRepo(path string) (string, string, error) {
	trimmed := strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, repo, found := strings.Cut(trimmed, "/")
	if !found || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("repository identity does not match lease scope")
	}
	return owner, repo, nil
}

func (a *githubBrokerScopeAuthorizer) authorizeRepositoryIdentity(ctx context.Context, scope gitcredentials.Scope) error {
	repository, err := a.repo.GetRepository(ctx, scope.RepositoryID)
	if err != nil {
		return err
	}
	return authorizeRepositoryIdentityRecord(repository, scope)
}

func (a *githubBrokerScopeAuthorizer) authorizeLegacyGitHubRepositoryIdentity(
	ctx context.Context, scope gitcredentials.Scope,
) error {
	if strings.TrimSpace(scope.ParentProviderID) != "" {
		return fmt.Errorf("repository parent identity does not match lease scope")
	}
	repository, err := a.repo.GetRepository(ctx, scope.RepositoryID)
	if err != nil {
		return err
	}
	if identity := strings.TrimSpace(scope.IdentityProviderID); identity != "" &&
		(repository == nil || !strings.EqualFold(strings.TrimSpace(repository.ProviderRepoID), identity)) {
		return fmt.Errorf("repository provider identity does not match lease scope")
	}
	return authorizeRepositoryIdentityRecord(repository, scope)
}

func authorizeRepositoryIdentityRecord(repository *taskmodels.Repository, scope gitcredentials.Scope) error {
	if repository == nil {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	providerID := strings.TrimSpace(repository.Provider)
	if providerID == "" {
		providerID = gitCredentialGitHubProviderID
	}
	if repository.WorkspaceID != scope.WorkspaceID ||
		!strings.EqualFold(providerID, scope.ProviderID) {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	host, path, err := repositoryHTTPSIdentity(repository)
	if err != nil || !strings.EqualFold(host, scope.Host) ||
		githubauth.CanonicalCredentialPath(path) != githubauth.CanonicalCredentialPath(scope.Path) {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	return nil
}

func repositoryHTTPSIdentity(repository *taskmodels.Repository) (string, string, error) {
	if repository == nil {
		return "", "", fmt.Errorf("repository is required")
	}
	identity, err := gitcredentials.ResolveRepositoryIdentity(gitcredentials.RepositoryIdentityInput{
		RepositoryID: repository.ID, Provider: repository.Provider, ProviderHost: repository.ProviderHost,
		ProviderOwner: repository.ProviderOwner, ProviderName: repository.ProviderName, RemoteURL: repository.RemoteURL,
	})
	if err != nil {
		return "", "", err
	}
	return identity.Host, identity.Path, nil
}
