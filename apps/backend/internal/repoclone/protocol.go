package repoclone

import (
	"fmt"
	"strings"
)

// ProtocolSSH is the SSH git protocol.
const ProtocolSSH = "ssh"

// ProtocolHTTPS is the HTTPS git protocol.
const ProtocolHTTPS = "https"

// DetectGitProtocol returns the user's preferred git clone protocol.
// Currently returns "ssh" unconditionally. In the future this may be
// configurable or auto-detected from gh CLI / SSH config.
func DetectGitProtocol() string {
	return ProtocolSSH
}

// CloneURL builds a clone URL for the given provider, owner, name, and protocol.
// For SSH: git@github.com:{owner}/{name}.git
// For HTTPS: https://github.com/{owner}/{name}.git
// Returns an error if the provider is not supported.
func CloneURL(provider, owner, name, protocol string) (string, error) {
	host, err := providerHost(provider)
	if err != nil {
		return "", err
	}
	if protocol == ProtocolSSH {
		return fmt.Sprintf("git@%s:%s/%s.git", host, owner, name), nil
	}
	return fmt.Sprintf("https://%s/%s/%s.git", host, owner, name), nil
}

// providerHost maps a provider name to its git host.
// Only "github" (and the empty string) are currently supported.
// "gitlab" and "bitbucket" entries are placeholders for future support.
func providerHost(provider string) (string, error) {
	switch strings.ToLower(provider) {
	case "github", "":
		return "github.com", nil
	case "gitlab":
		// TODO: GitLab support is not yet implemented
		return "gitlab.com", nil
	case "bitbucket":
		// TODO: Bitbucket support is not yet implemented
		return "bitbucket.org", nil
	default:
		return "", fmt.Errorf("unsupported git provider: %q", provider)
	}
}
