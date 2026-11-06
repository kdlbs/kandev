package github

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/githubhost"
	"github.com/kandev/kandev/internal/gitcredentials"
)

var ErrGitHubHostMismatch = errors.New("GitHub host does not match the workspace connection")

type expectedGitHubHostKey struct{}

func withExpectedGitHubHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, expectedGitHubHostKey{}, host)
}

func (s *Service) selectedGitHubHost(ctx context.Context, workspaceID string) (githubhost.Host, error) {
	if s.resolver == nil || s.resolver.connections == nil {
		return githubhost.Public(), nil
	}
	connection, err := s.resolver.connections.GetWorkspaceConnection(ctx, workspaceID)
	if err != nil {
		return githubhost.Host{}, err
	}
	if connection == nil || connection.GitHubHost == "" {
		return githubhost.Public(), nil
	}
	return githubhost.Parse(connection.GitHubHost)
}

func (s *Service) validateExpectedGitHubHost(ctx context.Context, workspaceID string) error {
	if expected, _ := ctx.Value(expectedGitHubHostKey{}).(string); expected == "" {
		return nil
	}
	selected, err := s.selectedGitHubHost(ctx, workspaceID)
	if err != nil {
		return err
	}
	return requireExpectedGitHubHost(ctx, selected)
}

func requireExpectedGitHubHost(ctx context.Context, selected githubhost.Host) error {
	expected, _ := ctx.Value(expectedGitHubHostKey{}).(string)
	if expected == "" {
		return nil
	}
	host, err := githubhost.Parse(expected)
	if err != nil {
		return err
	}
	if host != selected {
		return ErrGitHubHostMismatch
	}
	return nil
}

func (r *resolvedServiceClient) host() githubhost.Host {
	if r != nil && r.credential != nil {
		if h, err := githubhost.Parse(r.credential.GitHubHost); err == nil {
			return h
		}
	}
	if r != nil {
		if h, err := githubhost.Parse(r.Principal.GitHubHost); err == nil {
			return h
		}
		if c, ok := r.Client.(interface{ Host() githubhost.Host }); ok {
			return c.Host()
		}
	}
	return githubhost.Public()
}

func (s *Service) requireCurrentHost(ctx context.Context, workspaceID string, resolved *resolvedServiceClient) error {
	current, err := s.selectedGitHubHost(ctx, workspaceID)
	if err != nil {
		return err
	}
	if resolved == nil || current != resolved.host() {
		return ErrGitHubHostMismatch
	}
	generation := int64(0)
	if resolved.credential != nil {
		generation = resolved.credential.WorkspaceCredentialGeneration
		if generation == 0 && resolved.Principal.Source != ConnectionSourceGitHubAppUser {
			generation = resolved.credential.CredentialGeneration
		}
	}
	if generation > 0 && s.resolver != nil {
		connection, err := s.resolver.connections.GetWorkspaceConnection(ctx, workspaceID)
		if err != nil {
			return err
		}
		if connection == nil || connection.CredentialGeneration != generation {
			return ErrGitHubHostMismatch
		}
	}
	return nil
}

func providerGitHubURLHost(raw string) (githubhost.Host, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return githubhost.Host{}, githubhost.ErrInvalidHost
	}
	return githubhost.Parse(parsed.Scheme + "://" + parsed.Host)
}

func parseIssueReferenceForHost(input, owner, repo string, host githubhost.Host) (string, string, int, error) {
	trimmed := strings.TrimSpace(input)
	if n, err := strconv.Atoi(strings.TrimPrefix(trimmed, "#")); err == nil && n > 0 {
		return parseIssueReference(trimmed, owner, repo)
	}
	return parseGitHubResourceForHost(trimmed, "issues", host, ErrInvalidIssueReference)
}

func parseGitHubContributionURLForHost(input string, host githubhost.Host) (string, string, int, error) {
	if strings.TrimSpace(input) != input {
		return "", "", 0, ErrInvalidRemoteContributionURL
	}
	return parseGitHubResourceForHost(input, "pull", host, ErrInvalidRemoteContributionURL)
}

func parseGitHubResourceForHost(input, kind string, host githubhost.Host, invalid error) (string, string, int, error) {
	parsed, err := url.Parse(input)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !safeGitHubPath(parsed.EscapedPath()) {
		return "", "", 0, invalid
	}
	actual, err := providerGitHubURLHost(input)
	if err != nil || actual != host {
		return "", "", 0, invalid
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != kind || !validGitHubPathPart(parts[0]) || !validGitHubPathPart(parts[1]) {
		return "", "", 0, invalid
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return "", "", 0, invalid
	}
	return parts[0], parts[1], number, nil
}

func (s *Service) validatePRWatchHost(ctx context.Context, workspaceID string, watch *PRWatch) error {
	expected := watch.GitHubHost
	if expected != "" {
		return s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, expected), workspaceID)
	}
	if watch.ID == "" || s.store == nil {
		return s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, defaultGitHubHost), workspaceID)
	}
	stored, err := s.store.GetPRWatch(ctx, watch.ID)
	if err != nil {
		return err
	}
	if stored != nil && stored.GitHubHost == "" {
		return ErrGitHubHostMismatch
	}
	return s.validateExpectedGitHubHost(withExpectedGitHubHost(ctx, defaultGitHubHost), workspaceID)
}

type githubOperationFenceKey struct{}
type githubOperationFence struct {
	service     *Service
	workspaceID string
	resolved    *resolvedServiceClient
}

func withGitHubOperationFence(ctx context.Context, s *Service, workspaceID string, resolved *resolvedServiceClient) context.Context {
	return context.WithValue(ctx, githubOperationFenceKey{}, githubOperationFence{service: s, workspaceID: workspaceID, resolved: resolved})
}
func requireGitHubOperationCurrent(ctx context.Context) error {
	fence, ok := ctx.Value(githubOperationFenceKey{}).(githubOperationFence)
	if !ok {
		return nil
	}
	return fence.service.requireCurrentHost(ctx, fence.workspaceID, fence.resolved)
}

func githubResourceResponseKey(host, owner, repo string, number int) string {
	key := prStatusCacheKey(owner, repo, number)
	if host != "" && host != defaultGitHubHost {
		return host + "/" + key
	}
	return key
}

type githubWorkspaceMutationKey struct{}

func (s *Service) lockResourceMutation(ctx context.Context, workspaceID string) (context.Context, func(), error) {
	if workspaceID == "" {
		return ctx, func() {}, requireGitHubOperationCurrent(ctx)
	}
	if held, _ := ctx.Value(githubWorkspaceMutationKey{}).(string); held == workspaceID {
		return ctx, func() {}, requireGitHubOperationCurrent(ctx)
	}
	lock := s.workspaceConnectionMutationLock(workspaceID)
	lock.Lock()
	if err := requireGitHubOperationCurrent(ctx); err != nil {
		lock.Unlock()
		return ctx, func() {}, err
	}
	return context.WithValue(ctx, githubWorkspaceMutationKey{}, workspaceID), lock.Unlock, nil
}

func (s *Service) requireAttachedGitHubHost(ctx context.Context, repositoryID string, host githubhost.Host) error {
	if repositoryID == "" {
		return nil
	}
	store := s.getTaskIssueStore()
	if store == nil {
		return nil
	}
	repository, err := store.GetRepository(ctx, repositoryID)
	if err != nil {
		return err
	}
	if repository == nil {
		return ErrGitHubHostMismatch
	}
	if repository.ProviderHost == "" && repository.RemoteURL == "" && host.IsPublic() {
		return nil
	}
	identity, err := gitcredentials.ResolveRepositoryIdentity(gitcredentials.RepositoryIdentityInput{
		RepositoryID: repository.ID, Provider: repository.Provider, ProviderHost: repository.ProviderHost, ProviderOwner: repository.ProviderOwner, ProviderName: repository.ProviderName, RemoteURL: repository.RemoteURL,
	})
	if err != nil {
		return ErrGitHubHostMismatch
	}
	origin, err := githubhost.Parse(identity.Host)
	if err != nil || origin != host {
		return ErrGitHubHostMismatch
	}
	return nil
}
