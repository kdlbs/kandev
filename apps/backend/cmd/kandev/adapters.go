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
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
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
func (a *lifecycleAdapter) PromptAgent(ctx context.Context, agentInstanceID string, prompt string) (*executor.PromptResult, error) {
	result, err := a.mgr.PromptAgent(ctx, agentInstanceID, prompt)
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
		Plan:                       info.Plan,
		CLIPassthrough:             info.CLIPassthrough,
	}, nil
}

// orchestratorAdapter adapts the orchestrator.Service to the taskhandlers.OrchestratorService interface
type orchestratorAdapter struct {
	svc *orchestrator.Service
}

// PromptTask forwards to the orchestrator service and converts the result type
func (a *orchestratorAdapter) PromptTask(ctx context.Context, taskID, taskSessionID, prompt, model string, planMode bool) (*taskhandlers.PromptResult, error) {
	result, err := a.svc.PromptTask(ctx, taskID, taskSessionID, prompt, model, planMode)
	if err != nil {
		return nil, err
	}
	return &taskhandlers.PromptResult{
		StopReason:   result.StopReason,
		AgentMessage: result.AgentMessage,
	}, nil
}

func (a *orchestratorAdapter) ResumeTaskSession(ctx context.Context, taskID, taskSessionID string) error {
	_, err := a.svc.ResumeTaskSession(ctx, taskID, taskSessionID)
	return err
}

// messageCreatorAdapter adapts the task service to the orchestrator.MessageCreator interface
type messageCreatorAdapter struct {
	svc *taskservice.Service
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
func (a *messageCreatorAdapter) CreateUserMessage(ctx context.Context, taskID, content, agentSessionID, turnID string) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       content,
		AuthorType:    "user",
	})
	return err
}

// CreateToolCallMessage creates a message for a tool call
func (a *messageCreatorAdapter) CreateToolCallMessage(ctx context.Context, taskID, toolCallID, title, status, agentSessionID, turnID string, normalized *streams.NormalizedPayload) error {
	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}
	// Add normalized tool data to metadata for frontend consumption
	if normalized != nil {
		metadata["normalized"] = normalized
	}

	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		TurnID:        turnID,
		Content:       title,
		AuthorType:    "agent",
		Type:          "tool_call",
		Metadata:      metadata,
	})
	return err
}

// UpdateToolCallMessage updates a tool call message's status
func (a *messageCreatorAdapter) UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, status, result, agentSessionID, title string, normalized *streams.NormalizedPayload) error {
	return a.svc.UpdateToolCallMessage(ctx, agentSessionID, toolCallID, status, result, title, normalized)
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
