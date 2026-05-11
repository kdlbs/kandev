package repoclone

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ProtocolSSH is the SSH git protocol.
const ProtocolSSH = "ssh"

// ProtocolHTTPS is the HTTPS git protocol.
const ProtocolHTTPS = "https"

// DetectGitProtocol returns the user's preferred git clone protocol.
// It checks the gh CLI config (`gh config get git_protocol`). If gh reports
// "https", it returns ProtocolHTTPS. Otherwise it defaults to ProtocolSSH.
func DetectGitProtocol() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "config", "get", "git_protocol").Output()
	if err == nil {
		if strings.TrimSpace(string(out)) == ProtocolHTTPS {
			return ProtocolHTTPS
		}
	}
	return ProtocolSSH
}

// CloneURL builds a clone URL for the given provider, owner, name, and protocol.
// For SSH: git@github.com:{owner}/{name}.git
// For HTTPS: https://github.com/{owner}/{name}.git
// Returns an error if the provider is not supported.
func CloneURL(provider, owner, name, protocol string) (string, error) {
	return CloneURLWithHost(provider, "", owner, name, protocol)
}

// CloneURLWithHost is like CloneURL but accepts an explicit host. When host
// is non-empty (and stripped of scheme/trailing-slash), it overrides the
// provider's default — used for self-managed GitLab. When host is empty,
// behaves exactly like CloneURL.
func CloneURLWithHost(provider, host, owner, name, protocol string) (string, error) {
	resolvedHost := strings.TrimRight(host, "/")
	resolvedHost = strings.TrimPrefix(strings.TrimPrefix(resolvedHost, "https://"), "http://")
	if resolvedHost == "" {
		var err error
		resolvedHost, err = providerHost(provider)
		if err != nil {
			return "", err
		}
	}
	if protocol == ProtocolSSH {
		return fmt.Sprintf("git@%s:%s/%s.git", resolvedHost, owner, name), nil
	}
	return fmt.Sprintf("https://%s/%s/%s.git", resolvedHost, owner, name), nil
}

// providerHost maps a provider name to its git host.
func providerHost(provider string) (string, error) {
	switch strings.ToLower(provider) {
	case "github", "":
		return "github.com", nil
	case "gitlab":
		return "gitlab.com", nil
	case "bitbucket":
		// TODO: Bitbucket support is not yet implemented
		return "bitbucket.org", nil
	default:
		return "", fmt.Errorf("unsupported git provider: %q", provider)
	}
}
