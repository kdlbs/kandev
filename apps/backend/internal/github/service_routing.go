package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// resolvedServiceClient is the operation-scoped GitHub client selected for a
// workspace and purpose. The cache namespace must accompany the client so
// service-level caches never coalesce results across credentials.
type resolvedServiceClient struct {
	Client       Client
	Principal    AuthPrincipal
	CacheScope   string
	Capabilities map[GitHubAppCapability]bool
}

// repositoryAccessChecker performs a read-only repository-root probe. It is
// intentionally separate from merge-method discovery so an authorization
// check cannot populate operation caches or consume operation-specific test
// seams.
type repositoryAccessChecker interface {
	HasRepositoryAccess(context.Context, string, string) (bool, error)
}

const automationVisibilityConcurrency = 8

func requireGitHubCapability(resolved *resolvedServiceClient, capability GitHubAppCapability) error {
	if resolved == nil {
		return ErrGitHubNotConfigured
	}
	// Human PAT, CLI, and App-user OAuth permissions are ultimately enforced
	// by GitHub for that user. Installation principals have an explicit,
	// verifiable permission map and must fail before making the provider call.
	if resolved.Principal.Kind == AuthPrincipalApp && !resolved.Capabilities[capability] {
		return fmt.Errorf("%w: %s", ErrGitHubCapabilityDenied, capability)
	}
	return nil
}

func (s *Service) resolveServiceClient(
	ctx context.Context,
	workspaceID string,
	userID string,
	purpose CredentialPurpose,
	owner string,
	repo string,
) (*resolvedServiceClient, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrGitHubWorkspaceRequired
	}
	if s == nil || s.resolver == nil {
		return nil, ErrGitHubNotConfigured
	}
	resolved, err := s.resolver.Resolve(ctx, ResolveCredentialRequest{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Purpose:     purpose,
		RepoOwner:   owner,
		RepoName:    repo,
	})
	if err != nil {
		return nil, err
	}
	if resolved == nil || resolved.Client == nil {
		return nil, ErrGitHubNotConfigured
	}
	return &resolvedServiceClient{
		Client:       resolved.Client,
		Principal:    resolved.Principal,
		CacheScope:   credentialCacheScope(resolved, purpose),
		Capabilities: resolved.Capabilities,
	}, nil
}

func credentialCacheScope(resolved *ResolvedCredential, purpose CredentialPurpose) string {
	principal := resolved.Principal
	return fmt.Sprintf(
		"%d:%s|%d:%s|%d:%s|%d:%s|%d:%s|%d|%d|%d:%s|%d|%d:%s",
		len(principal.WorkspaceID), principal.WorkspaceID,
		len(principal.UserID), principal.UserID,
		len(principal.Source), principal.Source,
		len(principal.Kind), principal.Kind,
		len(principal.Login), principal.Login,
		principal.InstallationID, resolved.CredentialGeneration,
		len(resolved.AppRegistrationID), resolved.AppRegistrationID,
		resolved.AppCredentialGeneration,
		len(purpose), purpose,
	)
}

func scopedCacheKey(scope, key string) string {
	return fmt.Sprintf("%d:%s|%s", len(scope), scope, key)
}

func (s *Service) isRepoCachedAsMissingForScope(scope, owner, repo string) bool {
	if s == nil || s.repoErrorCache == nil {
		return false
	}
	v, ok := s.repoErrorCache.get(scopedCacheKey(scope, repoErrorCacheKey(owner, repo)))
	if !ok {
		return false
	}
	_, isErr := v.(cachedErr)
	return isErr
}

func (s *Service) markRepoAsMissingForScope(scope, owner, repo string, gen uint64) {
	if s == nil || s.repoErrorCache == nil {
		return
	}
	s.repoErrorCache.setIfCurrentGeneration(
		scopedCacheKey(scope, repoErrorCacheKey(owner, repo)),
		cachedErr{err: ErrRepoNotResolvable},
		gen,
	)
}

func (s *Service) resolveAutomationClient(
	ctx context.Context,
	workspaceID string,
	owner string,
	repo string,
) (*resolvedServiceClient, error) {
	return s.resolveServiceClient(ctx, workspaceID, "", CredentialPurposeAutomation, owner, repo)
}

func (s *Service) resolvePersonalReadClient(
	ctx context.Context,
	workspaceID string,
	userID string,
	owner string,
	repo string,
) (*resolvedServiceClient, error) {
	if err := s.ensurePersonalRepositoryBoundary(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	return s.resolveServiceClient(ctx, workspaceID, userID, CredentialPurposePersonalRead, owner, repo)
}

func (s *Service) resolvePersonalWriteClient(
	ctx context.Context,
	workspaceID string,
	userID string,
	owner string,
	repo string,
) (*resolvedServiceClient, error) {
	if err := s.ensurePersonalRepositoryBoundary(ctx, workspaceID, owner, repo); err != nil {
		return nil, err
	}
	return s.resolveServiceClient(ctx, workspaceID, userID, CredentialPurposePersonalWrite, owner, repo)
}

func (s *Service) ensurePersonalRepositoryBoundary(
	ctx context.Context,
	workspaceID, owner, repo string,
) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return nil
	}
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return err
	}
	automation, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return err
	}
	checker, ok := automation.Client.(repositoryAccessChecker)
	if !ok {
		return fmt.Errorf("verify automation repository access for %s/%s: unsupported client", owner, repo)
	}
	visible, err := checker.HasRepositoryAccess(ctx, owner, repo)
	if err == nil && visible {
		return nil
	}
	var apiErr *GitHubAPIError
	if err == nil || isRepoNotResolvableErr(err) || (errors.As(err, &apiErr) && apiErr.StatusCode == 404) {
		return fmt.Errorf("%w: automation connection cannot access %s/%s", ErrRepoNotResolvable, owner, repo)
	}
	return fmt.Errorf("verify automation repository access for %s/%s: %w", owner, repo, err)
}

// automationRepositoryVisibility returns the exact subset of repository
// identities visible to the workspace automation principal. Personal list
// and batch operations use it after provider search so a broad user token
// cannot reveal repositories outside an App installation.
func (s *Service) automationRepositoryVisibility(
	ctx context.Context,
	workspaceID string,
	repositories []RepoFilter,
) (map[string]bool, error) {
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, "", "")
	if err != nil {
		return nil, err
	}
	checker, ok := resolved.Client.(repositoryAccessChecker)
	if !ok {
		return nil, errors.New("GitHub automation client cannot verify repository access")
	}

	unique := make(map[string]RepoFilter, len(repositories))
	for _, repository := range repositories {
		key := repositoryIdentityKey(repository.Owner, repository.Name)
		if key != "/" && key != "" {
			unique[key] = repository
		}
	}
	visible := make(map[string]bool, len(unique))
	var resultMu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, automationVisibilityConcurrency)
	var wg sync.WaitGroup
	for key, repository := range unique {
		wg.Add(1)
		go func(key string, repository RepoFilter) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				resultMu.Lock()
				if firstErr == nil {
					firstErr = ctx.Err()
				}
				resultMu.Unlock()
				return
			}
			defer func() { <-sem }()
			allowed, checkErr := checker.HasRepositoryAccess(ctx, repository.Owner, repository.Name)
			if checkErr != nil && !repositoryAccessDenied(checkErr) {
				resultMu.Lock()
				if firstErr == nil {
					firstErr = checkErr
				}
				resultMu.Unlock()
				return
			}
			resultMu.Lock()
			visible[key] = checkErr == nil && allowed
			resultMu.Unlock()
		}(key, repository)
	}
	wg.Wait()
	return visible, firstErr
}

func repositoryAccessDenied(err error) bool {
	if err == nil || isRepoNotResolvableErr(err) {
		return err != nil
	}
	var apiErr *GitHubAPIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

func repositoryIdentityKey(owner, repo string) string {
	return strings.ToLower(strings.TrimSpace(owner)) + "/" + strings.ToLower(strings.TrimSpace(repo))
}

// ensureRepositoryInWorkspaceScope enforces Kandev's configured repository
// boundary before a provider call. GitHub App installation visibility is
// enforced by GitHub itself, so the two checks form an intersection.
func (s *Service) ensureRepositoryInWorkspaceScope(ctx context.Context, workspaceID, owner, repo string) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" {
		return nil
	}
	settings, err := s.GetWorkspaceSettings(ctx, workspaceID)
	if err != nil {
		return err
	}
	if repositoryInWorkspaceScope(settings, owner, repo) {
		return nil
	}
	return ErrRepoNotResolvable
}

func repositoryInWorkspaceScope(settings *WorkspaceSettings, owner, repo string) bool {
	if settings == nil || settings.RepoScopeMode == "" || settings.RepoScopeMode == RepoScopeModeAll {
		return true
	}
	switch settings.RepoScopeMode {
	case RepoScopeModeOrgs:
		for _, org := range settings.RepoScopeOrgs {
			if strings.EqualFold(strings.TrimSpace(org), strings.TrimSpace(owner)) {
				return true
			}
		}
	case RepoScopeModeRepos:
		for _, candidate := range settings.RepoScopeRepos {
			if strings.EqualFold(candidate.Owner, owner) && strings.EqualFold(candidate.Name, repo) {
				return true
			}
		}
	}
	return false
}
