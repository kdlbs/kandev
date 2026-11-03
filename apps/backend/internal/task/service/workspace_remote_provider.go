package service

import (
	"context"
	"errors"
	"strings"
)

// resolveWorkspaceRemoteProvider resolves an unhinted, non-public remote URL to
// exactly one workspace-verified provider descriptor. Built-in GitHub
// Enterprise and GitLab connections and active repository plugins all
// contribute candidates; more than one successful candidate is ambiguous and is
// rejected before any repository or task write.
func (s *Service) resolveWorkspaceRemoteProvider(
	ctx context.Context, workspaceID string, input TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	candidates := make([]TaskRepositoryInput, 0, 3)
	failures := &repositorySelectionFailureSet{}
	for _, provider := range []string{providerGitHub, providerGitLab} {
		candidate, failure := s.builtInRemoteProviderCandidate(ctx, workspaceID, input, provider)
		if failure != nil {
			failures.add(failure)
			continue
		}
		if candidate.Provider != "" {
			candidates = append(candidates, candidate)
		}
	}
	pluginCandidates, pluginFailure, err := s.discoverPluginRemoteProviders(ctx, workspaceID, input)
	if err != nil {
		return TaskRepositoryInput{}, err
	}
	failures.add(pluginFailure)
	candidates = append(candidates, pluginCandidates...)

	// A caller cancellation or discovery deadline invalidates the attempt even
	// when a candidate already succeeded: the observed candidate set is not
	// authoritative once the pass was interrupted.
	if err := ctx.Err(); err != nil {
		return TaskRepositoryInput{}, normalizeRepositorySelectionError(err)
	}

	switch len(candidates) {
	case 1:
		// An unrelated candidate's ordinary inspection error never replaces a
		// unique, completed positive match.
		return candidates[0], nil
	case 0:
		return TaskRepositoryInput{}, failures.err()
	default:
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
}

// builtInRemoteProviderCandidate returns the verified built-in descriptor for
// one provider, or a genuine unavailable failure. An invalid parse or an origin
// mismatch contributes no candidate.
func (s *Service) builtInRemoteProviderCandidate(
	ctx context.Context, workspaceID string, input TaskRepositoryInput, provider string,
) (TaskRepositoryInput, *RepositorySelectionError) {
	candidate := input
	candidate.Provider = provider
	if _, _, _, _, err := parseRemoteRepositoryURL(effectiveRemoteURL(candidate), provider); err != nil {
		return TaskRepositoryInput{}, nil
	}
	if err := s.verifyRemoteRepositoryOrigin(ctx, workspaceID, &candidate); err != nil {
		var selectionErr *RepositorySelectionError
		if errors.As(err, &selectionErr) && selectionErr.Code == RepositorySelectionErrorUnavailable {
			return TaskRepositoryInput{}, selectionErr
		}
		return TaskRepositoryInput{}, nil
	}
	return candidate, nil
}

// discoverPluginRemoteProviders asks the plugin service for every validated
// candidate. An operation error aborts admission; an ordinary candidate failure
// only participates in final error selection.
func (s *Service) discoverPluginRemoteProviders(
	ctx context.Context, workspaceID string, input TaskRepositoryInput,
) ([]TaskRepositoryInput, *RepositorySelectionError, error) {
	if s.repositorySelectionResolver == nil {
		return nil, nil, nil
	}
	discovery, err := s.repositorySelectionResolver.DiscoverRepositorySelections(ctx, workspaceID, input)
	if err != nil {
		return nil, nil, normalizeRepositorySelectionError(err)
	}
	failures := &repositorySelectionFailureSet{}
	failures.add(discovery.Failure)
	candidates := make([]TaskRepositoryInput, 0, len(discovery.Candidates))
	for _, discovered := range discovery.Candidates {
		resolved, trustErr := trustDiscoveredRepositorySelection(input, discovered)
		if trustErr != nil {
			var selectionErr *RepositorySelectionError
			if errors.As(trustErr, &selectionErr) {
				failures.add(selectionErr)
			}
			continue
		}
		candidates = append(candidates, resolved)
	}
	return candidates, failures.best, nil
}

// trustDiscoveredRepositorySelection applies the explicit-selection trust rules
// to a server-discovered descriptor. The discovered provider comes from server
// authority, so only the caller's scope and immutable-ID pins are enforced.
func trustDiscoveredRepositorySelection(
	requested, resolved TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	if strings.TrimSpace(resolved.Provider) == "" {
		return TaskRepositoryInput{}, NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	authorized := requested
	authorized.Provider = resolved.Provider
	return trustResolvedRepositorySelection(authorized, resolved)
}

// workspaceRemoteProviderNeeded reports whether a bare, non-public URL must be
// resolved through workspace-verified provider candidates.
func workspaceRemoteProviderNeeded(input TaskRepositoryInput) bool {
	if input.RepositoryID != "" || input.TrustedProviderDescriptor {
		return false
	}
	if effectiveRemoteProvider(input) != "" {
		return false
	}
	url := effectiveRemoteURL(input)
	if url == "" {
		return false
	}
	_, _, _, _, err := parseRemoteRepositoryURL(url, "")
	return err != nil
}

// repositorySelectionFailureSet keeps the most severe ordinary failure so the
// caller receives a stable error when no candidate succeeded.
type repositorySelectionFailureSet struct {
	best *RepositorySelectionError
}

func (f *repositorySelectionFailureSet) add(err *RepositorySelectionError) {
	if err == nil {
		return
	}
	if f.best == nil || repositorySelectionFailureRank(err.Code) > repositorySelectionFailureRank(f.best.Code) {
		f.best = err
	}
}

func (f *repositorySelectionFailureSet) err() error {
	if f.best != nil {
		return f.best
	}
	return NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
}

func repositorySelectionFailureRank(code RepositorySelectionErrorCode) int {
	switch code {
	case RepositorySelectionErrorUnavailable:
		return 3
	case RepositorySelectionErrorInvalid:
		return 2
	case RepositorySelectionErrorNotFound:
		return 1
	default:
		return 0
	}
}
