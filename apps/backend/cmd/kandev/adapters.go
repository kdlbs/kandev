package main

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/pkg/api/v1"
)

// taskRepositoryAdapter adapts the task repository for the orchestrator's scheduler
type taskRepositoryAdapter struct {
	repo repository.Repository
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

// lifecycleAdapter adapts the lifecycle manager as an AgentManagerClient
type lifecycleAdapter struct {
	mgr      *lifecycle.Manager
	registry *registry.Registry
	logger   *logger.Logger
}

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
	// The RepositoryURL field contains a local filesystem path for the workspace
	// If empty, the agent will run without a mounted workspace
	launchReq := &lifecycle.LaunchRequest{
		TaskID:          req.TaskID,
		SessionID:       req.SessionID,
		TaskTitle:       req.TaskTitle,
		AgentProfileID:  req.AgentProfileID,
		WorkspacePath:   req.RepositoryURL, // May be empty - lifecycle manager handles this
		TaskDescription: req.TaskDescription,
		Env:             req.Env,
		ACPSessionID:    req.ACPSessionID,
		Metadata:        req.Metadata,
		ModelOverride:   req.ModelOverride,
		ExecutorType:    req.ExecutorType,
		// Worktree configuration for concurrent agent execution
		UseWorktree:          req.UseWorktree,
		RepositoryID:         req.RepositoryID,
		RepositoryPath:       req.RepositoryPath,
		BaseBranch:           req.BaseBranch,
		WorktreeBranchPrefix: req.WorktreeBranchPrefix,
		PullBeforeWorktree:   req.PullBeforeWorktree,
	}

	// Create the agentctl execution (does NOT start agent process)
	execution, err := a.mgr.Launch(ctx, launchReq)
	if err != nil {
		return nil, err
	}

	// Extract worktree info from metadata if available
	var worktreeID, worktreePath, worktreeBranch string
	if execution.Metadata != nil {
		if id, ok := execution.Metadata["worktree_id"].(string); ok {
			worktreeID = id
		}
		if path, ok := execution.Metadata["worktree_path"].(string); ok {
			worktreePath = path
		}
		if branch, ok := execution.Metadata["worktree_branch"].(string); ok {
			worktreeBranch = branch
		}
	}

	return &executor.LaunchAgentResponse{
		AgentExecutionID: execution.ID,
		ContainerID:      execution.ContainerID,
		Status:           execution.Status,
		WorktreeID:       worktreeID,
		WorktreePath:     worktreePath,
		WorktreeBranch:   worktreeBranch,
	}, nil
}

// StartAgentProcess starts the agent subprocess for an instance.
// The command is built internally based on the instance's agent profile.
func (a *lifecycleAdapter) StartAgentProcess(ctx context.Context, agentInstanceID string) error {
	return a.mgr.StartAgentProcess(ctx, agentInstanceID)
}

// StopAgent stops a running agent
func (a *lifecycleAdapter) StopAgent(ctx context.Context, agentInstanceID string, force bool) error {
	return a.mgr.StopAgent(ctx, agentInstanceID, force)
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
		result = append(result, cfg.ToAPIType())
	}
	return result, nil
}

// PromptAgent sends a follow-up prompt to a running agent
// Attachments (images) are passed to the agent if provided
func (a *lifecycleAdapter) PromptAgent(ctx context.Context, agentInstanceID string, prompt string, attachments []v1.MessageAttachment) (*executor.PromptResult, error) {
	result, err := a.mgr.PromptAgent(ctx, agentInstanceID, prompt, attachments)
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

// RespondToPermissionBySessionID sends a response to a permission request for a session
func (a *lifecycleAdapter) RespondToPermissionBySessionID(ctx context.Context, sessionID, pendingID, optionID string, cancelled bool) error {
	return a.mgr.RespondToPermissionBySessionID(sessionID, pendingID, optionID, cancelled)
}

// IsAgentRunningForSession checks if an agent is actually running for a session
// This probes the actual agent (Docker container or standalone process)
func (a *lifecycleAdapter) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	return a.mgr.IsAgentRunningForSession(ctx, sessionID)
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
		AutoApprove:                info.AutoApprove,
		DangerouslySkipPermissions: info.DangerouslySkipPermissions,
		CLIPassthrough:             info.CLIPassthrough,
	}, nil
}

// orchestratorWrapper wraps orchestrator.Service to implement taskhandlers.OrchestratorService.
// The wrapper only adapts ResumeTaskSession which returns *executor.TaskExecution that we don't need.
type orchestratorWrapper struct {
	svc *orchestrator.Service
}

// PromptTask forwards directly to the orchestrator service.
// Attachments (images) are passed through to the agent.
func (w *orchestratorWrapper) PromptTask(ctx context.Context, taskID, taskSessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment) (*orchestrator.PromptResult, error) {
	return w.svc.PromptTask(ctx, taskID, taskSessionID, prompt, model, planMode, attachments)
}

// ResumeTaskSession forwards to the orchestrator service, discarding the TaskExecution result.
func (w *orchestratorWrapper) ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error {
	_, err := w.svc.ResumeTaskSession(ctx, taskID, taskSessionID)
	return err
}

// messageCreatorAdapter adapts the task service to the orchestrator.MessageCreator interface
type messageCreatorAdapter struct {
	svc    *taskservice.Service
	logger *logger.Logger
}

// CreateAgentMessage creates a message with author_type="agent"
func (a *messageCreatorAdapter) CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID, turnID string) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
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
func (a *messageCreatorAdapter) CreatePermissionRequestMessage(ctx context.Context, taskID, sessionID, pendingID, toolCallID, title, turnID string, options []map[string]interface{}, actionType string, actionDetails map[string]interface{}) (string, error) {
	metadata := map[string]interface{}{
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
func (a *messageCreatorAdapter) UpdatePermissionMessage(ctx context.Context, sessionID, pendingID, status string) error {
	return a.svc.UpdatePermissionMessage(ctx, sessionID, pendingID, status)
}

// CreateClarificationRequestMessage creates a message for a clarification request.
// This allows clarification requests to appear in the chat as messages.
func (a *messageCreatorAdapter) CreateClarificationRequestMessage(ctx context.Context, taskID, sessionID, pendingID string, question clarification.Question, clarificationContext string) (string, error) {
	// Convert question options to interface{} for metadata storage
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
		"pending_id": pendingID,
		"question":   questionData,
		"context":    clarificationContext,
		"status":     "pending",
	}

	msg, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		Content:       question.Prompt,
		AuthorType:    "agent",
		Type:          "clarification_request",
		Metadata:      metadata,
		RequestsInput: true, // This marks the session as waiting for input
	})
	if err != nil {
		return "", err
	}
	return msg.ID, nil
}

// UpdateClarificationMessage updates a clarification message's status and response
func (a *messageCreatorAdapter) UpdateClarificationMessage(ctx context.Context, sessionID, pendingID, status string, answer *clarification.Answer) error {
	return a.svc.UpdateClarificationMessage(ctx, sessionID, pendingID, status, answer)
}

// CreateAgentMessageStreaming creates a new agent message with a pre-generated ID.
// This is used for real-time streaming where content arrives incrementally.
func (a *messageCreatorAdapter) CreateAgentMessageStreaming(ctx context.Context, messageID, taskID, content, agentSessionID, turnID string) error {
	_, err := a.svc.CreateMessageWithID(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "agent",
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
	_, err := a.svc.CreateMessageWithID(ctx, messageID, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       "",
		AuthorType:    "agent",
		Type:          "thinking",
		Metadata: map[string]interface{}{
			"thinking": content,
		},
	})
	return err
}

// AppendThinkingMessage appends additional content to an existing streaming thinking message.
func (a *messageCreatorAdapter) AppendThinkingMessage(ctx context.Context, messageID, additionalContent string) error {
	return a.svc.AppendThinkingContent(ctx, messageID, additionalContent)
}

// turnServiceAdapter adapts the task service to the orchestrator.TurnService interface
type turnServiceAdapter struct {
	svc *taskservice.Service
}

func (a *turnServiceAdapter) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.StartTurn(ctx, sessionID)
}

func (a *turnServiceAdapter) CompleteTurn(ctx context.Context, turnID string) error {
	return a.svc.CompleteTurn(ctx, turnID)
}

func (a *turnServiceAdapter) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	return a.svc.GetActiveTurn(ctx, sessionID)
}

func newTurnServiceAdapter(svc *taskservice.Service) *turnServiceAdapter {
	return &turnServiceAdapter{svc: svc}
}

// worktreeRecreatorAdapter adapts the worktree.Recreator to the orchestrator.WorktreeRecreator interface.
type worktreeRecreatorAdapter struct {
	recreator *worktree.Recreator
}

func newWorktreeRecreatorAdapter(recreator *worktree.Recreator) *worktreeRecreatorAdapter {
	return &worktreeRecreatorAdapter{recreator: recreator}
}

// RecreateWorktree implements orchestrator.WorktreeRecreator.
func (a *worktreeRecreatorAdapter) RecreateWorktree(ctx context.Context, req orchestrator.WorktreeRecreateRequest) (*orchestrator.WorktreeRecreateResult, error) {
	result, err := a.recreator.Recreate(ctx, worktree.RecreateRequest{
		SessionID:    req.SessionID,
		TaskID:       req.TaskID,
		TaskTitle:    req.TaskTitle,
		RepositoryID: req.RepositoryID,
		BaseBranch:   req.BaseBranch,
		WorktreeID:   req.WorktreeID,
	})
	if err != nil {
		return nil, err
	}
	return &orchestrator.WorktreeRecreateResult{
		WorktreePath:   result.WorktreePath,
		WorktreeBranch: result.WorktreeBranch,
	}, nil
}
