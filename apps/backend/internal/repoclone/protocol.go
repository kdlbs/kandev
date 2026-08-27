package repoclone

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
)

// ProtocolSSH is the SSH git protocol.
const ProtocolSSH = "ssh"

// ProtocolHTTPS is the HTTPS git protocol.
const ProtocolHTTPS = "https"

const gitProtocolLookupTimeout = 5 * time.Second

// GitProtocolResolver resolves the clone protocol for a provider host.
type GitProtocolResolver interface {
	ResolveGitProtocol(context.Context, string) string
}

type gitProtocolCommandRunner func(context.Context, ...string) ([]byte, error)

type gitProtocolResolver struct {
	run gitProtocolCommandRunner
}

// NewGitProtocolResolver creates a resolver backed by the host gh CLI.
func NewGitProtocolResolver() GitProtocolResolver {
	return &gitProtocolResolver{run: runGHConfigCommand}
}

func newGitProtocolResolver(run gitProtocolCommandRunner) GitProtocolResolver {
	if run == nil {
		run = runGHConfigCommand
	}
	return &gitProtocolResolver{run: run}
}

func runGHConfigCommand(ctx context.Context, args ...string) ([]byte, error) {
	return subproc.RunGHOutput(ctx, exec.CommandContext(ctx, "gh", args...))
}

func (r *gitProtocolResolver) ResolveGitProtocol(ctx context.Context, providerHost string) string {
	lookupCtx, cancel := context.WithTimeout(ctx, gitProtocolLookupTimeout)
	defer cancel()
	host, _, err := normalizeGitProviderHost(providerHost)
	if err == nil && host != "" {
		if protocol := r.lookup(lookupCtx, "-h", host, "git_protocol"); protocol != "" {
			return protocol
		}
	}
	if lookupCtx.Err() == nil {
		if protocol := r.lookup(lookupCtx, "git_protocol"); protocol != "" {
			return protocol
		}
	}
	return ProtocolSSH
}

func (r *gitProtocolResolver) lookup(ctx context.Context, args ...string) string {
	out, err := r.run(ctx, append([]string{"config", "get"}, args...)...)
	if err != nil || ctx.Err() != nil {
		return ""
	}
	protocol := strings.TrimSpace(string(out))
	if protocol == ProtocolSSH || protocol == ProtocolHTTPS {
		return protocol
	}
	return ""
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
	resolvedHost, httpsScheme, err := normalizeGitProviderHost(host)
	if err != nil {
		return "", err
	}
	if resolvedHost == "" {
		resolvedHost, err = providerHost(provider)
		if err != nil {
			return "", err
		}
	}
	if protocol == ProtocolSSH {
		// scp-style "git@host:path" can't carry a port — when the host has
		// one (gitlab.acme.corp:2222) fall back to the ssh:// URL form,
		// which git understands and accepts a port natively.
		if strings.Contains(resolvedHost, ":") {
			return fmt.Sprintf("ssh://git@%s/%s/%s.git", resolvedHost, owner, name), nil
		}
		return fmt.Sprintf("git@%s:%s/%s.git", resolvedHost, owner, name), nil
	}
	return fmt.Sprintf("%s://%s/%s/%s.git", httpsScheme, resolvedHost, owner, name), nil
}

func normalizeGitProviderHost(host string) (string, string, error) {
	resolvedHost := strings.TrimSpace(strings.TrimRight(host, "/"))
	httpsScheme := "https"
	if strings.Contains(resolvedHost, "://") {
		parsed, err := url.Parse(resolvedHost)
		if err != nil || parsed.Host == "" || parsed.User != nil ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") ||
			(parsed.Path != "" && parsed.Path != "/") {
			return "", "", fmt.Errorf("invalid git provider host: %q", host)
		}
		resolvedHost = parsed.Host
		httpsScheme = parsed.Scheme
	}
	return resolvedHost, httpsScheme, nil
}

// providerHost maps a provider name to its git host.
func providerHost(provider string) (string, error) {
	switch strings.ToLower(provider) {
	case "github", "":
		return "github.com", nil
	case "gitlab":
		return "gitlab.com", nil
	default:
		return "", fmt.Errorf("unsupported git provider: %q", provider)
	}
}
