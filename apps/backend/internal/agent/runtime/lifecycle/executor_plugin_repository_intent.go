package lifecycle

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/plugins/workspacedescriptor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/protobuf/proto"
)

const (
	pluginExecutorCapabilityRepositoryIntent = "repository_intent"

	pluginRepositoryIntentMaxEntries = 32
	pluginRepositoryLocatorMaxBytes  = 2048
	pluginAgentEnvMaxEntries         = 64
	pluginAgentEnvMaxBytes           = 16 * 1024
)

// pluginAgentEnvAllowlist is the fixed set of instance environment names a
// provider-prepared workspace may receive.
var pluginAgentEnvAllowlist = []string{
	"KANDEV_TASK_ID", "KANDEV_SESSION_ID", "KANDEV_INSTANCE_ID", "KANDEV_TASK_ENVIRONMENT_ID",
	"KANDEV_AGENT_PROFILE_ID", "KANDEV_EXECUTION_PROFILE_ID", "LANG", "LC_ALL", "TZ",
}

// pluginAgentEnvSecretFragments classify a name as secret-like, case-insensitively.
var pluginAgentEnvSecretFragments = []string{"TOKEN", "SECRET", "PASSWORD", "KEY", "CREDENTIAL"}

// PluginRepositorySource is one repository the host approved for a launch, in
// the form the intent is built from. A nil Position is a missing position.
type PluginRepositorySource struct {
	RepositoryID string
	Locator      string
	Branch       string
	BaseBranch   string
	CheckoutRef  string
	Position     *int
}

// pluginRepositorySources projects the launch's repositories. The checkout ref
// follows the selected-checkout convention: a pull request head ref, else the
// branch head, else empty.
func pluginRepositorySources(specs []RepoLaunchSpec) []PluginRepositorySource {
	sources := make([]PluginRepositorySource, 0, len(specs))
	for _, spec := range specs {
		ref := ""
		switch {
		case spec.PRNumber > 0:
			ref = fmt.Sprintf("refs/pull/%d/head", spec.PRNumber)
		case spec.CheckoutBranch != "":
			ref = "refs/heads/" + spec.CheckoutBranch
		}
		sources = append(sources, PluginRepositorySource{
			RepositoryID: spec.RepositoryID, Locator: spec.RepositoryURL, Branch: spec.CheckoutBranch,
			BaseBranch: spec.BaseBranch, CheckoutRef: ref, Position: spec.Position,
		})
	}
	return sources
}

// buildPluginRepositoryIntent orders the sources by position ascending (a
// missing position after every present one), then by repository ID in byte
// order, and strips every credential from the locator. A set over the limits
// fails as invalid_config; it is never truncated.
func buildPluginRepositoryIntent(sources []PluginRepositorySource) ([]*pluginsdk.ExecutorRepositoryIntent, error) {
	if len(sources) > pluginRepositoryIntentMaxEntries {
		return nil, fmt.Errorf("%w: repository intent has %d entries, at most %d are allowed",
			models.ErrExecutorInvalidConfig, len(sources), pluginRepositoryIntentMaxEntries)
	}
	ordered := append([]PluginRepositorySource(nil), sources...)
	sort.SliceStable(ordered, func(i, j int) bool { return repositoryIntentBefore(ordered[i], ordered[j]) })
	intent := make([]*pluginsdk.ExecutorRepositoryIntent, 0, len(ordered))
	for _, source := range ordered {
		locator, err := credentialFreeLocator(source.Locator)
		if err != nil {
			return nil, err
		}
		intent = append(intent, &pluginsdk.ExecutorRepositoryIntent{
			Id: source.RepositoryID, Locator: locator, Branch: source.Branch,
			BaseBranch: source.BaseBranch, CheckoutRef: source.CheckoutRef,
		})
	}
	return intent, nil
}

func repositoryIntentBefore(a, b PluginRepositorySource) bool {
	switch {
	case a.Position != nil && b.Position == nil:
		return true
	case a.Position == nil && b.Position != nil:
		return false
	case a.Position != nil && *a.Position != *b.Position:
		return *a.Position < *b.Position
	}
	return a.RepositoryID < b.RepositoryID
}

// credentialFreeLocator removes userinfo, query and fragment from a URL
// locator and bounds its size.
func credentialFreeLocator(locator string) (string, error) {
	if strings.Contains(locator, "://") {
		parsed, err := url.Parse(locator)
		if err != nil {
			return "", fmt.Errorf("%w: repository locator is not a valid URL", models.ErrExecutorInvalidConfig)
		}
		parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
		locator = parsed.String()
	}
	if len(locator) > pluginRepositoryLocatorMaxBytes {
		return "", fmt.Errorf("%w: repository locator exceeds %d bytes", models.ErrExecutorInvalidConfig, pluginRepositoryLocatorMaxBytes)
	}
	return locator, nil
}

// buildPluginAgentEnv selects the non-secret environment a provider receives:
// the profile's plain entries and the allowlisted instance names, minus every
// managed credential, broker lease and secret-like name. A set over 64 entries
// or 16 KiB fails as invalid_config; it is never truncated.
func buildPluginAgentEnv(instanceEnv, plainProfileEnv map[string]string) (map[string]string, error) {
	selected := make(map[string]string)
	for _, name := range pluginAgentEnvAllowlist {
		if value, ok := instanceEnv[name]; ok {
			selected[name] = value
		}
	}
	for name, value := range plainProfileEnv {
		selected[name] = value
	}
	size := 0
	for name, value := range selected {
		if pluginAgentEnvSecretLike(name) {
			delete(selected, name)
			continue
		}
		size += len(name) + len(value)
	}
	if len(selected) > pluginAgentEnvMaxEntries || size > pluginAgentEnvMaxBytes {
		return nil, fmt.Errorf("%w: agent environment exceeds %d entries or %d bytes",
			models.ErrExecutorInvalidConfig, pluginAgentEnvMaxEntries, pluginAgentEnvMaxBytes)
	}
	return selected, nil
}

// pluginAgentEnvSecretLike reports whether a name is a managed credential, a
// broker lease, git configuration injection, or matches a secret pattern.
func pluginAgentEnvSecretLike(name string) bool {
	upper := strings.ToUpper(name)
	for _, fragment := range pluginAgentEnvSecretFragments {
		if strings.Contains(upper, fragment) {
			return true
		}
	}
	if strings.HasPrefix(upper, "GIT_CONFIG_") {
		return true
	}
	for _, managed := range managedPluginAgentEnvNames() {
		if upper == strings.ToUpper(managed) {
			return true
		}
	}
	return false
}

func managedPluginAgentEnvNames() []string {
	return append([]string{
		"GITHUB_TOKEN", "GH_TOKEN",
		githubauth.CredentialHelperPathEnv, githubauth.CredentialCLIShimDirEnv,
		githubauth.CredentialCLIBashEnvEnv, githubauth.CredentialParentBashEnv,
	}, managedGitCredentialBrokerEnvKeys...)
}

// pluginRepositorySourcesForLaunch projects the launch's repositories for the
// intent. The primary repository of a single-repository launch carries its
// clone URL in the launch metadata.
func pluginRepositorySourcesForLaunch(req *LaunchRequest) ([]PluginRepositorySource, error) {
	specs := append([]RepoLaunchSpec(nil), req.RepoSpecs()...)
	if len(specs) > 0 && len(req.Repositories) == 0 && specs[0].RepositoryURL == "" {
		specs[0].RepositoryURL = getMetadataString(req.Metadata, "repository_clone_url")
	}
	for _, spec := range specs {
		if spec.RepositoryURL == "" {
			return nil, fmt.Errorf("%w: repository %q has no locator", models.ErrExecutorInvalidConfig, spec.RepositoryID)
		}
	}
	return pluginRepositorySources(specs), nil
}

// pluginPlainProfileEnv returns the profile's environment entries that carry a
// literal value and no secret reference.
func pluginPlainProfileEnv(entries []settingsmodels.ProfileEnvVar) map[string]string {
	plain := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.Key != "" && entry.SecretID == "" {
			plain[entry.Key] = entry.Value
		}
	}
	return plain
}

// pluginManagedGitCredentialMode names the managed Git credential mode a launch
// selected, or "" when it selected none.
func pluginManagedGitCredentialMode(env map[string]string) string {
	if managedGitCredentialBrokerEnv(env) != nil {
		return "managed Git credentials (credential broker)"
	}
	return ""
}

// rejectManagedCredentialsForProviderPreparation refuses, before any
// allocation, a launch that selected managed Git credentials for a workspace
// the provider prepares: provider preparation uses executor-owned credentials
// only.
func rejectManagedCredentialsForProviderPreparation(provider models.ExecutorProvider, env map[string]string) error {
	if provider.WorkspacePreparation != pluginWorkspacePreparationProvider {
		return nil
	}
	if mode := pluginManagedGitCredentialMode(env); mode != "" {
		return fmt.Errorf("%w: %s cannot be used when the provider prepares the workspace", models.ErrExecutorInvalidConfig, mode)
	}
	return nil
}

const (
	pluginWorkspacePreparationProvider = "provider"
	pluginWorkspacePreparationCore     = "core"

	cleanupStageRepositoryInventory          = "repository_inventory"
	cleanupReasonRepositoryInventoryMismatch = "repository_inventory_mismatch"
)

// pluginExecutorRepository is one provider-prepared repository recorded in the
// envelope: the approved ID, its path inside the workspace and its head commit.
type pluginExecutorRepository struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	HeadCommit string `json:"head_commit"`
}

func pluginWorkspacePreparationMode(provider models.ExecutorProvider) string {
	if provider.WorkspacePreparation == pluginWorkspacePreparationProvider {
		return pluginWorkspacePreparationProvider
	}
	return pluginWorkspacePreparationCore
}

func pluginProviderPreparesWorkspace(inventory pluginExecutorInventory) bool {
	return inventory.WorkspacePreparation == pluginWorkspacePreparationProvider
}

// pluginRepositoryHandoff builds what the provision request carries for a
// provider that requires repository_intent: the ordered intent and the agent
// environment. A managed credential selection is refused before any
// allocation when the provider prepares the workspace.
func pluginRepositoryHandoff(req *ExecutorCreateRequest, provider models.ExecutorProvider) ([]*pluginsdk.ExecutorRepositoryIntent, map[string]string, error) {
	if err := rejectManagedCredentialsForProviderPreparation(provider, req.Env); err != nil {
		return nil, nil, err
	}
	if !negotiatedDeclares(negotiatedCapabilities(provider), pluginExecutorCapabilityRepositoryIntent) {
		return nil, nil, nil
	}
	intent, err := buildPluginRepositoryIntent(req.PluginExecutor.Repositories)
	if err != nil {
		return nil, nil, err
	}
	agentEnv, err := buildPluginAgentEnv(req.Env, req.PluginExecutor.PlainEnv)
	if err != nil {
		return nil, nil, err
	}
	return intent, agentEnv, nil
}

// checkPluginExecutorRepositories verifies the repositories a provider
// returned and returns the inventory to record. A provider that does not
// prepare the workspace must return none.
func checkPluginExecutorRepositories(inventory pluginExecutorInventory, descriptor *workspacedescriptor.Descriptor, resource *pluginsdk.ExecutorResourceDescriptor, intent []*pluginsdk.ExecutorRepositoryIntent) ([]pluginExecutorRepository, error) {
	if !pluginProviderPreparesWorkspace(inventory) {
		if len(resource.GetRepositories()) > 0 {
			return nil, fmt.Errorf("%w: provider returned repositories without preparing the workspace", models.ErrExecutorConflict)
		}
		return nil, nil
	}
	if descriptor == nil {
		return nil, inventoryMismatch("no workspace descriptor to contain the repositories")
	}
	if err := verifyPluginExecutorRepositoryInventory(resource.GetPlatform(), descriptor.Path, intentIDs(intent), intent, resource.GetRepositories()); err != nil {
		return nil, err
	}
	recorded := make([]pluginExecutorRepository, 0, len(resource.GetRepositories()))
	for _, state := range resource.GetRepositories() {
		recorded = append(recorded, pluginExecutorRepository{ID: state.GetId(), Path: state.GetPath(), HeadCommit: state.GetHeadCommit()})
	}
	sort.Slice(recorded, func(i, j int) bool { return recorded[i].ID < recorded[j].ID })
	return recorded, nil
}

// verifyRecoveredPluginExecutorRepositories re-checks an attached environment
// against the persisted inventory: the set, path containment and head commit
// format. The branch is verified at launch only.
func verifyRecoveredPluginExecutorRepositories(inventory pluginExecutorInventory, attached *pluginsdk.ExecutorResourceDescriptor) error {
	if !pluginProviderPreparesWorkspace(inventory) {
		return nil
	}
	if inventory.WorkspaceDescriptor == nil {
		return inventoryMismatch("environment has no recorded workspace descriptor")
	}
	ids := make([]string, 0, len(inventory.Repositories))
	for _, repository := range inventory.Repositories {
		ids = append(ids, repository.ID)
	}
	sort.Strings(ids)
	return verifyPluginExecutorRepositoryInventory(attached.GetPlatform(), inventory.WorkspaceDescriptor.Path, ids, nil, attached.GetRepositories())
}

// publishedPluginExecutorRepos is the environment repository projection of a
// provider-prepared workspace, in recorded order.
func publishedPluginExecutorRepos(inventory pluginExecutorInventory) []*models.TaskEnvironmentRepo {
	if !pluginProviderPreparesWorkspace(inventory) {
		return nil
	}
	repos := make([]*models.TaskEnvironmentRepo, 0, len(inventory.Repositories))
	for index, repository := range inventory.Repositories {
		repos = append(repos, &models.TaskEnvironmentRepo{RepositoryID: repository.ID, WorkspaceRelativePath: repository.Path, Position: index})
	}
	return repos
}

// pluginRepositoryInventoryError is the failure of a repository inventory check.
type pluginRepositoryInventoryError struct{ reason string }

func (e *pluginRepositoryInventoryError) Error() string {
	return "repository inventory mismatch: " + e.reason
}

func inventoryMismatch(format string, args ...any) error {
	return &pluginRepositoryInventoryError{reason: fmt.Sprintf(format, args...)}
}

// verifyPluginExecutorRepositoryInventory checks the provider's reported
// repositories against the approved set: no missing, extra or duplicate entry,
// each path inside the workspace, each head commit a full hash. When intent is
// non-nil the checked-out branch must also equal the intent's branch (its base
// branch when none is named).
func verifyPluginExecutorRepositoryInventory(platform, workspacePath string, wantIDs []string, intent []*pluginsdk.ExecutorRepositoryIntent, reported []*pluginsdk.ExecutorRepositoryState) error {
	want := make(map[string]struct{}, len(wantIDs))
	for _, id := range wantIDs {
		want[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(reported))
	for _, state := range reported {
		id := state.GetId()
		if _, ok := want[id]; !ok {
			return inventoryMismatch("repository %q was not approved", id)
		}
		if _, dup := seen[id]; dup {
			return inventoryMismatch("repository %q is reported twice", id)
		}
		seen[id] = struct{}{}
		if !workspacedescriptor.Within(platform, workspacePath, state.GetPath()) {
			return inventoryMismatch("repository %q is outside the workspace", id)
		}
		if !fullCommitHash(state.GetHeadCommit()) {
			return inventoryMismatch("repository %q has no full head commit", id)
		}
	}
	if len(seen) != len(want) {
		return inventoryMismatch("%d of %d approved repositories were reported", len(seen), len(want))
	}
	return verifyInventoryBranches(intent, reported)
}

func verifyInventoryBranches(intent []*pluginsdk.ExecutorRepositoryIntent, reported []*pluginsdk.ExecutorRepositoryState) error {
	byID := make(map[string]*pluginsdk.ExecutorRepositoryState, len(reported))
	for _, state := range reported {
		byID[state.GetId()] = state
	}
	for _, entry := range intent {
		expected := entry.GetBranch()
		if expected == "" {
			expected = entry.GetBaseBranch()
		}
		if expected == "" {
			continue
		}
		if got := byID[entry.GetId()].GetBranch(); got != expected {
			return inventoryMismatch("repository %q is on a different branch than approved", entry.GetId())
		}
	}
	return nil
}

// fullCommitHash reports whether value is a full SHA-1 or SHA-256 object name.
func fullCommitHash(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func intentIDs(intent []*pluginsdk.ExecutorRepositoryIntent) []string {
	ids := make([]string, 0, len(intent))
	for _, entry := range intent {
		ids = append(ids, entry.GetId())
	}
	sort.Strings(ids)
	return ids
}

// attachPluginRepositoryInputs records on the launch what the repository intent
// and agent_env are built from.
func attachPluginRepositoryInputs(execReq *ExecutorCreateRequest, req *LaunchRequest, profileInfo *AgentProfileInfo) error {
	sources, err := pluginRepositorySourcesForLaunch(req)
	if err != nil {
		return err
	}
	execReq.PluginExecutor.Repositories = sources
	if profileInfo != nil {
		execReq.PluginExecutor.PlainEnv = pluginPlainProfileEnv(profileInfo.EnvVars)
	}
	return nil
}

// pluginProviderPreparedLaunch reports whether the provider, not agentctl,
// prepares this launch's repositories.
func pluginProviderPreparedLaunch(execReq *ExecutorCreateRequest) bool {
	return execReq != nil && execReq.PluginExecutor != nil &&
		pluginWorkspacePreparationMode(execReq.PluginExecutor.Profile.Provider) == pluginWorkspacePreparationProvider
}

// withoutPluginExecutorRepositories copies a resource descriptor minus the
// reported repositories; the envelope records them once, in its inventory.
func withoutPluginExecutorRepositories(resource *pluginsdk.ExecutorResourceDescriptor) *pluginsdk.ExecutorResourceDescriptor {
	if len(resource.GetRepositories()) == 0 {
		return resource
	}
	trimmed := proto.Clone(resource).(*pluginsdk.ExecutorResourceDescriptor)
	trimmed.Repositories = nil
	return trimmed
}
