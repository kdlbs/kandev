package backendapp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/agentctl"
	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	githubsvc "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/pkg/api/v1"
)

// taskGetterRepo is the minimal interface needed by the scheduler adapter.
type taskGetterRepo interface {
	GetTask(ctx context.Context, id string) (*models.Task, error)
}

// taskRepositoryAdapter adapts the task repository for the orchestrator's scheduler
type taskRepositoryAdapter struct {
	repo taskGetterRepo
	svc  *taskservice.Service
}

// GetTask retrieves a task by ID and converts it to API type
func (a *taskRepositoryAdapter) GetTask(ctx context.Context, taskID string) (*v1.Task, error) {
	task, err := a.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return task.ToAPI(), nil
}

// UpdateTaskState updates task state via the service
func (a *taskRepositoryAdapter) UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error {
	_, err := a.svc.UpdateTaskState(ctx, taskID, state)
	return err
}

// UpdateTaskStateIfCurrentIn transitions task state via the service when the
// current state is in allowed.
func (a *taskRepositoryAdapter) UpdateTaskStateIfCurrentIn(
	ctx context.Context, taskID string, state v1.TaskState, allowed []v1.TaskState,
) (bool, error) {
	return a.svc.UpdateTaskStateIfCurrentIn(ctx, taskID, state, allowed)
}

// UpdateTaskStateIfNotArchived is UpdateTaskStateIfCurrentIn without the
// prior-state constraint (see scheduler.TaskRepository doc).
func (a *taskRepositoryAdapter) UpdateTaskStateIfNotArchived(
	ctx context.Context, taskID string, state v1.TaskState,
) (bool, error) {
	return a.svc.UpdateTaskStateIfNotArchived(ctx, taskID, state)
}

func (a *taskRepositoryAdapter) UpdateTaskStateIfSessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	return a.svc.UpdateTaskStateIfSessionState(ctx, taskID, sessionID, expectedSessionState, state)
}

func (a *taskRepositoryAdapter) UpdateTaskStateIfPrimarySessionState(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionState models.TaskSessionState,
	state v1.TaskState,
) (bool, error) {
	return a.svc.UpdateTaskStateIfPrimarySessionState(
		ctx, taskID, sessionID, expectedSessionState, state,
	)
}

// lifecycleAdapter adapts the lifecycle manager as an AgentManagerClient
type lifecycleAdapter struct {
	mgr      *lifecycle.Manager
	registry *registry.Registry
	logger   *logger.Logger
}

// orchestrator.Service.currentACPSessionID selects the generation-safety seam
// for resetAgentContext by asserting the agent manager to this unexported
// interface; that assertion fails silently at runtime if this method is
// missing or its signature drifts, leaving storeResumeToken's stale-event
// guard and the reset-failure reconcile permanently inert against a defunct
// or dead-wrong ACP session id. Pin the exact shape here so drift is a build
// error, not a silently-dead production guard.
var _ interface {
	OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
	GetPromptGenerationForSession(ctx context.Context, sessionID string) (uint64, error)
	GetACPSessionIDForSession(sessionID string) (string, bool)
} = (*lifecycleAdapter)(nil)

// newLifecycleAdapter creates a new lifecycle adapter
func newLifecycleAdapter(mgr *lifecycle.Manager, reg *registry.Registry, log *logger.Logger) *lifecycleAdapter {
	return &lifecycleAdapter{
		mgr:      mgr,
		registry: reg,
		logger:   log.WithFields(zap.String("component", "lifecycle_adapter")),
	}
}

// LaunchAgent creates a new agentctl instance for a task.
// Agent subprocess is NOT started - call StartAgentProcess() explicitly.
func (a *lifecycleAdapter) LaunchAgent(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	// WorkspacePath wins when set (repo-less task with picked folder); otherwise
	// fall back to RepositoryURL (legacy: this carries a local filesystem path
	// for the workspace). Empty is also valid — lifecycle manager creates a
	// scratch workspace.
	workspacePath := req.WorkspacePath
	if workspacePath == "" {
		workspacePath = req.RepositoryURL
	}
	officeProfileID := req.OfficeAgentProfileID
	if officeProfileID == "" {
		officeProfileID = req.AgentProfileID
	}
	launchReq := buildLifecycleLaunchRequest(req, workspacePath, officeProfileID)

	// Create the agentctl execution (does NOT start agent process)
	execution, err := a.mgr.Launch(ctx, launchReq)
	if err != nil {
		return nil, err
	}

	// Extract worktree info from metadata if available
	metadata := execution.MetadataSnapshot()
	var worktreeID, worktreePath, worktreeBranch string
	if metadata != nil {
		if id, ok := metadata["worktree_id"].(string); ok {
			worktreeID = id
		}
		if path, ok := metadata["worktree_path"].(string); ok {
			worktreePath = path
		}
		if branch, ok := metadata["worktree_branch"].(string); ok {
			worktreeBranch = branch
		}
	}

	// Surface per-repo worktree results from the prepare step so the orchestrator
	// can persist N TaskEnvironmentRepo / TaskSessionWorktree rows when multi-repo.
	var worktrees []executor.RepoWorktreeResult
	var requestedBaseBranch, baseBranch, baseBranchFallbackWarning string
	if execution.PrepareResult != nil {
		requestedBaseBranch = execution.PrepareResult.RequestedBaseBranch
		baseBranch = execution.PrepareResult.BaseBranch
		baseBranchFallbackWarning = execution.PrepareResult.BaseBranchFallbackWarning
	}
	if execution.PrepareResult != nil && len(execution.PrepareResult.Worktrees) > 0 {
		worktrees = make([]executor.RepoWorktreeResult, 0, len(execution.PrepareResult.Worktrees))
		for _, w := range execution.PrepareResult.Worktrees {
			worktrees = append(worktrees, executor.RepoWorktreeResult{
				TaskRepositoryID:          w.TaskRepositoryID,
				RepositoryID:              w.RepositoryID,
				BranchSlug:                w.BranchSlug,
				WorktreeID:                w.WorktreeID,
				WorktreeBranch:            w.WorktreeBranch,
				WorktreePath:              w.WorktreePath,
				MainRepoGitDir:            w.MainRepoGitDir,
				RequestedBaseBranch:       w.RequestedBaseBranch,
				BaseBranch:                w.BaseBranch,
				BaseBranchFallbackWarning: w.BaseBranchFallbackWarning,
				ErrorMessage:              w.ErrorMessage,
			})
		}
	}

	return &executor.LaunchAgentResponse{
		AgentExecutionID:          execution.ID,
		ContainerID:               execution.ContainerID,
		Status:                    execution.Status,
		WorktreeID:                worktreeID,
		WorktreePath:              worktreePath,
		WorktreeBranch:            worktreeBranch,
		RequestedBaseBranch:       requestedBaseBranch,
		BaseBranch:                baseBranch,
		BaseBranchFallbackWarning: baseBranchFallbackWarning,
		WorkspacePath:             execution.WorkspacePath,
		Metadata:                  metadata,
		PrepareResult:             execution.PrepareResult,
		Worktrees:                 worktrees,
	}, nil
}

func buildLifecycleLaunchRequest(
	req *executor.LaunchAgentRequest, workspacePath, officeProfileID string,
) *lifecycle.LaunchRequest {
	launchReq := &lifecycle.LaunchRequest{
		TaskID:                        req.TaskID,
		WorkspaceID:                   req.WorkspaceID,
		SessionID:                     req.SessionID,
		TaskEnvironmentID:             req.TaskEnvironmentID,
		WorkspaceReuseRequired:        req.WorkspaceReuseRequired,
		TaskTitle:                     req.TaskTitle,
		AgentProfileID:                officeProfileID,
		ExecutionProfileID:            req.AgentProfileID,
		StartAgent:                    req.StartAgent,
		TurnID:                        req.TurnID,
		WorkspacePath:                 workspacePath,
		TaskDescription:               req.TaskDescription,
		Attachments:                   convertToLifecycleAttachments(req.Attachments),
		Env:                           req.Env,
		ApprovedSecretEnvKeys:         append([]string(nil), req.ApprovedSecretEnvKeys...),
		EnvironmentDefinitions:        append([]runtimeenv.Definition(nil), req.EnvironmentDefinitions...),
		EnvironmentResolutionRequired: req.EnvironmentResolutionRequired,
		ACPSessionID:                  req.ACPSessionID,
		Metadata:                      req.Metadata,
		ModelOverride:                 req.ModelOverride,
		ExecutorType:                  req.ExecutorType,
		ExecutorConfig:                req.ExecutorConfig,
		PreviousExecutionID:           req.PreviousExecutionID,
		McpMode:                       req.McpMode,
		McpProviders:                  req.McpProviders,
		McpProfile:                    req.McpProfile,
		IsEphemeral:                   req.IsEphemeral,
		IsPassthrough:                 req.IsPassthrough,
		SetupScript:                   req.SetupScript,
		CopyFiles:                     req.CopyFiles,
		UseWorktree:                   req.UseWorktree,
		WorktreeID:                    req.WorktreeID,
		RepositoryID:                  req.RepositoryID,
		TaskRepositoryID:              req.TaskRepositoryID,
		RepositoryPath:                req.RepositoryPath,
		BaseBranch:                    req.BaseBranch,
		DefaultBranch:                 req.DefaultBranch,
		CheckoutBranch:                req.CheckoutBranch,
		PRNumber:                      req.PRNumber,
		RemoteContribution:            req.RemoteContribution,
		ContributionDestination:       req.ContributionDestination,
		ComparisonTarget:              req.ComparisonTarget,
		WorktreeBranchPrefix:          req.WorktreeBranchPrefix,
		WorktreeBranchTemplate:        req.WorktreeBranchTemplate,
		WorktreeBranchTicket:          req.WorktreeBranchTicket,
		PullBeforeWorktree:            req.PullBeforeWorktree,
		RemoteSyncHandled:             req.RemoteSyncHandled,
		RefreshRepository:             req.RefreshRepository,
		TaskDirName:                   req.TaskDirName,
		RepoName:                      req.RepoName,
		BranchSlug:                    req.BranchSlug,
		BranchIdentitySlug:            req.BranchIdentitySlug,
	}
	launchReq.WorkspaceFolders = lifecycleWorkspaceFolders(req.WorkspaceFolders)
	launchReq.RouteOverride = lifecycleRouteOverride(req.RouteOverride)
	launchReq.Repositories = lifecycleRepoLaunchSpecs(req.Repositories)
	return launchReq
}

func lifecycleWorkspaceFolders(folders []executor.WorkspaceFolderSpec) []lifecycle.WorkspaceFolderSpec {
	if len(folders) == 0 {
		return nil
	}
	result := make([]lifecycle.WorkspaceFolderSpec, 0, len(folders))
	for _, f := range folders {
		result = append(result, lifecycle.WorkspaceFolderSpec{Name: f.Name, LocalPath: f.LocalPath})
	}
	return result
}

func lifecycleRouteOverride(override *executor.RouteOverride) *lifecycle.RouteOverride {
	if override == nil {
		return nil
	}
	return &lifecycle.RouteOverride{
		ExecutionProfileID: override.ExecutionProfileID,
		ProviderID:         override.ProviderID,
		Model:              override.Model,
		Tier:               override.Tier,
		Mode:               override.Mode,
		Flags:              override.Flags,
		Env:                override.Env,
	}
}

func lifecycleRepoLaunchSpecs(repos []executor.RepoSpec) []lifecycle.RepoLaunchSpec {
	if len(repos) == 0 {
		return nil
	}
	specs := make([]lifecycle.RepoLaunchSpec, 0, len(repos))
	for _, r := range repos {
		specs = append(specs, lifecycle.RepoLaunchSpec{
			TaskRepositoryID:        r.TaskRepositoryID,
			RepositoryID:            r.RepositoryID,
			RepositoryPath:          r.RepositoryPath,
			RepositoryURL:           r.RepositoryURL,
			RepoName:                r.RepoName,
			BaseBranch:              r.BaseBranch,
			DefaultBranch:           r.DefaultBranch,
			CheckoutBranch:          r.CheckoutBranch,
			PRNumber:                r.PRNumber,
			RemoteContribution:      r.RemoteContribution,
			ContributionDestination: r.ContributionDestination,
			ComparisonTarget:        r.ComparisonTarget,
			WorktreeID:              r.WorktreeID,
			WorktreeBranchPrefix:    r.WorktreeBranchPrefix,
			WorktreeBranchTemplate:  r.WorktreeBranchTemplate,
			WorktreeBranchTicket:    r.WorktreeBranchTicket,
			PullBeforeWorktree:      r.PullBeforeWorktree,
			RemoteSyncHandled:       r.RemoteSyncHandled,
			RefreshRepository:       r.RefreshRepository,
			RepoSetupScript:         r.RepoSetupScript,
			RepoCleanupScript:       r.RepoCleanupScript,
			CopyFiles:               r.CopyFiles,
			BranchSlug:              r.BranchSlug,
			BranchIdentitySlug:      r.BranchIdentitySlug,
		})
	}
	return specs
}

// convertToLifecycleAttachments converts v1.MessageAttachment to lifecycle.MessageAttachment.
func convertToLifecycleAttachments(attachments []v1.MessageAttachment) []lifecycle.MessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]lifecycle.MessageAttachment, len(attachments))
	for i, att := range attachments {
		result[i] = lifecycle.MessageAttachment{
			AttachmentID: att.AttachmentID,
			Type:         att.Type,
			Data:         att.Data,
			MimeType:     att.MimeType,
			Name:         att.Name,
			SizeBytes:    att.SizeBytes,
			DeliveryMode: att.DeliveryMode,
		}
	}
	return result
}

// SetExecutionDescription updates the task description in an existing execution's metadata.
func (a *lifecycleAdapter) SetExecutionDescription(ctx context.Context, agentExecutionID string, description string) error {
	return a.mgr.SetExecutionDescription(ctx, agentExecutionID, description)
}

func (a *lifecycleAdapter) SetPromptTurnID(ctx context.Context, agentExecutionID, turnID string) error {
	return a.mgr.SetPromptTurnID(ctx, agentExecutionID, turnID)
}

// RequiresCloneURL implements executor.ExecutorTypeCapabilities by delegating to
// the lifecycle manager. Without this, executor types like local_docker and
// sprites can't tell the orchestrator they need a clone URL.
func (a *lifecycleAdapter) RequiresCloneURL(executorType string) bool {
	return a.mgr.RequiresCloneURL(executorType)
}

// ShouldApplyPreferredShell implements executor.ExecutorTypeCapabilities.
func (a *lifecycleAdapter) ShouldApplyPreferredShell(executorType string) bool {
	return a.mgr.ShouldApplyPreferredShell(executorType)
}

// SetExecutionEnv updates per-run env vars in an existing execution.
func (a *lifecycleAdapter) SetExecutionEnv(ctx context.Context, agentExecutionID string, env map[string]string) error {
	return a.mgr.SetExecutionEnv(ctx, agentExecutionID, env)
}

// SetMcpMode changes the MCP tool mode on an existing execution's agentctl instance.
func (a *lifecycleAdapter) SetMcpMode(ctx context.Context, executionID string, mode string) error {
	return a.mgr.SetMcpMode(ctx, executionID, mode)
}

// StartAgentProcess starts the agent subprocess for an instance.
// The command is built internally based on the instance's agent profile.
func (a *lifecycleAdapter) StartAgentProcess(ctx context.Context, agentInstanceID string) error {
	return a.mgr.StartAgentProcess(ctx, agentInstanceID)
}

func (a *lifecycleAdapter) IsAgentCommandConfigured(agentInstanceID string) bool {
	return a.mgr.IsAgentCommandConfigured(agentInstanceID)
}

// StopAgent stops a running agent
func (a *lifecycleAdapter) StopAgent(ctx context.Context, agentInstanceID string, force bool) error {
	return a.mgr.StopAgent(ctx, agentInstanceID, force)
}

// StopAgentWithReason stops a running agent and propagates the stop reason to runtime teardown.
// The lifecycle-tier not-found sentinel is normalized to the public runtime
// sentinel at this seam so orchestrator consumers depend only on
// runtimeapi.ErrNotFound and never import runtime/lifecycle.
func (a *lifecycleAdapter) StopAgentWithReason(ctx context.Context, agentInstanceID string, reason string, force bool) error {
	return normalizeRuntimeStopError(a.mgr.StopAgentWithReason(ctx, agentInstanceID, reason, force))
}

// normalizeRuntimeStopError maps the backend lifecycle not-found sentinel onto
// the public runtime not-found sentinel while preserving the original error for
// diagnostics. Unrelated errors (and nil) pass through unchanged so a transient
// runtime failure is never mistaken for an absent runtime.
func normalizeRuntimeStopError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, lifecycle.ErrExecutionNotFound) {
		return errors.Join(runtimeapi.ErrNotFound, err)
	}
	return err
}

// RowLiveness classifies the liveness of the OS process backing an
// executors_running row using the runtime-aware host-local probe. It is the
// orchestrator's window into the platform-split liveness check (kept in the
// lifecycle package) used by startup reconciliation
// (#1597 runtime-aware liveness). A local check never runs against a
// remote/SSH row — such rows return Unknown.
func (a *lifecycleAdapter) RowLiveness(row *models.ExecutorRunning) models.ProcessLiveness {
	return lifecycle.RowProcessLiveness(row)
}

// GetAgentStatus returns the status of an agent execution
func (a *lifecycleAdapter) GetAgentStatus(ctx context.Context, agentInstanceID string) (*v1.AgentExecution, error) {
	execution, found := a.mgr.GetExecution(agentInstanceID)
	if !found {
		return nil, fmt.Errorf("agent execution %q not found", agentInstanceID)
	}

	containerID := execution.ContainerID
	now := time.Now()
	result := &v1.AgentExecution{
		ID:             execution.ID,
		TaskID:         execution.TaskID,
		AgentProfileID: execution.AgentProfileID,
		ContainerID:    &containerID,
		Status:         execution.Status,
		StartedAt:      &execution.StartedAt,
		StoppedAt:      execution.FinishedAt,
		CreatedAt:      execution.StartedAt,
		UpdatedAt:      now,
	}

	if execution.ExitCode != nil {
		result.ExitCode = execution.ExitCode
	}
	if execution.ErrorMessage != "" {
		result.ErrorMessage = &execution.ErrorMessage
	}

	return result, nil
}

// ListAgentTypes returns available agent types
func (a *lifecycleAdapter) ListAgentTypes(ctx context.Context) ([]*v1.AgentType, error) {
	configs := a.registry.List()
	result := make([]*v1.AgentType, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, registry.ToAPIType(cfg))
	}
	return result, nil
}

func (a *lifecycleAdapter) WasSessionInitialized(executionID string) bool {
	return a.mgr.WasSessionInitialized(executionID)
}

func (a *lifecycleAdapter) OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool {
	return a.mgr.OwnsPromptGeneration(sessionID, executionID, generation)
}

func (a *lifecycleAdapter) GetPromptGenerationForSession(ctx context.Context, sessionID string) (uint64, error) {
	return a.mgr.GetPromptGenerationForSession(ctx, sessionID)
}

// GetACPSessionIDForSession forwards to the lifecycle manager so
// orchestrator.Service.currentACPSessionID observes the live execution's
// actual ACP session identity in production, not just in tests against
// mockAgentManager (see the compile-time assertion above this type).
func (a *lifecycleAdapter) GetACPSessionIDForSession(sessionID string) (string, bool) {
	return a.mgr.GetACPSessionIDForSession(sessionID)
}

func (a *lifecycleAdapter) GetSessionAuthMethods(sessionID string) []streams.AuthMethodInfo {
	return a.mgr.GetSessionAuthMethods(sessionID)
}

// PromptAgent sends a follow-up prompt to a running agent
// Attachments (images) are passed to the agent if provided
func (a *lifecycleAdapter) PromptAgent(ctx context.Context, agentInstanceID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool) (*executor.PromptResult, error) {
	result, err := a.mgr.PromptAgent(ctx, agentInstanceID, prompt, attachments, dispatchOnly)
	if err != nil {
		return nil, err
	}
	return &executor.PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

func (a *lifecycleAdapter) PromptAgentWithDispatchCallback(ctx context.Context, agentInstanceID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, onDispatched func()) (*executor.PromptResult, error) {
	result, err := a.mgr.PromptAgentWithDispatchCallback(ctx, agentInstanceID, prompt, attachments, dispatchOnly, onDispatched)
	if err != nil {
		return nil, err
	}
	return &executor.PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

// Compile-time guard for the steer capability. The executor selects the steer
// path by asserting the agent manager to its unexported
// steerAgentWithDispatchCallback interface (internal/orchestrator/executor); that
// assertion fails silently at runtime if this method's signature drifts,
// disabling every production steer. Pin the exact shape here so drift is a build
// error, not a runtime regression.
var _ interface {
	SteerAgentWithDispatchCallback(context.Context, string, string, []v1.MessageAttachment, bool, func()) (*executor.PromptResult, error)
} = (*lifecycleAdapter)(nil)

// SteerAgentWithDispatchCallback forwards a mid-turn steer to the lifecycle
// manager. Without it the executor's steer path type-assertion fails and every
// eligible steer returns ErrPromptDispatchCallbackUnsupported, so the production
// agent-manager client must satisfy the steerAgentWithDispatchCallback capability
// exactly as it does the ordinary dispatch-callback prompt.
func (a *lifecycleAdapter) SteerAgentWithDispatchCallback(ctx context.Context, agentInstanceID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool, onDispatched func()) (*executor.PromptResult, error) {
	result, err := a.mgr.SteerAgentWithDispatchCallback(ctx, agentInstanceID, prompt, attachments, dispatchOnly, onDispatched)
	if err != nil {
		return nil, err
	}
	return &executor.PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

// CancelAgent interrupts the current agent turn without terminating the process.
func (a *lifecycleAdapter) CancelAgent(ctx context.Context, sessionID string) error {
	return a.mgr.CancelAgentBySessionID(ctx, sessionID)
}

// RestartAgentProcess stops the agent subprocess and starts a fresh one with a new ACP session.
func (a *lifecycleAdapter) RestartAgentProcess(ctx context.Context, agentExecutionID string) error {
	return a.mgr.RestartAgentProcess(ctx, agentExecutionID)
}

// ResetAgentContext resets the agent's context using the fastest available strategy.
func (a *lifecycleAdapter) ResetAgentContext(ctx context.Context, agentExecutionID string) error {
	return a.mgr.ResetAgentContext(ctx, agentExecutionID)
}

// SetSessionModelBySessionID attempts an in-place model switch via ACP session/set_model.
func (a *lifecycleAdapter) SetSessionModelBySessionID(ctx context.Context, sessionID, modelID string) error {
	return a.mgr.SetSessionModelBySessionID(ctx, sessionID, modelID)
}

// SetSessionConfigOptionBySessionID applies an ACP dynamic session option.
// Workflow conditional configuration uses this optional seam so older agent
// manager test doubles and non-ACP providers can fail closed without widening
// the executor launch contract.
func (a *lifecycleAdapter) SetSessionConfigOptionBySessionID(ctx context.Context, sessionID, configID, value string) error {
	return a.mgr.SetSessionConfigOptionBySessionID(ctx, sessionID, configID, value)
}

// SetSessionModeBySessionID applies a session permission mode via ACP session/set_mode.
func (a *lifecycleAdapter) SetSessionModeBySessionID(ctx context.Context, sessionID, modeID string) error {
	return a.mgr.SetSessionModeBySessionID(ctx, sessionID, modeID)
}

// RespondToPermissionBySessionID sends a response to a permission request for a session
func (a *lifecycleAdapter) RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	return a.mgr.RespondToPermissionBySessionID(sessionID, pendingID, optionID, cancelled)
}

func (a *lifecycleAdapter) ListPendingPermissionsBySessionID(ctx context.Context, sessionID string) ([]streams.PendingAgentPermission, error) {
	return a.mgr.ListPendingPermissionsBySessionID(ctx, sessionID)
}

func (a *lifecycleAdapter) ResolvePermissionBySessionID(ctx context.Context, sessionID, requestID, pendingID, optionID string) (*streams.PermissionResolveResponse, error) {
	return a.mgr.ResolvePermissionBySessionID(ctx, sessionID, requestID, pendingID, optionID)
}

func (a *lifecycleAdapter) CancelPermissionBySessionID(ctx context.Context, sessionID, requestID, pendingID string) (*streams.PermissionCancelResponse, error) {
	return a.mgr.CancelPermissionBySessionID(ctx, sessionID, requestID, pendingID)
}

// IsAgentRunningForSession checks if an agent is actually running for a session
// This probes the actual agent (Docker container or standalone process)
func (a *lifecycleAdapter) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	return a.mgr.IsAgentRunningForSession(ctx, sessionID)
}

// ProbeAgentRunningForSession preserves probe errors so orchestrator cleanup
// can distinguish an unavailable status check from a confirmed dead agent.
func (a *lifecycleAdapter) ProbeAgentRunningForSession(ctx context.Context, sessionID string) (bool, error) {
	return a.mgr.ProbeAgentRunningForSession(ctx, sessionID)
}

// IsAgentReadyForPrompt checks if the session can accept a prompt immediately.
func (a *lifecycleAdapter) IsAgentReadyForPrompt(ctx context.Context, sessionID string) bool {
	return a.mgr.IsAgentReadyForPrompt(ctx, sessionID)
}

func (a *lifecycleAdapter) RecoverAgentPromptStream(ctx context.Context, sessionID string) error {
	return a.mgr.RecoverAgentPromptStream(ctx, sessionID)
}

// IsPassthroughSession checks if the given session is running in passthrough (PTY) mode.
func (a *lifecycleAdapter) IsPassthroughSession(ctx context.Context, sessionID string) bool {
	return a.mgr.IsPassthroughSession(ctx, sessionID)
}

func (a *lifecycleAdapter) WritePassthroughStdin(ctx context.Context, sessionID string, data string) error {
	return a.mgr.WritePassthroughStdin(ctx, sessionID, data)
}

// ResolvePassthroughConfig returns the resolved PassthroughConfig for a session's agent.
func (a *lifecycleAdapter) ResolvePassthroughConfig(ctx context.Context, sessionID string) (agents.PassthroughConfig, error) {
	return a.mgr.ResolvePassthroughConfig(ctx, sessionID)
}

func (a *lifecycleAdapter) MarkPassthroughRunning(sessionID string) error {
	return a.mgr.MarkPassthroughRunning(sessionID)
}

func (a *lifecycleAdapter) PollRemoteStatusForRecords(ctx context.Context, records []executor.RemoteStatusPollRequest) {
	lcRecords := make([]lifecycle.RemoteStatusPollRecord, len(records))
	for i, r := range records {
		lcRecords[i] = lifecycle.RemoteStatusPollRecord{
			SessionID:        r.SessionID,
			Runtime:          r.Runtime,
			AgentExecutionID: r.AgentExecutionID,
			ContainerID:      r.ContainerID,
			Metadata:         r.Metadata,
		}
	}
	a.mgr.PollRemoteStatusForRecords(ctx, lcRecords)
}

func (a *lifecycleAdapter) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	return a.mgr.CleanupStaleExecutionBySessionID(ctx, sessionID)
}

func (a *lifecycleAdapter) CleanupStaleExecutionBySessionIDIfCurrent(
	ctx context.Context,
	sessionID, expectedExecutionID string,
	expectedUpdatedAt time.Time,
) error {
	return a.mgr.CleanupStaleExecutionBySessionIDIfCurrent(
		ctx, sessionID, expectedExecutionID, expectedUpdatedAt,
	)
}

func (a *lifecycleAdapter) EnsureWorkspaceExecutionForSession(ctx context.Context, taskID, sessionID string) error {
	_, err := a.mgr.EnsureWorkspaceExecutionForSession(ctx, taskID, sessionID)
	return err
}

func (a *lifecycleAdapter) GetExecutionIDForSession(ctx context.Context, sessionID string) (string, error) {
	return a.mgr.GetExecutionIDForSession(ctx, sessionID)
}

func (a *lifecycleAdapter) GetRemoteRuntimeStatusBySession(ctx context.Context, sessionID string) (*executor.RemoteRuntimeStatus, error) {
	status, ok := a.mgr.GetRemoteStatusBySessionID(ctx, sessionID)
	if !ok || status == nil {
		return nil, nil
	}
	return &executor.RemoteRuntimeStatus{
		RuntimeName:   status.RuntimeName,
		RemoteName:    status.RemoteName,
		State:         status.State,
		CreatedAt:     status.CreatedAt,
		LastCheckedAt: status.LastCheckedAt,
		ErrorMessage:  status.ErrorMessage,
	}, nil
}

// ResolveAgentProfile resolves an agent profile ID to profile information
func (a *lifecycleAdapter) ResolveAgentProfile(ctx context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	info, err := a.mgr.ResolveAgentProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return &executor.AgentProfileInfo{
		ProfileID:                  info.ProfileID,
		ProfileName:                info.ProfileName,
		AgentID:                    info.AgentID,
		AgentName:                  info.AgentName,
		Model:                      info.Model,
		Mode:                       info.Mode,
		ConfigOptions:              info.ConfigOptions,
		AutoApprove:                info.AutoApprove,
		DangerouslySkipPermissions: info.DangerouslySkipPermissions,
		CLIPassthrough:             info.CLIPassthrough,
		SupportsMCP:                info.SupportsMCP,
	}, nil
}

// GetGitLog retrieves the git log for a session from baseCommit to HEAD.
// If targetBranch is provided, uses dynamic merge-base calculation for accurate filtering.
func (a *lifecycleAdapter) GetGitLog(ctx context.Context, sessionID, baseCommit string, limit int, targetBranch string) (*client.GitLogResult, error) {
	execution, ok := a.mgr.GetExecutionBySessionID(sessionID)
	if !ok {
		return nil, nil // No execution, not an error
	}
	agentClient := execution.GetAgentCtlClient()
	if agentClient == nil {
		return nil, nil
	}
	return agentClient.GitLog(ctx, baseCommit, limit, targetBranch, "")
}

// GetCumulativeDiff retrieves the cumulative diff for a session from baseCommit to HEAD.
func (a *lifecycleAdapter) GetCumulativeDiff(ctx context.Context, sessionID, baseCommit string) (*client.CumulativeDiffResult, error) {
	execution, ok := a.mgr.GetExecutionBySessionID(sessionID)
	if !ok {
		return nil, nil // No execution, not an error
	}
	agentClient := execution.GetAgentCtlClient()
	if agentClient == nil {
		return nil, nil
	}
	// Orchestrator-side cumulative diffs (snapshot tracking) anchor to the
	// caller-provided base SHA — no dynamic merge-base recomputation. The
	// live panel uses the WS-handler path, which passes targetBranch.
	return agentClient.GetCumulativeDiff(ctx, baseCommit, "")
}

// GetGitStatus retrieves the current git status for a session.
func (a *lifecycleAdapter) GetGitStatus(ctx context.Context, sessionID string) (*client.GitStatusResult, error) {
	execution, ok := a.mgr.GetExecutionBySessionID(sessionID)
	if !ok {
		return nil, nil // No execution, not an error
	}
	agentClient := execution.GetAgentCtlClient()
	if agentClient == nil {
		return nil, nil
	}
	return agentClient.GetGitStatus(ctx)
}

// GetGitStatusFresh retrieves a fresh (non-cached) git status for a session.
func (a *lifecycleAdapter) GetGitStatusFresh(ctx context.Context, sessionID string) (*client.GitStatusResult, error) {
	execution, ok := a.mgr.GetExecutionBySessionID(sessionID)
	if !ok {
		return nil, nil
	}
	agentClient := execution.GetAgentCtlClient()
	if agentClient == nil {
		return nil, nil
	}
	return agentClient.GetGitStatusFresh(ctx)
}

// WaitForAgentctlReady waits for the agentctl HTTP server to be ready for a session.
func (a *lifecycleAdapter) WaitForAgentctlReady(ctx context.Context, sessionID string) error {
	return a.mgr.WaitForAgentctlReadyForSession(ctx, sessionID)
}

// orchestratorWrapper wraps orchestrator.Service to implement taskhandlers.OrchestratorService.
// The wrapper only adapts ResumeTaskSession which returns *executor.TaskExecution that we don't need.
type orchestratorWrapper struct {
	svc *orchestrator.Service
}

// PromptTask forwards directly to the orchestrator service.
// Attachments (images) are passed through to the agent.
func (w *orchestratorWrapper) PromptTask(ctx context.Context, taskID, taskSessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool) (*orchestrator.PromptResult, error) {
	return w.svc.PromptTask(ctx, taskID, taskSessionID, prompt, model, planMode, attachments, dispatchOnly)
}

// ResumeTaskSession forwards to the orchestrator service, discarding the TaskExecution result.
func (w *orchestratorWrapper) ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error {
	_, err := w.svc.ResumeTaskSession(ctx, taskID, taskSessionID)
	return err
}

// StartCreatedSession forwards to the orchestrator service, discarding the TaskExecution result.
func (w *orchestratorWrapper) StartCreatedSession(ctx context.Context, taskID, sessionID, agentProfileID, prompt string, skipMessageRecord, planMode, autoStart bool, attachments []v1.MessageAttachment, references []v1.EntityReference) error {
	_, err := w.svc.StartCreatedSession(ctx, taskID, sessionID, agentProfileID, prompt, skipMessageRecord, planMode, autoStart, attachments, references)
	return err
}

type githubTaskIssueStoreAdapter struct {
	svc *taskservice.Service
}

func (a githubTaskIssueStoreAdapter) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	task, err := a.svc.GetTask(ctx, taskID)
	if err != nil {
		return nil, wrapGitHubTaskIssueStoreError(err)
	}
	return task, nil
}

func (a githubTaskIssueStoreAdapter) ListTaskRepositories(ctx context.Context, taskID string) ([]*models.TaskRepository, error) {
	task, err := a.svc.GetTask(ctx, taskID)
	if err != nil {
		return nil, wrapGitHubTaskIssueStoreError(err)
	}
	return task.Repositories, nil
}

func (a githubTaskIssueStoreAdapter) GetRepository(ctx context.Context, repositoryID string) (*models.Repository, error) {
	return a.svc.GetRepository(ctx, repositoryID)
}

func (a githubTaskIssueStoreAdapter) UpdateTaskMetadata(ctx context.Context, taskID string, metadata map[string]interface{}) (*models.Task, error) {
	task, err := a.svc.UpdateTask(ctx, taskID, &taskservice.UpdateTaskRequest{Metadata: metadata})
	if err != nil {
		return nil, wrapGitHubTaskIssueStoreError(err)
	}
	return task, nil
}

func wrapGitHubTaskIssueStoreError(err error) error {
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return fmt.Errorf("%w: %w", githubsvc.ErrTaskNotFound, err)
	}
	return err
}

// ProcessOnTurnStart forwards to the orchestrator service.
func (w *orchestratorWrapper) ProcessOnTurnStart(ctx context.Context, taskID, sessionID string) (orchestrator.ProcessOnTurnStartResult, error) {
	return w.svc.ProcessOnTurnStart(ctx, taskID, sessionID)
}

// QueueUserPrompt forwards a prompt that must wait for workflow admission.
func (w *orchestratorWrapper) QueueUserPrompt(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment, metadata map[string]interface{}, userMessageRecorded bool) error {
	return w.svc.QueueUserPrompt(ctx, taskID, sessionID, prompt, model, planMode, attachments, metadata, userMessageRecorded)
}

// StepRequiresCompletionSignal forwards to the orchestrator service.
func (w *orchestratorWrapper) StepRequiresCompletionSignal(ctx context.Context, taskID string) bool {
	return w.svc.StepRequiresCompletionSignal(ctx, taskID)
}

// ForegroundActivity forwards to the orchestrator service (ADR-0049).
func (w *orchestratorWrapper) ForegroundActivity(sessionID string) v1.ForegroundActivity {
	return w.svc.ForegroundActivity(sessionID)
}

func (w *orchestratorWrapper) SteerEligible(sessionID string, state models.TaskSessionState) bool {
	return w.svc.SteerEligible(sessionID, state)
}

func (w *orchestratorWrapper) SteerTask(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment) (*orchestrator.PromptResult, error) {
	return w.svc.SteerTask(ctx, taskID, sessionID, prompt, model, planMode, attachments)
}

// subagentContextAdapter adapts the task service to the
// orchestrator.SubagentContextRecorder interface.
type subagentContextAdapter struct {
	svc *taskservice.Service
}

func (a *subagentContextAdapter) RecordSubagentContext(ctx context.Context, req taskservice.RecordSubagentContextRequest) {
	a.svc.RecordSubagentContext(ctx, req)
}

// messageCreatorAdapter adapts the task service to the orchestrator.MessageCreator interface
type messageCreatorAdapter struct {
	svc    *taskservice.Service
	logger *logger.Logger

	// sessionModelCache caches the resolved model per session to avoid repeated DB lookups.
	// Invalidated via InvalidateModelCache when the model changes (e.g., model switch).
	sessionModelMu    sync.RWMutex
	sessionModelCache map[string]string
}

// getSessionModel resolves the model from the session's agent profile snapshot.
// Results are cached per session ID to avoid repeated DB queries during streaming.
func (a *messageCreatorAdapter) getSessionModel(ctx context.Context, sessionID string) string {
	// Check cache first
	a.sessionModelMu.RLock()
	if model, ok := a.sessionModelCache[sessionID]; ok {
		a.sessionModelMu.RUnlock()
		return model
	}
	a.sessionModelMu.RUnlock()

	// Cache miss — fetch from DB
	session, err := a.svc.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.AgentProfileSnapshot == nil {
		return ""
	}
	model, _ := session.AgentProfileSnapshot["model"].(string)

	// Store in cache
	a.sessionModelMu.Lock()
	if a.sessionModelCache == nil {
		a.sessionModelCache = make(map[string]string)
	}
	a.sessionModelCache[sessionID] = model
	a.sessionModelMu.Unlock()

	return model
}

// InvalidateModelCache clears the cached model for a session so the next message
// re-reads it from the DB. Called after model switches.
func (a *messageCreatorAdapter) InvalidateModelCache(sessionID string) {
	a.sessionModelMu.Lock()
	delete(a.sessionModelCache, sessionID)
	a.sessionModelMu.Unlock()
}

// CreateAgentMessage creates a message with author_type="agent"
func (a *messageCreatorAdapter) CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID, turnID string) error {
	var metadata map[string]interface{}
	if model := a.getSessionModel(ctx, agentSessionID); model != "" {
		metadata = map[string]interface{}{"model": model}
	}
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
		Metadata:      metadata,
	})
	return err
}

// CreateUserMessage creates a message with author_type="user"
func (a *messageCreatorAdapter) CreateUserMessage(ctx context.Context, taskID, content, agentSessionID, turnID string, metadata map[string]interface{}) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "user",
		Metadata:      metadata,
	})
	return err
}

// CreateToolCallMessage creates a message for a tool call
func (a *messageCreatorAdapter) CreateToolCallMessage(ctx context.Context, taskID, toolCallID, parentToolCallID, title, status, agentSessionID, turnID string, normalized *streams.NormalizedPayload) error {
	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}
	// Add parent tool call ID for subagent nesting (if present)
	if parentToolCallID != "" {
		metadata["parent_tool_call_id"] = parentToolCallID
	}
	// Add normalized tool data to metadata for frontend consumption
	if normalized != nil {
		metadata["normalized"] = normalized
	}

	// Determine message type from the normalized tool kind
	msgType := "tool_call"
	if normalized != nil {
		msgType = normalized.Kind().ToMessageType()
	}

	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       title,
		AuthorType:    "agent",
		Type:          msgType,
		Metadata:      metadata,
	})
	return err
}

// UpdateToolCallMessage updates a tool call message's status.
// If the message doesn't exist, it creates it using taskID, turnID, and msgType.
func (a *messageCreatorAdapter) UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, parentToolCallID, status, result, agentSessionID, title, turnID, msgType string, normalized *streams.NormalizedPayload) error {
	return a.svc.UpdateToolCallMessageWithCreate(ctx, agentSessionID, toolCallID, parentToolCallID, status, result, title, normalized, taskID, turnID, msgType)
}

// CreateSessionMessage creates a message for non-chat session updates (status/progress/error/etc).
func (a *messageCreatorAdapter) CreateSessionMessage(ctx context.Context, taskID, content, agentSessionID, messageType, turnID string, metadata map[string]interface{}, requestsInput bool) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
		Type:          messageType,
		Metadata:      metadata,
		RequestsInput: requestsInput,
	})
	return err
}

// CreatePermissionRequestMessage creates a message for a permission request
func (a *messageCreatorAdapter) CreatePermissionRequestMessage(ctx context.Context, taskID, sessionID, requestID, pendingID, toolCallID, title, turnID string, options []map[string]interface{}, actionType string, actionDetails map[string]interface{}) (string, error) {
	metadata := map[string]interface{}{
		"request_id":     requestID,
		"pending_id":     pendingID,
		"tool_call_id":   toolCallID,
		"options":        options,
		"action_type":    actionType,
		"action_details": actionDetails,
	}

	msg, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       title,
		AuthorType:    "agent",
		Type:          "permission_request",
		Metadata:      metadata,
	})
	if err != nil {
		return "", err
	}
	return msg.ID, nil
}

// UpdatePermissionMessage updates a permission message's status
func (a *messageCreatorAdapter) UpdatePermissionMessage(ctx context.Context, taskID, sessionID, requestID, pendingID string, status models.PermissionStatus) error {
	return a.svc.UpdatePermissionMessage(ctx, taskID, sessionID, requestID, pendingID, status)
}

func (a *messageCreatorAdapter) ClaimPermissionResolution(ctx context.Context, request models.PermissionResolutionClaimRequest) (*models.PermissionResolutionClaimResult, error) {
	return a.svc.ClaimPermissionResolution(ctx, request)
}

func (a *messageCreatorAdapter) FinalizePermissionResolution(ctx context.Context, request models.PermissionResolutionFinalizeRequest) (*models.PermissionResolutionFinalizeResult, error) {
	return a.svc.FinalizePermissionResolution(ctx, request)
}

func (a *messageCreatorAdapter) GetPermissionResolutionAudit(ctx context.Context, taskID, sessionID, requestID, pendingID string) (*models.PermissionResolutionAudit, error) {
	return a.svc.GetPermissionResolutionAudit(ctx, taskID, sessionID, requestID, pendingID)
}

// CreateClarificationRequestMessages emits one chat message per question in a
// clarification request bundle, all sharing the given pending_id. The frontend
// renders them as a stacked group; only the last message sets RequestsInput=true
// so the chat scrolls to the bottom of the group when the bundle arrives.
// Returns the created message IDs in the same order as the input questions.
//
// If any individual CreateMessage call fails, every previously created message
// in the bundle is deleted so we don't leave a half-rendered group dangling in
// the chat. Best-effort: if cleanup itself fails the caller still receives the
// original error and the orphan messages stay (logged at warn-level).
func (a *messageCreatorAdapter) CreateClarificationRequestMessages(ctx context.Context, taskID, sessionID, pendingID string, questions []clarification.Question, clarificationContext string) ([]string, error) {
	ids := make([]string, 0, len(questions))
	total := len(questions)
	for i, question := range questions {
		options := make([]interface{}, len(question.Options))
		for j, opt := range question.Options {
			options[j] = map[string]interface{}{
				"option_id":   opt.ID,
				"label":       opt.Label,
				"description": opt.Description,
			}
		}

		questionData := map[string]interface{}{
			"id":      question.ID,
			"title":   question.Title,
			"prompt":  question.Prompt,
			"options": options,
		}

		metadata := map[string]interface{}{
			"pending_id":     pendingID,
			"question_id":    question.ID,
			"question":       questionData,
			"question_index": i,
			"question_total": total,
			"context":        clarificationContext,
			"status":         "pending",
		}

		// Only the last message marks the session as waiting for input so the
		// chat scrolls to the end of the group when the bundle arrives.
		requestsInput := i == total-1

		msg, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
			TaskSessionID: sessionID,
			TaskID:        taskID,
			Content:       question.Prompt,
			AuthorType:    "agent",
			Type:          "clarification_request",
			Metadata:      metadata,
			RequestsInput: requestsInput,
		})
		if err != nil {
			a.rollbackPartialBundle(ctx, ids, pendingID)
			return nil, err
		}
		ids = append(ids, msg.ID)
	}
	return ids, nil
}

// rollbackPartialBundle removes the messages already created when later writes
// in a multi-question bundle fail. Best-effort — failures are logged but do
// not bubble up because the caller already has a more useful error.
func (a *messageCreatorAdapter) rollbackPartialBundle(ctx context.Context, ids []string, pendingID string) {
	for _, id := range ids {
		if err := a.svc.DeleteMessage(ctx, id); err != nil {
			a.logger.Warn("failed to roll back partial clarification bundle message",
				zap.String("pending_id", pendingID),
				zap.String("message_id", id),
				zap.Error(err))
		}
	}
}

// UpdateClarificationMessage updates a clarification message's status and
// answer.
//
// A nil answer must be forwarded as an untyped nil, not as a nil
// *clarification.Answer boxed into the service method's interface{}
// parameter (COR-001): a typed nil pointer boxed into an interface produces
// a non-nil interface value, so the service's own `answer != nil` check
// would fire and write a literal "response": null into every rejected or
// cancelled clarification message's metadata.
func (a *messageCreatorAdapter) UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, questionID, status string, answer *clarification.Answer) error {
	if answer == nil {
		return a.svc.UpdateClarificationMessageForQuestion(ctx, sessionID, pendingID, questionID, status, nil)
	}
	return a.svc.UpdateClarificationMessageForQuestion(ctx, sessionID, pendingID, questionID, status, answer)
}

func (a *messageCreatorAdapter) CompleteActiveClarificationBundle(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
) ([]*models.Message, bool, error) {
	return a.svc.CompleteActiveClarificationBundle(ctx, pendingID, status, responses)
}

func (a *messageCreatorAdapter) FinalizeClarificationResponseDelivery(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*models.Message,
) ([]*models.Message, bool, error) {
	return a.svc.FinalizeClarificationResponseDelivery(
		ctx,
		pendingID,
		terminalStatus,
		claimedMessages,
	)
}

func (a *messageCreatorAdapter) RestoreActiveClarificationBundle(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*models.Message,
) ([]*models.Message, bool, error) {
	return a.svc.RestoreActiveClarificationBundle(
		ctx,
		pendingID,
		terminalStatus,
		claimedMessages,
	)
}

func (a *messageCreatorAdapter) PublishClarificationBundleUpdates(
	ctx context.Context,
	messages []*models.Message,
) error {
	return a.svc.PublishClarificationBundleUpdates(ctx, messages)
}

// CreateAgentMessageStreaming creates a new agent message with a pre-generated ID.
// This is used for real-time streaming where content arrives incrementally.
func (a *messageCreatorAdapter) CreateAgentMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error {
	var metadata map[string]interface{}
	if model := a.getSessionModel(ctx, agentSessionID); model != "" {
		metadata = map[string]interface{}{"model": model}
	}
	_, err := a.svc.CreateMessageWithID(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
		Metadata:      metadata,
	})
	return err
}

// AppendAgentMessage appends additional content to an existing streaming message.
func (a *messageCreatorAdapter) AppendAgentMessage(ctx context.Context, messageID, additionalContent string) error {
	return a.svc.AppendMessageContent(ctx, messageID, additionalContent)
}

// CreateThinkingMessageStreaming creates a new thinking message with a pre-generated ID.
// This is used for real-time streaming of agent thinking/reasoning content.
func (a *messageCreatorAdapter) CreateThinkingMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error {
	metadata := map[string]interface{}{
		"thinking": content,
	}
	if model := a.getSessionModel(ctx, agentSessionID); model != "" {
		metadata["model"] = model
	}
	_, err := a.svc.CreateMessageWithID(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       "",
		AuthorType:    "agent",
		Type:          "thinking",
		Metadata:      metadata,
	})
	return err
}

// AppendThinkingMessage appends additional content to an existing streaming thinking message.
func (a *messageCreatorAdapter) AppendThinkingMessage(ctx context.Context, messageID, additionalContent string) error {
	return a.svc.AppendThinkingContent(ctx, messageID, additionalContent)
}
