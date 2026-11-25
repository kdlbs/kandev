package service

import "context"

// GitLabRepositoryOriginVerifier checks a locator origin against the owning
// workspace connection without trusting browser-supplied host metadata.
type GitLabRepositoryOriginVerifier interface {
	VerifyGitLabRepositoryOrigin(context.Context, string, string) error
}

// SetGitLabRepositoryOriginVerifier wires the workspace-owned origin boundary.
func (s *Service) SetGitLabRepositoryOriginVerifier(verifier GitLabRepositoryOriginVerifier) {
	s.gitLabRepositoryOriginVerifier = verifier
}

func (s *Service) verifyRemoteRepositoryOrigin(ctx context.Context, workspaceID string, input *TaskRepositoryInput) error {
	// Saved repository IDs and local inputs carry no remote locator of their own;
	// resolution re-checks workspace ownership for the ID and never re-parses the
	// extra remote metadata a caller may include with it.
	if input.RepositoryID != "" || input.TrustedProviderDescriptor || effectiveRemoteURL(*input) == "" {
		return nil
	}
	provider, _, _, canonicalURL, err := parseRemoteRepositoryURL(effectiveRemoteURL(*input), effectiveRemoteProvider(*input))
	if err != nil {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid, err)
	}
	origin := remoteProviderHost(provider, canonicalURL)
	if provider == providerGitHub {
		return s.verifyEnterpriseGitHubOrigin(ctx, workspaceID, *input)
	}
	if provider != providerGitLab || origin == "https://gitlab.com" {
		return nil
	}
	if s.gitLabRepositoryOriginVerifier == nil {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	if err := s.gitLabRepositoryOriginVerifier.VerifyGitLabRepositoryOrigin(ctx, workspaceID, origin); err != nil {
		return normalizeRepositorySelectionError(err)
	}
	input.TrustedRemote = true
	return nil
}
