package github

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/githubhost"
)

// TokenCredentialKind identifies the authority represented by a token. It is
// intentionally separate from the transport so callers retain attribution and
// routing information after a REST/GraphQL client is constructed.
type TokenCredentialKind string

const (
	TokenCredentialPAT          TokenCredentialKind = "pat"
	TokenCredentialCLI          TokenCredentialKind = "gh_cli"
	TokenCredentialInstallation TokenCredentialKind = "github_app_installation"
	TokenCredentialUser         TokenCredentialKind = "github_app_user"
)

// TokenPrincipal describes the GitHub actor behind a bearer token.
type TokenPrincipal struct {
	Kind                    TokenCredentialKind `json:"kind"`
	PrincipalID             string              `json:"principal_id"`
	Login                   string              `json:"login,omitempty"`
	InstallationID          int64               `json:"installation_id,omitempty"`
	AppRegistrationID       string              `json:"app_registration_id,omitempty"`
	AppCredentialGeneration int64               `json:"app_credential_generation,omitempty"`
}

// TokenClient implements GitHub REST and GraphQL operations for any bearer
// token source. The token itself remains private while principal metadata is
// available for authorization, rate-limit, cache, and attribution keys.
type TokenClient struct {
	host          githubhost.Host
	token         string
	httpClient    *http.Client
	username      string // cached after first GetAuthenticatedUser call
	rateTracker   *RateTracker
	rateAdmission *RateAdmission
	principal     TokenPrincipal
	mergePollWait func(context.Context, time.Duration) error
}

// PATClient remains an alias so existing callers retain source compatibility.
type PATClient = TokenClient

// NewTokenClient constructs a GitHub client for a token and explicit actor.
func NewTokenClient(token string, principal TokenPrincipal) *TokenClient {
	return NewTokenClientForHost(token, principal, githubhost.Public())
}

// NewPATClient creates a PAT-based GitHub client.
func NewPATClient(token string) *PATClient {
	return NewTokenClient(token, TokenPrincipal{Kind: TokenCredentialPAT})
}

// NewAppUserTokenClient creates a client for a GitHub App user access token.
func NewAppUserTokenClient(token string, githubUserID int64, login string) *TokenClient {
	return NewTokenClient(token, TokenPrincipal{
		Kind:        TokenCredentialUser,
		PrincipalID: fmt.Sprintf("user:%d", githubUserID),
		Login:       login,
	})
}

// Principal returns the actor metadata attached at construction.
func (c *TokenClient) Principal() TokenPrincipal {
	return c.principal
}

// NewTokenClientForHost binds all provider requests to a validated host.
func NewTokenClientForHost(token string, principal TokenPrincipal, host githubhost.Host) *TokenClient {
	return &TokenClient{token: token, principal: principal, host: host, httpClient: newGitHubHTTPClient(30 * time.Second)}
}
func NewPATClientForHost(token string, host githubhost.Host) *PATClient {
	return NewTokenClientForHost(token, TokenPrincipal{Kind: TokenCredentialPAT}, host)
}

func forbiddenGitHubIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// Host returns the immutable provider host selected at construction.
func (c *TokenClient) Host() githubhost.Host { return c.host }

func newGitHubHTTPClient(timeout time.Duration) *http.Client {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{Timeout: timeout, Transport: http.DefaultTransport, CheckRedirect: rejectGitHubRedirect}
	}
	proxied := base.Clone()
	direct := proxied.Clone()
	direct.Proxy = nil
	direct.DialContext = dialGitHubDestination
	return &http.Client{Timeout: timeout, Transport: &githubNetworkTransport{direct: direct, proxied: proxied}, CheckRedirect: rejectGitHubRedirect}
}

type githubNetworkTransport struct{ direct, proxied *http.Transport }

func (t *githubNetworkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.proxied.Proxy != nil {
		proxy, err := t.proxied.Proxy(req)
		if err != nil {
			return nil, err
		}
		if proxy != nil {
			return t.proxied.RoundTrip(req)
		}
	}
	return t.direct.RoundTrip(req)
}

func rejectGitHubRedirect(*http.Request, []*http.Request) error {
	return errors.New("GitHub authenticated redirects are forbidden")
}

// The operator's configured proxy remains a trusted network boundary.
func dialGitHubDestination(ctx context.Context, network, address string) (net.Conn, error) {
	name, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, name)
	if err != nil {
		return nil, err
	}
	for _, addr := range addresses {
		if forbiddenGitHubIP(addr.IP) {
			return nil, errors.New("GitHub direct destination is forbidden")
		}
	}
	dialer := net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, addr := range addresses {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = errors.New("GitHub host resolved no addresses")
	}
	return nil, lastErr
}

func doGitHubRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	if err := requireGitHubOperationCurrent(req.Context()); err != nil {
		return nil, err
	}
	bounded := *client
	bounded.CheckRedirect = rejectGitHubRedirect
	return bounded.Do(req)
}

func (c *TokenClient) restURL(endpoint string) (string, error) {
	if !c.host.Valid() {
		return "", githubhost.ErrInvalidHost
	}
	return githubRESTURL(c.host.RESTURL(), endpoint)
}

func githubRESTURL(base, endpoint string) (string, error) {
	root, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	target, err := url.Parse(endpoint)
	if err != nil || target.User != nil || target.Fragment != "" || strings.HasPrefix(endpoint, "//") {
		return "", errors.New("invalid GitHub API destination")
	}
	if target.IsAbs() {
		if err := validateGitHubAbsoluteURL(root, target); err != nil {
			return "", err
		}
	} else if err := resolveGitHubRelativeURL(root, target, endpoint); err != nil {
		return "", err
	}
	if !safeGitHubPath(target.EscapedPath()) {
		return "", errors.New("invalid GitHub API path")
	}
	return target.String(), nil
}

func safeGitHubPath(escaped string) bool {
	for i := 0; i < 4; i++ {
		lower := strings.ToLower(escaped)
		if strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.ContainsAny(escaped, "\\") {
			return false
		}
		decoded, err := url.PathUnescape(escaped)
		if err != nil {
			return false
		}
		for _, part := range strings.Split(decoded, "/") {
			if part == "." || part == ".." {
				return false
			}
		}
		if decoded == escaped {
			return true
		}
		escaped = decoded
	}
	return false
}

func validateGitHubAbsoluteURL(root, target *url.URL) error {
	if target.Scheme != root.Scheme || target.Host != root.Host {
		return errors.New("GitHub API host mismatch")
	}
	if root.Path != "" && !strings.HasPrefix(target.Path, root.Path+"/") {
		return errors.New("GitHub API path mismatch")
	}
	return nil
}
func resolveGitHubRelativeURL(root, target *url.URL, endpoint string) error {
	if target.Host != "" || !strings.HasPrefix(endpoint, "/") {
		return errors.New("invalid GitHub API path")
	}
	target.Scheme = root.Scheme
	target.Host = root.Host
	target.Path = strings.TrimRight(root.Path, "/") + target.Path
	if target.RawPath != "" {
		target.RawPath = strings.TrimRight(root.Path, "/") + target.RawPath
	}
	return nil
}
