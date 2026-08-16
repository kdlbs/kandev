package models

import (
	"errors"
	"fmt"
	"strings"
)

const (
	ContributionDestinationMetadataKey = "contribution_destination"
	ContributionDestinationVersion     = 1

	ContributionDestinationProviderGitHub = "github"
)

// ContributionDestinationRepository is an alias for the shared, validated
// credential-free repository identity used by remote contributions.
type ContributionDestinationRepository = RemoteContributionRepository

// ContributionDestinationCredentialBinding identifies the exact workspace
// automation connection that authorized a managed destination. It contains
// only stable connection metadata, never a token or secret.
type ContributionDestinationCredentialBinding struct {
	Source                  string `json:"source"`
	Login                   string `json:"login,omitempty"`
	InstallationID          int64  `json:"installation_id,omitempty"`
	AppRegistrationID       string `json:"app_registration_id,omitempty"`
	CredentialGeneration    int64  `json:"credential_generation"`
	AppCredentialGeneration int64  `json:"app_credential_generation,omitempty"`
}

func (b ContributionDestinationCredentialBinding) Validate() error {
	if strings.TrimSpace(b.Source) == "" {
		return errors.New("contribution destination credential source is required")
	}
	if b.CredentialGeneration <= 0 {
		return errors.New("contribution destination credential generation must be positive")
	}
	switch b.Source {
	case "pat", "gh_cli", "legacy_shared":
		if strings.TrimSpace(b.Login) == "" {
			return errors.New("contribution destination human credential login is required")
		}
	case "github_app_installation":
		if b.InstallationID <= 0 || strings.TrimSpace(b.AppRegistrationID) == "" || b.AppCredentialGeneration <= 0 {
			return errors.New("contribution destination app credential identity is incomplete")
		}
	default:
		return fmt.Errorf("contribution destination credential source %q is unsupported", b.Source)
	}
	return nil
}

// ContributionDestination identifies the canonical repository and the exact
// repository that may receive the task branch before a pull request exists.
// It is authored by the server after provider verification and persisted on
// the canonical task-repository attachment.
type ContributionDestination struct {
	Version           int                                       `json:"version"`
	Provider          string                                    `json:"provider"`
	SourceRepository  ContributionDestinationRepository         `json:"source_repository"`
	TargetRepository  ContributionDestinationRepository         `json:"target_repository"`
	CredentialBinding *ContributionDestinationCredentialBinding `json:"credential_binding,omitempty"`
}

// Validate is the fail-closed boundary for a destination rehydrated from task
// metadata. Provider-specific parent and permission checks happen before a
// binding is created by the GitHub service.
func (d ContributionDestination) Validate() error {
	if d.Version != ContributionDestinationVersion {
		return fmt.Errorf("contribution destination version %d is not supported", d.Version)
	}
	if d.Provider != ContributionDestinationProviderGitHub {
		return fmt.Errorf("contribution destination provider %q is unsupported", d.Provider)
	}
	if err := validateContributionDestinationRepository("source_repository", d.SourceRepository); err != nil {
		return err
	}
	if err := validateContributionDestinationRepository("target_repository", d.TargetRepository); err != nil {
		return err
	}
	if !strings.EqualFold(d.SourceRepository.Host, d.TargetRepository.Host) {
		return errors.New("contribution destination source and target hosts do not match")
	}
	if strings.EqualFold(d.SourceRepository.Path, d.TargetRepository.Path) ||
		(d.SourceRepository.ProviderID != "" && d.SourceRepository.ProviderID == d.TargetRepository.ProviderID) {
		return errors.New("contribution destination source and target must be different repositories")
	}
	return nil
}

func validateContributionDestinationRepository(label string, repository ContributionDestinationRepository) error {
	if err := validateRemoteContributionRepository(repository); err != nil {
		return fmt.Errorf("contribution destination %s: %w", label, err)
	}
	if !strings.EqualFold(repository.Host, "github.com") {
		return fmt.Errorf("contribution destination %s host %q is not GitHub", label, repository.Host)
	}
	parts := strings.Split(repository.Path, "/")
	if len(parts) != 2 || !validRemoteContributionPath(parts[0]) || !validRemoteContributionPath(parts[1]) {
		return fmt.Errorf("contribution destination %s repository identity is invalid", label)
	}
	if strings.TrimSpace(repository.ProviderID) == "" || strings.TrimSpace(repository.ProviderID) != repository.ProviderID ||
		strings.ContainsAny(repository.ProviderID, "\r\n\t ") {
		return fmt.Errorf("contribution destination %s provider_id is required", label)
	}
	return nil
}

// ContributionRemoteName returns the stable remote name for the exact
// destination. It intentionally shares the remote namespace with an existing
// RemoteContribution whose source repository is the same fork.
func (d ContributionDestination) ContributionRemoteName() string {
	return contributionRemoteName(d.Provider, d.TargetRepository)
}

// ContributionDestinationRemoteName is the explicit pre-PR spelling used by
// callers that need to distinguish a destination from a post-PR contribution.
func (d ContributionDestination) ContributionDestinationRemoteName() string {
	return d.ContributionRemoteName()
}

// PutContributionDestination stores a validated server-authored binding while
// preserving unrelated task-repository metadata.
func PutContributionDestination(metadata map[string]interface{}, destination *ContributionDestination) error {
	if destination == nil {
		return errors.New("contribution destination is required")
	}
	if err := destination.Validate(); err != nil {
		return err
	}
	if metadata == nil {
		return errors.New("metadata map is required")
	}
	metadata[ContributionDestinationMetadataKey] = destination
	return nil
}

// LoadContributionDestination decodes and validates a destination from typed
// or JSON-rehydrated metadata. A missing key is not an error.
func LoadContributionDestination(metadata map[string]interface{}) (ContributionDestination, bool, error) {
	return loadValidatedMetadata(
		metadata,
		ContributionDestinationMetadataKey,
		"contribution destination",
		"contribution destination is nil",
		ContributionDestination.Validate,
	)
}
