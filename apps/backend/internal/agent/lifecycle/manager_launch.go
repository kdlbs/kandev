package lifecycle

import (
	"context"
	"fmt"
	"path/filepath"
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
	cmdOpts := agents.CommandOptions{
		Model:            model,
		SessionID:        req.ACPSessionID,
		AutoApprove:      autoApprove,
		PermissionValues: permissionValues,
	}
	return agentCommands{
		initial:   m.commandBuilder.BuildCommandString(agentConfig, cmdOpts),
		continue_: m.commandBuilder.BuildContinueCommandString(agentConfig, cmdOpts),
	}
}

// launchResolveWorkspacePath resolves the effective workspace path, handling worktree
// creation if requested. Returns workspacePath, mainRepoGitDir, worktreeID, worktreeBranch.
func (m *Manager) launchResolveWorkspacePath(ctx context.Context, req *LaunchRequest) (workspacePath, mainRepoGitDir, worktreeID, worktreeBranch string) {
	workspacePath = req.WorkspacePath
	if req.UseWorktree && m.worktreeMgr != nil && req.RepositoryPath != "" {
		wt, err := m.getOrCreateWorktree(ctx, req)
		if err != nil {
			m.logger.Warn("failed to create worktree, falling back to direct mount",
				zap.String("task_id", req.TaskID),
				zap.Error(err))
			// Fall back to direct mount if worktree creation fails
			workspacePath = req.RepositoryPath
		} else {
			workspacePath = wt.Path
			worktreeID = wt.ID
			worktreeBranch = wt.Branch
			// Git worktrees reference the main repo's .git directory via a .git file.
			// We need to mount the entire .git directory for git commands to work.
			mainRepoGitDir = filepath.Join(req.RepositoryPath, ".git")
		}
	} else if req.RepositoryPath != "" && workspacePath == "" {
		workspacePath = req.RepositoryPath
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
		})
	}

	metadata := buildLaunchMetadata(reqWithWorktree, mainRepoGitDir, worktreeID, worktreeBranch)

	execReq := &ExecutorCreateRequest{
		InstanceID:     executionID,
		TaskID:         reqWithWorktree.TaskID,
		SessionID:      reqWithWorktree.SessionID,
		AgentProfileID: reqWithWorktree.AgentProfileID,
		WorkspacePath:  reqWithWorktree.WorkspacePath,
		Protocol:       string(agentConfig.Runtime().Protocol),
		Env:            env,
		Metadata:       metadata,
		AgentConfig:    agentConfig,
		McpServers:     mcpServers,
		OnProgress: func(step PrepareStep, stepIndex int, totalSteps int) {
			m.eventPublisher.PublishPrepareProgress(reqWithWorktree.SessionID, &PrepareProgressEventPayload{
				TaskID:     reqWithWorktree.TaskID,
				SessionID:  reqWithWorktree.SessionID,
				StepName:   step.Name,
				StepIndex:  stepIndex,
				TotalSteps: totalSteps,
				Status:     string(step.Status),
				Output:     step.Output,
				Error:      step.Error,
			})
		},
	}

	execInstance, err := rt.CreateInstance(ctx, execReq)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create execution: %w", err)
	}
	return execReq, execInstance, rt, nil
}

// runEnvironmentPreparer runs the environment preparer for the executor type, if one is registered.
// Preparation failures are non-fatal and logged as warnings.
func (m *Manager) runEnvironmentPreparer(ctx context.Context, req *LaunchRequest, workspacePath string) {
	if m.preparerRegistry == nil {
		return
	}
	execName := executor.ExecutorTypeToBackend(models.ExecutorType(req.ExecutorType))
	preparer := m.preparerRegistry.Get(execName)
	if preparer == nil {
		return
	}

	prepReq := &EnvPrepareRequest{
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		ExecutorType:   execName,
		WorkspacePath:  workspacePath,
		RepositoryPath: req.RepositoryPath,
		UseWorktree:    req.UseWorktree,
		SetupScript:    req.SetupScript,
		Env:            req.Env,
	}

	onProgress := func(step PrepareStep, stepIndex int, totalSteps int) {
		m.eventPublisher.PublishPrepareProgress(req.SessionID, &PrepareProgressEventPayload{
			TaskID:     req.TaskID,
			SessionID:  req.SessionID,
			StepName:   step.Name,
			StepIndex:  stepIndex,
			TotalSteps: totalSteps,
			Status:     string(step.Status),
			Output:     step.Output,
			Error:      step.Error,
		})
	}

	result, err := preparer.Prepare(ctx, prepReq, onProgress)
	if err != nil {
		m.logger.Warn("environment preparation failed",
			zap.String("task_id", req.TaskID),
			zap.String("preparer", preparer.Name()),
			zap.Error(err))
		m.eventPublisher.PublishPrepareCompleted(req.SessionID, &PrepareCompletedEventPayload{
			TaskID:       req.TaskID,
			SessionID:    req.SessionID,
			Success:      false,
			ErrorMessage: err.Error(),
		})
		return
	}

	m.eventPublisher.PublishPrepareCompleted(req.SessionID, &PrepareCompletedEventPayload{
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		Success:       result.Success,
		ErrorMessage:  result.ErrorMessage,
		DurationMs:    result.Duration.Milliseconds(),
		WorkspacePath: result.WorkspacePath,
	})
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

	// 4. Resolve workspace path (with optional worktree creation)
	workspacePath, mainRepoGitDir, worktreeID, worktreeBranch := m.launchResolveWorkspacePath(ctx, req)

	// 4b. Run environment preparation (if preparer registered for this executor type)
	m.runEnvironmentPreparer(ctx, req, workspacePath)

	// 5 & 6. Prepare the request copy with metadata and profile env
	reqWithWorktree, executionID := m.launchPrepareRequest(req, profileInfo, workspacePath)

	// 7. Build runtime request and create instance (agent not started yet)
	execReq, execInstance, rt, err := m.launchBuildExecutorRequest(ctx, executionID, &reqWithWorktree, agentConfig, mainRepoGitDir, worktreeID, worktreeBranch)
	if err != nil {
		return nil, err
	}

	// Convert to AgentExecution and set the runtime name
	execution := execInstance.ToAgentExecution(execReq)
	execution.RuntimeName = string(rt.Name())

	if req.ACPSessionID != "" {
		execution.ACPSessionID = req.ACPSessionID
	}
	cmds := m.buildAgentCommand(req, profileInfo, agentConfig)
	execution.AgentCommand = cmds.initial
	execution.ContinueCommand = cmds.continue_

	// 8. Track the execution
	m.executionStore.Add(execution)

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
