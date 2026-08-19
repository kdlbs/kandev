package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
)

const (
	ComparisonTargetMetadataKey = "comparison_target"
	ComparisonTargetVersion     = 1

	ComparisonTargetProviderGitHub = RemoteContributionProviderGitHub
	ComparisonTargetProviderGitLab = RemoteContributionProviderGitLab

	ComparisonTargetKindPullRequest  = RemoteContributionKindPullRequest
	ComparisonTargetKindMergeRequest = RemoteContributionKindMergeRequest
)

// ComparisonTargetRepository is the credential-free provider identity used
// by a comparison target. Keeping the shape shared with remote contributions
// ensures both metadata formats have the same URL and path validation.
type ComparisonTargetRepository = RemoteContributionRepository

// ComparisonTarget binds a task-repository attachment to the exact provider
// repository and branch that should be used as its comparison base.
type ComparisonTarget struct {
	Version          int                        `json:"version"`
	Provider         string                     `json:"provider"`
	Kind             string                     `json:"kind"`
	Number           int                        `json:"number"`
	HeadBranch       string                     `json:"head_branch"`
	TargetBranch     string                     `json:"target_branch"`
	HeadRepository   ComparisonTargetRepository `json:"head_repository"`
	TargetRepository ComparisonTargetRepository `json:"target_repository"`
}

// ComparisonTargetCandidate is the provider-owned identity presented during
// PR/MR association. The task service decides whether it matches an attached
// checkout before persisting it as a ComparisonTarget.
type ComparisonTargetCandidate struct {
	Provider         string                     `json:"provider"`
	Kind             string                     `json:"kind"`
	Number           int                        `json:"number"`
	HeadBranch       string                     `json:"head_branch"`
	TargetBranch     string                     `json:"target_branch"`
	HeadRepository   ComparisonTargetRepository `json:"head_repository"`
	TargetRepository ComparisonTargetRepository `json:"target_repository"`
}

// ComparisonTargetReconciliationStatus describes the durable outcome of
// matching a provider change to task-repository attachments.
type ComparisonTargetReconciliationStatus string

const (
	ComparisonTargetMatched        ComparisonTargetReconciliationStatus = "matched"
	ComparisonTargetSameRepository ComparisonTargetReconciliationStatus = "same_repository"
	ComparisonTargetNoMatch        ComparisonTargetReconciliationStatus = "no_match"
	ComparisonTargetAmbiguous      ComparisonTargetReconciliationStatus = "ambiguous"
)

// ComparisonTargetReconciliation is returned by the task service so provider
// integrations can distinguish a persisted binding from a deliberate no-op.
type ComparisonTargetReconciliation struct {
	Status           ComparisonTargetReconciliationStatus `json:"status"`
	TaskRepositoryID string                               `json:"task_repository_id,omitempty"`
	Target           *ComparisonTarget                    `json:"target,omitempty"`
}

// Build validates and materializes a provider candidate into the versioned
// persisted form.
func (c ComparisonTargetCandidate) Build() (ComparisonTarget, error) {
	target := ComparisonTarget{
		Version:          ComparisonTargetVersion,
		Provider:         c.Provider,
		Kind:             c.Kind,
		Number:           c.Number,
		HeadBranch:       c.HeadBranch,
		TargetBranch:     c.TargetBranch,
		HeadRepository:   c.HeadRepository,
		TargetRepository: c.TargetRepository,
	}
	if err := target.Validate(); err != nil {
		return ComparisonTarget{}, err
	}
	return target, nil
}

// Validate rejects malformed or credential-bearing data rehydrated from task
// metadata. Provider services perform live PR/MR authorization separately.
func (c ComparisonTarget) Validate() error {
	if c.Version != ComparisonTargetVersion {
		return fmt.Errorf("comparison target version %d is not supported", c.Version)
	}
	if err := validateRemoteContributionKind(c.Provider, c.Kind); err != nil {
		return fmt.Errorf("comparison target provider: %w", err)
	}
	if c.Number <= 0 {
		return errors.New("comparison target number must be positive")
	}
	headBranch, err := NormalizeComparisonBranch(c.HeadBranch)
	if err != nil {
		return fmt.Errorf("comparison target head_branch: %w", err)
	}
	targetBranch, err := NormalizeComparisonBranch(c.TargetBranch)
	if err != nil {
		return fmt.Errorf("comparison target target_branch: %w", err)
	}
	if headBranch != c.HeadBranch || targetBranch != c.TargetBranch {
		return errors.New("comparison target branches must be normalized")
	}
	if err := validateComparisonTargetProviderHost(c.Provider, c.HeadRepository.Host); err != nil {
		return fmt.Errorf("comparison target provider: %w", err)
	}
	if err := validateComparisonTargetProviderHost(c.Provider, c.TargetRepository.Host); err != nil {
		return fmt.Errorf("comparison target provider: %w", err)
	}
	if !strings.EqualFold(c.HeadRepository.Host, c.TargetRepository.Host) {
		return errors.New("comparison target repositories must use the same provider host")
	}
	if err := validateRemoteContributionRepository(c.HeadRepository); err != nil {
		return fmt.Errorf("comparison target head_repository: %w", err)
	}
	if err := validateRemoteContributionRepository(c.TargetRepository); err != nil {
		return fmt.Errorf("comparison target target_repository: %w", err)
	}
	return nil
}

func validateComparisonTargetProviderHost(provider, host string) error {
	if strings.TrimSpace(host) == "" {
		return errors.New("repository host is required")
	}
	if provider == ComparisonTargetProviderGitHub && !strings.EqualFold(host, "github.com") {
		return fmt.Errorf("GitHub repository host %q is unsupported", host)
	}
	return nil
}

func NormalizeComparisonBranch(branch string) (string, error) {
	branch = strings.TrimPrefix(branch, "refs/heads/")
	if strings.HasPrefix(branch, "refs/") || strings.HasPrefix(branch, "origin/") {
		return "", errors.New("branch must be a local branch name")
	}
	if !securityutil.IsValidBranchName(branch) {
		return "", errors.New("branch name is invalid")
	}
	return branch, nil
}

// ComparisonTargetRepositoriesEqual compares provider repository identity
// without considering display names or unrelated local configuration.
func ComparisonTargetRepositoriesEqual(left, right ComparisonTargetRepository) bool {
	return repositoryIdentityEqual(left, right)
}

// Equal reports whether two targets identify the same persisted binding.
func (c ComparisonTarget) Equal(other ComparisonTarget) bool {
	return reflect.DeepEqual(c, other)
}

// ChangeIdentityEqual reports whether two targets belong to the same provider
// change, independent of a retargeted branch or repository.
func (c ComparisonTarget) ChangeIdentityEqual(other ComparisonTarget) bool {
	return c.Provider == other.Provider &&
		c.Kind == other.Kind &&
		c.Number == other.Number &&
		ComparisonTargetRepositoriesEqual(c.TargetRepository, other.TargetRepository)
}

// IsSameRepository compares provider repository identity, not display names.
func (c ComparisonTarget) IsSameRepository(repository ComparisonTargetRepository) bool {
	return repositoryIdentityEqual(c.TargetRepository, repository)
}

func repositoryIdentityEqual(left, right ComparisonTargetRepository) bool {
	if !strings.EqualFold(left.Host, right.Host) || !strings.EqualFold(left.Path, right.Path) {
		return false
	}
	if left.ProviderID != "" && right.ProviderID != "" {
		return left.ProviderID == right.ProviderID
	}
	return strings.EqualFold(left.RemoteURL, right.RemoteURL)
}

// DisplayIdentity is the concise, credential-free target shown in summaries.
func (c ComparisonTarget) DisplayIdentity() string {
	return c.TargetRepository.Path + ":" + c.TargetBranch
}

// ComparisonRemoteName returns the deterministic, comparison-only remote
// name. It includes the target branch so two targets on one repository cannot
// accidentally share a fetched remote ref.
func (c ComparisonTarget) ComparisonRemoteName() string {
	identity := strings.Join([]string{
		c.Provider,
		c.Kind,
		c.TargetRepository.Host,
		c.TargetRepository.Path,
		c.TargetRepository.ProviderID,
		c.TargetRepository.RemoteURL,
		c.TargetBranch,
	}, "|")
	sum := sha256.Sum256([]byte(identity))
	return "compare-" + hex.EncodeToString(sum[:])[:12]
}

// ComparisonRef returns the authoritative remote-tracking ref for this
// target. Callers must fetch this exact ref and must not substitute origin.
func (c ComparisonTarget) ComparisonRef() string {
	return "refs/remotes/" + c.ComparisonRemoteName() + "/" + c.TargetBranch
}

// PutComparisonTarget stores a validated target while preserving unrelated
// task-repository metadata.
func PutComparisonTarget(metadata map[string]interface{}, target *ComparisonTarget) error {
	if target == nil {
		return errors.New("comparison target is required")
	}
	if err := target.Validate(); err != nil {
		return err
	}
	if metadata == nil {
		return errors.New("metadata map is required")
	}
	metadata[ComparisonTargetMetadataKey] = target
	return nil
}

// LoadComparisonTarget decodes and validates a target from typed or
// JSON-rehydrated metadata. A missing key is not an error.
func LoadComparisonTarget(metadata map[string]interface{}) (ComparisonTarget, bool, error) {
	return loadValidatedMetadata(
		metadata,
		ComparisonTargetMetadataKey,
		"comparison target",
		"comparison target is nil",
		ComparisonTarget.Validate,
	)
}

// RemoveComparisonTarget removes a target only when expected owns the current
// binding. A nil expected value means an explicitly requested unconditional
// removal, used by manual comparison selection.
func RemoveComparisonTarget(metadata map[string]interface{}, expected *ComparisonTarget) (bool, error) {
	if metadata == nil {
		return false, nil
	}
	current, ok, err := LoadComparisonTarget(metadata)
	if err != nil || !ok {
		return false, err
	}
	if expected != nil && !current.ChangeIdentityEqual(*expected) {
		return false, nil
	}
	delete(metadata, ComparisonTargetMetadataKey)
	return true, nil
}
