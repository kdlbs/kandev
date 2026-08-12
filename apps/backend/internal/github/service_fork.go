package github

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

const (
	contributionForkPreparationTimeout = 30 * time.Second
)

var contributionForkPollInterval = 200 * time.Millisecond

// ContributionForkStatus reports the workspace automation capability without
// exposing credentials or relying on an ambient executor identity.
type ContributionForkStatus string

const (
	ContributionForkStatusDirectWrite ContributionForkStatus = "direct"
	ContributionForkStatusReady       ContributionForkStatus = "ready"
	ContributionForkStatusCreatable   ContributionForkStatus = "creatable"
	ContributionForkStatusBlocked     ContributionForkStatus = "blocked"
)

// ContributionForkResolution contains transient provider information. Only
// Destination is suitable for persistence after Validate succeeds.
type ContributionForkResolution struct {
	Status      ContributionForkStatus
	ActorLogin  string
	Repository  *GitHubRepository
	Destination *taskmodels.ContributionDestination
}

var (
	ErrContributionForkConflict       = errors.New("GitHub contribution fork has the wrong parent")
	ErrContributionForkAppUnsupported = errors.New("GitHub App cannot prepare a contribution fork")
	ErrContributionForkNotWritable    = errors.New("GitHub contribution fork is not writable")
	ErrContributionForkNotReady       = errors.New("GitHub contribution fork is not ready")
	ErrContributionForkUnsupported    = errors.New("GitHub client cannot prepare a contribution fork")
)

type contributionForkClient interface {
	GetAuthenticatedUser(context.Context) (string, error)
	GetRepository(context.Context, string, string) (*GitHubRepository, error)
	CreateFork(context.Context, string, string) (*GitHubRepository, error)
}

// ResolveContributionForkForWorkspace checks or creates the exact managed
// publication destination for one canonical repository. Set create to false
// for a read-only bootstrap capability probe.
func (s *Service) ResolveContributionForkForWorkspace(
	ctx context.Context,
	workspaceID, owner, repo string,
	create bool,
) (ContributionForkResolution, error) {
	if err := s.ensureRepositoryInWorkspaceScope(ctx, workspaceID, owner, repo); err != nil {
		return ContributionForkResolution{}, err
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return ContributionForkResolution{}, err
	}
	client, ok := resolved.Client.(contributionForkClient)
	if !ok {
		return ContributionForkResolution{}, ErrContributionForkUnsupported
	}
	prepareCtx, cancel := context.WithTimeout(ctx, contributionForkPreparationTimeout)
	defer cancel()
	return resolveContributionFork(prepareCtx, client, resolved.Principal, owner, repo, create)
}

// ProbeContributionForkCapabilityForWorkspace returns direct, ready, or
// creatable capability without mutating GitHub. Configuration failures are
// returned with a blocked status so callers can preserve the issue-only path.
func (s *Service) ProbeContributionForkCapabilityForWorkspace(
	ctx context.Context,
	workspaceID, owner, repo string,
) (ContributionForkResolution, error) {
	result, err := s.ResolveContributionForkForWorkspace(ctx, workspaceID, owner, repo, false)
	if err == nil {
		return result, nil
	}
	result.Status = ContributionForkStatusBlocked
	return result, err
}

// ResolveContributionDestinationForWorkspace prepares and returns the
// server-authored destination. Direct target write returns a nil binding.
func (s *Service) ResolveContributionDestinationForWorkspace(
	ctx context.Context,
	workspaceID, owner, repo string,
) (*taskmodels.ContributionDestination, error) {
	result, err := s.ResolveContributionForkForWorkspace(ctx, workspaceID, owner, repo, true)
	if err != nil {
		return nil, err
	}
	return result.Destination, nil
}

func resolveContributionFork(
	ctx context.Context,
	client contributionForkClient,
	principal AuthPrincipal,
	owner, repo string,
	create bool,
) (ContributionForkResolution, error) {
	if client == nil {
		return ContributionForkResolution{}, ErrContributionForkUnsupported
	}
	canonical, err := client.GetRepository(ctx, owner, repo)
	if err != nil {
		return ContributionForkResolution{}, fmt.Errorf("verify canonical repository %s/%s: %w", owner, repo, err)
	}
	if err := validateCanonicalRepository(canonical, owner, repo); err != nil {
		return ContributionForkResolution{}, err
	}
	if repositoryWritable(canonical) {
		login, err := contributionActorLogin(ctx, client, principal)
		if err != nil {
			return ContributionForkResolution{}, err
		}
		return ContributionForkResolution{
			Status:     ContributionForkStatusDirectWrite,
			ActorLogin: login,
			Repository: canonical,
		}, nil
	}
	if principal.Kind == AuthPrincipalApp {
		return ContributionForkResolution{}, fmt.Errorf("%w: direct write to %s/%s is required", ErrContributionForkAppUnsupported, owner, repo)
	}
	login, err := contributionActorLogin(ctx, client, principal)
	if err != nil {
		return ContributionForkResolution{}, err
	}
	result, err := findContributionFork(ctx, client, canonical, login, owner, repo)
	if err == nil {
		return result, nil
	}
	if !errors.Is(err, errContributionForkMissing) {
		return ContributionForkResolution{}, err
	}
	if !create {
		return ContributionForkResolution{Status: ContributionForkStatusCreatable, ActorLogin: login, Repository: canonical}, nil
	}
	created, err := client.CreateFork(ctx, owner, repo)
	if err != nil {
		return ContributionForkResolution{}, fmt.Errorf("create contribution fork for %s/%s: %w", owner, repo, err)
	}
	forkOwner, forkName, err := repositoryOwnerAndName(created)
	if err != nil {
		return ContributionForkResolution{}, fmt.Errorf("create contribution fork for %s/%s: %w", owner, repo, err)
	}
	fork, err := waitForContributionFork(ctx, client, canonical, forkOwner, forkName)
	if err != nil {
		return ContributionForkResolution{}, err
	}
	return buildContributionForkResolution(canonical, fork, login, forkOwner, forkName)
}

var errContributionForkMissing = errors.New("contribution fork is missing")

func findContributionFork(
	ctx context.Context,
	client contributionForkClient,
	canonical *GitHubRepository,
	login, owner, repo string,
) (ContributionForkResolution, error) {
	fork, err := client.GetRepository(ctx, login, repo)
	if err != nil {
		if isContributionRepositoryNotFound(err) {
			return ContributionForkResolution{}, errContributionForkMissing
		}
		return ContributionForkResolution{}, fmt.Errorf("verify contribution fork %s/%s: %w", login, repo, err)
	}
	return buildContributionForkResolution(canonical, fork, login, login, repo)
}

func buildContributionForkResolution(
	canonical, fork *GitHubRepository,
	actorLogin, expectedOwner, expectedRepo string,
) (ContributionForkResolution, error) {
	if err := validateContributionFork(canonical, fork, expectedOwner, expectedRepo); err != nil {
		return ContributionForkResolution{}, err
	}
	destination, err := makeContributionDestination(canonical, fork)
	if err != nil {
		return ContributionForkResolution{}, err
	}
	return ContributionForkResolution{
		Status:      ContributionForkStatusReady,
		ActorLogin:  actorLogin,
		Repository:  fork,
		Destination: destination,
	}, nil
}

func validateCanonicalRepository(repository *GitHubRepository, owner, repo string) error {
	if repository == nil || !sameRepositoryName(repository, owner, repo) || repository.Fork {
		return fmt.Errorf("verify canonical repository %s/%s: provider identity does not match", owner, repo)
	}
	if repository.ID <= 0 {
		return fmt.Errorf("verify canonical repository %s/%s: provider ID is missing", owner, repo)
	}
	return nil
}

func validateContributionFork(canonical, fork *GitHubRepository, login, repo string) error {
	if fork == nil || !sameRepositoryName(fork, login, repo) || !fork.Fork ||
		fork.ParentID != canonical.ID || !strings.EqualFold(fork.ParentFullName, canonical.FullName) {
		return fmt.Errorf("%w: expected %s/%s to fork %s", ErrContributionForkConflict, login, repo, canonical.FullName)
	}
	if !repositoryWritable(fork) {
		return fmt.Errorf("%w: %s/%s", ErrContributionForkNotWritable, login, repo)
	}
	return nil
}

func makeContributionDestination(canonical, fork *GitHubRepository) (*taskmodels.ContributionDestination, error) {
	source, err := repositoryBinding(canonical)
	if err != nil {
		return nil, fmt.Errorf("build contribution source: %w", err)
	}
	target, err := repositoryBinding(fork)
	if err != nil {
		return nil, fmt.Errorf("build contribution target: %w", err)
	}
	destination := &taskmodels.ContributionDestination{
		Version:          taskmodels.ContributionDestinationVersion,
		Provider:         taskmodels.ContributionDestinationProviderGitHub,
		SourceRepository: source,
		TargetRepository: target,
	}
	if err := destination.Validate(); err != nil {
		return nil, err
	}
	return destination, nil
}

func repositoryBinding(repository *GitHubRepository) (taskmodels.RemoteContributionRepository, error) {
	if repository == nil || repository.ID <= 0 {
		return taskmodels.RemoteContributionRepository{}, errors.New("provider repository identity is missing")
	}
	fullName := repositoryFullName(repository)
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return taskmodels.RemoteContributionRepository{}, errors.New("provider repository full name is invalid")
	}
	remoteURL := strings.TrimSpace(repository.CloneURL)
	if remoteURL == "" {
		remoteURL = "https://github.com/" + fullName + ".git"
	}
	return taskmodels.RemoteContributionRepository{
		Host:       "github.com",
		Path:       fullName,
		ProviderID: strconv.FormatInt(repository.ID, 10),
		RemoteURL:  remoteURL,
	}, nil
}

func repositoryFullName(repository *GitHubRepository) string {
	if repository == nil {
		return ""
	}
	if strings.TrimSpace(repository.FullName) != "" {
		return strings.TrimSpace(repository.FullName)
	}
	return strings.TrimSpace(repository.Owner) + "/" + strings.TrimSpace(repository.Name)
}

func repositoryOwnerAndName(repository *GitHubRepository) (string, string, error) {
	parts := strings.Split(repositoryFullName(repository), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", errors.New("provider repository full name is invalid")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func sameRepositoryName(repository *GitHubRepository, owner, repo string) bool {
	return strings.EqualFold(repositoryFullName(repository), strings.TrimSpace(owner)+"/"+strings.TrimSpace(repo))
}

func repositoryWritable(repository *GitHubRepository) bool {
	return repository != nil && (repository.PushAccess || repository.AdminAccess)
}

func contributionActorLogin(ctx context.Context, client contributionForkClient, principal AuthPrincipal) (string, error) {
	if login := strings.TrimSpace(principal.Login); login != "" {
		return login, nil
	}
	login, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve GitHub automation login: %w", err)
	}
	if strings.TrimSpace(login) == "" {
		return "", errors.New("GitHub automation connection did not report a login")
	}
	return strings.TrimSpace(login), nil
}

func waitForContributionFork(
	ctx context.Context,
	client contributionForkClient,
	canonical *GitHubRepository,
	owner, repo string,
) (*GitHubRepository, error) {
	for {
		fork, err := client.GetRepository(ctx, owner, repo)
		if err == nil {
			if validationErr := validateContributionFork(canonical, fork, owner, repo); validationErr == nil {
				return fork, nil
			} else if !errors.Is(validationErr, ErrContributionForkNotWritable) {
				return nil, validationErr
			}
		} else if !isContributionRepositoryNotFound(err) {
			return nil, fmt.Errorf("verify created contribution fork %s/%s: %w", owner, repo, err)
		}
		timer := time.NewTimer(contributionForkPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("%w: %s/%s: %v", ErrContributionForkNotReady, owner, repo, ctx.Err())
		case <-timer.C:
		}
	}
}

func isContributionRepositoryNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}
