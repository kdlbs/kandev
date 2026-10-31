package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/common/githubhost"
)

const (
	publicGitHubHostname = "github.com"
	legacyGitHubHostname = "www.github.com"
	protocolSSH          = "ssh"
	gitSSHUser           = "git"
)

// GitHubRepositoryOriginVerifier checks an Enterprise locator against the
// owning workspace connection before task creation can use or persist it.
type GitHubRepositoryOriginVerifier interface {
	VerifyGitHubRepositoryOrigin(context.Context, string, string) error
}

func (s *Service) SetGitHubRepositoryOriginVerifier(verifier GitHubRepositoryOriginVerifier) {
	s.gitHubRepositoryOriginVerifier = verifier
}

func effectiveRemoteProvider(input TaskRepositoryInput) string {
	if provider := strings.TrimSpace(input.Provider); provider != "" {
		return provider
	}
	if strings.TrimSpace(input.RemoteURL) == "" && strings.TrimSpace(input.GitHubURL) != "" {
		return providerGitHub
	}
	return ""
}

// A bare locator identifies GitHub only when the workspace connection confirms
// its origin. An arbitrary hostname alone is not a provider identity.
func (s *Service) inferEnterpriseGitHubProvider(ctx context.Context, workspaceID string, input *TaskRepositoryInput) error {
	if input.RepositoryID != "" || input.TrustedProviderDescriptor || effectiveRemoteProvider(*input) != "" || effectiveRemoteURL(*input) == "" {
		return nil
	}
	if _, _, _, _, err := parseRemoteRepositoryURL(effectiveRemoteURL(*input), ""); err == nil {
		return nil
	}
	candidate := *input
	candidate.Provider = providerGitHub
	if err := s.verifyEnterpriseGitHubOrigin(ctx, workspaceID, candidate); err != nil {
		return err
	}
	input.Provider = providerGitHub
	return nil
}

func parseEnterpriseGitHubRemote(parsed *url.URL, parts []string, sshStyle bool) (string, string, string, string, error) {
	host, err := githubhost.Parse(parsed.Host)
	if err != nil || (parsed.Scheme != protocolHTTPS && parsed.Scheme != protocolSSH) || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return "", "", "", "", fmt.Errorf("invalid Enterprise GitHub repository URL")
	}
	if parsed.User != nil && (parsed.Scheme != protocolSSH || parsed.User.String() != gitSSHUser) {
		return "", "", "", "", fmt.Errorf("invalid Enterprise GitHub repository user")
	}
	provider, owner, name, canonical, err := parseGitHubRemote(host.String(), parts, sshStyle)
	if err == nil && parsed.Scheme == protocolSSH && !sshStyle {
		canonical = "ssh://" + gitSSHUser + "@" + host.String() + "/" + owner + "/" + name + ".git"
	}
	return provider, owner, name, canonical, err
}

func enterpriseGitHubRemoteOrigin(raw string) string {
	parsed, _, err := normalizeRemoteRepositoryURL(raw)
	if err != nil {
		return ""
	}
	authority := strings.ToLower(parsed.Host)
	if authority == legacyGitHubHostname {
		authority = publicGitHubHostname
	}
	host, err := githubhost.Parse(authority)
	if err != nil {
		return ""
	}
	return host.WebURL()
}

func (s *Service) verifyEnterpriseGitHubOrigin(ctx context.Context, workspaceID string, input TaskRepositoryInput) error {
	if input.RepositoryID != "" || input.TrustedProviderDescriptor || effectiveRemoteURL(input) == "" {
		return nil
	}
	provider, _, _, canonical, err := parseRemoteRepositoryURL(effectiveRemoteURL(input), effectiveRemoteProvider(input))
	if err != nil {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid, err)
	}
	if provider != providerGitHub || remoteProviderHost(provider, canonical) == githubProviderHost {
		return nil
	}
	if s.gitHubRepositoryOriginVerifier == nil {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	if err := s.gitHubRepositoryOriginVerifier.VerifyGitHubRepositoryOrigin(ctx, workspaceID, remoteProviderHost(provider, canonical)); err != nil {
		return normalizeRepositorySelectionError(err)
	}
	return nil
}
