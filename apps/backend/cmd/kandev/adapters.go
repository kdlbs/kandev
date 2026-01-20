package main

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
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
		// Worktree configuration for concurrent agent execution
		UseWorktree:          req.UseWorktree,
		RepositoryID:         req.RepositoryID,
		RepositoryPath:       req.RepositoryPath,
		BaseBranch:           req.BaseBranch,
		WorktreeBranchPrefix: req.WorktreeBranchPrefix,
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

// GetRecoveredExecutions returns executions recovered from Docker during startup
func (a *lifecycleAdapter) GetRecoveredExecutions() []executor.RecoveredExecutionInfo {
	recovered := a.mgr.GetRecoveredExecutions()
	result := make([]executor.RecoveredExecutionInfo, len(recovered))
	for i, r := range recovered {
		result[i] = executor.RecoveredExecutionInfo{
			ExecutionID:    r.ExecutionID,
			TaskID:         r.TaskID,
			SessionID:      r.SessionID,
			ContainerID:    r.ContainerID,
			AgentProfileID: r.AgentProfileID,
		}
	}
	return result
}

// IsAgentRunningForSession checks if an agent is actually running for a session
// This probes the actual agent (Docker container or standalone process)
func (a *lifecycleAdapter) IsAgentRunningForSession(ctx context.Context, sessionID string) bool {
	return a.mgr.IsAgentRunningForSession(ctx, sessionID)
}

// CleanupStaleExecutionBySessionID removes a stale agent execution from tracking without trying to stop it.
func (a *lifecycleAdapter) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	return a.mgr.CleanupStaleExecutionBySessionID(ctx, sessionID)
}

// orchestratorAdapter adapts the orchestrator.Service to the taskhandlers.OrchestratorService interface
type orchestratorAdapter struct {
	svc *orchestrator.Service
}

// PromptTask forwards to the orchestrator service and converts the result type
func (a *orchestratorAdapter) PromptTask(ctx context.Context, taskID, taskSessionID, prompt string) (*taskhandlers.PromptResult, error) {
	result, err := a.svc.PromptTask(ctx, taskID, taskSessionID, prompt)
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
func (a *messageCreatorAdapter) CreateAgentMessage(ctx context.Context, taskID, content, agentSessionID string) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		Content:       content,
		AuthorType:    "agent",
	})
	return err
}

// CreateUserMessage creates a message with author_type="user"
func (a *messageCreatorAdapter) CreateUserMessage(ctx context.Context, taskID, content, agentSessionID string) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		Content:       content,
		AuthorType:    "user",
	})
	return err
}

// CreateToolCallMessage creates a message for a tool call with type="tool_call"
func (a *messageCreatorAdapter) CreateToolCallMessage(ctx context.Context, taskID, toolCallID, title, status, agentSessionID string, args map[string]interface{}) error {
	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"title":        title,
		"status":       status,
	}

	if len(args) > 0 {
		metadata["args"] = args
		if kind, ok := args["kind"].(string); ok && kind != "" {
			metadata["tool_name"] = kind
		}
	}

	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		Content:       title,
		AuthorType:    "agent",
		Type:          "tool_call",
		Metadata:      metadata,
	})
	return err
}

// UpdateToolCallMessage updates a tool call message's status
func (a *messageCreatorAdapter) UpdateToolCallMessage(ctx context.Context, taskID, toolCallID, status, result, agentSessionID string) error {
	return a.svc.UpdateToolCallMessage(ctx, agentSessionID, toolCallID, status, result)
}

// CreateSessionMessage creates a message for non-chat session updates (status/progress/error/etc).
func (a *messageCreatorAdapter) CreateSessionMessage(ctx context.Context, taskID, content, agentSessionID, messageType string, metadata map[string]interface{}, requestsInput bool) error {
	_, err := a.svc.CreateMessage(ctx, &taskservice.CreateMessageRequest{
		TaskSessionID: agentSessionID,
		TaskID:        taskID,
		Content:       content,
		AuthorType:    "agent",
		Type:          messageType,
		Metadata:      metadata,
		RequestsInput: requestsInput,
	})
	return err
}

// CreatePermissionRequestMessage creates a message for a permission request
func (a *messageCreatorAdapter) CreatePermissionRequestMessage(ctx context.Context, taskID, sessionID, pendingID, toolCallID, title string, options []map[string]interface{}, actionType string, actionDetails map[string]interface{}) (string, error) {
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
