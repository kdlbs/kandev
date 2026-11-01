package manifest

import (
	"fmt"
	"regexp"

	"github.com/kandev/kandev/internal/plugins/hostcaps"
)

const (
	// DefaultExecutorStateBudgetBytes is the provider state budget applied to a
	// provider that declares a capability and no state_budget_bytes.
	DefaultExecutorStateBudgetBytes = 8192
	// MinExecutorStateBudgetBytes and MaxExecutorStateBudgetBytes bound
	// state_budget_bytes; the maximum plus the 20480-byte host reserve is the
	// 32768-byte envelope cap.
	MinExecutorStateBudgetBytes = 1024
	MaxExecutorStateBudgetBytes = 12288

	// RuntimeAPIURLHostPublic and RuntimeAPIURLProviderForwarded are the two
	// values of a provider's runtime_api_url.
	RuntimeAPIURLHostPublic        = "host_public"
	RuntimeAPIURLProviderForwarded = "provider_forwarded"

	// WorkspacePreparationCore and WorkspacePreparationProvider are the two
	// values of a provider's workspace_preparation.
	WorkspacePreparationCore     = "core"
	WorkspacePreparationProvider = "provider"

	// DefaultExecutorDestroyTimeoutSeconds is the destroy deadline; a provider
	// that requires drain_receipt can raise it up to MaxExecutorDestroyTimeoutSeconds.
	DefaultExecutorDestroyTimeoutSeconds = 30
	MaxExecutorDestroyTimeoutSeconds     = 300

	maxLaunchPolicyIDs    = 16
	maxStateSchemaVersion = 16
)

var launchPolicyIDPattern = regexp.MustCompile(`^[a-z0-9_.-]{1,64}$`)

// HostCapabilityRequirement is one entry of requires_host_capabilities.
type HostCapabilityRequirement struct {
	Name    string `yaml:"name" json:"name"`
	Version int    `yaml:"version" json:"version"`
}

// ExecutorStateSchema is a second spelling of supported_state_versions: the
// contiguous range [MinReadable, Current].
type ExecutorStateSchema struct {
	Current     int `yaml:"current" json:"current"`
	MinReadable int `yaml:"min_readable" json:"min_readable"`
}

// DeclaresHostCapabilities reports whether the provider requires any capability.
func (p *ExecutorProvider) DeclaresHostCapabilities() bool {
	return len(p.RequiresHostCapabilities) > 0
}

// HostCapabilityRequirements converts the declaration to registry requirements.
func (p *ExecutorProvider) HostCapabilityRequirements() []hostcaps.Requirement {
	out := make([]hostcaps.Requirement, len(p.RequiresHostCapabilities))
	for i, req := range p.RequiresHostCapabilities {
		out[i] = hostcaps.Requirement{Name: req.Name, Version: req.Version}
	}
	return out
}

// expandStateSchema turns state_schema into the contiguous readable list.
func expandStateSchema(prefix string, provider *ExecutorProvider) []error {
	schema := provider.StateSchema
	if schema == nil {
		return nil
	}
	if len(provider.SupportedStateVersions) > 0 {
		return []error{fmt.Errorf("%s declares both state_schema and supported_state_versions", prefix)}
	}
	switch {
	case schema.Current < 1:
		return []error{fmt.Errorf("%s.state_schema.current must be at least 1", prefix)}
	case schema.MinReadable < 1 || schema.MinReadable > schema.Current:
		return []error{fmt.Errorf("%s.state_schema.min_readable must be between 1 and current", prefix)}
	case schema.Current-schema.MinReadable+1 > maxStateSchemaVersion:
		return []error{fmt.Errorf("%s.state_schema must span at most %d versions", prefix, maxStateSchemaVersion)}
	}
	for version := schema.MinReadable; version <= schema.Current; version++ {
		provider.SupportedStateVersions = append(provider.SupportedStateVersions, version)
	}
	return nil
}

// validateHostCapabilities checks requires_host_capabilities, the minimum host
// version, state_budget_bytes, and launch_policy_ids for one provider.
func (m *Manifest) validateHostCapabilities(prefix string, provider *ExecutorProvider) []error {
	var errs []error
	reqs := provider.HostCapabilityRequirements()
	for _, err := range hostcaps.ValidateRequirements(reqs) {
		errs = append(errs, fmt.Errorf("%s.requires_host_capabilities: %w", prefix, err))
	}
	errs = append(errs, m.validateCapabilityMinimumRelease(prefix, reqs)...)
	errs = append(errs, validateStateBudget(prefix, provider)...)
	errs = append(errs, validateDestroyTimeout(prefix, provider)...)
	errs = append(errs, validateRuntimeAPIURL(prefix, provider)...)
	errs = append(errs, validateWorkspacePreparation(prefix, provider)...)
	errs = append(errs, validateLaunchPolicyIDs(prefix, provider)...)
	return errs
}

func (m *Manifest) validateCapabilityMinimumRelease(prefix string, reqs []hostcaps.Requirement) []error {
	release := hostcaps.HighestIntroducedIn(reqs)
	if release == "" {
		return nil
	}
	minimum, valid := NormalizeReleaseVersion(m.MinKandevVersion)
	if valid && CompareVersions(minimum, release) >= 0 {
		return nil
	}
	return []error{fmt.Errorf("%s.requires_host_capabilities needs min_kandev_version >= %s (introduced by %s)",
		prefix, release, highestCapabilityName(reqs, release))}
}

func highestCapabilityName(reqs []hostcaps.Requirement, release string) string {
	for _, req := range reqs {
		if entry, ok := hostcaps.Lookup(req.Name); ok && entry.IntroducedIn == release {
			return req.Name
		}
	}
	return ""
}

func validateStateBudget(prefix string, provider *ExecutorProvider) []error {
	if !provider.DeclaresHostCapabilities() {
		if provider.StateBudgetBytes != 0 {
			return []error{fmt.Errorf("%s.state_budget_bytes requires a declared host capability", prefix)}
		}
		return nil
	}
	if provider.StateBudgetBytes == 0 {
		provider.StateBudgetBytes = DefaultExecutorStateBudgetBytes
		return nil
	}
	if provider.StateBudgetBytes < MinExecutorStateBudgetBytes || provider.StateBudgetBytes > MaxExecutorStateBudgetBytes {
		return []error{fmt.Errorf("%s.state_budget_bytes must be between %d and %d", prefix, MinExecutorStateBudgetBytes, MaxExecutorStateBudgetBytes)}
	}
	return nil
}

func validateDestroyTimeout(prefix string, provider *ExecutorProvider) []error {
	timeout := provider.DestroyTimeoutSeconds
	if timeout == 0 {
		if declaresCapability(provider, "drain_receipt") {
			provider.DestroyTimeoutSeconds = DefaultExecutorDestroyTimeoutSeconds
		}
		return nil
	}
	if timeout < DefaultExecutorDestroyTimeoutSeconds || timeout > MaxExecutorDestroyTimeoutSeconds {
		return []error{fmt.Errorf("%s.destroy_timeout_seconds must be between %d and %d",
			prefix, DefaultExecutorDestroyTimeoutSeconds, MaxExecutorDestroyTimeoutSeconds)}
	}
	if timeout > DefaultExecutorDestroyTimeoutSeconds && !declaresCapability(provider, "drain_receipt") {
		return []error{fmt.Errorf("%s.destroy_timeout_seconds over %d requires the drain_receipt capability",
			prefix, DefaultExecutorDestroyTimeoutSeconds)}
	}
	return nil
}

func validateRuntimeAPIURL(prefix string, provider *ExecutorProvider) []error {
	switch provider.RuntimeAPIURL {
	case "":
		provider.RuntimeAPIURL = RuntimeAPIURLHostPublic
	case RuntimeAPIURLHostPublic:
	case RuntimeAPIURLProviderForwarded:
		if !declaresCapability(provider, "api_url_forward") {
			return []error{fmt.Errorf("%s.runtime_api_url provider_forwarded requires the api_url_forward capability", prefix)}
		}
	default:
		return []error{fmt.Errorf("%s.runtime_api_url must be %s or %s", prefix, RuntimeAPIURLHostPublic, RuntimeAPIURLProviderForwarded)}
	}
	return nil
}

func validateWorkspacePreparation(prefix string, provider *ExecutorProvider) []error {
	switch provider.WorkspacePreparation {
	case "":
		provider.WorkspacePreparation = WorkspacePreparationCore
	case WorkspacePreparationCore:
	case WorkspacePreparationProvider:
		if !declaresCapability(provider, "repository_intent") || !declaresCapability(provider, "workspace_descriptor") {
			return []error{fmt.Errorf("%s.workspace_preparation provider requires the repository_intent and workspace_descriptor capabilities", prefix)}
		}
	default:
		return []error{fmt.Errorf("%s.workspace_preparation must be %s or %s", prefix, WorkspacePreparationCore, WorkspacePreparationProvider)}
	}
	return nil
}

func validateLaunchPolicyIDs(prefix string, provider *ExecutorProvider) []error {
	if len(provider.LaunchPolicyIDs) == 0 {
		return nil
	}
	if !declaresCapability(provider, "launch_policy") {
		return []error{fmt.Errorf("%s.launch_policy_ids requires the launch_policy capability", prefix)}
	}
	if len(provider.LaunchPolicyIDs) > maxLaunchPolicyIDs {
		return []error{fmt.Errorf("%s.launch_policy_ids must list at most %d ids", prefix, maxLaunchPolicyIDs)}
	}
	var errs []error
	seen := make(map[string]struct{}, len(provider.LaunchPolicyIDs))
	for _, id := range provider.LaunchPolicyIDs {
		if !launchPolicyIDPattern.MatchString(id) {
			errs = append(errs, fmt.Errorf("%s.launch_policy_ids contains an invalid id %q", prefix, id))
		}
		if _, dup := seen[id]; dup {
			errs = append(errs, fmt.Errorf("%s.launch_policy_ids contains duplicate id %q", prefix, id))
		}
		seen[id] = struct{}{}
	}
	return errs
}

// RequiresHostCapability reports whether the provider's manifest requires the
// named host capability.
func (p *ExecutorProvider) RequiresHostCapability(name string) bool {
	return declaresCapability(p, name)
}

func declaresCapability(provider *ExecutorProvider, name string) bool {
	for _, req := range provider.RequiresHostCapabilities {
		if req.Name == name {
			return true
		}
	}
	return false
}

// ExecutorProviderWarnings lists non-fatal findings, such as a deprecated
// capability version that still installs.
func (m *Manifest) ExecutorProviderWarnings() []string {
	var warnings []string
	for i := range m.ExecutorProviders {
		warnings = append(warnings, hostcaps.DeprecationWarnings(m.ExecutorProviders[i].HostCapabilityRequirements())...)
	}
	return warnings
}
