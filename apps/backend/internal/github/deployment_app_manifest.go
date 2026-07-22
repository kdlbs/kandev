package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	DeploymentAppManifestRevision = 1
	deploymentAppManifestFlowTTL  = time.Hour
	deploymentAppNameMaxLength    = 34
)

var (
	ErrDeploymentAppManifestOwnerInvalid     = errors.New("GitHub App owner is invalid")
	ErrAppRegistrationIDInvalid              = errors.New("GitHub App registration ID is invalid")
	ErrAppRegistrationVisibilityInvalid      = errors.New("GitHub App visibility is invalid")
	ErrDeploymentAppManifestStateUnavailable = errors.New(
		"GitHub App manifest state is expired, consumed, or invalid",
	)
	ErrPublicGitHubBaseURLInvalid = errors.New(
		"public GitHub base URL must be an HTTPS origin without credentials, path, query, or fragment",
	)
	ErrPublicGitHubBaseURLNotGlobal = errors.New(
		"public GitHub base URL must resolve only to globally routable addresses",
	)
	ErrPublicGitHubBaseURLUnresolvable = errors.New(
		"public GitHub base URL could not be resolved",
	)
	githubOwnerLoginPattern              = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
	globallyRoutableSpecialAddressRanges = mustParseAddressPrefixes(
		"192.0.0.9/32",
		"192.0.0.10/32",
		"2001:1::1/128",
		"2001:1::2/128",
		"2001:1::3/128",
		"2001:3::/32",
		"2001:4:112::/48",
		"2001:20::/28",
		"2001:30::/28",
	)
	nonGlobalAddressRanges = mustParseAddressPrefixes(
		"0.0.0.0/8",
		"10.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"192.88.99.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",
		"::/128",
		"::1/128",
		"64:ff9b:1::/48",
		"100::/64",
		"100:0:0:1::/64",
		"2001::/23",
		"2001:db8::/32",
		"2002::/16",
		"3fff::/20",
		"5f00::/16",
		"fc00::/7",
		"fe80::/10",
		"fec0::/10",
		"ff00::/8",
	)
)

type ManifestOwnerType string

const (
	ManifestOwnerUser         ManifestOwnerType = "user"
	ManifestOwnerOrganization ManifestOwnerType = "organization"
)

type DeploymentAppManifestHook struct {
	URL    string `json:"url"`
	Active bool   `json:"active"`
}

type DeploymentAppManifest struct {
	Name                  string                    `json:"name"`
	Description           string                    `json:"description"`
	URL                   string                    `json:"url"`
	HookAttributes        DeploymentAppManifestHook `json:"hook_attributes"`
	RedirectURL           string                    `json:"redirect_url"`
	CallbackURLs          []string                  `json:"callback_urls"`
	SetupURL              string                    `json:"setup_url"`
	Public                bool                      `json:"public"`
	DefaultPermissions    map[string]string         `json:"default_permissions"`
	DefaultEvents         []string                  `json:"default_events"`
	RequestOAuthOnInstall bool                      `json:"request_oauth_on_install"`
	SetupOnUpdate         bool                      `json:"setup_on_update"`
}

type DeploymentAppManifestSubmission struct {
	Revision        int
	RegistrationURL string
	Manifest        DeploymentAppManifest
}

type AppRegistrationManifestRequest struct {
	RegistrationID string
	OwnerType      ManifestOwnerType
	OwnerLogin     string
	PublicBaseURL  string
	Visibility     AppRegistrationVisibility
}

func BuildAppRegistrationManifest(
	request AppRegistrationManifestRequest,
) (DeploymentAppManifestSubmission, error) {
	registrationID, err := uuid.Parse(request.RegistrationID)
	if err != nil || registrationID.String() != request.RegistrationID {
		return DeploymentAppManifestSubmission{}, ErrAppRegistrationIDInvalid
	}
	public, err := manifestVisibility(request.Visibility)
	if err != nil {
		return DeploymentAppManifestSubmission{}, err
	}
	submission, err := BuildDeploymentAppManifest(
		request.OwnerType, request.OwnerLogin, request.PublicBaseURL,
	)
	if err != nil {
		return DeploymentAppManifestSubmission{}, err
	}
	baseURL := strings.TrimRight(submission.Manifest.URL, "/") +
		"/api/v1/github/app/registrations/" + registrationID.String()
	submission.Manifest.HookAttributes.URL = baseURL + "/webhook"
	submission.Manifest.RedirectURL = baseURL + "/manifest/callback"
	submission.Manifest.CallbackURLs = []string{baseURL + "/personal/callback"}
	submission.Manifest.SetupURL = baseURL + "/install/callback"
	submission.Manifest.Public = public
	return submission, nil
}

func manifestVisibility(visibility AppRegistrationVisibility) (bool, error) {
	switch visibility {
	case "", AppRegistrationVisibilityPrivate:
		return false, nil
	case AppRegistrationVisibilityPublic:
		return true, nil
	default:
		return false, ErrAppRegistrationVisibilityInvalid
	}
}

type PublicGitHubBaseURLResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

func ValidatePublicGitHubBaseURL(
	ctx context.Context,
	rawURL string,
	resolver PublicGitHubBaseURLResolver,
) (string, error) {
	parsed, err := parsePublicGitHubBaseURL(rawURL)
	if err != nil {
		return "", err
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if address, parseErr := netip.ParseAddr(hostname); parseErr == nil {
		if !isGloballyRoutable(address) {
			return "", ErrPublicGitHubBaseURLNotGlobal
		}
		return canonicalPublicOrigin(parsed, hostname, address), nil
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	addresses, err := resolver.LookupNetIP(ctx, "ip", hostname)
	if err != nil || len(addresses) == 0 {
		return "", ErrPublicGitHubBaseURLUnresolvable
	}
	for _, address := range addresses {
		if !isGloballyRoutable(address) {
			return "", ErrPublicGitHubBaseURLNotGlobal
		}
	}
	return canonicalPublicOrigin(parsed, hostname, netip.Addr{}), nil
}

func BuildDeploymentAppManifest(
	ownerType ManifestOwnerType,
	ownerLogin, publicBaseURL string,
) (DeploymentAppManifestSubmission, error) {
	if !validManifestOwner(ownerType, ownerLogin) {
		return DeploymentAppManifestSubmission{}, ErrDeploymentAppManifestOwnerInvalid
	}
	baseURL, err := canonicalManifestBaseURL(publicBaseURL)
	if err != nil {
		return DeploymentAppManifestSubmission{}, err
	}
	return DeploymentAppManifestSubmission{
		Revision:        DeploymentAppManifestRevision,
		RegistrationURL: manifestRegistrationURL(ownerType, ownerLogin),
		Manifest: DeploymentAppManifest{
			Name:        deploymentAppName(ownerLogin, baseURL),
			Description: "Repository automation for Kandev workspaces.",
			URL:         baseURL,
			HookAttributes: DeploymentAppManifestHook{
				URL: baseURL + "/api/v1/github/app/webhook", Active: true,
			},
			RedirectURL:  baseURL + "/api/v1/github/app/registration/callback",
			CallbackURLs: []string{baseURL + "/api/v1/github/personal-connection/callback"},
			SetupURL:     baseURL + "/api/v1/github/app/install/callback",
			Public:       false,
			DefaultPermissions: map[string]string{
				"actions": "read", "administration": "read", "checks": "read",
				"contents": "write", "issues": "write", "members": "read",
				"metadata": "read", "pull_requests": "write", "statuses": "read",
				"workflows": "write",
			},
			DefaultEvents: []string{
				"installation", "installation_repositories", "github_app_authorization",
			},
			RequestOAuthOnInstall: true,
			SetupOnUpdate:         false,
		},
	}, nil
}

func DeploymentAppManifestFlowExpiresAt(createdAt time.Time) time.Time {
	return createdAt.UTC().Add(deploymentAppManifestFlowTTL)
}

func ValidateDeploymentAppManifestFlow(expiresAt time.Time, consumedAt *time.Time, now time.Time) error {
	if consumedAt != nil || expiresAt.IsZero() || !expiresAt.After(now.UTC()) {
		return ErrDeploymentAppManifestStateUnavailable
	}
	return nil
}

func parsePublicGitHubBaseURL(rawURL string) (*url.URL, error) {
	if strings.TrimSpace(rawURL) != rawURL || rawURL == "" {
		return nil, ErrPublicGitHubBaseURLInvalid
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !validPublicOriginShape(parsed) {
		return nil, ErrPublicGitHubBaseURLInvalid
	}
	if parsed.Hostname() == "" || strings.HasSuffix(parsed.Host, ":") {
		return nil, ErrPublicGitHubBaseURLInvalid
	}
	return parsed, nil
}

func validPublicOriginShape(parsed *url.URL) bool {
	return parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil &&
		parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == "" &&
		(parsed.Path == "" || parsed.Path == "/") && parsed.RawPath == "" && parsed.Opaque == ""
}

func canonicalManifestBaseURL(rawURL string) (string, error) {
	parsed, err := parsePublicGitHubBaseURL(rawURL)
	if err != nil {
		return "", err
	}
	hostname := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	address, _ := netip.ParseAddr(hostname)
	return canonicalPublicOrigin(parsed, hostname, address), nil
}

func canonicalPublicOrigin(parsed *url.URL, hostname string, address netip.Addr) string {
	host := hostname
	if address.IsValid() && address.Is6() {
		host = "[" + hostname + "]"
	}
	if port := parsed.Port(); port != "" && port != "443" {
		host = net.JoinHostPort(hostname, port)
	}
	return "https://" + host
}

func validManifestOwner(ownerType ManifestOwnerType, ownerLogin string) bool {
	return (ownerType == ManifestOwnerUser || ownerType == ManifestOwnerOrganization) &&
		githubOwnerLoginPattern.MatchString(ownerLogin)
}

func manifestRegistrationURL(ownerType ManifestOwnerType, ownerLogin string) string {
	if ownerType == ManifestOwnerOrganization {
		return "https://github.com/organizations/" + url.PathEscape(ownerLogin) + "/settings/apps/new"
	}
	return "https://github.com/settings/apps/new"
}

func deploymentAppName(ownerLogin, baseURL string) string {
	digest := sha256.Sum256([]byte(ownerLogin + "\x00" + baseURL))
	suffix := hex.EncodeToString(digest[:4])
	prefix := "Kandev " + ownerLogin
	maxPrefixLength := deploymentAppNameMaxLength - len(suffix) - 1
	if len(prefix) > maxPrefixLength {
		prefix = strings.TrimRight(prefix[:maxPrefixLength], "-")
	}
	return fmt.Sprintf("%s-%s", prefix, suffix)
}

func isGloballyRoutable(address netip.Addr) bool {
	if !address.IsValid() || address.Zone() != "" {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() ||
		address.IsUnspecified() {
		return false
	}
	for _, addressRange := range globallyRoutableSpecialAddressRanges {
		if addressRange.Contains(address) {
			return true
		}
	}
	for _, addressRange := range nonGlobalAddressRanges {
		if addressRange.Contains(address) {
			return false
		}
	}
	return true
}

func mustParseAddressPrefixes(values ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefixes = append(prefixes, netip.MustParsePrefix(value))
	}
	return prefixes
}
