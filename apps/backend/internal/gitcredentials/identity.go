package gitcredentials

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/repoclone"
)

const publicGitHubHost = "github.com"

// RepositoryIdentityInput is the non-secret persisted repository metadata used
// to authorize a managed Git credential scope. Provider remains routing
// metadata; ProviderHost alone establishes the authorized HTTPS origin.
type RepositoryIdentityInput struct {
	RepositoryID  string
	Provider      string
	ProviderHost  string
	ProviderOwner string
	ProviderName  string
	RemoteURL     string
}

// RepositoryIdentity is the canonical HTTPS identity used by issuers and the
// credential broker.
type RepositoryIdentity struct {
	Host       string
	Path       string
	Owner      string
	Repository string
}

func ResolveRepositoryIdentity(input RepositoryIdentityInput) (RepositoryIdentity, error) {
	remoteURL := strings.TrimSpace(input.RemoteURL)
	if remoteURL == "" {
		if input.ProviderOwner == "" || input.ProviderName == "" {
			return RepositoryIdentity{}, repositoryIdentityError(input.RepositoryID, "has no clone URL")
		}
		origin, err := trustedOrigin(input, "")
		if err != nil {
			return RepositoryIdentity{}, err
		}
		remoteURL = origin + "/" + strings.Trim(input.ProviderOwner, "/") + "/" + strings.Trim(input.ProviderName, "/") + ".git"
	}

	remote, err := parseRemote(remoteURL)
	if err != nil {
		return RepositoryIdentity{}, repositoryIdentityError(input.RepositoryID, "must use a credential-free HTTPS or SSH clone URL")
	}
	return resolveParsedRepositoryIdentity(input, remote)
}

func resolveParsedRepositoryIdentity(input RepositoryIdentityInput, remote parsedRemote) (RepositoryIdentity, error) {
	origin, err := trustedOrigin(input, remote.hostname)
	if err != nil {
		return RepositoryIdentity{}, err
	}
	originURL, _ := url.Parse(origin)
	if remote.isHTTPS {
		if err := repoclone.ValidateHTTPSCloneOrigin(remote.canonicalURL, origin); err != nil {
			return RepositoryIdentity{}, repositoryIdentityError(input.RepositoryID, "remote host conflicts with its provider origin")
		}
	} else if !strings.EqualFold(remote.hostname, originURL.Hostname()) {
		return RepositoryIdentity{}, repositoryIdentityError(input.RepositoryID, "remote host conflicts with its provider origin")
	}

	canonicalPath, err := canonicalIdentityPath(input, remote.path)
	if err != nil {
		return RepositoryIdentity{}, err
	}
	owner, repository, err := identityPath(canonicalPath)
	if err != nil {
		return RepositoryIdentity{}, repositoryIdentityError(input.RepositoryID, "has an invalid clone path")
	}
	return RepositoryIdentity{Host: originURL.Host, Path: canonicalPath, Owner: owner, Repository: repository}, nil
}

func canonicalIdentityPath(input RepositoryIdentityInput, remotePath string) (string, error) {
	if !strings.EqualFold(strings.TrimSpace(input.Provider), "github") || input.ProviderOwner == "" || input.ProviderName == "" {
		return remotePath, nil
	}
	remoteOwner, remoteRepository, err := identityPath(remotePath)
	if err != nil || !strings.EqualFold(remoteOwner, strings.TrimSpace(input.ProviderOwner)) ||
		!strings.EqualFold(strings.TrimSuffix(remoteRepository, ".git"), strings.TrimSuffix(strings.TrimSpace(input.ProviderName), ".git")) {
		return "", repositoryIdentityError(input.RepositoryID, "remote path conflicts with its provider identity")
	}
	return "/" + strings.Trim(input.ProviderOwner, "/") + "/" + strings.Trim(input.ProviderName, "/") + ".git", nil
}

type parsedRemote struct {
	hostname     string
	path         string
	canonicalURL string
	isHTTPS      bool
}

func parseRemote(raw string) (parsedRemote, error) {
	value := strings.TrimSpace(raw)
	if !hasSafeRemoteAuthority(value) {
		return parsedRemote{}, fmt.Errorf("invalid clone URL authority")
	}
	canonical := repoclone.CanonicalHTTPSCloneURL(value)
	if canonical != "" {
		if err := validateSSHRemote(value); err != nil {
			return parsedRemote{}, err
		}
		parsed, err := url.Parse(canonical)
		if err != nil || parsed.Host == "" || parsed.Path == "" {
			return parsedRemote{}, fmt.Errorf("invalid SSH clone URL")
		}
		return parsedRemote{hostname: parsed.Hostname(), path: parsed.Path, canonicalURL: canonical}, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.Path == "" {
		return parsedRemote{}, fmt.Errorf("invalid HTTPS clone URL")
	}
	return parsedRemote{hostname: parsed.Hostname(), path: parsed.Path, canonicalURL: "https://" + strings.ToLower(parsed.Host) + parsed.Path, isHTTPS: true}, nil
}

func validateSSHRemote(value string) error {
	if !strings.Contains(value, "://") {
		if strings.ContainsAny(value, "?#") {
			return fmt.Errorf("invalid SSH clone URL")
		}
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "ssh") || parsed.Hostname() == "" ||
		parsed.Path == "" || parsed.Path == "/" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return fmt.Errorf("invalid SSH clone URL")
	}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			return fmt.Errorf("invalid SSH clone URL")
		}
	}
	return nil
}

func trustedOrigin(input RepositoryIdentityInput, remoteHost string) (string, error) {
	if host := strings.TrimSpace(input.ProviderHost); host != "" {
		if !hasSafeProviderHostAuthority(host) {
			return "", repositoryIdentityError(input.RepositoryID, "has an invalid provider host")
		}
		origin, err := repoclone.HTTPSProviderOrigin(host)
		if err != nil {
			return "", repositoryIdentityError(input.RepositoryID, "has an invalid provider host")
		}
		return origin, nil
	}
	if strings.EqualFold(remoteHost, publicGitHubHost) {
		return "https://" + publicGitHubHost, nil
	}
	if strings.EqualFold(strings.TrimSpace(input.Provider), "github") && remoteHost == "" {
		return "https://" + publicGitHubHost, nil
	}
	return "", repositoryIdentityError(input.RepositoryID, "needs a provider host for managed credentials")
}

func identityPath(path string) (string, string, error) {
	trimmed := strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, repository, found := strings.Cut(trimmed, "/")
	if !found || owner == "" || repository == "" {
		return "", "", fmt.Errorf("path needs owner and repository")
	}
	return owner, repository, nil
}

func repositoryIdentityError(repositoryID, detail string) error {
	if strings.TrimSpace(repositoryID) == "" {
		return fmt.Errorf("repository identity %s", detail)
	}
	return fmt.Errorf("repository %q %s; repair its clone URL or provider host", repositoryID, detail)
}

func hasSafeRemoteAuthority(value string) bool {
	authority, found := rawRemoteAuthority(value)
	return found && repoclone.IsSafeRawAuthority(authority)
}

func rawRemoteAuthority(value string) (string, bool) {
	if schemeEnd := strings.Index(value, "://"); schemeEnd >= 0 && isURLScheme(value[:schemeEnd]) {
		return authorityBeforePath(value[schemeEnd+len("://"):])
	}
	user, hostAndPath, found := strings.Cut(value, "@")
	if !found || user == "" {
		return "", false
	}
	host, _, found := strings.Cut(hostAndPath, ":")
	if !found || host == "" {
		return "", false
	}
	return user + "@" + host, true
}

func hasSafeProviderHostAuthority(value string) bool {
	if schemeEnd := strings.Index(value, "://"); schemeEnd >= 0 {
		if !isURLScheme(value[:schemeEnd]) {
			return false
		}
		value = value[schemeEnd+len("://"):]
	}
	authority, found := authorityBeforePath(value)
	return found && repoclone.IsSafeRawAuthority(authority)
}

func authorityBeforePath(value string) (string, bool) {
	if authorityEnd := strings.IndexAny(value, "/?#"); authorityEnd >= 0 {
		value = value[:authorityEnd]
	}
	return value, value != ""
}

func isURLScheme(value string) bool {
	if value == "" || !isASCIILetter(value[0]) {
		return false
	}
	for i := 1; i < len(value); i++ {
		if !isASCIILetter(value[i]) && (value[i] < '0' || value[i] > '9') && value[i] != '+' && value[i] != '-' && value[i] != '.' {
			return false
		}
	}
	return true
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
