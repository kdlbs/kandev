package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// resolveAgentProfile resolves the agent profile and returns the agent type name and profile info.
func (m *Manager) resolveAgentProfile(ctx context.Context, req *LaunchRequest) (string, *AgentProfileInfo, error) {
	if m.profileResolver == nil {
		// Fallback: treat AgentProfileID as agent type directly (for backward compat)
		m.logger.Warn("no profile resolver configured, using profile ID as agent type",
			zap.String("agent_type", req.AgentProfileID))
		return req.AgentProfileID, nil, nil
	}
	profileInfo, err := m.profileResolver.ResolveProfile(ctx, req.AgentProfileID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to resolve agent profile: %w", err)
	}
	m.logger.Debug("resolved agent profile",
		zap.String("profile_id", req.AgentProfileID),
		zap.String("agent_name", profileInfo.AgentName),
		zap.String("agent_type", profileInfo.AgentName))
	return profileInfo.AgentName, profileInfo, nil
}

// buildLaunchMetadata builds runtime metadata for the Launch request.
func buildLaunchMetadata(req *LaunchRequest, mainRepoGitDir, worktreeID, worktreeBranch string) map[string]interface{} {
	metadata := make(map[string]interface{})
	for k, v := range req.Metadata {
		metadata[k] = v
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
	return metadata
}

// agentCommands holds the initial and continue command strings for an agent execution.
type agentCommands struct {
	initial   string
	continue_ string // continue command for one-shot agents (empty if not applicable)
}

// buildAgentCommand builds the agent command strings for the execution.
// Returns both the initial command and the continue command (for one-shot agents like Amp).
func (m *Manager) buildAgentCommand(req *LaunchRequest, profileInfo *AgentProfileInfo, agentConfig agents.Agent) agentCommands {
	model := ""
	autoApprove := false
	permissionValues := make(map[string]bool)
	if profileInfo != nil {
		model = profileInfo.Model
		autoApprove = profileInfo.AutoApprove
		permissionValues["auto_approve"] = profileInfo.AutoApprove
		permissionValues["allow_indexing"] = profileInfo.AllowIndexing
		permissionValues["dangerously_skip_permissions"] = profileInfo.DangerouslySkipPermissions
	}
	// Allow model override from request (for dynamic model switching)
	if req.ModelOverride != "" {
		model = req.ModelOverride
	}
	// Only pass SessionID (for --resume flag) if the agent supports recovery.
	// Agents with CanRecover=false (e.g. Auggie) use history context injection instead.
	sessionID := req.ACPSessionID
	if rt := agentConfig.Runtime(); rt != nil && !rt.SessionConfig.SupportsRecovery() {
		sessionID = ""
	}
	cmdOpts := agents.CommandOptions{
		Model:            model,
		SessionID:        sessionID,
		AutoApprove:      autoApprove,
		PermissionValues: permissionValues,
	}
	return agentCommands{
		initial:   m.commandBuilder.BuildCommandString(agentConfig, cmdOpts),
		continue_: m.commandBuilder.BuildContinueCommandString(agentConfig, cmdOpts),
	}
}

// launchResolveWorkspacePath resolves the effective workspace path for non-worktree executors.
// For worktree executors, workspace resolution is handled by the WorktreePreparer.
// For tasks without repositories, creates a workspace directory in ~/.kandev/quick-chat/.
// Returns workspacePath, mainRepoGitDir, worktreeID, worktreeBranch.
func (m *Manager) launchResolveWorkspacePath(ctx context.Context, req *LaunchRequest) (workspacePath, mainRepoGitDir, worktreeID, worktreeBranch string) {
	if req.UseWorktree {
		// Worktree executors: preparer handles worktree creation.
		// Return empty path; it will be populated from preparer results.
		return "", "", "", ""
	}
	workspacePath = req.WorkspacePath
	if req.RepositoryPath != "" && workspacePath == "" {
		workspacePath = req.RepositoryPath
	}
	// For tasks without repositories (e.g., quick chat), create a workspace in ~/.kandev/quick-chat/
	// These directories are cleaned up when the ephemeral task is deleted (see task service performTaskCleanup).
	if workspacePath == "" && req.SessionID != "" && m.dataDir != "" {
		quickChatDir := filepath.Join(m.dataDir, "quick-chat")
		if err := os.MkdirAll(quickChatDir, 0755); err != nil {
			m.logger.Warn("failed to create quick-chat directory, continuing without workspace",
				zap.String("session_id", req.SessionID),
				zap.String("quick_chat_dir", quickChatDir),
				zap.Error(err))
			return "", "", "", ""
		}
		// Validate SessionID doesn't contain path separators (security: prevent path traversal)
		if strings.ContainsAny(req.SessionID, `/\`) {
			m.logger.Warn("session ID contains path separator, rejecting",
				zap.String("session_id", req.SessionID))
			return "", "", "", ""
		}
		// Use session ID as directory name for easy cleanup
		workspacePath = filepath.Join(quickChatDir, req.SessionID)
		if err := os.MkdirAll(workspacePath, 0755); err != nil {
			m.logger.Warn("failed to create session workspace, continuing without workspace",
				zap.String("session_id", req.SessionID),
				zap.String("workspace_path", workspacePath),
				zap.Error(err))
			return "", "", "", ""
		}

		// Initialize git repository in the workspace (if not already initialized)
		if err := m.initGitRepo(ctx, workspacePath); err != nil {
			m.logger.Warn("failed to initialize git repository in quick chat workspace",
				zap.String("session_id", req.SessionID),
				zap.String("workspace_path", workspacePath),
				zap.Error(err))
			// Continue anyway - git is optional for quick chat
		}

		m.logger.Info("created quick chat workspace",
			zap.String("session_id", req.SessionID),
			zap.String("workspace_path", workspacePath))
	}
	return
}

// launchPrepareRequest copies the launch request, sets the resolved workspace path,
// populates metadata from the request fields, and injects profile environment variables.
func (m *Manager) launchPrepareRequest(req *LaunchRequest, profileInfo *AgentProfileInfo, workspacePath string) (LaunchRequest, string) {
	executionID := uuid.New().String()
	reqWithWorktree := *req
	reqWithWorktree.WorkspacePath = workspacePath

	if reqWithWorktree.Metadata == nil {
		reqWithWorktree.Metadata = make(map[string]interface{})
	}
	if req.TaskDescription != "" {
		reqWithWorktree.Metadata["task_description"] = req.TaskDescription
	}
	if req.SessionID != "" {
		reqWithWorktree.Metadata["session_id"] = req.SessionID
	}

	if profileInfo != nil {
		if reqWithWorktree.Env == nil {
			reqWithWorktree.Env = make(map[string]string)
		}
		if profileInfo.Model != "" {
			reqWithWorktree.Env["AGENT_MODEL"] = profileInfo.Model
		}
		if profileInfo.AutoApprove {
			reqWithWorktree.Env["AGENTCTL_AUTO_APPROVE_PERMISSIONS"] = "true"
		}
	}
	return reqWithWorktree, executionID
}

// newProgressCallback builds a PrepareProgressCallback that publishes progress events for a task/session.
func (m *Manager) newProgressCallback(taskID, sessionID string) PrepareProgressCallback {
	return func(step PrepareStep, stepIndex int, totalSteps int) {
		m.eventPublisher.PublishPrepareProgress(sessionID, &PrepareProgressEventPayload{
			TaskID:        taskID,
			SessionID:     sessionID,
			StepName:      step.Name,
			StepIndex:     stepIndex,
			TotalSteps:    totalSteps,
			Status:        string(step.Status),
			Output:        step.Output,
			Error:         step.Error,
			Warning:       step.Warning,
			WarningDetail: step.WarningDetail,
		})
	}
}

// launchBuildExecutorRequest resolves MCP servers, builds the ExecutorCreateRequest,
// and creates the runtime instance.
func (m *Manager) launchBuildExecutorRequest(ctx context.Context, executionID string, reqWithWorktree *LaunchRequest, agentConfig agents.Agent, mainRepoGitDir, worktreeID, worktreeBranch string) (*ExecutorCreateRequest, *ExecutorInstance, ExecutorBackend, error) {
	rt, err := m.getExecutorBackend(reqWithWorktree.ExecutorType)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("no runtime configured: %w", err)
	}

	env := m.buildEnvForExecution(executionID, reqWithWorktree, agentConfig)

	acpMcpServers, err := m.resolveMcpServersWithParams(ctx, reqWithWorktree.AgentProfileID, reqWithWorktree.Metadata, agentConfig)
	if err != nil {
		m.logger.Warn("failed to resolve MCP servers for launch", zap.Error(err))
	}

	var mcpServers []McpServerConfig
	for _, srv := range acpMcpServers {
		mcpServers = append(mcpServers, McpServerConfig{
			Name:    srv.Name,
			URL:     srv.URL,
			Type:    srv.Type,
			Command: srv.Command,
			Args:    srv.Args,
			Env:     srv.Env,
			Headers: srv.Headers,
		})
	}

	metadata := buildLaunchMetadata(reqWithWorktree, mainRepoGitDir, worktreeID, worktreeBranch)

	execReq := &ExecutorCreateRequest{
		InstanceID:          executionID,
		TaskID:              reqWithWorktree.TaskID,
		SessionID:           reqWithWorktree.SessionID,
		AgentProfileID:      reqWithWorktree.AgentProfileID,
		WorkspacePath:       reqWithWorktree.WorkspacePath,
		Protocol:            string(agentConfig.Runtime().Protocol),
		Env:                 env,
		Metadata:            metadata,
		AgentConfig:         agentConfig,
		McpServers:          mcpServers,
		PreviousExecutionID: reqWithWorktree.PreviousExecutionID,
		McpMode:             reqWithWorktree.McpMode,
		OnProgress:          m.newProgressCallback(reqWithWorktree.TaskID, reqWithWorktree.SessionID),
	}

	if resumer, ok := rt.(RemoteSessionResumer); ok {
		if err := resumer.ResumeRemoteInstance(ctx, execReq); err != nil {
			return nil, nil, nil, fmt.Errorf("failed remote resume preflight: %w", err)
		}
	}

	execInstance, err := rt.CreateInstance(ctx, execReq)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create execution: %w", err)
	}
	return execReq, execInstance, rt, nil
}

// runEnvironmentPreparer runs the environment preparer for the executor type, if one is registered.
// Returns the prepare result (nil if no preparer ran). Does NOT publish PrepareCompleted;
// the caller is responsible for publishing based on the returned result.
func (m *Manager) runEnvironmentPreparer(
	ctx context.Context,
	req *LaunchRequest,
	workspacePath string,
) *EnvPrepareResult {
	if m.preparerRegistry == nil {
		return nil
	}
	// Look up preparer by the raw executor type first (e.g. "worktree"),
	// then fall back to the backend name (e.g. "standalone").
	// This allows executor types that share a backend (local and worktree
	// both map to standalone) to have distinct preparation logic.
	execName := executor.Name(req.ExecutorType)
	preparer := m.preparerRegistry.Get(execName)
	if preparer == nil {
		execName = executor.ExecutorTypeToBackend(models.ExecutorType(req.ExecutorType))
		preparer = m.preparerRegistry.Get(execName)
	}
	if preparer == nil {
		return nil
	}

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

	prepReq := &EnvPrepareRequest{
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		TaskTitle:            req.TaskTitle,
		ExecutorType:         execName,
		WorkspacePath:        workspacePath,
		RepositoryPath:       req.RepositoryPath,
		RepositoryID:         req.RepositoryID,
		UseWorktree:          req.UseWorktree,
		SetupScript:          req.SetupScript,
		BaseBranch:           req.BaseBranch,
		CheckoutBranch:       req.CheckoutBranch,
		WorktreeBranchPrefix: req.WorktreeBranchPrefix,
		PullBeforeWorktree:   req.PullBeforeWorktree,
		Env:                  req.Env,
	}

	result, err := preparer.Prepare(ctx, prepReq, m.newProgressCallback(req.TaskID, req.SessionID))
	if err != nil {
		m.logger.Warn("environment preparation failed",
			zap.String("task_id", req.TaskID),
			zap.String("preparer", preparer.Name()),
			zap.Error(err))
		return &EnvPrepareResult{
			Success:      false,
			ErrorMessage: err.Error(),
		}
	}

	return result
}

// launchApplyPrepareResult applies workspace metadata from the preparer result and publishes completion.
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
	m.eventPublisher.PublishPrepareCompleted(req.SessionID, &PrepareCompletedEventPayload{
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		Success:       true,
		DurationMs:    result.Duration.Milliseconds(),
		WorkspacePath: result.WorkspacePath,
		Steps:         result.Steps,
	})
	return nil
}

// Launch launches a new agent for a task
func (m *Manager) Launch(ctx context.Context, req *LaunchRequest) (*AgentExecution, error) {
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

	// 3. Check if session already has an agent running
	if req.SessionID != "" {
		if existingExecution, exists := m.executionStore.GetBySessionID(req.SessionID); exists {
			return nil, fmt.Errorf("session %q already has an agent running (execution: %s)", req.SessionID, existingExecution.ID)
		}
	}

	// 4. Resolve workspace path (non-worktree executors use this directly)
	workspacePath, mainRepoGitDir, worktreeID, worktreeBranch := m.launchResolveWorkspacePath(ctx, req)

	// 4b. Run environment preparation (if preparer registered for this executor type).
	// For worktree executors, the preparer creates the worktree and returns workspace metadata.
	prepResult := m.runEnvironmentPreparer(ctx, req, workspacePath)
	if prepResult != nil {
		if err := m.launchApplyPrepareResult(req, prepResult, &workspacePath, &mainRepoGitDir, &worktreeID, &worktreeBranch); err != nil {
			return nil, err
		}
	}

	// 5 & 6. Prepare the request copy with metadata and profile env
	reqWithWorktree, executionID := m.launchPrepareRequest(req, profileInfo, workspacePath)

	// 7. Build runtime request and create instance (agent not started yet)
	execReq, execInstance, rt, err := m.launchBuildExecutorRequest(ctx, executionID, &reqWithWorktree, agentConfig, mainRepoGitDir, worktreeID, worktreeBranch)
	if err != nil {
		m.eventPublisher.PublishPrepareCompleted(req.SessionID, &PrepareCompletedEventPayload{
			TaskID:       req.TaskID,
			SessionID:    req.SessionID,
			Success:      false,
			ErrorMessage: err.Error(),
		})
		return nil, err
	}
	// Publish PrepareCompleted for CreateInstance only if no preparer ran
	// (preparers publish their own completion above).
	if prepResult == nil {
		m.eventPublisher.PublishPrepareCompleted(req.SessionID, &PrepareCompletedEventPayload{
			TaskID:     req.TaskID,
			SessionID:  req.SessionID,
			Success:    true,
			DurationMs: 0,
		})
	}

	// Convert to AgentExecution and set the runtime name
	execution := execInstance.ToAgentExecution(execReq)
	execution.RuntimeName = string(rt.Name())

	if req.ACPSessionID != "" {
		execution.ACPSessionID = req.ACPSessionID
	}
	if req.PreviousExecutionID != "" {
		execution.isResumedSession = true
	}
	cmds := m.buildAgentCommand(req, profileInfo, agentConfig)
	execution.AgentCommand = cmds.initial
	execution.ContinueCommand = cmds.continue_

	// 8. Track the execution
	m.executionStore.Add(execution)
	go m.pollOneRemoteStatus(context.Background(), execution)

	// 9. Publish agent.started event
	m.eventPublisher.PublishAgentEvent(ctx, events.AgentStarted, execution)
	m.eventPublisher.PublishAgentctlEvent(ctx, events.AgentctlStarting, execution, "")

	// 10. Wait for agentctl to be ready (for shell/workspace access)
	// NOTE: This does NOT start the agent process - call StartAgentProcess() explicitly
	go m.waitForAgentctlReady(execution)

	m.logger.Debug("agentctl execution created (agent not started)",
		zap.String("execution_id", executionID),
		zap.String("task_id", req.TaskID),
		zap.String("runtime", execution.RuntimeName))

	return execution, nil
}

// SetExecutionDescription updates the task description stored in an execution's metadata.
// This is used when starting an agent on a workspace that was launched without a prompt.
func (m *Manager) SetExecutionDescription(_ context.Context, executionID string, description string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	if execution.Metadata == nil {
		execution.Metadata = make(map[string]interface{})
	}
	execution.Metadata["task_description"] = description
	return nil
}

// SetMcpMode changes the MCP tool mode on an existing execution's agentctl instance.
// This is used when a session transitions to plan/config mode after the workspace was
// already prepared with the default (task) mode.
func (m *Manager) SetMcpMode(ctx context.Context, executionID string, mode string) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return fmt.Errorf("execution %q not found", executionID)
	}
	if execution.agentctl == nil {
		return fmt.Errorf("execution %q has no agentctl client", executionID)
	}
	return execution.agentctl.SetMcpMode(ctx, mode)
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
// Returns the message and stop channel (both nil if bootMessageService is not configured).
func (m *Manager) createBootMessage(ctx context.Context, execution *AgentExecution, bootCommand, agentDisplayName string) (*models.Message, chan struct{}) {
	if m.bootMessageService == nil {
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
	go m.pollAgentStderr(execution, execution.agentctl, bootMsg, bootStopCh)
	return bootMsg, bootStopCh
}

// getTaskDescriptionFromMetadata extracts the task description string from execution metadata.
func getTaskDescriptionFromMetadata(execution *AgentExecution) string {
	if execution.Metadata == nil {
		return ""
	}
	if desc, ok := execution.Metadata["task_description"].(string); ok {
		return desc
	}
	return ""
}

// configureAndStartAgent configures the agent command and starts the agent subprocess.
// Returns the effective boot command (full command with adapter args, or base command).
func (m *Manager) configureAndStartAgent(ctx context.Context, execution *AgentExecution, taskDescription, approvalPolicy string) (string, error) {
	env := map[string]string{}
	if taskDescription != "" {
		env["TASK_DESCRIPTION"] = taskDescription
	}

	if err := execution.agentctl.ConfigureAgent(ctx, execution.AgentCommand, env, approvalPolicy, execution.ContinueCommand); err != nil {
		return "", fmt.Errorf("failed to configure agent: %w", err)
	}

	fullCommand, err := execution.agentctl.Start(ctx)
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

// initializeAgentSession handles post-startup initialization: boot message, ACP session,
// MCP servers. It finalizes the boot message on success or failure.
func (m *Manager) initializeAgentSession(ctx context.Context, execution *AgentExecution, bootCommand, agentDisplayName, taskDescription string) error {
	bootMsg, bootStopCh := m.createBootMessage(ctx, execution, bootCommand, agentDisplayName)

	// Give the agent process a moment to initialize
	time.Sleep(500 * time.Millisecond)

	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		m.finalizeBootMessage(execution, bootMsg, bootStopCh, execution.agentctl, "failed")
		return fmt.Errorf("failed to get agent config: %w", err)
	}

	mcpServers, err := m.resolveMcpServers(ctx, execution, agentConfig)
	if err != nil {
		m.finalizeBootMessage(execution, bootMsg, bootStopCh, execution.agentctl, "failed")
		m.updateExecutionError(execution.ID, "failed to resolve MCP config: "+err.Error())
		return fmt.Errorf("failed to resolve MCP config: %w", err)
	}

	if err := m.initializeACPSession(ctx, execution, agentConfig, taskDescription, mcpServers); err != nil {
		m.finalizeBootMessage(execution, bootMsg, bootStopCh, execution.agentctl, "failed")
		m.updateExecutionError(execution.ID, "failed to initialize ACP: "+err.Error())
		return fmt.Errorf("failed to initialize ACP: %w", err)
	}

	m.finalizeBootMessage(execution, bootMsg, bootStopCh, execution.agentctl, containerStateExited)
	return nil
}

// initGitRepo initializes a git repository in the given directory.
// Creates an initial commit so the workspace has a clean git state.
// This function is idempotent - it skips initialization if .git already exists.
func (m *Manager) initGitRepo(ctx context.Context, workspacePath string) error {
	// Check if git repository already exists (idempotent)
	gitDir := filepath.Join(workspacePath, ".git")
	if info, err := os.Stat(gitDir); err == nil {
		if info.IsDir() {
			return nil // Already initialized
		}
	} else if !os.IsNotExist(err) {
		// Non-ENOENT error (permissions, I/O, etc.) - fail explicitly
		return fmt.Errorf("failed to check for .git directory: %w", err)
	}

	// Initialize git repository
	cmd := exec.CommandContext(ctx, "git", "init")
	cmd.Dir = workspacePath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init failed: %w (output: %s)", err, string(output))
	}

	// Configure git user (required for initial commit)
	configName := exec.CommandContext(ctx, "git", "config", "user.name", "Kandev Quick Chat")
	configName.Dir = workspacePath
	_ = configName.Run() // Ignore error - might already be configured globally

	configEmail := exec.CommandContext(ctx, "git", "config", "user.email", "quickchat@kandev.local")
	configEmail.Dir = workspacePath
	_ = configEmail.Run() // Ignore error - might already be configured globally

	// Create initial commit with empty .gitkeep file
	gitkeepPath := filepath.Join(workspacePath, ".gitkeep")
	if err := os.WriteFile(gitkeepPath, []byte(""), 0644); err != nil {
		return fmt.Errorf("failed to create .gitkeep: %w", err)
	}

	addCmd := exec.CommandContext(ctx, "git", "add", ".gitkeep")
	addCmd.Dir = workspacePath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %w (output: %s)", err, string(output))
	}

	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", "Initial commit")
	commitCmd.Dir = workspacePath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %w (output: %s)", err, string(output))
	}

	return nil
}
