package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/gitconfigenv"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const legacyExecutorTypeLocalPC = "local_pc"

const registeredLaunchRollbackRetries = 3

var registeredLaunchRollbackRetryDelays = [...]time.Duration{
	100 * time.Millisecond,
	500 * time.Millisecond,
	2 * time.Second,
}

var errTaskCleanupActive = errors.New("task cleanup is active")

// resolveAgentProfile resolves the agent profile and returns the agent type name and profile info.
func (m *Manager) resolveAgentProfile(ctx context.Context, req *LaunchRequest) (string, *AgentProfileInfo, error) {
	profileID := executionProfileID(req)
	if m.profileResolver == nil {
		// Fallback: treat AgentProfileID as agent type directly (for backward compat)
		m.logger.Warn("no profile resolver configured, using profile ID as agent type",
			zap.String("agent_type", profileID))
		return profileID, nil, nil
	}
	profileInfo, err := m.profileResolver.ResolveProfile(ctx, profileID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to resolve agent profile: %w", err)
	}
	// Legacy model-only routes still use overlays until their persisted config
	// is migrated. A concrete execution profile is authoritative and must not
	// be mixed with fields from another provider.
	if !hasConcreteRouteExecutionProfile(req) {
		applyRouteOverrideToProfile(profileInfo, req)
	}
	m.logger.Debug("resolved agent profile",
		zap.String("profile_id", profileID),
		zap.String("agent_name", profileInfo.AgentName),
		zap.String("agent_type", profileInfo.AgentName))
	return profileInfo.AgentName, profileInfo, nil
}

func executionProfileID(req *LaunchRequest) string {
	if req == nil {
		return ""
	}
	if req.ExecutionProfileID != "" {
		return req.ExecutionProfileID
	}
	return req.AgentProfileID
}

func hasConcreteRouteExecutionProfile(req *LaunchRequest) bool {
	return req != nil && req.RouteOverride != nil && req.RouteOverride.ExecutionProfileID != ""
}

// appendRouteOverrideFlags preserves legacy model-only routing overlays.
// Concrete execution profiles own their complete CLI configuration.
func appendRouteOverrideFlags(tokens []string, req *LaunchRequest) []string {
	if req == nil || hasConcreteRouteExecutionProfile(req) || req.RouteOverride == nil || len(req.RouteOverride.Flags) == 0 {
		return tokens
	}
	out := make([]string, 0, len(tokens)+len(req.RouteOverride.Flags))
	out = append(out, tokens...)
	out = append(out, req.RouteOverride.Flags...)
	return out
}

// applyRouteOverrideToProfile mutates profileInfo in-place when the
// request carries a RouteOverride. Empty fields on the override are
// preserved on the profile — the override only replaces explicit values.
// Mode override is applied unconditionally because routing's mode
// belongs to the provider, not the base profile.
func applyRouteOverrideToProfile(profile *AgentProfileInfo, req *LaunchRequest) {
	if profile == nil || req == nil || req.RouteOverride == nil {
		return
	}
	ov := req.RouteOverride
	if ov.ProviderID != "" {
		profile.AgentName = ov.ProviderID
	}
	if ov.Model != "" {
		profile.Model = ov.Model
	}
	profile.Mode = ov.Mode
}

// trustedExecutorConfigKeys are the metadata keys whose value MUST come from
// the configured executor record — never from request-supplied metadata —
// because they steer the connection (host, fingerprint, identity). Letting a
// task override them would allow pivoting an SSH launch to a different host
// or bypassing the pinned host-key.
var trustedExecutorConfigKeys = map[string]bool{
	MetadataKeySSHHost:                         true,
	MetadataKeySSHHostAlias:                    true,
	MetadataKeySSHPort:                         true,
	MetadataKeySSHUser:                         true,
	MetadataKeySSHHostFingerprint:              true,
	MetadataKeySSHIdentitySource:               true,
	MetadataKeySSHIdentityFile:                 true,
	MetadataKeySSHProxyJump:                    true,
	MetadataKeyKubernetesAuthMode:              true,
	MetadataKeyKubernetesKubeconfigPath:        true,
	MetadataKeyKubernetesKubeContext:           true,
	MetadataKeyKubernetesConfigNamespace:       true,
	MetadataKeyKubernetesRequestTimeoutSeconds: true,
}

func isTrustedExecutorConfigKey(k string) bool { return trustedExecutorConfigKeys[k] }

var kubernetesProfileMetadataKeys = [...]string{
	MetadataKeyKubernetesProfilePlatform,
	MetadataKeyKubernetesProfileMainContainer,
	MetadataKeyKubernetesPodTemplateYAML,
	MetadataKeyKubernetesWorkspaceMode,
	MetadataKeyKubernetesWorkspaceSize,
	MetadataKeyKubernetesWorkspaceStorageClass,
	MetadataKeyKubernetesWorkspaceAccessModes,
	MetadataKeyKubernetesWorkspaceClaimName,
}

var kubernetesConnectionMetadataKeys = [...]string{
	MetadataKeyKubernetesAuthMode,
	MetadataKeyKubernetesKubeconfigPath,
	MetadataKeyKubernetesKubeContext,
	MetadataKeyKubernetesConfigNamespace,
	MetadataKeyKubernetesRequestTimeoutSeconds,
}

// applyAuthoritativeKubernetesProfileConfig replaces fresh-launch Pod and
// workspace inputs with the stored executor-profile value. A retained launch
// keeps its immutable lifecycle-produced snapshot so profile edits or deletion
// cannot change or prevent recovery of the recorded workload.
func (m *Manager) applyAuthoritativeKubernetesProfileConfig(
	ctx context.Context,
	req *LaunchRequest,
	metadata map[string]interface{},
) error {
	if req == nil || req.ExecutorType != string(models.ExecutorTypeKubernetes) {
		return nil
	}
	if req.PreviousExecutionID != "" && hasCompleteKubernetesRecordedResumeMetadata(metadata) {
		return nil
	}
	if m.executorProfileReader == nil {
		return errors.New("load Kubernetes executor profile: profile reader is not configured")
	}
	profileID := strings.TrimSpace(getMetadataString(metadata, MetadataKeyExecutorProfileID))
	if profileID == "" {
		return errors.New("load Kubernetes executor profile: profile ID is missing")
	}
	profile, err := m.executorProfileReader.GetExecutorProfile(ctx, profileID)
	if err != nil {
		return fmt.Errorf("load Kubernetes executor profile %q: %w", profileID, err)
	}
	if profile == nil {
		return fmt.Errorf("load Kubernetes executor profile %q: not found", profileID)
	}
	if strings.TrimSpace(profile.ExecutorID) == "" {
		return fmt.Errorf("load Kubernetes executor profile %q: executor ID is missing", profileID)
	}
	executorID := strings.TrimSpace(getMetadataString(metadata, "executor_id"))
	if executorID != "" && executorID != profile.ExecutorID {
		return fmt.Errorf(
			"load Kubernetes executor profile %q: belongs to executor %q, not %q",
			profileID, profile.ExecutorID, executorID,
		)
	}
	typedProfile, err := kubeexecutor.ParseProfileConfig(profile.Config)
	if err != nil {
		return fmt.Errorf("load Kubernetes executor profile %q: invalid config: %w", profileID, err)
	}
	metadata["executor_id"] = profile.ExecutorID
	if err := applyKubernetesProfileConfigToMetadata(metadata, typedProfile); err != nil {
		return fmt.Errorf("load Kubernetes executor profile %q: canonicalize config: %w", profileID, err)
	}
	return nil
}

func applyKubernetesProfileConfigToMetadata(
	metadata map[string]interface{},
	profile kubeexecutor.ProfileConfig,
) error {
	accessModes := ""
	if len(profile.Workspace.AccessModes) > 0 {
		encoded, err := json.Marshal(profile.Workspace.AccessModes)
		if err != nil {
			return err
		}
		accessModes = string(encoded)
	}
	values := map[string]string{
		MetadataKeyKubernetesProfilePlatform:       string(profile.Platform),
		MetadataKeyKubernetesProfileMainContainer:  profile.MainContainer,
		MetadataKeyKubernetesPodTemplateYAML:       profile.PodTemplateYAML,
		MetadataKeyKubernetesWorkspaceMode:         string(profile.Workspace.Mode),
		MetadataKeyKubernetesWorkspaceSize:         profile.Workspace.Size,
		MetadataKeyKubernetesWorkspaceStorageClass: profile.Workspace.StorageClass,
		MetadataKeyKubernetesWorkspaceAccessModes:  accessModes,
		MetadataKeyKubernetesWorkspaceClaimName:    profile.Workspace.ClaimName,
	}
	for _, key := range kubernetesProfileMetadataKeys {
		metadata[key] = values[key]
	}
	return nil
}

func hasCompleteKubernetesRecordedResumeMetadata(metadata map[string]interface{}) bool {
	required := []string{
		MetadataKeyKubernetesNamespace,
		MetadataKeyKubernetesPodName,
		MetadataKeyKubernetesPodUID,
		MetadataKeyKubernetesMainContainer,
		MetadataKeyKubernetesRuntimeWorkspaceMode,
		MetadataKeyKubernetesAgentctlRemotePort,
		MetadataKeyKubernetesAgentctlInstanceID,
		MetadataKeyKubernetesResourceExecutorID,
		MetadataKeyKubernetesResourceProfileID,
		MetadataKeyKubernetesResourceInstanceID,
		MetadataKeyKubernetesResourceTaskID,
		MetadataKeyKubernetesResourceSessionID,
		MetadataKeyKubernetesResourceEnvironmentID,
		MetadataKeyKubernetesExecutorConfigHash,
		MetadataKeyKubernetesProfileConfigHash,
		MetadataKeyKubernetesTemplateHash,
		MetadataKeyKubernetesProfileSnapshot,
	}
	for _, key := range required {
		if getMetadataString(metadata, key) == "" {
			return false
		}
	}
	switch getMetadataString(metadata, MetadataKeyKubernetesRuntimeWorkspaceMode) {
	case string(kubeexecutor.WorkspaceModeEmptyDir):
		return true
	case string(kubeexecutor.WorkspaceModeManagedPVC):
		return getMetadataString(metadata, MetadataKeyKubernetesPVCName) != "" &&
			getMetadataString(metadata, MetadataKeyKubernetesPVCUID) != "" &&
			getMetadataBool(metadata, MetadataKeyKubernetesPVCCreated)
	case string(kubeexecutor.WorkspaceModeExistingClaim):
		return getMetadataString(metadata, MetadataKeyKubernetesPVCName) != "" &&
			getMetadataString(metadata, MetadataKeyKubernetesPVCUID) != ""
	default:
		return false
	}
}

// buildLaunchMetadata builds runtime metadata for the Launch request.
//
// Per-executor config (host, fingerprint, ssh identity, …) is the trusted
// source for connection-routing decisions, so it overrides any same-key
// values the caller passed in req.Metadata. Other keys (per-task settings
// like setup_script, base_branch, repo_setup_script, etc.) keep the caller's
// value when present.
func buildLaunchMetadata(req *LaunchRequest, mainRepoGitDir, worktreeID, worktreeBranch string) map[string]interface{} {
	metadata := make(map[string]interface{})
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	for k, v := range req.ExecutorConfig {
		if isTrustedExecutorConfigKey(k) {
			// Executor config wins for connection-routing keys so a malicious
			// or buggy task metadata payload can't swap out the SSH host /
			// pinned fingerprint and pivot the launch to a different target.
			metadata[k] = v
			continue
		}
		if _, exists := metadata[k]; !exists {
			metadata[k] = v
		}
	}
	if req.ExecutorType == string(models.ExecutorTypeKubernetes) {
		for _, key := range kubernetesConnectionMetadataKeys {
			metadata[key] = req.ExecutorConfig[key]
		}
	}
	if mainRepoGitDir != "" {
		metadata[MetadataKeyMainRepoGitDir] = mainRepoGitDir
	}
	if worktreeID != "" {
		metadata[MetadataKeyWorktreeID] = worktreeID
	}
	if worktreeBranch != "" {
		metadata[MetadataKeyWorktreeBranch] = worktreeBranch
	}
	// Pass repo info for remote executors (Sprites, remote docker, etc.)
	if req.RepositoryPath != "" {
		metadata[MetadataKeyRepositoryPath] = req.RepositoryPath
	}
	if req.SetupScript != "" {
		metadata[MetadataKeySetupScript] = req.SetupScript
	}
	if req.BaseBranch != "" {
		metadata[MetadataKeyBaseBranch] = req.BaseBranch
	}
	if branches := collectBaseBranches(req); len(branches) > 0 {
		metadata[MetadataKeyBaseBranches] = branches
	}
	return metadata
}

// collectComparisonTargets projects the validated per-repository comparison
// targets into the same workspace-subpath keys used by base branches. The
// target remains credential-free and is revalidated at the runtime boundary.
func collectComparisonTargets(req *LaunchRequest) (map[string]models.ComparisonTarget, error) {
	if req == nil {
		return nil, nil
	}
	specs := req.RepoSpecs()
	if len(specs) == 0 {
		if req.ComparisonTarget == nil {
			return nil, nil
		}
		if err := req.ComparisonTarget.Validate(); err != nil {
			return nil, fmt.Errorf("validate comparison target: %w", err)
		}
		return map[string]models.ComparisonTarget{"": *req.ComparisonTarget}, nil
	}
	targets := make(map[string]models.ComparisonTarget)
	for index, spec := range specs {
		if spec.ComparisonTarget == nil {
			continue
		}
		if err := spec.ComparisonTarget.Validate(); err != nil {
			return nil, fmt.Errorf("validate comparison target for repository %q: %w", spec.RepoName, err)
		}
		key := ""
		if index > 0 {
			key = baseBranchMetadataKey(spec)
		}
		if existing, ok := targets[key]; ok && !existing.Equal(*spec.ComparisonTarget) {
			return nil, fmt.Errorf("multiple comparison targets map to workspace repository %q", key)
		}
		targets[key] = *spec.ComparisonTarget
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return targets, nil
}

func comparisonTargetsFromWorkspaceRepositories(specs []WorkspaceRepositorySpec) (map[string]models.ComparisonTarget, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	targets := make(map[string]models.ComparisonTarget)
	for index, spec := range specs {
		if spec.ComparisonTarget == nil {
			continue
		}
		if err := spec.ComparisonTarget.Validate(); err != nil {
			return nil, fmt.Errorf("validate comparison target for repository %q: %w", spec.RepoName, err)
		}
		key := ""
		if index > 0 {
			key = baseBranchMetadataKey(RepoLaunchSpec{
				RepoName:   spec.RepoName,
				BranchSlug: spec.BranchSlug,
			})
		}
		if existing, ok := targets[key]; ok && !existing.Equal(*spec.ComparisonTarget) {
			return nil, fmt.Errorf("comparison target collision for workspace repository %q", key)
		}
		targets[key] = *spec.ComparisonTarget
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return targets, nil
}

// collectRemoteContributions projects the validated per-repository bindings
// into the workspace-subpath keys understood by agentctl. The first repository
// owns the workspace root; sibling destinations use the same deterministic key
// as base-branch and workspace materialization projection.
func collectRemoteContributions(req *LaunchRequest) (map[string]models.RemoteContribution, error) {
	if req == nil {
		return nil, nil
	}
	specs := req.RepoSpecs()
	if len(specs) == 0 {
		if req.RemoteContribution == nil {
			return nil, nil
		}
		if err := req.RemoteContribution.Validate(); err != nil {
			return nil, fmt.Errorf("validate remote contribution: %w", err)
		}
		return map[string]models.RemoteContribution{"": *req.RemoteContribution}, nil
	}
	bindings := make(map[string]models.RemoteContribution)
	for index, spec := range specs {
		if spec.RemoteContribution == nil {
			continue
		}
		if err := spec.RemoteContribution.Validate(); err != nil {
			return nil, fmt.Errorf("validate remote contribution for repository %q: %w", spec.RepoName, err)
		}
		key := ""
		if index > 0 {
			key = baseBranchMetadataKey(spec)
		}
		if existing, ok := bindings[key]; ok && existing.CanonicalURL != spec.RemoteContribution.CanonicalURL {
			return nil, fmt.Errorf("multiple remote contributions target workspace repository %q", key)
		}
		bindings[key] = *spec.RemoteContribution
	}
	if len(bindings) == 0 {
		return nil, nil
	}
	return bindings, nil
}

// collectContributionDestinations projects server-authored managed fork
// destinations using the same workspace-subpath keys as remote contributions.
func collectContributionDestinations(req *LaunchRequest) (map[string]models.ContributionDestination, error) {
	if req == nil {
		return nil, nil
	}
	specs := req.RepoSpecs()
	if len(specs) == 0 {
		if req.ContributionDestination == nil {
			return nil, nil
		}
		if err := req.ContributionDestination.Validate(); err != nil {
			return nil, fmt.Errorf("validate contribution destination: %w", err)
		}
		return map[string]models.ContributionDestination{"": *req.ContributionDestination}, nil
	}
	destinations := make(map[string]models.ContributionDestination)
	for index, spec := range specs {
		if spec.ContributionDestination == nil {
			continue
		}
		if err := spec.ContributionDestination.Validate(); err != nil {
			return nil, fmt.Errorf("validate contribution destination for repository %q: %w", spec.RepoName, err)
		}
		key := ""
		if index > 0 {
			key = baseBranchMetadataKey(spec)
		}
		if existing, ok := destinations[key]; ok {
			if !sameContributionDestinationTarget(existing, *spec.ContributionDestination) {
				return nil, fmt.Errorf("multiple contribution destinations target workspace repository %q", key)
			}
			continue
		}
		destinations[key] = *spec.ContributionDestination
	}
	if len(destinations) == 0 {
		return nil, nil
	}
	return destinations, nil
}

func sameContributionDestinationTarget(left, right models.ContributionDestination) bool {
	return strings.EqualFold(left.TargetRepository.Host, right.TargetRepository.Host) &&
		left.TargetRepository.Path == right.TargetRepository.Path &&
		left.TargetRepository.ProviderID == right.TargetRepository.ProviderID &&
		left.TargetRepository.RemoteURL == right.TargetRepository.RemoteURL
}

// collectBaseBranches builds the per-repo {RepositoryName → base_branch}
// map that agentctl reads to scope diff stats. Single-repo legacy launches
// are recorded under the empty key "" so single-repo trackers (which have
// no repositoryName) still find their value. Repos missing a base_branch
// are skipped so the existing fallback list applies to them.
func collectBaseBranches(req *LaunchRequest) map[string]string {
	specs := req.RepoSpecs()
	if len(specs) == 0 {
		return nil
	}
	out := make(map[string]string, len(specs)+1)
	for _, spec := range specs {
		if spec.BaseBranch == "" {
			continue
		}
		if key := baseBranchMetadataKey(spec); key != "" {
			out[key] = spec.BaseBranch
		}
	}
	if req.BaseBranch != "" {
		if _, ok := out[""]; !ok {
			out[""] = req.BaseBranch
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func baseBranchMetadataKey(spec RepoLaunchSpec) string {
	repoName := worktree.SanitizeRepoDirName(spec.RepoName)
	if repoName == "" {
		repoName = spec.RepoName
	}
	branchSlug := worktree.SanitizeBranchSlug(spec.BranchSlug)
	if branchSlug == "" {
		return repoName
	}
	return repoName + "-" + branchSlug
}

// agentCommands holds both the display strings and structured argv for an agent execution.
type agentCommands struct {
	initial      string
	continue_    string // continue command for one-shot agents (empty if not applicable)
	args         []string
	continueArgs []string
}

func newAgentCommands(args, continueArgs []string) agentCommands {
	return agentCommands{
		initial:      strings.Join(args, " "),
		continue_:    strings.Join(continueArgs, " "),
		args:         args,
		continueArgs: continueArgs,
	}
}

// resolveProfileLaunchTokens resolves the user-configured cli_flags argv tokens
// and the launcher command_prefix tokens for a profile. Both must be applied on
// every command build — the initial launch AND fresh restarts (context reset) —
// or a configured profile could silently relaunch without its wrapper.
//
// cli_flags are best-effort: a malformed entry is logged and dropped so a typo
// doesn't block the task. command_prefix is launcher policy, so it fails closed:
// a profile that configured a prefix which cannot be resolved returns an error
// and aborts the launch rather than running the agent unwrapped.
func (m *Manager) resolveProfileLaunchTokens(profileInfo *AgentProfileInfo) (cliFlagTokens, commandPrefixTokens []string, err error) {
	if profileInfo == nil {
		return nil, nil, nil
	}
	if tokens, resolveErr := cliflags.Resolve(profileInfo.CLIFlags); resolveErr != nil {
		m.logger.Warn("failed to resolve cli_flags for profile, launching without user-configured flags",
			zap.String("profile_id", profileInfo.ProfileID),
			zap.Error(resolveErr))
	} else {
		cliFlagTokens = tokens
	}
	if strings.TrimSpace(profileInfo.CommandPrefix) != "" {
		if validateErr := cliflags.ValidateCommandPrefix(profileInfo.CommandPrefix); validateErr != nil {
			return nil, nil, fmt.Errorf("resolve command_prefix for profile %s: %w",
				profileInfo.ProfileID, validateErr)
		}
		commandPrefixTokens, err = cliflags.Tokenise(profileInfo.CommandPrefix)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve command_prefix for profile %s: %w",
				profileInfo.ProfileID, err)
		}
	}
	return cliFlagTokens, commandPrefixTokens, nil
}

func (m *Manager) buildAgentCommandWithContext(
	ctx context.Context,
	req *LaunchRequest,
	profileInfo *AgentProfileInfo,
	agentConfig agents.Agent,
	preferNative bool,
) (agentCommands, error) {
	model := ""
	autoApprove := false
	permissionValues := make(map[string]bool)
	if profileInfo != nil {
		model = profileInfo.Model
		autoApprove = profileInfo.AutoApprove
		permissionValues[agents.PermissionKeyAutoApprove] = profileInfo.AutoApprove
		permissionValues["allow_indexing"] = profileInfo.AllowIndexing
		permissionValues["dangerously_skip_permissions"] = profileInfo.DangerouslySkipPermissions
	}
	cliFlagTokens, commandPrefixTokens, err := m.resolveProfileLaunchTokens(profileInfo)
	if err != nil {
		return agentCommands{}, err
	}
	// Allow model override from request (for dynamic model switching)
	if req.ModelOverride != "" {
		model = req.ModelOverride
	}
	cliFlagTokens = appendRouteOverrideFlags(cliFlagTokens, req)
	runtime := models.ExecutorType(req.ExecutorType).Runtime()
	managedRuntimeVersion, err := m.resolveManagedRuntimeVersion(ctx, runtime, agentConfig)
	if err != nil {
		return agentCommands{}, err
	}
	// Only pass SessionID (for --resume flag) if the agent supports recovery.
	// Agents with CanRecover=false (e.g. Auggie) use history context injection instead.
	sessionID := req.ACPSessionID
	if rt := agentConfig.Runtime(); rt != nil && !rt.SessionConfig.SupportsRecovery() {
		sessionID = ""
	}
	cmdOpts := agents.CommandOptions{
		Model:                 model,
		SessionID:             sessionID,
		AutoApprove:           autoApprove,
		PermissionValues:      permissionValues,
		CLIFlagTokens:         cliFlagTokens,
		CommandPrefixTokens:   commandPrefixTokens,
		Runtime:               runtime,
		PreferNativeBinary:    preferNative,
		ManagedRuntimeVersion: managedRuntimeVersion,
	}
	args := m.commandBuilder.BuildCommandArgs(agentConfig, cmdOpts)
	continueArgs := m.commandBuilder.BuildContinueCommandArgs(agentConfig, cmdOpts)
	if err := validateBuiltAgentCommands(args, continueArgs); err != nil {
		return agentCommands{}, err
	}
	return newAgentCommands(args, continueArgs), nil
}

func (m *Manager) resolveManagedRuntimeVersion(
	ctx context.Context,
	_ agentruntime.Runtime,
	agentConfig agents.Agent,
) (string, error) {
	managed, ok := agentConfig.(agents.ManagedNPMRuntimeAgent)
	if !ok {
		return "", nil
	}
	spec := managed.ManagedNPMRuntime()
	effectiveVersion := spec.DefaultVersion
	if effectiveVersion == "" {
		// Test and embedded agents may construct a spec literal. PackageSpec
		// still resolves a known built-in package's reviewed default.
		packageSpec := spec.PackageSpec("")
		if packageSpec != spec.Package {
			effectiveVersion = strings.TrimPrefix(packageSpec, spec.Package+"@")
		}
	}
	if m.managedRuntimeSelections == nil {
		return effectiveVersion, nil
	}
	selection, found, err := m.managedRuntimeSelections.Get(ctx, agentConfig.ID(), spec.Package)
	if err != nil {
		return "", fmt.Errorf("resolve active managed runtime version for %s: %w", agentConfig.ID(), err)
	}
	if !found || selection.Package != spec.Package {
		return effectiveVersion, nil
	}
	return selection.Version, nil
}

func validateBuiltAgentCommands(args, continueArgs []string) error {
	if err := cliflags.ValidateCommandArgs(args); err != nil {
		return fmt.Errorf("validate agent command: %w", err)
	}
	if continueArgs != nil {
		if err := cliflags.ValidateCommandArgs(continueArgs); err != nil {
			return fmt.Errorf("validate continue command: %w", err)
		}
	}
	return nil
}

// launchResolveWorkspacePath resolves the effective workspace path for non-worktree executors.
// For worktree executors, workspace resolution is handled by the WorktreePreparer.
// For tasks without repositories, creates a workspace directory in ~/.kandev/quick-chat/.
// Returns workspacePath, mainRepoGitDir, worktreeID, worktreeBranch.
// resolveResumeWorktreePath resolves workspace path for worktree resume using the provider.
func (m *Manager) resolveResumeWorktreePath(ctx context.Context, req *LaunchRequest) (string, string, string, string) {
	ws := m.resolveWorkspaceFromProvider(ctx, req)
	if ws == "" {
		m.logger.Warn("could not resolve workspace path for worktree resume",
			zap.String("session_id", req.SessionID))
		return "", "", "", ""
	}
	var mainRepoGitDir string
	if req.RepositoryPath != "" {
		mainRepoGitDir = filepath.Join(req.RepositoryPath, ".git")
	}
	return ws, mainRepoGitDir, req.WorktreeID, ""
}

// resolveWorkspaceFromProvider looks up the workspace path from the info provider.
func (m *Manager) resolveWorkspaceFromProvider(ctx context.Context, req *LaunchRequest) string {
	if m.workspaceInfoProvider == nil {
		return ""
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, req.TaskID, req.SessionID)
	if err != nil || info.WorkspacePath == "" {
		return ""
	}
	m.logger.Debug("resolved workspace from provider for resume",
		zap.String("session_id", req.SessionID),
		zap.String("path", info.WorkspacePath))
	return info.WorkspacePath
}

func (m *Manager) launchResolveWorkspacePath(ctx context.Context, req *LaunchRequest) (workspacePath, mainRepoGitDir, worktreeID, worktreeBranch string) {
	// Clone-based runtimes own their workspace filesystem. In particular, never
	// pass a host checkout into Docker, SSH, or Sprites on reset/reconnect.
	if backend, err := m.getExecutorBackend(req.ExecutorType); err == nil && backend.RequiresCloneURL() {
		return "", "", "", ""
	}
	// Worktree mode requires a repository. Repo-less tasks fall through to the
	// scratch workspace path below — even if the executor type was worktree.
	useWorktree := req.UseWorktree && req.RepositoryPath != ""
	if useWorktree && req.ACPSessionID == "" {
		return "", "", "", ""
	}
	if useWorktree && req.ACPSessionID != "" {
		return m.resolveResumeWorktreePath(ctx, req)
	}
	workspacePath = req.WorkspacePath
	if req.RepositoryPath != "" && workspacePath == "" {
		workspacePath = req.RepositoryPath
	}
	if workspacePath == "" && req.ACPSessionID != "" {
		if resolved := m.resolveWorkspaceFromProvider(ctx, req); resolved != "" {
			return resolved, "", "", ""
		}
	}
	// For tasks without a repository, create a scratch workspace.
	// - Non-ephemeral repo-less tasks: <homeDir>/tasks/<workspaceID>/<taskID>/
	//   (task-scoped, persists across sessions, mirrors the worktree task layout).
	// - Ephemeral tasks (quick chat): <dataDir>/quick-chat/<sessionID>/
	//   (session-scoped, cleaned up on task delete via performTaskCleanup).
	// Office tasks that have no repo (onboarding, planning) take the
	// non-ephemeral branch and land under <homeDir>/tasks/...
	if workspacePath == "" && req.SessionID != "" && m.dataDir != "" {
		workspacePath = m.resolveScratchWorkspace(ctx, req)
	}
	return
}

func shouldPrepareEnvironment(req *LaunchRequest) bool {
	if req == nil || req.ACPSessionID == "" {
		return true
	}
	return req.UseWorktree && req.RepositoryPath != ""
}

// resolveScratchWorkspace creates and returns the scratch workspace path for a
// repo-less task. Returns empty string when the path could not be created.
func (m *Manager) resolveScratchWorkspace(ctx context.Context, req *LaunchRequest) string {
	scratchPath := m.scratchWorkspacePath(req)
	if scratchPath == "" {
		return ""
	}
	if err := os.MkdirAll(scratchPath, 0755); err != nil {
		m.logger.Warn("failed to create scratch workspace, continuing without workspace",
			zap.String("session_id", req.SessionID),
			zap.String("workspace_path", scratchPath),
			zap.Error(err))
		return ""
	}
	if !req.IsEphemeral {
		if err := storageworkspaces.WriteOwnershipMarker(scratchPath, storageworkspaces.OwnershipMarker{
			TaskID: req.TaskID, WorkspaceID: req.WorkspaceID, TaskDirName: req.TaskID,
			LayoutVersion: storageworkspaces.LayoutVersionScratch,
		}); err != nil {
			m.logger.Warn("failed to mark scratch workspace ownership",
				zap.String("workspace_path", scratchPath), zap.Error(err))
			return ""
		}
	}
	if err := m.initGitRepo(ctx, scratchPath); err != nil {
		m.logger.Warn("failed to initialize git repository in scratch workspace",
			zap.String("session_id", req.SessionID),
			zap.String("workspace_path", scratchPath),
			zap.Error(err))
		// Continue anyway - git is optional for repo-less workspaces.
	}
	m.logger.Info("created scratch workspace",
		zap.String("session_id", req.SessionID),
		zap.String("task_id", req.TaskID),
		zap.String("workspace_path", scratchPath))
	return scratchPath
}

// scratchWorkspacePath computes the scratch workspace path for a launch request.
// Returns empty string if the inputs are invalid (path traversal guard, missing IDs).
func (m *Manager) scratchWorkspacePath(req *LaunchRequest) string {
	if req.IsEphemeral {
		// Legacy quick-chat path — session-scoped, kept for backward compat with
		// ephemeral one-shot flows.
		if strings.ContainsAny(req.SessionID, `/\`) {
			m.logger.Warn("session ID contains path separator, rejecting",
				zap.String("session_id", req.SessionID))
			return ""
		}
		return filepath.Join(m.dataDir, "quick-chat", req.SessionID)
	}
	// Non-ephemeral repo-less task: place under <homeDir>/tasks/<workspaceID>/<taskID>/
	// so it sits alongside repo-bound worktrees and persists across sessions.
	if req.TaskID == "" || req.WorkspaceID == "" {
		m.logger.Warn("scratch workspace requires task_id and workspace_id",
			zap.String("session_id", req.SessionID),
			zap.String("task_id", req.TaskID),
			zap.String("workspace_id", req.WorkspaceID))
		return ""
	}
	if invalidScratchPathID(req.TaskID) || invalidScratchPathID(req.WorkspaceID) {
		m.logger.Warn("task or workspace ID contains path separator, rejecting",
			zap.String("task_id", req.TaskID),
			zap.String("workspace_id", req.WorkspaceID))
		return ""
	}
	// m.dataDir is misnamed — cmd/kandev/agents.go passes cfg.ResolvedHomeDir()
	// (the kandev root, e.g. ~/.kandev), not ResolvedDataDir(). So scratch
	// workspaces live alongside the existing repo-bound worktree task dirs
	// at <kandevHome>/tasks/<workspaceID>/<taskID>/.
	return filepath.Join(m.dataDir, "tasks", req.WorkspaceID, req.TaskID)
}

func invalidScratchPathID(id string) bool {
	return id == "." || id == ".." || strings.ContainsAny(id, `/\`)
}

// launchPrepareRequest copies the launch request, sets the resolved workspace path,
// and populates metadata from the request fields. Runtime/profile environment
// values are composed later, after every managed source has been collected.
func (m *Manager) launchPrepareRequest(req *LaunchRequest, profileInfo *AgentProfileInfo, workspacePath string) (LaunchRequest, string, error) {
	executionID := uuid.New().String()
	reqWithWorktree := *req
	reqWithWorktree.WorkspacePath = workspacePath

	if reqWithWorktree.Metadata == nil {
		reqWithWorktree.Metadata = make(map[string]interface{})
	}
	if req.TaskDescription != "" {
		reqWithWorktree.Metadata["task_description"] = req.TaskDescription
	}
	if len(req.Attachments) > 0 {
		reqWithWorktree.Metadata["attachments"] = req.Attachments
	}
	if req.SessionID != "" {
		reqWithWorktree.Metadata["session_id"] = req.SessionID
	}
	if req.TurnID != "" {
		reqWithWorktree.Metadata["prompt_turn_id"] = req.TurnID
	}

	if err := mergeRouteOverrideEnv(&reqWithWorktree); err != nil {
		return LaunchRequest{}, "", err
	}
	return reqWithWorktree, executionID, nil
}

// mergeRouteOverrideEnv preserves legacy model-only routing overlays.
// Concrete execution profiles own their complete environment.
func mergeRouteOverrideEnv(req *LaunchRequest) error {
	if req == nil || hasConcreteRouteExecutionProfile(req) || req.RouteOverride == nil || len(req.RouteOverride.Env) == 0 {
		return nil
	}
	if req.Env == nil {
		req.Env = make(map[string]string, len(req.RouteOverride.Env))
	}
	merged, err := gitconfigenv.Merge(req.Env, req.RouteOverride.Env)
	if err != nil {
		return fmt.Errorf("compose legacy route environment: %w", err)
	}
	req.Env = merged
	return nil
}

// newProgressCallback builds a PrepareProgressCallback that publishes progress events for a task/session.
func (m *Manager) newProgressCallback(taskID, sessionID string) PrepareProgressCallback {
	return func(step PrepareStep, stepIndex int, totalSteps int) {
		m.eventPublisher.PublishPrepareProgress(sessionID, &PrepareProgressEventPayload{
			TaskID:        taskID,
			SessionID:     sessionID,
			StepName:      step.Name,
			StepCommand:   step.Command,
			StepIndex:     stepIndex,
			TotalSteps:    totalSteps,
			Status:        string(step.Status),
			Output:        step.Output,
			Error:         step.Error,
			Warning:       step.Warning,
			WarningDetail: step.WarningDetail,
			StartedAt:     step.StartedAt,
			EndedAt:       step.EndedAt,
		})
	}
}

type prepareProgressRecorder struct {
	mu       sync.Mutex
	steps    []PrepareStep
	callback PrepareProgressCallback
}

func newPrepareProgressRecorder(callback PrepareProgressCallback) *prepareProgressRecorder {
	return &prepareProgressRecorder{callback: callback}
}

func (r *prepareProgressRecorder) Callback(offset int) PrepareProgressCallback {
	return func(step PrepareStep, stepIndex int, totalSteps int) {
		absoluteIndex := stepIndex + offset
		r.recordStep(step, absoluteIndex)
		if r.callback != nil {
			r.callback(step, absoluteIndex, totalSteps+offset)
		}
	}
}

func (r *prepareProgressRecorder) Merge(steps []PrepareStep) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, step := range steps {
		if i >= len(r.steps) {
			r.steps = append(r.steps, step)
			continue
		}
		if r.steps[i].Name == "" {
			r.steps[i] = step
		}
	}
}

func (r *prepareProgressRecorder) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.steps)
}

func (r *prepareProgressRecorder) Steps() []PrepareStep {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]PrepareStep, len(r.steps))
	copy(out, r.steps)
	return out
}

func (r *prepareProgressRecorder) recordStep(step PrepareStep, index int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.steps) <= index {
		r.steps = append(r.steps, PrepareStep{})
	}
	r.steps[index] = step
}

// launchBuildExecutorRequest resolves MCP servers, builds the ExecutorCreateRequest,
// and creates the runtime instance.
func (m *Manager) launchBuildExecutorRequest(ctx context.Context, executionID string, reqWithWorktree *LaunchRequest, agentConfig agents.Agent, profileInfo *AgentProfileInfo, mainRepoGitDir, worktreeID, worktreeBranch string, onProgress PrepareProgressCallback) (*ExecutorCreateRequest, *ExecutorInstance, ExecutorBackend, error) {
	rt, err := m.getExecutorBackend(reqWithWorktree.ExecutorType)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("no runtime configured: %w", err)
	}

	env, err := m.buildEnvForExecution(ctx, executionID, reqWithWorktree, agentConfig, profileInfo)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build launch environment: %w", err)
	}

	acpMcpServers, err := m.resolveMcpServersForLaunch(ctx, reqWithWorktree, agentConfig)
	if err != nil {
		m.logger.Warn("failed to resolve MCP servers for launch", zap.Error(err))
	}

	var mcpServers []McpServerConfig
	for _, srv := range acpMcpServers {
		mcpServers = append(mcpServers, McpServerConfig{
			Name:               srv.Name,
			URL:                srv.URL,
			Type:               srv.Type,
			Command:            srv.Command,
			Args:               srv.Args,
			Env:                srv.Env,
			Headers:            srv.Headers,
			DefinitionID:       srv.DefinitionID,
			DefinitionRevision: srv.DefinitionRevision,
			Origins:            append([]agentctltypes.McpServerOrigin(nil), srv.Origins...),
		})
	}

	metadata := buildLaunchMetadata(reqWithWorktree, mainRepoGitDir, worktreeID, worktreeBranch)
	if err := m.applyAuthoritativeKubernetesProfileConfig(ctx, reqWithWorktree, metadata); err != nil {
		return nil, nil, nil, err
	}
	remoteContributions, err := collectRemoteContributions(reqWithWorktree)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(remoteContributions) > 0 {
		metadata[MetadataKeyRemoteContributions] = remoteContributions
	}
	comparisonTargets, err := collectComparisonTargets(reqWithWorktree)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(comparisonTargets) > 0 {
		metadata[MetadataKeyComparisonTargets] = comparisonTargets
	}
	contributionDestinations, err := collectContributionDestinations(reqWithWorktree)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(contributionDestinations) > 0 {
		metadata[MetadataKeyContributionDestinations] = contributionDestinations
	}

	launchAuthToken, err := m.resolveLaunchAuthToken(ctx, reqWithWorktree, metadata)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve launch auth token: %w", err)
	}

	var autoApproveOverride *bool
	if profileInfo != nil {
		autoApproveOverride = boolPtr(profileInfo.AutoApprove)
	}
	execReq := &ExecutorCreateRequest{
		InstanceID:                     executionID,
		TaskID:                         reqWithWorktree.TaskID,
		TaskTitle:                      reqWithWorktree.TaskTitle,
		SessionID:                      reqWithWorktree.SessionID,
		TaskEnvironmentID:              reqWithWorktree.TaskEnvironmentID,
		WorkspaceID:                    reqWithWorktree.WorkspaceID,
		RepositoryIDs:                  repositoryIDsFromLaunch(reqWithWorktree),
		WorkspaceReuseRequired:         reqWithWorktree.WorkspaceReuseRequired,
		AgentProfileID:                 executionProfileID(reqWithWorktree),
		OfficeAgentProfileID:           reqWithWorktree.AgentProfileID,
		PromptTurnID:                   reqWithWorktree.TurnID,
		WorkspacePath:                  reqWithWorktree.WorkspacePath,
		WorkspaceSourceRoots:           workspaceSourceRoots(reqWithWorktree.WorkspaceFolders, workspaceRepositorySpecsFromLaunch(reqWithWorktree)),
		Protocol:                       string(agentConfig.Runtime().Protocol),
		Env:                            env,
		AutoApprovePermissions:         profileInfo != nil && profileInfo.AutoApprove,
		AutoApprovePermissionsOverride: autoApproveOverride,
		Metadata:                       metadata,
		AgentConfig:                    agentConfig,
		ApprovedSecretEnvKeys:          append([]string(nil), reqWithWorktree.ApprovedSecretEnvKeys...),
		McpServers:                     mcpServers,
		PreviousExecutionID:            reqWithWorktree.PreviousExecutionID,
		McpMode:                        reqWithWorktree.McpMode,
		McpProviders:                   reqWithWorktree.McpProviders,
		McpProfile:                     reqWithWorktree.McpProfile,
		AuthToken:                      launchAuthToken,
		BootstrapNonce:                 m.revealRuntimeSecret(ctx, metadata, MetadataKeyBootstrapNonceSecret),
		AgentctlStartupConfig:          m.agentctlStartupConfig,
		OnProgress:                     onProgress,
		RemoteContributions:            remoteContributions,
		ContributionDestinations:       contributionDestinations,
		ComparisonTargets:              comparisonTargets,
	}
	m.wireKubernetesInventoryPersistence(execReq, reqWithWorktree.ExecutorType)

	launchCtx, launchCancel := withLaunchPhaseTimeout(ctx)
	defer launchCancel()
	if err := resumeRemoteInstancePreflight(launchCtx, rt, execReq); err != nil {
		return nil, nil, nil, err
	}

	execInstance, err := rt.CreateInstance(launchCtx, execReq)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create execution: %w", err)
	}
	return execReq, execInstance, rt, nil
}

func isDockerExecutorType(executorType string) bool {
	return executorType == string(models.ExecutorTypeLocalDocker) || executorType == string(models.ExecutorTypeRemoteDocker)
}

// resolveLaunchAuthToken returns the agentctl token a launch/resume hands the
// backend. Docker uses the environment-scoped container control token (#2843)
// so a sibling session can authenticate to a shared container, falling back
// to the session token only when workspace reuse is required and no
// container-control secret exists yet. Every other executor uses its own
// session handshake token directly, which SSH resume requires.
func (m *Manager) resolveLaunchAuthToken(ctx context.Context, req *LaunchRequest, metadata map[string]interface{}) (string, error) {
	if isDockerExecutorType(req.ExecutorType) {
		return m.revealContainerControlAuthToken(ctx, metadata, req.WorkspaceReuseRequired)
	}
	return m.revealRuntimeSecretValue(ctx, metadata, MetadataKeyAuthTokenSecret)
}

func resumeRemoteInstancePreflight(ctx context.Context, rt ExecutorBackend, req *ExecutorCreateRequest) error {
	resumer, ok := rt.(RemoteSessionResumer)
	if !ok {
		return nil
	}
	if err := resumer.ResumeRemoteInstance(ctx, req); err != nil {
		return fmt.Errorf("failed remote resume preflight: %w", err)
	}
	return nil
}

// runEnvironmentPreparer runs the environment preparer for the executor type, if one is registered.
// Returns the prepare result (nil if no preparer ran). Does NOT publish PrepareCompleted;
// the caller is responsible for publishing based on the returned result.
func (m *Manager) runEnvironmentPreparer(
	ctx context.Context,
	req *LaunchRequest,
	workspacePath string,
) *EnvPrepareResult {
	return m.runEnvironmentPreparerWithProgress(ctx, req, workspacePath, m.newProgressCallback(req.TaskID, req.SessionID))
}

func (m *Manager) runEnvironmentPreparerWithProgress(
	ctx context.Context,
	req *LaunchRequest,
	workspacePath string,
	onProgress PrepareProgressCallback,
) *EnvPrepareResult {
	if m.preparerRegistry == nil {
		return nil
	}
	// Preparer registry is keyed by ExecutorType (the "local"/"worktree"/
	// "local_docker"/... taxonomy), not Runtime — so executor types that
	// share a runtime backend (local + worktree both run on standalone)
	// can still get distinct preparation logic.
	execType := models.ExecutorType(req.ExecutorType)
	preparer := m.preparerRegistry.Get(execType)
	if preparer == nil && execType == "" {
		// Fall back to LocalPreparer only for a genuinely empty ExecutorType
		// — legacy task rows (e.g. PR-watcher-created tasks without an
		// explicit executor) rely on local environment prep, including
		// missing-branch detection. Typed-but-unregistered values like
		// "remote_docker" intentionally return nil so the caller skips prep
		// rather than running local git operations against a remote executor.
		preparer = m.preparerRegistry.Get(models.ExecutorTypeLocal)
	}
	if preparer == nil {
		return nil
	}
	// The EnvPrepareRequest carries the resolved Runtime (executor.Name),
	// which preparer_script.go uses for runtime-level decisions like
	// picking the default prepare template.
	execName := execType.Runtime()

	// Skip environment preparation for repo-less tasks (e.g. config chat).
	// Preparers assume a repository is available; without one the session
	// falls through to the quick-chat workspace path instead.
	if req.RepositoryPath == "" {
		m.logger.Debug("skipping environment preparer — no repository path",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
			zap.String("preparer", preparer.Name()))
		return nil
	}

	prepReq := buildEnvPrepareRequest(req, workspacePath, execName)

	result, err := preparer.Prepare(ctx, prepReq, onProgress)
	if err != nil {
		m.logger.Warn("environment preparation failed",
			zap.String("task_id", req.TaskID),
			zap.String("preparer", preparer.Name()),
			zap.Error(err))
		return &EnvPrepareResult{
			Success:      false,
			ErrorMessage: err.Error(),
			Error:        err,
		}
	}

	return result
}

func buildEnvPrepareRequest(req *LaunchRequest, workspacePath string, execName executor.Name) *EnvPrepareRequest {
	repoSetupScript, _ := req.Metadata[MetadataKeyRepoSetupScript].(string)
	prepReq := &EnvPrepareRequest{
		TaskID:                     req.TaskID,
		WorkspaceID:                req.WorkspaceID,
		SessionID:                  req.SessionID,
		TaskEnvironmentID:          req.TaskEnvironmentID,
		TaskTitle:                  req.TaskTitle,
		ExecutorType:               execName,
		WorkspacePath:              workspacePath,
		RepositoryPath:             req.RepositoryPath,
		RepositoryID:               req.RepositoryID,
		TaskRepositoryID:           req.TaskRepositoryID,
		UseWorktree:                req.UseWorktree,
		WorkspaceReuseRequired:     req.WorkspaceReuseRequired,
		AllowBranchReplacement:     req.AllowBranchReplacement,
		WorktreeID:                 req.WorktreeID,
		SetupScript:                req.SetupScript,
		RepoSetupScript:            repoSetupScript,
		BaseBranch:                 req.BaseBranch,
		DefaultBranch:              req.DefaultBranch,
		CheckoutBranch:             req.CheckoutBranch,
		PRNumber:                   req.PRNumber,
		RemoteContribution:         req.RemoteContribution,
		ContributionDestination:    req.ContributionDestination,
		WorktreeBranch:             getMetadataString(req.Metadata, MetadataKeyWorktreeBranch),
		WorktreeBranchPrefix:       req.WorktreeBranchPrefix,
		WorktreeBranchTemplate:     req.WorktreeBranchTemplate,
		WorktreeBranchTicket:       req.WorktreeBranchTicket,
		PullBeforeWorktree:         req.PullBeforeWorktree,
		RemoteSyncHandled:          req.RemoteSyncHandled,
		RefreshRepository:          req.RefreshRepository,
		RefreshRepositoryWithState: req.RefreshRepositoryWithState,
		RemoteRefState:             req.RemoteRefState,
		TaskDirName:                req.TaskDirName,
		RepoName:                   req.RepoName,
		BranchSlug:                 req.BranchSlug,
		BranchIdentitySlug:         req.BranchIdentitySlug,
		Env:                        req.Env,
	}
	// Multi-repo: forward the repo list when the launch request carries one.
	// Each per-repo entry inherits the request-level RepoSetupScript when its
	// own is empty so single-repo callers continue to work unchanged.
	if len(req.Repositories) > 0 {
		specs := make([]RepoPrepareSpec, 0, len(req.Repositories))
		for _, r := range req.Repositories {
			setup := r.RepoSetupScript
			if setup == "" {
				setup = repoSetupScript
			}
			specs = append(specs, RepoPrepareSpec{
				TaskRepositoryID:           r.TaskRepositoryID,
				RepositoryID:               r.RepositoryID,
				RepositoryPath:             r.RepositoryPath,
				RepoName:                   r.RepoName,
				BaseBranch:                 r.BaseBranch,
				DefaultBranch:              r.DefaultBranch,
				CheckoutBranch:             r.CheckoutBranch,
				PRNumber:                   r.PRNumber,
				RemoteContribution:         r.RemoteContribution,
				WorktreeID:                 r.WorktreeID,
				AllowBranchReplacement:     req.AllowBranchReplacement || r.AllowBranchReplacement,
				WorktreeBranchPrefix:       r.WorktreeBranchPrefix,
				WorktreeBranchTemplate:     r.WorktreeBranchTemplate,
				WorktreeBranchTicket:       r.WorktreeBranchTicket,
				PullBeforeWorktree:         r.PullBeforeWorktree,
				RemoteSyncHandled:          r.RemoteSyncHandled,
				RefreshRepository:          r.RefreshRepository,
				RefreshRepositoryWithState: r.RefreshRepositoryWithState,
				RemoteRefState:             r.RemoteRefState,
				RepoSetupScript:            setup,
				BranchSlug:                 r.BranchSlug,
				BranchIdentitySlug:         r.BranchIdentitySlug,
				ContributionDestination:    r.ContributionDestination,
			})
		}
		prepReq.Repositories = specs
	}
	return prepReq
}

// launchApplyPrepareResult applies workspace metadata from the preparer result.
// Returns an error if the preparer failed.
func (m *Manager) launchApplyPrepareResult(
	req *LaunchRequest,
	result *EnvPrepareResult,
	workspacePath, mainRepoGitDir, worktreeID, worktreeBranch *string,
) error {
	if !result.Success {
		m.eventPublisher.PublishPrepareCompleted(req.SessionID, &PrepareCompletedEventPayload{
			TaskID:       req.TaskID,
			SessionID:    req.SessionID,
			Success:      false,
			ErrorMessage: result.ErrorMessage,
			Steps:        result.Steps,
		})
		// Prefer the typed chain on result.Error so errors.Is/errors.As reach
		// the underlying sentinel (worktree.ErrBranchCheckedOut, etc.). Fall
		// back to the textual ErrorMessage when the preparer did not supply a
		// typed error. The formatted message is identical in both cases.
		if result.Error != nil {
			displayMessage := result.ErrorMessage
			if displayMessage == "" {
				displayMessage = result.Error.Error()
			}
			return fmt.Errorf("environment preparation failed: %w", &prepareResultError{
				message: displayMessage,
				cause:   result.Error,
			})
		}
		return fmt.Errorf("environment preparation failed: %s", result.ErrorMessage)
	}
	if result.WorkspacePath != "" {
		*workspacePath = result.WorkspacePath
	}
	if result.MainRepoGitDir != "" {
		*mainRepoGitDir = result.MainRepoGitDir
	}
	if result.WorktreeID != "" {
		*worktreeID = result.WorktreeID
	}
	if result.WorktreeBranch != "" {
		*worktreeBranch = result.WorktreeBranch
	}
	return nil
}

func (m *Manager) publishLaunchPrepareCompleted(req *LaunchRequest, result *EnvPrepareResult, recorder *prepareProgressRecorder, workspacePath string, success bool, err error) {
	if req.ACPSessionID != "" && !shouldPrepareEnvironment(req) {
		return
	}

	steps := recorder.Steps()
	if len(steps) == 0 && result != nil {
		steps = result.Steps
	}

	payload := &PrepareCompletedEventPayload{
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		Success:       success,
		WorkspacePath: workspacePath,
		Steps:         steps,
	}
	if result != nil {
		payload.DurationMs = result.Duration.Milliseconds()
		if payload.WorkspacePath == "" {
			payload.WorkspacePath = result.WorkspacePath
		}
	}
	if err != nil {
		payload.Success = false
		payload.ErrorMessage = err.Error()
	}
	m.eventPublisher.PublishPrepareCompleted(req.SessionID, payload)
}

// Launch launches a new agent for a task. Concurrent calls for the same
// req.SessionID are collapsed via the same singleflight bucket used by
// EnsureWorkspaceExecutionForSession and GetOrEnsureExecution — this closes
// the check-then-act race that previously could spawn a runtime instance,
// fail at executionStore.Add (race), and then have the orchestrator persist
// the orphan execution_id to disk before rollback completed (the original
// agent-execution-id divergence bug).
//
// If req.SessionID is empty (quick chat / pre-session contexts), no
// deduplication key exists and we fall through to direct execution.
func (m *Manager) Launch(ctx context.Context, req *LaunchRequest) (*AgentExecution, error) {
	if req.SessionID == "" {
		activityLease, err := m.acquireActivity(ctx, activity.KindExecutionStarting)
		if err != nil {
			return nil, err
		}
		transferredActivity := false
		defer func() {
			if !transferredActivity {
				activityLease.Release()
			}
		}()
		activityLease.SetKind(activity.KindExecutionPreparing)
		execution, launchErr := m.launchInternal(ctx, req)
		if launchErr == nil && req.StartAgent {
			m.markAgentStartPending(execution)
			m.trackActivity(executionActivityKey(execution.ID), activityLease)
			transferredActivity = true
		}
		return execution, launchErr
	}
	value, err := m.doCoalescedExecution(ctx, req.SessionID, func(sharedCtx context.Context) (interface{}, error) {
		activityLease, acquireErr := m.acquireActivity(sharedCtx, activity.KindExecutionStarting)
		if acquireErr != nil {
			return nil, acquireErr
		}
		defer activityLease.Release()
		activityLease.SetKind(activity.KindExecutionPreparing)
		return m.launchInternal(sharedCtx, req)
	})
	if err != nil {
		return nil, err
	}
	execution := value.(*AgentExecution)
	// If this Launch call joined a workspace-only ensure peer's singleflight
	// slot (EnsureWorkspaceExecutionForSession / GetOrEnsureExecution), the
	// returned execution has no AgentCommand and the orchestrator's subsequent
	// StartAgentProcess() would fail with "no agent command configured".
	// Promote it in place so the agent subprocess can start against the
	// existing agentctl instance.
	if execution.AgentCommand == "" {
		if err := m.promoteWorkspaceExecution(ctx, execution, req); err != nil {
			return nil, err
		}
	}
	if req.StartAgent {
		activityLease, err := m.acquireActivity(ctx, activity.KindExecutionPreparing)
		if err != nil {
			return nil, err
		}
		m.markAgentStartPending(execution)
		m.trackActivity(executionActivityKey(execution.ID), activityLease)
	}
	return execution, nil
}

// markAgentStartPending distinguishes a launch that owns an imminent agent
// start from a workspace-only execution. It happens synchronously before
// Launch returns so a concurrent resume cannot mistake the registered runtime
// for stale state while StartAgentProcess is waiting for agentctl readiness.
func (m *Manager) markAgentStartPending(execution *AgentExecution) {
	if execution == nil {
		return
	}
	_ = m.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
		current.Status = v1.AgentStatusStarting
		current.StartedAt = time.Now()
	})
}

// promoteWorkspaceExecution populates the agent command fields on a
// workspace-only execution so a subsequent StartAgentProcess() can configure
// and start the agent subprocess. Concurrent promoters serialize through a
// dedicated singleflight key so they don't race on the shared AgentExecution
// pointer.
func (m *Manager) promoteWorkspaceExecution(ctx context.Context, execution *AgentExecution, req *LaunchRequest) error {
	_, err := m.doCoalescedExecution(ctx, req.SessionID, func(sharedCtx context.Context) (interface{}, error) {
		activityLease, acquireErr := m.acquireActivity(sharedCtx, activity.KindExecutionPreparing)
		if acquireErr != nil {
			return nil, acquireErr
		}
		defer activityLease.Release()
		if len(req.McpProviders) > 0 {
			client, releaseClient := execution.AcquireAgentCtlClient()
			defer releaseClient()
			if client == nil {
				return nil, fmt.Errorf("execution %q has no agentctl client for MCP provider promotion", execution.ID)
			}
			if err := client.SetMcpProviders(sharedCtx, req.McpProviders); err != nil {
				return nil, fmt.Errorf("set MCP providers during workspace execution promotion: %w", err)
			}
		}
		// Re-check after acquiring the slot — a peer Launch may have already
		// promoted while we were waiting.
		if execution.AgentCommand != "" {
			return nil, nil
		}
		agentTypeName, profileInfo, err := m.resolveAgentProfile(sharedCtx, req)
		if err != nil {
			return nil, err
		}
		agentConfig, ok := m.registry.Get(agentTypeName)
		if !ok {
			return nil, fmt.Errorf("agent type %q not found in registry", agentTypeName)
		}
		if !agentConfig.Enabled() {
			return nil, fmt.Errorf("agent type %q is disabled", agentTypeName)
		}
		preferNative := m.preferNativeBinary(agentConfig, execution.RuntimeName, execution.MetadataSnapshot())
		cmds, err := m.buildAgentCommandWithContext(sharedCtx, req, profileInfo, agentConfig, preferNative)
		if err != nil {
			return nil, err
		}
		execution.AgentCommand = cmds.initial
		execution.ContinueCommand = cmds.continue_
		execution.AgentArgs = cmds.args
		execution.ContinueArgs = cmds.continueArgs
		if req.ACPSessionID != "" && execution.ACPSessionID == "" {
			execution.ACPSessionID = req.ACPSessionID
		}
		if req.PreviousExecutionID != "" {
			execution.isResumedSession = true
		}
		execution.IsPassthrough = req.IsPassthrough
		if !req.IsPassthrough {
			if err := m.materializeRuntimeProjectMCP(sharedCtx, execution, agentConfig); err != nil {
				execution.AgentCommand = ""
				execution.ContinueCommand = ""
				execution.AgentArgs = nil
				execution.ContinueArgs = nil
				execution.isResumedSession = false
				execution.IsPassthrough = false
				return nil, err
			}
		}
		m.logger.Info("promoted workspace-only execution to agent execution",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", req.SessionID),
			zap.String("agent_profile_id", req.AgentProfileID),
			zap.Bool("resume", req.ACPSessionID != ""))
		return nil, nil
	})
	return err
}

// launchInternal is the body of Launch run inside the per-session singleflight
// slot. Callers must not invoke this directly except via Launch.
func (m *Manager) launchInternal(ctx context.Context, req *LaunchRequest) (*AgentExecution, error) {
	m.logger.Debug("launching agent",
		zap.String("task_id", req.TaskID),
		zap.String("agent_profile_id", req.AgentProfileID),
		zap.Bool("use_worktree", req.UseWorktree))

	// 1. Resolve the agent profile to get agent type info
	agentTypeName, profileInfo, err := m.resolveAgentProfile(ctx, req)
	if err != nil {
		return nil, err
	}

	// 2. Get agent config from registry
	agentConfig, ok := m.registry.Get(agentTypeName)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found in registry", agentTypeName)
	}
	if !agentConfig.Enabled() {
		return nil, fmt.Errorf("agent type %q is disabled", agentTypeName)
	}
	if err := m.prepareManagedGoCacheEnvironment(ctx, req); err != nil {
		return nil, err
	}

	// 3. Check if session already has an agent running. A workspace-only
	// execution created by EnsureWorkspaceExecutionForSession /
	// GetOrEnsureExecution has no AgentCommand — return it so the outer Launch
	// can promote it instead of erroring as if a real agent were running.
	if req.SessionID != "" {
		if existingExecution, exists := m.executionStore.GetBySessionID(req.SessionID); exists {
			if existingExecution.AgentCommand == "" {
				return existingExecution, nil
			}
			return nil, fmt.Errorf("%w: session %q (execution: %s)", ErrAgentAlreadyRunning, req.SessionID, existingExecution.ID)
		}
	}

	// 4. Resolve workspace path (non-worktree executors use this directly)
	workspacePath, mainRepoGitDir, worktreeID, worktreeBranch := m.launchResolveWorkspacePath(ctx, req)
	owner := ownedDirectoryLinkOwner(req.TaskID, req.TaskDirName)
	if err := reconcileWorkspaceSources(ctx, workspacePath, req.WorkspaceFolders, owner); err != nil {
		return nil, err
	}
	if req.ExecutorType == string(models.ExecutorTypeLocal) || req.ExecutorType == legacyExecutorTypeLocalPC {
		if err := reconcileWorkspaceRepositories(workspacePath, workspaceRepositorySpecsFromLaunch(req), m.logger, owner); err != nil {
			return nil, err
		}
	}
	progressRecorder := newPrepareProgressRecorder(m.newProgressCallback(req.TaskID, req.SessionID))

	// Compose the request before preparation so setup scripts receive the same
	// final snapshot that the runtime, agent, shell, and terminal will use.
	reqWithWorktree, executionID, err := m.launchPrepareRequest(req, profileInfo, workspacePath)
	if err != nil {
		m.publishLaunchPrepareCompleted(req, nil, progressRecorder, workspacePath, false, err)
		return nil, err
	}
	finalEnv, err := m.buildEnvForExecution(ctx, executionID, &reqWithWorktree, agentConfig, profileInfo)
	if err != nil {
		m.publishLaunchPrepareCompleted(req, nil, progressRecorder, workspacePath, false, err)
		return nil, err
	}
	reqWithWorktree.Env = finalEnv
	reqWithWorktree.EnvironmentDefinitions = nil
	reqWithWorktree.EnvironmentResolutionRequired = false
	reqWithWorktree.EnvironmentFinalized = true

	// 4b. Run environment preparation (if preparer registered for this executor type).
	// Native ACP resume normally reuses the already-prepared workspace, but a
	// worktree resume must re-enter preparation so a missing branch can be
	// reported and an explicit replacement action can materialize it.
	var prepResult *EnvPrepareResult
	if shouldPrepareEnvironment(req) {
		prepResult = m.runEnvironmentPreparerWithProgress(ctx, &reqWithWorktree, workspacePath, progressRecorder.Callback(0))
	} else {
		m.logger.Debug("skipping environment preparation for resumed session",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID))
	}
	if prepResult != nil {
		progressRecorder.Merge(prepResult.Steps)
		if err := m.launchApplyPrepareResult(&reqWithWorktree, prepResult, &workspacePath, &mainRepoGitDir, &worktreeID, &worktreeBranch); err != nil {
			return nil, err
		}
		// The preparer owns the final workspace location for worktree-backed
		// launches. Keep the executor request in sync with the local launch
		// state; otherwise standalone receives the repository path (or an empty
		// path) that was present before preparation completed.
		reqWithWorktree.WorkspacePath = workspacePath
	}

	// 6b. Deploy per-profile skills + custom prompt (ADR 0005 Wave A).
	// Best-effort: a deploy failure is logged but does not abort the launch
	// — the agent can still start with whatever skills were already on disk.
	m.runSkillDeploy(ctx, req, &reqWithWorktree)

	// 7. Build runtime request and create instance (agent not started yet)
	var runtimeProgress PrepareProgressCallback
	if shouldPrepareEnvironment(req) {
		runtimeProgress = progressRecorder.Callback(progressRecorder.Len())
	}
	execReq, execInstance, rt, err := m.launchBuildExecutorRequest(ctx, executionID, &reqWithWorktree, agentConfig, profileInfo, mainRepoGitDir, worktreeID, worktreeBranch, runtimeProgress)
	if err != nil {
		m.publishLaunchPrepareCompleted(req, prepResult, progressRecorder, workspacePath, false, err)
		return nil, err
	}
	// A reset/relaunch receives the complete durable repository projection from
	// the orchestrator. Reconcile it through the fresh live agentctl rather than
	// relying on the legacy primary-repository prepare script alone.
	if rt.RequiresCloneURL() && len(reqWithWorktree.RepoSpecs()) > 1 && execInstance != nil && execInstance.Client != nil {
		projection, projectionErr := remoteWorkspaceProjectionFromLaunch(&reqWithWorktree)
		if projectionErr == nil {
			projectionErr = materializeWorkspaceRepositories(ctx, execInstance.Client, projection)
		}
		if projectionErr != nil {
			rollbackErr := stopRuntimeInstanceAndRelease(context.WithoutCancel(ctx), rt, execInstance, true)
			err = errors.Join(
				fmt.Errorf("reconstruct remote workspace repositories: %w", projectionErr),
				rollbackErr,
			)
			m.publishLaunchPrepareCompleted(req, prepResult, progressRecorder, workspacePath, false, err)
			return nil, err
		}
	}

	// Remote executors (Docker, Sprites) clone the workspace inside the
	// container, so the worktree path's host-side copy_files never ran.
	// Ship the bytes through agentctl now that the instance is up. The
	// worktree path is already gated by reqWithWorktree.UseWorktree, so
	// it's safe to skip when that's true. For multi-repo launches, loop
	// over every per-repo spec — each repo's CopyFiles ships into its
	// own RepoName subdir under the workspace.
	if !reqWithWorktree.UseWorktree && execInstance != nil && execInstance.Client != nil {
		shipRemoteCopyfilesForLaunch(ctx, m.logger, &reqWithWorktree, execInstance.Client, runtimeProgress, progressRecorder)
	}

	if prepResult != nil {
		prepResult.Steps = progressRecorder.Steps()
	}
	m.publishLaunchPrepareCompleted(req, prepResult, progressRecorder, workspacePath, true, nil)

	// Build the in-memory AgentExecution from the runtime instance. Extracted
	// to keep launchInternal under the cyclomatic-complexity budget.
	execution, err := m.buildExecutionFromInstance(ctx, req, execReq, execInstance, rt, profileInfo, agentConfig, prepResult)
	if err != nil {
		// Command resolution failed (e.g. a configured command_prefix could not
		// be tokenised). The execution isn't built yet, so stop the runtime
		// instance directly to avoid leaking it, then fail closed.
		if rt != nil && execInstance != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if stopErr := stopRuntimeInstanceAndRelease(cleanupCtx, rt, execInstance, true); stopErr != nil {
				m.logger.Warn("failed to stop runtime instance after command resolution error",
					zap.Error(stopErr))
			}
			cancel()
		}
		if execInstance != nil && execInstance.Client != nil {
			execInstance.Client.Close()
		}
		return nil, err
	}
	if !reqWithWorktree.IsPassthrough {
		if err := m.materializeRuntimeProjectMCP(ctx, execution, agentConfig); err != nil {
			m.rollbackLaunchExecution(ctx, rt, execInstance, execution, "project MCP materialization failed")
			return nil, err
		}
	}

	// Track + persist + publish. Returns the rollback error if Add lost a race.
	if err := m.registerAndPublishExecution(ctx, execution, rt, execInstance, req.SessionID); err != nil {
		return nil, err
	}

	m.logger.Debug("agentctl execution created (agent not started)",
		zap.String("execution_id", executionID),
		zap.String("task_id", req.TaskID),
		zap.Stringer("runtime", execution.RuntimeName))

	return execution, nil
}

// buildExecutionFromInstance turns the spawned ExecutorInstance + request shape
// into an in-memory *AgentExecution ready for Add. Pulled out of launchInternal
// to keep the orchestration loop's cyclomatic complexity within the linter budget.
func (m *Manager) buildExecutionFromInstance(
	ctx context.Context,
	req *LaunchRequest,
	execReq *ExecutorCreateRequest,
	execInstance *ExecutorInstance,
	rt ExecutorBackend,
	profileInfo *AgentProfileInfo,
	agentConfig agents.Agent,
	prepResult *EnvPrepareResult,
) (*AgentExecution, error) {
	execution := execInstance.ToAgentExecution(execReq)
	execution.RuntimeName = rt.Name()
	if req.ACPSessionID != "" {
		execution.ACPSessionID = req.ACPSessionID
	}
	execution.PrepareResult = prepResult
	if req.PreviousExecutionID != "" {
		execution.isResumedSession = true
	}
	execution.IsPassthrough = req.IsPassthrough
	// Use the resolved runtime (set from rt.Name() above), matching
	// promoteWorkspaceExecution's call site rather than re-deriving from the
	// requested ExecutorType.
	preferNative := m.preferNativeBinary(agentConfig, execution.RuntimeName, execReq.Metadata)
	cmds, err := m.buildAgentCommandWithContext(ctx, req, profileInfo, agentConfig, preferNative)
	if err != nil {
		return nil, err
	}
	execution.AgentCommand = cmds.initial
	execution.ContinueCommand = cmds.continue_
	execution.AgentArgs = cmds.args
	execution.ContinueArgs = cmds.continueArgs
	return execution, nil
}

// registerAndPublishExecution does the post-spawn lockstep dance: track in the
// in-memory store, persist the executors_running row, publish events, kick off
// the readiness poll. On a session-conflict race during Add, rolls back the
// runtime instance so we never leak a subprocess.
func (m *Manager) registerAndPublishExecution(
	ctx context.Context,
	execution *AgentExecution,
	rt ExecutorBackend,
	execInstance *ExecutorInstance,
	sessionID string,
) error {
	if err := m.ensureLaunchSessionStillActive(ctx, sessionID); err != nil {
		m.rollbackLaunchExecution(ctx, rt, execInstance, execution, "session ended during runtime creation")
		return err
	}
	if addErr := m.executionStore.Add(execution); addErr != nil {
		if errors.Is(addErr, ErrExecutionAlreadyExistsForSession) {
			m.rollbackRacedExecution(ctx, rt, execInstance, execution)
			return fmt.Errorf("%w: session %q (race resolved during register)", ErrAgentAlreadyRunning, sessionID)
		}
		return fmt.Errorf("failed to register execution: %w", addErr)
	}
	isKubernetes := execution.RuntimeName == agentruntime.RuntimeKubernetes
	var createdRuntimeSecrets map[string]bool
	if isKubernetes {
		var err error
		createdRuntimeSecrets, err = m.persistRequiredKubernetesRuntimeSecrets(ctx, execInstance, execution)
		if err != nil {
			m.rollbackRegisteredLaunch(rt, execInstance, execution, "Kubernetes runtime secret persistence failed")
			return err
		}
	}
	// Make the execution visible to durable cleanup before the final session
	// read. This closes the precheck -> Add -> persist gap: deletion cleanup can
	// now inventory the row, while a deletion that already ran is caught below.
	if err := m.persistExecutorRunningResult(ctx, execution); err != nil {
		secretCleanupErr := m.deleteCreatedRuntimeSecrets(ctx, execution, createdRuntimeSecrets)
		m.rollbackRegisteredLaunchAfterPersistFailure(rt, execInstance, execution)
		return errors.Join(fmt.Errorf("persist execution registration: %w", err), secretCleanupErr)
	}

	if err := m.ensureLaunchSessionStillActive(ctx, sessionID); err != nil {
		if errors.Is(err, errTaskCleanupActive) {
			m.rollbackRegisteredLaunchForTaskCleanup(rt, execInstance, execution)
		} else {
			m.rollbackRegisteredLaunch(rt, execInstance, execution, "session ended during execution registration")
		}
		return err
	}
	m.setRuntimeInterest(execution.SessionID, true)

	if !isKubernetes {
		if err := m.persistRuntimeSecrets(ctx, execInstance, execution); err != nil {
			m.rollbackRegisteredLaunch(rt, execInstance, execution, "runtime secret persistence failed")
			return err
		}
	}

	go m.pollOneRemoteStatus(context.Background(), execution)

	m.eventPublisher.PublishAgentEvent(ctx, events.AgentStarted, execution)
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlStarting, execution, "")

	// Wait for agentctl to be ready (for shell/workspace access).
	// NOTE: This does NOT start the agent process — call StartAgentProcess() explicitly.
	go m.waitForAgentctlReady(execution)
	return nil
}

// ensureLaunchSessionStillActive closes the remote-runtime creation race: SSH,
// Docker, and other remote CreateInstance calls can outlive a concurrent task
// delete. Callers read both immediately before and after registration. The
// durable cleanup-intent check is the admission boundary between them: either
// launch persists first and cleanup's final inventory observes it, or cleanup
// persists first and launch rolls the runtime back.
func (m *Manager) ensureLaunchSessionStillActive(ctx context.Context, sessionID string) error {
	if m.executorProfileReader == nil || sessionID == "" {
		return nil
	}
	session, err := m.executorProfileReader.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("verify session before registering execution: %w", err)
	}
	if session == nil {
		return fmt.Errorf("verify session before registering execution: session %q not found", sessionID)
	}
	cleanupActive, err := m.executorProfileReader.HasActiveTaskResourceCleanupJob(ctx, session.TaskID)
	if err != nil {
		return fmt.Errorf("verify task cleanup before registering execution: %w", err)
	}
	if cleanupActive {
		return fmt.Errorf("verify task cleanup before registering execution: %w for task %q", errTaskCleanupActive, session.TaskID)
	}
	// Re-read after the cleanup-intent lookup. A cleanup can transition to a
	// terminal state between separate queries; its job then no longer appears
	// active, but the session deletion/cancellation remains authoritative.
	session, err = m.executorProfileReader.GetTaskSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("reverify session after task cleanup admission: %w", err)
	}
	if session == nil {
		return fmt.Errorf("reverify session after task cleanup admission: session %q not found", sessionID)
	}
	switch session.State {
	case models.TaskSessionStateCancelled,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed:
		return fmt.Errorf("verify session before registering execution: session %q is %s: %w", sessionID, session.State, ErrSessionTerminal)
	default:
		return nil
	}
}

// rollbackRegisteredLaunchForTaskCleanup cannot assume the session is
// terminal: the task mutation may still fail after persisting its prepared
// cleanup intent. Preserve resumable state while removing the rejected live
// execution; committed cleanup can subsequently remove the stopped row.
func (m *Manager) rollbackRegisteredLaunchForTaskCleanup(
	rt ExecutorBackend,
	execInstance *ExecutorInstance,
	execution *AgentExecution,
) {
	m.rollbackRegisteredLaunchWithRetry(rt, execInstance, execution, true, "task cleanup won execution registration")
}

// rollbackRegisteredLaunchAfterPersistFailure preserves any prior durable row
// for the session. If the failed upsert actually committed before returning an
// ambiguous transport error, clean it up resume-safely only after confirming
// that it belongs to this exact execution.
func (m *Manager) rollbackRegisteredLaunchAfterPersistFailure(
	rt ExecutorBackend,
	execInstance *ExecutorInstance,
	execution *AgentExecution,
) {
	m.rollbackRegisteredLaunchWithRetry(rt, execInstance, execution, false, "execution registration persistence failed")
}

func (m *Manager) rollbackRegisteredLaunchWithRetry(
	rt ExecutorBackend,
	execInstance *ExecutorInstance,
	execution *AgentExecution,
	taskCleanupActive bool,
	reason string,
) {
	m.rollbackRegisteredLaunchWithRetryMode(rt, execInstance, execution, taskCleanupActive, false, reason)
}

func (m *Manager) rollbackRegisteredLaunchWithRetryMode(
	rt ExecutorBackend,
	execInstance *ExecutorInstance,
	execution *AgentExecution,
	taskCleanupActive bool,
	discardDurable bool,
	reason string,
) {
	if err := m.stopRegisteredLaunchRuntime(rt, execInstance, execution); err == nil {
		m.finishRegisteredLaunchRollback(execution, taskCleanupActive, discardDurable)
		return
	} else {
		m.logger.Warn("registered launch rollback retained ownership after stop failure",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID),
			zap.String("reason", reason),
			zap.Error(err))
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for attempt := 0; attempt < registeredLaunchRollbackRetries; attempt++ {
			timer := time.NewTimer(registeredLaunchRollbackRetryDelays[attempt])
			select {
			case <-m.stopCh:
				timer.Stop()
				return
			case <-timer.C:
			}
			if _, exists := m.executionStore.Get(execution.ID); !exists {
				return
			}
			if err := m.stopRegisteredLaunchRuntime(rt, execInstance, execution); err == nil {
				m.finishRegisteredLaunchRollback(execution, taskCleanupActive, discardDurable)
				return
			} else {
				m.logger.Warn("registered launch rollback retry failed",
					zap.String("execution_id", execution.ID),
					zap.Int("attempt", attempt+1),
					zap.Error(err))
			}
		}
		m.logger.Error("registered launch rollback exhausted retries; retaining cleanup ownership",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID),
			zap.String("reason", reason))
	}()
}

func (m *Manager) stopRegisteredLaunchRuntime(
	rt ExecutorBackend,
	execInstance *ExecutorInstance,
	execution *AgentExecution,
) error {
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	execution.agentctlLifecycleMu.Lock()
	defer execution.agentctlLifecycleMu.Unlock()
	if rt != nil && execInstance != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		retainedKubernetesResume := execution.RuntimeName == agentruntime.RuntimeKubernetes && execution.isResumedSession
		err := rt.StopInstance(cleanupCtx, execInstance, !retainedKubernetesResume)
		if err == nil && execution.RuntimeName == agentruntime.RuntimeKubernetes && !retainedKubernetesResume {
			err = m.deleteKubernetesRuntimeSecrets(cleanupCtx, execution.MetadataSnapshot())
		}
		if err == nil && !retainedKubernetesResume {
			err = releaseExecutorInstanceRuntimeInventory(cleanupCtx, execInstance)
			if err != nil {
				err = fmt.Errorf("release stopped runtime inventory: %w", err)
			}
		}
		cancel()
		if err != nil {
			return err
		}
	}
	if client := execution.currentAgentCtlClient(); client != nil { // protected by agentctlLifecycleMu
		client.Close()
	}
	execution.EndSessionSpan()
	return nil
}

func (m *Manager) finishRegisteredLaunchRollback(execution *AgentExecution, taskCleanupActive, discardDurable bool) {
	if execution.RuntimeName == agentruntime.RuntimeKubernetes && execution.isResumedSession {
		// A failed resume only owns the newly opened local client and forward. The
		// recorded Pod/PVC inventory remains the authority for a later retry or
		// terminal cleanup, so never discard its durable row here.
		m.executionStore.Remove(execution.ID)
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !discardDurable || taskCleanupActive {
		m.deleteExecutorRunning(cleanupCtx, execution.SessionID)
	} else if reader, ok := m.runningWriter.(executorRunningReader); ok {
		running, err := reader.GetExecutorRunningBySessionID(cleanupCtx, execution.SessionID)
		if err == nil && running != nil && running.AgentExecutionID == execution.ID {
			if err := m.deleteExecutorRunningRow(cleanupCtx, execution.SessionID, execution.ID); err != nil &&
				!errors.Is(err, models.ErrExecutorRunningNotFound) {
				m.logger.Warn("failed to delete exact executor-running row after launch rollback",
					zap.String("execution_id", execution.ID),
					zap.String("session_id", execution.SessionID),
					zap.Error(err))
			}
		}
	}
	m.executionStore.Remove(execution.ID)
}

func (m *Manager) rollbackLaunchExecution(_ context.Context, rt ExecutorBackend, execInstance *ExecutorInstance, execution *AgentExecution, reason string) {
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	execution.agentctlLifecycleMu.Lock()
	defer execution.agentctlLifecycleMu.Unlock()
	m.logger.Warn("rolling back launch execution",
		zap.String("execution_id", execution.ID),
		zap.String("session_id", execution.SessionID),
		zap.String("reason", reason))
	if rt != nil && execInstance != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if stopErr := stopRuntimeInstanceAndRelease(cleanupCtx, rt, execInstance, true); stopErr != nil {
			m.logger.Warn("failed to stop runtime instance during launch rollback",
				zap.String("execution_id", execution.ID),
				zap.Error(stopErr))
		}
	}
	if client := execution.currentAgentCtlClient(); client != nil { // protected by agentctlLifecycleMu
		client.Close()
	}
	execution.EndSessionSpan()
}

// rollbackRegisteredLaunch stops the runtime before removing either side of
// the execution registration. A failed stop retains both in-memory and durable
// ownership for retry; successful terminal rollback removes the exact row
// without normal resume-token repair.
func (m *Manager) rollbackRegisteredLaunch(rt ExecutorBackend, execInstance *ExecutorInstance, execution *AgentExecution, reason string) {
	m.rollbackRegisteredLaunchWithRetryMode(rt, execInstance, execution, false, true, reason)
}

// SetExecutionDescription updates the task description stored in an execution's metadata.
// This is used when starting an agent on a workspace that was launched without a prompt.
func (m *Manager) SetExecutionDescription(_ context.Context, executionID string, description string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	execution.setMetadataValue("task_description", description)
	return nil
}

// SetPromptTurnID binds the next prompt completion to a durable Kandev turn.
// The value is kept on the in-memory execution so it can be snapshotted onto
// the terminal stream event before AgentReady can admit a successor prompt.
func (m *Manager) SetPromptTurnID(_ context.Context, executionID, turnID string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	execution.setPromptTurnID(turnID)
	return nil
}

// SetExecutionEnv stores per-run environment variables for the next agent subprocess start.
func (m *Manager) SetExecutionEnv(_ context.Context, executionID string, env map[string]string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	execution.setMetadataValue("runtime_env", cloneStringMap(env))
	return nil
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// SetMcpMode changes the MCP tool mode on an existing execution's agentctl instance.
// This is used when a session transitions to plan/config mode after the workspace was
// already prepared with the default (task) mode.
func (m *Manager) SetMcpMode(ctx context.Context, executionID string, mode string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	return client.SetMcpMode(ctx, mode)
}

// SetMcpProvidersForSession replaces the task-mode MCP provider capabilities
// on the live execution attached to sessionID. The execution store is the
// source of truth for active agentctl instances; an absent execution is a
// successful no-op because the next launch or resume derives providers from
// the persisted task repositories.
func (m *Manager) SetMcpProvidersForSession(ctx context.Context, sessionID string, providers []string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	execution, exists := m.GetExecutionBySessionID(sessionID)
	if !exists || execution == nil {
		m.logger.Debug("MCP provider refresh skipped: no execution for session",
			zap.String("session_id", sessionID))
		return nil
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}
	if err := client.SetMcpProviders(ctx, providers); err != nil {
		return fmt.Errorf("set MCP providers for session %s: %w", sessionID, err)
	}
	return nil
}

// SetPluginToolsForAllExecutions pushes a complete revisioned catalog to each
// live agentctl. One unavailable execution does not prevent the others from
// converging; stale delivery is rejected by agentctl's snapshot revision.
func (m *Manager) SetPluginToolsForAllExecutions(ctx context.Context, snapshot plugintools.Snapshot) error {
	var refreshErr error
	for _, execution := range m.ListExecutions() {
		if execution == nil {
			continue
		}
		client, releaseClient := execution.AcquireAgentCtlClient()
		if client == nil {
			continue
		}
		err := client.SetPluginTools(ctx, snapshot)
		releaseClient()
		if err != nil {
			refreshErr = errors.Join(refreshErr, fmt.Errorf("refresh execution %s plugin tools: %w", execution.ID, err))
		}
	}
	return refreshErr
}

// resolveApprovalPolicyAndDisplayName resolves the approval policy and agent display name
// from the execution's agent profile and registry.
func (m *Manager) resolveApprovalPolicyAndDisplayName(ctx context.Context, execution *AgentExecution) (string, string) {
	approvalPolicy := ""
	agentDisplayName := ""
	if execution.AgentProfileID == "" || m.profileResolver == nil {
		return approvalPolicy, agentDisplayName
	}
	profileInfo, err := m.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
	if err != nil {
		return approvalPolicy, agentDisplayName
	}
	if profileInfo.AutoApprove {
		approvalPolicy = "never"
	} else {
		approvalPolicy = "untrusted"
	}
	// Look up display name from registry (e.g. "Claude", "Auggie", "Codex")
	if agentCfg, ok := m.registry.Get(profileInfo.AgentName); ok && agentCfg.DisplayName() != "" {
		agentDisplayName = agentCfg.DisplayName()
	} else {
		agentDisplayName = profileInfo.AgentName
	}
	return approvalPolicy, agentDisplayName
}

// createBootMessage creates a boot message and starts the stderr polling goroutine.
// Returns nil values when boot messages are unavailable.
func (m *Manager) createBootMessage(ctx context.Context, execution *AgentExecution, bootCommand, agentDisplayName string) (*models.Message, chan struct{}) {
	if m.bootMessageService == nil || execution == nil {
		return nil, nil
	}
	bootMsg, bootErr := m.bootMessageService.CreateMessage(ctx, &BootMessageRequest{
		TaskSessionID: execution.SessionID,
		TaskID:        execution.TaskID,
		Content:       "",
		AuthorType:    "agent",
		Type:          "script_execution",
		Metadata: map[string]interface{}{
			"script_type": "agent_boot",
			"agent_name":  agentDisplayName,
			"command":     bootCommand,
			"status":      "running",
			"is_resuming": execution.ACPSessionID != "",
			"started_at":  time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	if bootErr != nil {
		m.logger.Warn("failed to create boot message, continuing without boot output",
			zap.String("execution_id", execution.ID),
			zap.Error(bootErr))
		return nil, nil
	}
	bootStopCh := make(chan struct{})
	go m.pollAgentStderr(execution, bootMsg, bootStopCh)
	return bootMsg, bootStopCh
}

// getTaskDescriptionFromMetadata extracts the task description string from execution metadata.
func getTaskDescriptionFromMetadata(execution *AgentExecution) string {
	return execution.metadataString("task_description")
}

// getAttachmentsFromMetadata extracts attachments from execution metadata.
func getAttachmentsFromMetadata(execution *AgentExecution) []MessageAttachment {
	value, _ := execution.metadataValue("attachments")
	attachments, _ := value.([]MessageAttachment)
	return attachments
}

// configureAndStartAgent configures the agent command and starts the agent subprocess.
// Returns the effective boot command (full command with adapter args, or base command).
func (m *Manager) configureAndStartAgent(ctx context.Context, execution *AgentExecution, approvalPolicy string) (string, error) {
	env := execution.RuntimeEnvironment()
	metadataEnv := runtimeEnvFromMetadata(execution.MetadataSnapshot())
	if env == nil {
		env = metadataEnv
		if err := m.mergeAgentProfileEnvForExecution(ctx, execution, env); err != nil {
			m.updateExecutionError(execution.ID, "failed to resolve agent profile environment: "+err.Error())
			return "", fmt.Errorf("resolve agent profile environment: %w", err)
		}
	} else {
		// SetExecutionEnv carries per-run values such as repository credentials.
		// Overlay them on the launch snapshot without re-reading profile secrets.
		for key, value := range metadataEnv {
			env[key] = value
		}
	}
	if err := spillLargeWakePayloadEnv(env, execution.WorkspacePath, m.logger.Zap()); err != nil {
		m.updateExecutionError(execution.ID, "failed to prepare agent env: "+err.Error())
		return "", fmt.Errorf("failed to prepare agent env: %w", err)
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return "", fmt.Errorf("execution %q has no agentctl client", execution.ID)
	}

	if err := client.ConfigureAgent(ctx, execution.AgentCommand, execution.AgentArgs, env, approvalPolicy, execution.ContinueCommand, execution.ContinueArgs); err != nil {
		return "", fmt.Errorf("failed to configure agent: %w", err)
	}

	fullCommand, err := client.Start(ctx)
	if err != nil {
		m.updateExecutionError(execution.ID, "failed to start agent: "+err.Error())
		return "", fmt.Errorf("failed to start agent: %w", err)
	}

	bootCommand := fullCommand
	if bootCommand == "" {
		bootCommand = execution.AgentCommand
	}
	return bootCommand, nil
}

func runtimeEnvFromMetadata(metadata map[string]interface{}) map[string]string {
	env := map[string]string{}
	if metadata == nil {
		return env
	}
	if typed, ok := metadata["runtime_env"].(map[string]string); ok {
		for k, v := range typed {
			env[k] = v
		}
	}
	if raw, ok := metadata["runtime_env"].(map[string]interface{}); ok {
		for k, v := range raw {
			if str, strOK := v.(string); strOK {
				env[k] = str
			}
		}
	}
	return env
}

// initializeAgentSession handles post-startup initialization: boot message, ACP session,
// MCP servers. It finalizes the boot message on success or failure.
func (m *Manager) initializeAgentSession(ctx context.Context, execution *AgentExecution, bootCommand, agentDisplayName, taskDescription, approvalPolicy string) error {
	bootMsg, bootStopCh := m.createBootMessage(ctx, execution, bootCommand, agentDisplayName)

	// Give the agent process a moment to initialize
	time.Sleep(500 * time.Millisecond)

	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		m.finalizeBootMessage(execution, bootMsg, bootStopCh, "failed")
		return fmt.Errorf("failed to get agent config: %w", err)
	}

	mcpServers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		m.finalizeBootMessage(execution, bootMsg, bootStopCh, "failed")
		m.updateExecutionError(execution.ID, "failed to resolve MCP config: "+err.Error())
		return fmt.Errorf("failed to resolve MCP config: %w", err)
	}

	attachments := getAttachmentsFromMetadata(execution)
	if err := m.initializeACPSession(ctx, execution, agentConfig, taskDescription, attachments, mcpServers); err != nil {
		attempted, retryErr := m.retryManagedRuntimeStartup(
			ctx,
			execution,
			err,
			agentConfig,
			approvalPolicy,
			taskDescription,
			attachments,
			mcpServers,
		)
		if attempted {
			if retryErr == nil {
				m.finalizeBootMessage(execution, bootMsg, bootStopCh, containerStateExited)
				return nil
			}
			err = retryErr
		} else if retryErr != nil {
			err = retryErr
		}
		m.finalizeBootMessage(execution, bootMsg, bootStopCh, "failed")
		m.updateExecutionError(execution.ID, "failed to initialize ACP: "+err.Error())
		return fmt.Errorf("failed to initialize ACP: %w", err)
	}

	m.finalizeBootMessage(execution, bootMsg, bootStopCh, containerStateExited)
	return nil
}

// initGitRepo initializes a git repository in the given directory.
// Creates an initial commit so the workspace has a clean git state.
// Existing repositories are reused and keep Kandev metadata out of git status.
func (m *Manager) initGitRepo(ctx context.Context, workspacePath string) error {
	gitDir := filepath.Join(workspacePath, ".git")
	if info, err := os.Lstat(gitDir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("invalid git directory: %s", gitDir)
		}
		return excludeWorkspaceOwnershipMarker(gitDir)
	} else if !os.IsNotExist(err) {
		// Non-ENOENT error (permissions, I/O, etc.) - fail explicitly
		return fmt.Errorf("failed to check for .git directory: %w", err)
	}

	// Initialize git repository
	cmd := subproc.NewGitCommand(ctx, "init")
	cmd.Dir = workspacePath
	if output, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, cmd); err != nil {
		return fmt.Errorf("git init failed: %w (output: %s)", err, string(output))
	}

	// Configure git user (required for initial commit)
	configName := subproc.NewGitCommand(ctx, "config", "user.name", "Kandev Quick Chat")
	configName.Dir = workspacePath
	_ = subproc.RunGitClass(ctx, subproc.GitLifecycle, configName) // Ignore error - might already be configured globally

	configEmail := subproc.NewGitCommand(ctx, "config", "user.email", "quickchat@kandev.local")
	configEmail.Dir = workspacePath
	_ = subproc.RunGitClass(ctx, subproc.GitLifecycle, configEmail) // Ignore error - might already be configured globally

	// Create initial commit with empty .gitkeep file
	gitkeepPath := filepath.Join(workspacePath, ".gitkeep")
	if err := os.WriteFile(gitkeepPath, []byte(""), 0644); err != nil {
		return fmt.Errorf("failed to create .gitkeep: %w", err)
	}

	addCmd := subproc.NewGitCommand(ctx, "add", ".gitkeep")
	addCmd.Dir = workspacePath
	if output, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, addCmd); err != nil {
		return fmt.Errorf("git add failed: %w (output: %s)", err, string(output))
	}

	commitCmd := subproc.NewGitCommand(ctx, "commit", "-m", "Initial commit")
	commitCmd.Dir = workspacePath
	if output, err := subproc.RunGitCombinedOutputClass(ctx, subproc.GitLifecycle, commitCmd); err != nil {
		return fmt.Errorf("git commit failed: %w (output: %s)", err, string(output))
	}

	return excludeWorkspaceOwnershipMarker(gitDir)
}
