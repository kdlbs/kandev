package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
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
		destination, err := remoteWorkspaceEntryName(spec.RepoName, spec.BaseBranch, spec.CheckoutBranch)
		if err != nil {
			return nil, err
		}
		projection = append(projection, WorkspaceRepositoryMaterialization{
			RepositoryURL: spec.RepositoryURL, Destination: destination,
			BaseBranch: spec.BaseBranch, CheckoutBranch: spec.CheckoutBranch,
			RemoteContribution: spec.RemoteContribution, ContributionDestination: spec.ContributionDestination,
		})
	}
	return projection, nil
}

func remoteWorkspaceEntryName(repoName, baseBranch, checkoutBranch string) (string, error) {
	branch := checkoutBranch
	if branch == "" {
		branch = baseBranch
	}
	name, branchSlug := worktree.SanitizeRepoDirName(repoName), worktree.SanitizeBranchSlug(branch)
	if name == "" || branchSlug == "" {
		return "", fmt.Errorf("remote repository %q has unsafe runtime name", repoName)
	}
	return name + "-" + branchSlug, nil
}

func validatePhysicalTaskHostPositions(repositories []WorkspaceRepositorySpec) (bool, error) {
	hasPhysicalMapping := false
	allPhysicallyMapped := true
	physicalPositions := make(map[int]struct{}, len(repositories))
	for _, repository := range repositories {
		if repository.TaskHostPosition == nil {
			allPhysicallyMapped = false
			continue
		}
		hasPhysicalMapping = true
		position := *repository.TaskHostPosition
		if position < 0 {
			return false, fmt.Errorf("repository %q has invalid physical task-host position %d", repository.RepositoryID, position)
		}
		if _, exists := physicalPositions[position]; exists {
			return false, fmt.Errorf("duplicate physical task-host repository position %d", position)
		}
		physicalPositions[position] = struct{}{}
	}
	if hasPhysicalMapping && !allPhysicallyMapped {
		return false, fmt.Errorf("physical task-host repository mapping is incomplete")
	}
	return hasPhysicalMapping, nil
}

// taskHostWorkspaceProjection converts durable host-side workspace sources to
// the paths visible inside the task host's runtime. Docker establishes the
// primary repository at /workspace and materializes durable siblings below it;
// forwarding host paths would leave agentctl watching nonexistent directories.
func taskHostWorkspaceProjection(runtimeName agentruntime.Runtime, info *WorkspaceInfo) (string, []string, error) {
	if info == nil {
		return "", nil, fmt.Errorf("workspace info is required")
	}
	if runtimeName != agentruntime.RuntimeDocker {
		return info.WorkspacePath, workspaceSourceRoots(info.WorkspaceFolders, info.WorkspaceRepositories), nil
	}
	if len(info.WorkspaceFolders) > 0 {
		return "", nil, fmt.Errorf("docker task hosts do not support host workspace folders")
	}
	repositories := append([]WorkspaceRepositorySpec(nil), info.WorkspaceRepositories...)
	hasPhysicalMapping, err := validatePhysicalTaskHostPositions(repositories)
	if err != nil {
		return "", nil, err
	}
	sort.SliceStable(repositories, func(i, j int) bool {
		if hasPhysicalMapping {
			return *repositories[i].TaskHostPosition < *repositories[j].TaskHostPosition
		}
		return repositories[i].Position < repositories[j].Position
	})
	roots := make([]string, 0, len(repositories))
	seen := make(map[string]struct{}, len(repositories))
	for index, repository := range repositories {
		physicalPosition := index
		if hasPhysicalMapping {
			physicalPosition = *repository.TaskHostPosition
		}
		root := dockerWorkspacePath
		if physicalPosition > 0 {
			entry, err := remoteWorkspaceEntryName(repository.RepoName, repository.BaseBranch, repository.CheckoutBranch)
			if err != nil {
				return "", nil, err
			}
			root = path.Join(dockerWorkspacePath, entry)
		}
		if _, exists := seen[root]; exists {
			continue
		}
		seen[root] = struct{}{}
		roots = append(roots, root)
	}
	return dockerWorkspacePath, roots, nil
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
	created, err := materializeWorkspaceRepositoriesWithoutRescan(ctx, clients[0].client, repositories)
	if err != nil {
		return nil, err
	}
	rescanned := make([]workspaceRepositoryExecution, 0, len(clients))
	for _, execution := range clients {
		if err := execution.client.RescanWorkspace(ctx, "", execution.sourceRoots); err != nil {
			cleanupErr := rollbackMaterializedWorkspaceRepositories(ctx, clients[0].client, created)
			var reconcileErr error
			if cleanupErr == nil {
				reconcileErr = rollbackWorkspaceRepositoryRescans(ctx, rescanned)
			}
			return nil, fmt.Errorf("rescan materialized workspace for session %s: %w", execution.sessionID, errors.Join(err, cleanupErr, reconcileErr))
		}
		rescanned = append(rescanned, execution)
	}
	return workspaceRepositorySessionIDs(executions), nil
}

func materializeWorkspaceRepositories(ctx context.Context, client workspaceRepositoryClient, repositories []WorkspaceRepositoryMaterialization) error {
	created, err := materializeWorkspaceRepositoriesWithoutRescan(ctx, client, repositories)
	if err != nil {
		return err
	}
	if err := client.RescanWorkspace(ctx, ""); err != nil {
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
		})
	}
	return clients
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

func materializeWorkspaceRepositoriesWithoutRescan(ctx context.Context, client workspaceRepositoryClient, repositories []WorkspaceRepositoryMaterialization) ([]WorkspaceRepositoryMaterialization, error) {
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
