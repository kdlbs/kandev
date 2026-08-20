package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// WorkspaceRepositoryMaterialization is one durable repository projection
// prepared for a running remote workspace. It deliberately carries only a
// credential-free locator; executor launch environments provide Git auth.
type WorkspaceRepositoryMaterialization struct {
	RepositoryURL           string
	Destination             string
	BaseBranch              string
	CheckoutBranch          string
	RemoteContribution      *models.RemoteContribution
	ContributionDestination *models.ContributionDestination
}

const workspaceMaterializationRollbackTimeout = 10 * time.Second

func remoteWorkspaceProjectionFromLaunch(req *LaunchRequest) ([]WorkspaceRepositoryMaterialization, error) {
	if req == nil {
		return nil, fmt.Errorf("launch request is required")
	}
	specs := req.RepoSpecs()
	projection := make([]WorkspaceRepositoryMaterialization, 0, len(specs))
	// The first durable repository is established at the workspace root by
	// each remote executor's prepare path. Only sibling repositories belong in
	// agentctl-managed workspace subdirectories.
	for index, spec := range specs {
		if index == 0 {
			continue
		}
		if spec.RepositoryURL == "" {
			return nil, fmt.Errorf("remote repository %q has no clone URL", spec.RepoName)
		}
		branch := spec.CheckoutBranch
		if branch == "" {
			branch = spec.BaseBranch
		}
		name, branchSlug := worktree.SanitizeRepoDirName(spec.RepoName), worktree.SanitizeBranchSlug(branch)
		if name == "" || branchSlug == "" {
			return nil, fmt.Errorf("remote repository %q has unsafe runtime name", spec.RepoName)
		}
		projection = append(projection, WorkspaceRepositoryMaterialization{RepositoryURL: spec.RepositoryURL, Destination: name + "-" + branchSlug, BaseBranch: spec.BaseBranch, CheckoutBranch: spec.CheckoutBranch, RemoteContribution: spec.RemoteContribution, ContributionDestination: spec.ContributionDestination})
	}
	return projection, nil
}

// remoteWorkspaceSourceRoots returns only executor-visible, server-derived
// directories. Repository destinations have already been sanitized by the
// durable projection builder, so no host checkout path can enter agentctl.
func remoteWorkspaceSourceRoots(workspacePath string, repositories []WorkspaceRepositoryMaterialization) []string {
	roots := make([]string, 0, len(repositories)+1)
	if workspacePath == "" {
		return roots
	}
	roots = append(roots, workspacePath)
	for _, repository := range repositories {
		if repository.Destination != "" {
			roots = append(roots, filepath.Join(workspacePath, repository.Destination))
		}
	}
	return roots
}

type workspaceRepositoryClient interface {
	MaterializeRepository(context.Context, agentctl.MaterializeRepositoryRequest) (*agentctl.MaterializeRepositoryResponse, error)
	RemoveMaterializedRepository(context.Context, agentctl.RemoveMaterializedRepositoryRequest) error
	RescanWorkspace(context.Context, string, ...[]string) error
	ReconcileWorkspace(context.Context, ...[]string) error
}

// MaterializeRepositoriesForEnvironment reconciles the complete durable
// repository projection against the live agentctl workspace. It is intentionally
// environment-scoped so callers attach to the existing task/session execution.
func (m *Manager) MaterializeRepositoriesForEnvironment(ctx context.Context, taskEnvironmentID string, repositories []WorkspaceRepositoryMaterialization) ([]string, error) {
	executions, err := m.liveWorkspaceExecutionsForEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		return nil, err
	}
	clients := distinctWorkspaceRepositoryClients(executions)
	if len(clients) == 0 {
		return nil, fmt.Errorf("workspace execution has no agentctl client")
	}
	materialized := make([]materializedWorkspaceRepositoryClient, 0, len(clients))
	for _, execution := range clients {
		created, materializeErr := materializeWorkspaceRepositoriesWithoutRescan(ctx, execution.client, repositories, false)
		if materializeErr != nil {
			rollbackErr := rollbackMaterializedWorkspaceRepositoryClients(ctx, materialized)
			return nil, fmt.Errorf("materialize workspace repositories for session %s: %w", execution.sessionID, errors.Join(materializeErr, rollbackErr))
		}
		materialized = append(materialized, materializedWorkspaceRepositoryClient{execution: execution, created: created})
	}
	rescanned := make([]workspaceRepositoryExecution, 0, len(clients))
	refreshed := make([]cloneWorkspacePolicyRefresh, 0, len(clients))
	for _, execution := range clients {
		if isMutableCloneWorkspaceExecution(execution.execution) {
			refresh, refreshErr := m.refreshCloneWorkspacePolicyAfterMaterialization(ctx, execution.execution, repositories)
			if refreshErr != nil {
				cleanupErr := rollbackMaterializedWorkspaceRepositoryClients(ctx, materialized)
				rollbackErr := m.rollbackCloneWorkspacePolicyRefreshes(ctx, refreshed)
				return nil, fmt.Errorf("refresh clone workspace policy for session %s: %w", execution.sessionID, errors.Join(refreshErr, cleanupErr, rollbackErr))
			}
			refreshed = append(refreshed, refresh)
			continue
		}
		if err := execution.client.RescanWorkspace(ctx, "", execution.sourceRoots); err != nil {
			cleanupErr := rollbackMaterializedWorkspaceRepositoryClients(ctx, materialized)
			reconcileErr := rollbackWorkspaceRepositoryRescans(ctx, rescanned)
			reconcileErr = errors.Join(reconcileErr, m.rollbackCloneWorkspacePolicyRefreshes(ctx, refreshed))
			return nil, fmt.Errorf("rescan materialized workspace for session %s: %w", execution.sessionID, errors.Join(err, cleanupErr, reconcileErr))
		}
		rescanned = append(rescanned, execution)
	}
	return workspaceRepositorySessionIDs(executions), nil
}

type materializedWorkspaceRepositoryClient struct {
	execution workspaceRepositoryExecution
	created   []WorkspaceRepositoryMaterialization
}

// rollbackMaterializedWorkspaceRepositoryClients compensates every distinct
// executor even when an earlier deletion fails. Separate Docker, SSH, and
// Sprites workspaces each own their clone, so cleanup of one must never decide
// whether another receives rollback.
func rollbackMaterializedWorkspaceRepositoryClients(ctx context.Context, clients []materializedWorkspaceRepositoryClient) error {
	var errs []error
	for index := len(clients) - 1; index >= 0; index-- {
		client := clients[index]
		if err := rollbackMaterializedWorkspaceRepositories(ctx, client.execution.client, client.created); err != nil {
			errs = append(errs, fmt.Errorf("remove materialized workspace repositories for session %s: %w", client.execution.sessionID, err))
		}
	}
	return errors.Join(errs...)
}

func materializeWorkspaceRepositories(ctx context.Context, client workspaceRepositoryClient, repositories []WorkspaceRepositoryMaterialization, sourceRoots ...[]string) error {
	created, err := materializeWorkspaceRepositoriesWithoutRescan(ctx, client, repositories, true)
	if err != nil {
		return err
	}
	if err := client.RescanWorkspace(ctx, "", sourceRoots...); err != nil {
		cleanupErr := rollbackMaterializedWorkspaceRepositories(ctx, client, created)
		var reconcileErr error
		if cleanupErr == nil {
			reconcileErr = reconcileWorkspaceRepositoryClient(ctx, client, nil)
		}
		return fmt.Errorf("rescan materialized workspace: %w", errors.Join(err, cleanupErr, reconcileErr))
	}
	return nil
}

type workspaceRepositoryExecution struct {
	sessionID   string
	client      workspaceRepositoryClient
	sourceRoots []string
	execution   *AgentExecution
}

func (m *Manager) liveWorkspaceExecutionsForEnvironment(ctx context.Context, taskEnvironmentID string) ([]*AgentExecution, error) {
	if taskEnvironmentID == "" {
		return nil, fmt.Errorf("task_environment_id is required")
	}
	executions := make([]*AgentExecution, 0)
	for _, execution := range m.executionStore.List() {
		if execution != nil && execution.TaskEnvironmentID == taskEnvironmentID && execution.GetAgentCtlClient() != nil {
			executions = append(executions, execution)
		}
	}
	if len(executions) == 0 {
		execution, err := m.GetOrEnsureExecutionForEnvironment(ctx, taskEnvironmentID)
		if err != nil {
			return nil, fmt.Errorf("ensure workspace execution: %w", err)
		}
		if execution == nil || execution.GetAgentCtlClient() == nil {
			return nil, fmt.Errorf("workspace execution has no agentctl client")
		}
		executions = append(executions, execution)
	}
	sort.Slice(executions, func(i, j int) bool { return executions[i].ID < executions[j].ID })
	return executions, nil
}

func distinctWorkspaceRepositoryClients(executions []*AgentExecution) []workspaceRepositoryExecution {
	clients := make([]workspaceRepositoryExecution, 0, len(executions))
	seen := make(map[*agentctl.Client]struct{}, len(executions))
	for _, execution := range executions {
		client := execution.GetAgentCtlClient()
		if client == nil {
			continue
		}
		if _, exists := seen[client]; exists {
			continue
		}
		seen[client] = struct{}{}
		clients = append(clients, workspaceRepositoryExecution{
			sessionID:   execution.SessionID,
			client:      client,
			sourceRoots: append([]string(nil), execution.WorkspaceSourceRoots...),
			execution:   execution,
		})
	}
	return clients
}

// cloneWorkspacePolicyRefresh snapshots the last known-good child state while
// a later repository attachment temporarily extends its task-owned roots.
// It carries no host checkout information: all roots are executor-visible and
// every new GitDir is supplied by agentctl's final attestation.
type cloneWorkspacePolicyRefresh struct {
	execution   *AgentExecution
	oldRoots    []string
	oldEnv      map[string]string
	oldACP      string
	agentConfig agents.Agent
}

func isMutableCloneWorkspaceExecution(execution *AgentExecution) bool {
	if execution == nil {
		return false
	}
	switch execution.RuntimeName {
	case executor.NameDocker, executor.NameRemoteDocker, executor.NameSSH, executor.NameSprites:
		return true
	default:
		return false
	}
}

func cloneWorkspaceAttachmentRoots(execution *AgentExecution, repositories []WorkspaceRepositoryMaterialization) ([]string, error) {
	if execution == nil || execution.WorkspacePath == "" {
		return nil, errors.New("clone workspace is unavailable")
	}
	roots := append([]string(nil), execution.WorkspaceSourceRoots...)
	if len(roots) == 0 {
		roots = append(roots, execution.WorkspacePath)
	}
	seen := make(map[string]struct{}, len(roots)+len(repositories))
	for _, root := range roots {
		if root == "" {
			return nil, errors.New("clone workspace roots are invalid")
		}
		if _, duplicate := seen[root]; duplicate {
			return nil, errors.New("clone workspace roots are invalid")
		}
		seen[root] = struct{}{}
	}
	for _, repository := range repositories {
		if repository.Destination == "" || filepath.Base(repository.Destination) != repository.Destination {
			return nil, errors.New("clone workspace attachment is invalid")
		}
		root := filepath.Join(execution.WorkspacePath, repository.Destination)
		if _, exists := seen[root]; exists {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return roots, nil
}

// refreshCloneWorkspacePolicyAfterMaterialization makes a later remote
// attachment usable only after the live child has been stopped, configured
// from final executor-side proof, restarted, and restored. A rescan alone is
// insufficient because the existing child would retain its old CODEX_CONFIG.
func (m *Manager) refreshCloneWorkspacePolicyAfterMaterialization(ctx context.Context, execution *AgentExecution, repositories []WorkspaceRepositoryMaterialization) (cloneWorkspacePolicyRefresh, error) {
	refresh := cloneWorkspacePolicyRefresh{execution: execution}
	if execution == nil || execution.agentctl == nil {
		return refresh, unsupportedGitMetadataProjection("clone workspace policy refresh is unavailable; start a new session with a supported executor")
	}
	newRoots, err := cloneWorkspaceAttachmentRoots(execution, repositories)
	if err != nil {
		return refresh, unsupportedGitMetadataProjection("clone workspace policy refresh is unavailable; start a new session with a supported executor")
	}
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return refresh, unsupportedGitMetadataProjection("clone workspace policy refresh is unavailable; start a new session with a supported executor")
	}
	execution.promptLifecycleMu.Lock()
	defer execution.promptLifecycleMu.Unlock()
	if execution.Status != v1.AgentStatusReady {
		return refresh, unsupportedGitMetadataProjection("clone workspace policy refresh requires an idle session; wait for the current turn and try again")
	}
	refresh.oldRoots = append([]string(nil), execution.WorkspaceSourceRoots...)
	refresh.oldEnv = execution.RuntimeEnvironment()
	refresh.oldACP = execution.ACPSessionID
	refresh.agentConfig = agentConfig
	execution.Status = v1.AgentStatusStarting
	if err := execution.agentctl.Stop(ctx); err != nil {
		return refresh, m.failClosedCloneWorkspacePolicyRefresh(ctx, refresh)
	}
	if err := execution.agentctl.RescanWorkspace(ctx, "", newRoots); err != nil {
		return refresh, m.rollbackCloneWorkspacePolicyRefresh(ctx, refresh, unsupportedGitMetadataProjection("clone workspace policy refresh failed; start a new session with a supported executor"))
	}
	request := &ExecutorCreateRequest{
		WorkspacePath:          execution.WorkspacePath,
		WorkspaceSourceRoots:   newRoots,
		GitMetadataRequirement: cloneGitMetadataRequirement(true),
		AgentConfig:            agentConfig,
		Env:                    cloneStringMap(refresh.oldEnv),
	}
	newEnv, err := attestedCloneGitMetadataRuntimeEnv(ctx, request, execution.agentctl, execution.WorkspacePath, newRoots)
	if err != nil {
		return refresh, m.rollbackCloneWorkspacePolicyRefresh(ctx, refresh, unsupportedGitMetadataProjection("clone workspace policy refresh failed; start a new session with a supported executor"))
	}
	if err := m.configureCloneWorkspacePolicy(ctx, execution, newEnv); err != nil {
		return refresh, m.rollbackCloneWorkspacePolicyRefresh(ctx, refresh, unsupportedGitMetadataProjection("clone workspace policy refresh failed; start a new session with a supported executor"))
	}
	if _, err := execution.agentctl.Start(ctx); err != nil {
		return refresh, m.rollbackCloneWorkspacePolicyRefresh(ctx, refresh, unsupportedGitMetadataProjection("clone workspace policy refresh failed; start a new session with a supported executor"))
	}
	if refresh.oldACP != "" {
		if err := m.restoreReboundACPSession(ctx, execution, refresh.oldACP, m.startsNewSessionOnWorkspaceRebind(execution)); err != nil {
			return refresh, m.rollbackCloneWorkspacePolicyRefresh(ctx, refresh, unsupportedGitMetadataProjection("clone workspace policy refresh failed; start a new session with a supported executor"))
		}
	}
	execution.WorkspaceSourceRoots = newRoots
	execution.setRuntimeEnvironment(newEnv)
	execution.setMetadataValue("runtime_env", cloneStringMap(newEnv))
	execution.Status = v1.AgentStatusReady
	return refresh, nil
}

func (m *Manager) configureCloneWorkspacePolicy(ctx context.Context, execution *AgentExecution, runtimeEnv map[string]string) error {
	env := cloneStringMap(runtimeEnv)
	m.mergeAgentProfileEnvForExecution(ctx, execution, env)
	approvalPolicy, _ := m.resolveApprovalPolicyAndDisplayName(ctx, execution)
	return execution.agentctl.ConfigureAgent(ctx, execution.AgentCommand, execution.AgentArgs, env, approvalPolicy, execution.ContinueCommand, execution.ContinueArgs)
}

func (m *Manager) rollbackCloneWorkspacePolicyRefreshes(ctx context.Context, refreshes []cloneWorkspacePolicyRefresh) error {
	var errs []error
	for index := len(refreshes) - 1; index >= 0; index-- {
		refresh := refreshes[index]
		if refresh.execution == nil {
			continue
		}
		refresh.execution.promptLifecycleMu.Lock()
		if err := m.rollbackCloneWorkspacePolicyRefresh(ctx, refresh, errors.New("restore clone workspace policy after attachment failure")); err != nil {
			errs = append(errs, err)
		}
		refresh.execution.promptLifecycleMu.Unlock()
	}
	return errors.Join(errs...)
}

func (m *Manager) rollbackCloneWorkspacePolicyRefresh(ctx context.Context, refresh cloneWorkspacePolicyRefresh, cause error) error {
	execution := refresh.execution
	if execution == nil || execution.agentctl == nil {
		return cause
	}
	rollbackCtx := context.WithoutCancel(ctx)
	var rollbackErr error
	if err := execution.agentctl.Stop(rollbackCtx); err != nil {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback stop clone child: %w", err))
	}
	if err := execution.agentctl.ReconcileWorkspace(rollbackCtx, refresh.oldRoots); err != nil {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("rollback clone workspace roots: %w", err))
	}
	if rollbackErr != nil {
		return m.failClosedCloneWorkspacePolicyRefresh(rollbackCtx, refresh)
	}
	oldEnv, attestErr := m.attestedCloneWorkspacePolicyEnvironment(rollbackCtx, refresh)
	if attestErr != nil {
		return m.failClosedCloneWorkspacePolicyRefresh(rollbackCtx, refresh)
	}
	if rollbackErr := m.restoreCloneWorkspacePolicyChild(rollbackCtx, refresh, oldEnv); rollbackErr != nil {
		return m.failClosedCloneWorkspacePolicyRefresh(rollbackCtx, refresh)
	}
	execution.WorkspaceSourceRoots = append([]string(nil), refresh.oldRoots...)
	execution.setRuntimeEnvironment(oldEnv)
	execution.setMetadataValue("runtime_env", cloneStringMap(oldEnv))
	execution.Status = v1.AgentStatusReady
	return cause
}

func (m *Manager) attestedCloneWorkspacePolicyEnvironment(ctx context.Context, refresh cloneWorkspacePolicyRefresh) (map[string]string, error) {
	execution := refresh.execution
	if execution == nil || execution.agentctl == nil {
		return nil, errors.New("rollback clone workspace policy is unavailable")
	}
	request := &ExecutorCreateRequest{
		WorkspacePath:          execution.WorkspacePath,
		WorkspaceSourceRoots:   refresh.oldRoots,
		GitMetadataRequirement: cloneGitMetadataRequirement(true),
		AgentConfig:            refresh.agentConfig,
		Env:                    cloneStringMap(refresh.oldEnv),
	}
	env, err := attestedCloneGitMetadataRuntimeEnv(ctx, request, execution.agentctl, execution.WorkspacePath, refresh.oldRoots)
	if err != nil {
		return nil, fmt.Errorf("rollback attest clone workspace policy: %w", err)
	}
	return env, nil
}

func (m *Manager) restoreCloneWorkspacePolicyChild(ctx context.Context, refresh cloneWorkspacePolicyRefresh, runtimeEnv map[string]string) error {
	execution := refresh.execution
	if err := m.configureCloneWorkspacePolicy(ctx, execution, runtimeEnv); err != nil {
		return fmt.Errorf("rollback clone child policy: %w", err)
	}
	if _, err := execution.agentctl.Start(ctx); err != nil {
		return fmt.Errorf("rollback restart clone child: %w", err)
	}
	if refresh.oldACP == "" {
		return nil
	}
	if err := m.restoreReboundACPSession(ctx, execution, refresh.oldACP, false); err != nil {
		return fmt.Errorf("rollback restore clone child session: %w", err)
	}
	return nil
}

// failClosedCloneWorkspacePolicyRefresh records that a live clone child could
// not be proven back at its previous policy. The caller only receives recovery
// guidance: agentctl transport errors can contain executor filesystem details.
func (m *Manager) failClosedCloneWorkspacePolicyRefresh(ctx context.Context, refresh cloneWorkspacePolicyRefresh) error {
	execution := refresh.execution
	if execution == nil {
		return unsupportedGitMetadataProjection("clone workspace policy refresh failed; start a new session with a supported executor")
	}
	stopUnconfirmed := false
	if execution.agentctl != nil {
		// A second stop is deliberate. A failed stop reply does not prove the
		// child is gone, and a failed rollback must never leave it usable.
		if err := execution.agentctl.Stop(context.WithoutCancel(ctx)); err != nil {
			stopUnconfirmed = true
		}
	}
	execution.Status = v1.AgentStatusFailed
	execution.ErrorMessage = "clone workspace policy recovery required; start a new session"
	if stopUnconfirmed {
		execution.ErrorMessage = "clone workspace policy recovery required and child stop could not be confirmed; start a new session"
	}
	if m.executionStore != nil {
		m.executionStore.UpdateError(execution.ID, execution.ErrorMessage)
	}
	return unsupportedGitMetadataProjection("clone workspace policy refresh failed; start a new session with a supported executor")
}

func workspaceRepositorySessionIDs(executions []*AgentExecution) []string {
	ids := make([]string, 0, len(executions))
	seen := make(map[string]struct{}, len(executions))
	for _, execution := range executions {
		if execution.SessionID == "" {
			continue
		}
		if _, exists := seen[execution.SessionID]; !exists {
			seen[execution.SessionID] = struct{}{}
			ids = append(ids, execution.SessionID)
		}
	}
	return ids
}

func materializeWorkspaceRepositoriesWithoutRescan(ctx context.Context, client workspaceRepositoryClient, repositories []WorkspaceRepositoryMaterialization, requireGitMetadataAttestation bool) ([]WorkspaceRepositoryMaterialization, error) {
	if client == nil {
		return nil, fmt.Errorf("agentctl client is required")
	}
	created := make([]WorkspaceRepositoryMaterialization, 0, len(repositories))
	for _, repository := range repositories {
		response, err := client.MaterializeRepository(ctx, agentctl.MaterializeRepositoryRequest{
			RepositoryURL:           repository.RepositoryURL,
			Destination:             repository.Destination,
			BaseBranch:              repository.BaseBranch,
			CheckoutBranch:          repository.CheckoutBranch,
			RemoteContribution:      repository.RemoteContribution,
			ContributionDestination: repository.ContributionDestination,
		})
		if err != nil {
			rollbackErr := rollbackMaterializedWorkspaceRepositories(ctx, client, created)
			return nil, fmt.Errorf("materialize workspace repository %q: %w", repository.Destination, errors.Join(err, rollbackErr))
		}
		if response == nil {
			rollbackErr := rollbackMaterializedWorkspaceRepositories(ctx, client, created)
			return nil, fmt.Errorf("materialize workspace repository %q: %w", repository.Destination, errors.Join(errors.New("empty response"), rollbackErr))
		}
		if requireGitMetadataAttestation && !response.GitMetadataAttested {
			rollbackErr := rollbackMaterializedWorkspaceRepositories(ctx, client, created)
			return nil, fmt.Errorf("materialize workspace repository %q: %w", repository.Destination, errors.Join(errors.New("git metadata attestation missing"), rollbackErr))
		}
		if !response.Reused {
			created = append(created, repository)
		}
	}
	return created, nil
}

func rollbackWorkspaceRepositoryRescans(ctx context.Context, executions []workspaceRepositoryExecution) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspaceMaterializationRollbackTimeout)
	defer cancel()
	var errs []error
	for index := len(executions) - 1; index >= 0; index-- {
		execution := executions[index]
		if err := execution.client.ReconcileWorkspace(rollbackCtx, execution.sourceRoots); err != nil {
			errs = append(errs, fmt.Errorf("reconcile workspace for session %s: %w", execution.sessionID, err))
		}
	}
	return errors.Join(errs...)
}

func reconcileWorkspaceRepositoryClient(ctx context.Context, client workspaceRepositoryClient, sourceRoots []string) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspaceMaterializationRollbackTimeout)
	defer cancel()
	return client.ReconcileWorkspace(rollbackCtx, sourceRoots)
}

func rollbackMaterializedWorkspaceRepositories(ctx context.Context, client workspaceRepositoryClient, created []WorkspaceRepositoryMaterialization) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workspaceMaterializationRollbackTimeout)
	defer cancel()
	var errs []error
	for i := len(created) - 1; i >= 0; i-- {
		repository := created[i]
		if err := client.RemoveMaterializedRepository(cleanupCtx, agentctl.RemoveMaterializedRepositoryRequest{
			RepositoryURL: repository.RepositoryURL,
			Destination:   repository.Destination,
		}); err != nil {
			errs = append(errs, fmt.Errorf("remove materialized workspace repository %q: %w", repository.Destination, err))
		}
	}
	return errors.Join(errs...)
}
