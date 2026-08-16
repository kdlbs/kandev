package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
)

const (
	RemoteContributionMetadataKey = "remote_contribution"
	RemoteContributionVersion     = 1

	RemoteContributionProviderGitHub = "github"
	RemoteContributionProviderGitLab = "gitlab"

	RemoteContributionKindPullRequest  = "pull_request"
	RemoteContributionKindMergeRequest = "merge_request"

	RemoteContributionStateOpen = "open"
)

var (
	remoteContributionRepositoryPathPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	remoteContributionSHA                   = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
)

// RemoteContributionRepository identifies the provider repository that owns
// the writable head branch. It is deliberately credential-free and contains
// no provider-authored display text.
type RemoteContributionRepository struct {
	Host       string `json:"host"`
	Path       string `json:"path"`
	ProviderID string `json:"provider_id,omitempty"`
	RemoteURL  string `json:"remote_url"`
}

// RemoteContribution is the server-authored, versioned identity of an
// existing pull/merge request. It is stored on the target task-repository
// attachment rather than as a second repository attachment.
type RemoteContribution struct {
	Version              int                          `json:"version"`
	Provider             string                       `json:"provider"`
	Kind                 string                       `json:"kind"`
	CanonicalURL         string                       `json:"canonical_url"`
	Number               int                          `json:"number"`
	State                string                       `json:"state"`
	BaseBranch           string                       `json:"base_branch"`
	HeadBranch           string                       `json:"head_branch"`
	HeadSHA              string                       `json:"head_sha"`
	SourceRepository     RemoteContributionRepository `json:"source_repository"`
	CollaborationAllowed bool                         `json:"collaboration_allowed"`
}

// RemoteContributionResolution carries the transient target repository
// identity needed to attach a binding. The target is not duplicated in the
// persisted binding; the task-repository attachment remains its owner.
type RemoteContributionResolution struct {
	Binding             RemoteContribution
	TargetProvider      string
	TargetHost          string
	TargetPath          string
	TargetProviderID    string
	TargetRemoteURL     string
	TargetDefaultBranch string
}

// Validate rejects malformed, stale, or credential-bearing persisted data.
// Provider services perform the provider-specific identity and collaboration
// checks before constructing a binding; this method is the runtime fail-closed
// boundary for data rehydrated from task metadata.
func (c RemoteContribution) Validate() error {
	if c.Version != RemoteContributionVersion {
		return fmt.Errorf("remote contribution version %d is not supported", c.Version)
	}
	if err := validateRemoteContributionKind(c.Provider, c.Kind); err != nil {
		return err
	}
	if c.Number <= 0 {
		return errors.New("remote contribution number must be positive")
	}
	if c.State != RemoteContributionStateOpen {
		return fmt.Errorf("remote contribution state %q is not open", c.State)
	}
	if err := validateRemoteContributionRefs(c); err != nil {
		return err
	}
	canonicalURL, err := parseCredentialFreeURL(c.CanonicalURL)
	if err != nil {
		return fmt.Errorf("remote contribution canonical_url: %w", err)
	}
	if err := validateRemoteContributionURL(canonicalURL, c.Provider, c.Number); err != nil {
		return fmt.Errorf("remote contribution canonical_url: %w", err)
	}
	if err := validateRemoteContributionRepository(c.SourceRepository); err != nil {
		return fmt.Errorf("remote contribution source_repository: %w", err)
	}
	if !strings.EqualFold(canonicalURL.Host, c.SourceRepository.Host) {
		return errors.New("remote contribution canonical_url host does not match source repository host")
	}
	return nil
}

func validateRemoteContributionKind(provider, kind string) error {
	switch provider {
	case RemoteContributionProviderGitHub:
		if kind != RemoteContributionKindPullRequest {
			return fmt.Errorf("remote contribution kind %q is invalid for GitHub", kind)
		}
	case RemoteContributionProviderGitLab:
		if kind != RemoteContributionKindMergeRequest {
			return fmt.Errorf("remote contribution kind %q is invalid for GitLab", kind)
		}
	default:
		return fmt.Errorf("remote contribution provider %q is unsupported", provider)
	}
	return nil
}

func validateRemoteContributionRefs(binding RemoteContribution) error {
	if !securityutil.IsValidBaseBranchRef(binding.BaseBranch) {
		return fmt.Errorf("remote contribution base_branch %q is invalid", binding.BaseBranch)
	}
	if !securityutil.IsValidBranchName(binding.HeadBranch) {
		return fmt.Errorf("remote contribution head_branch %q is invalid", binding.HeadBranch)
	}
	if !remoteContributionSHA.MatchString(binding.HeadSHA) {
		return fmt.Errorf("remote contribution head_sha is invalid")
	}
	return nil
}

// ContributionRemoteName returns the stable, source-specific Git remote name
// used by worktree and remote-executor materialization. The input is limited
// to provider-authored identity fields and the credential-free source URL, so
// two validated bindings cannot accidentally alias different forks.
func (c RemoteContribution) ContributionRemoteName() string {
	return contributionRemoteName(c.Provider, c.SourceRepository)
}

func contributionRemoteName(provider string, repository RemoteContributionRepository) string {
	identity := provider + "|" + repository.Host + "|" + repository.Path + "|" + repository.RemoteURL
	sum := sha256.Sum256([]byte(identity))
	return "contrib-" + hex.EncodeToString(sum[:])[:12]
}

func validateRemoteContributionURL(parsed *url.URL, provider string, number int) error {
	path := strings.TrimRight(parsed.Path, "/")
	suffix := fmt.Sprintf("/%d", number)
	switch provider {
	case RemoteContributionProviderGitHub:
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if !strings.EqualFold(parsed.Host, "github.com") || len(parts) != 4 ||
			!validRemoteContributionPath(parts[0]+"/"+parts[1]) || parts[2] != "pull" || parts[3] != strings.TrimPrefix(suffix, "/") {
			return errors.New("does not identify the expected pull request")
		}
	case RemoteContributionProviderGitLab:
		marker := "/-/merge_requests"
		markerIndex := strings.LastIndex(path, marker)
		if markerIndex <= 0 || path[markerIndex+len(marker):] != suffix ||
			!validRemoteContributionPath(strings.Trim(path[:markerIndex], "/")) {
			return errors.New("does not identify the expected merge request")
		}
	}
	return nil
}

func validRemoteContributionPath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.Contains(path, "//") || strings.Contains(path, "..") {
		return false
	}
	return remoteContributionRepositoryPathPattern.MatchString(path)
}

func validateRemoteContributionRepository(repository RemoteContributionRepository) error {
	host := strings.TrimSpace(repository.Host)
	path := strings.TrimSpace(repository.Path)
	if host == "" || path == "" {
		return errors.New("host and path are required")
	}
	if err := validateRemoteContributionRepositoryIdentity(host, path); err != nil {
		return err
	}
	parsed, err := parseCredentialFreeURL(repository.RemoteURL)
	if err != nil {
		return fmt.Errorf("remote_url: %w", err)
	}
	if !strings.EqualFold(parsed.Host, host) {
		return errors.New("remote_url host does not match repository host")
	}
	remotePath := strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/")
	if !strings.EqualFold(remotePath, path) {
		return errors.New("remote_url path does not match repository path")
	}
	return nil
}

func validateRemoteContributionRepositoryIdentity(host, path string) error {
	if strings.ContainsAny(host, "/@?#") || !validRemoteContributionPath(path) {
		return errors.New("repository identity is invalid")
	}
	return validateRemoteContributionAuthority(host)
}

func validateRemoteContributionAuthority(host string) error {
	authority, err := url.Parse("https://" + host)
	if err != nil || authority.Host != host || authority.Hostname() == "" {
		return errors.New("repository identity is invalid")
	}
	if authority.User != nil || authority.Path != "" || authority.RawQuery != "" || authority.Fragment != "" {
		return errors.New("repository identity is invalid")
	}
	if err := validateRemoteContributionPort(authority.Port()); err != nil {
		return err
	}
	return nil
}

func validateRemoteContributionPort(port string) error {
	if port == "" {
		return nil
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return errors.New("repository identity is invalid")
	}
	return nil
}

func parseCredentialFreeURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) != raw || raw == "" {
		return nil, errors.New("URL must be non-empty and trimmed")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("URL must be an HTTPS URL without credentials, query, or fragment")
	}
	return parsed, nil
}

// PutRemoteContribution stores a validated binding under the reserved
// metadata key while preserving all unrelated metadata fields.
func PutRemoteContribution(metadata map[string]interface{}, binding *RemoteContribution) error {
	if binding == nil {
		return errors.New("remote contribution binding is required")
	}
	if err := binding.Validate(); err != nil {
		return err
	}
	if metadata == nil {
		return errors.New("metadata map is required")
	}
	metadata[RemoteContributionMetadataKey] = binding
	return nil
}

func loadValidatedMetadata[T any](
	metadata map[string]interface{},
	key string,
	label string,
	nilMessage string,
	validate func(T) error,
) (T, bool, error) {
	var zero T
	if metadata == nil {
		return zero, false, nil
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return zero, false, nil
	}
	var value T
	switch typed := raw.(type) {
	case *T:
		if typed == nil {
			return zero, false, errors.New(nilMessage)
		}
		value = *typed
	case T:
		value = typed
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return zero, false, fmt.Errorf("encode %s: %w", label, err)
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return zero, false, fmt.Errorf("decode %s: %w", label, err)
		}
	}
	if err := validate(value); err != nil {
		return zero, false, err
	}
	return value, true, nil
}

// LoadRemoteContribution decodes and validates a binding from typed or
// JSON-rehydrated metadata. A missing key is not an error.
func LoadRemoteContribution(metadata map[string]interface{}) (RemoteContribution, bool, error) {
	return loadValidatedMetadata(
		metadata,
		RemoteContributionMetadataKey,
		"remote contribution",
		"remote contribution binding is nil",
		RemoteContribution.Validate,
	)
}
