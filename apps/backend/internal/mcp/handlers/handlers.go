// Package handlers provides WebSocket handlers for MCP tool requests.
// These handlers are called by agentctl via the WS tunnel and execute
// operations against the backend services directly.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	workflowsvc "github.com/kandev/kandev/internal/workflow/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ClarificationService defines the interface for clarification operations.
type ClarificationService interface {
	CreateRequest(req *clarification.Request) string
	WaitForResponse(ctx context.Context, pendingID string) (*clarification.Response, error)
	CancelSession(sessionID string) []string
}

// MessageCreator creates messages for clarification requests.
type MessageCreator interface {
	CreateClarificationRequestMessage(ctx context.Context, taskID, sessionID, pendingID string, question clarification.Question, clarificationContext string) (string, error)
}

// SessionRepository interface for updating session state.
type SessionRepository interface {
	UpdateTaskSessionState(ctx context.Context, sessionID string, state models.TaskSessionState, errorMessage string) error
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
}

// TaskRepository interface for updating task state.
type TaskRepository interface {
	UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error
}

// EventBus interface for publishing events.
type EventBus interface {
	Publish(ctx context.Context, topic string, event *bus.Event) error
}

// SessionLauncher provides session launch capability for auto-starting tasks.
type SessionLauncher interface {
	LaunchSession(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error)
}

// Handlers provides MCP WebSocket handlers.
type Handlers struct {
	taskSvc          *service.Service
	workflowCtrl     *workflowctrl.Controller
	clarificationSvc ClarificationService
	messageCreator   MessageCreator
	sessionRepo      SessionRepository
	taskRepo         TaskRepository
	eventBus         EventBus
	planService      *service.PlanService
	sessionLauncher  SessionLauncher
	logger           *logger.Logger

	// Config-mode dependencies (optional, set via SetConfigDeps)
	workflowSvc       *workflowsvc.Service
	agentSettingsCtrl *agentsettingscontroller.Controller
	mcpConfigSvc      *mcpconfig.Service
}

// NewHandlers creates new MCP handlers.
func NewHandlers(
	taskSvc *service.Service,
	workflowCtrl *workflowctrl.Controller,
	clarificationSvc ClarificationService,
	messageCreator MessageCreator,
	sessionRepo SessionRepository,
	taskRepo TaskRepository,
	eventBus EventBus,
	planService *service.PlanService,
	sessionLauncher SessionLauncher,
	log *logger.Logger,
) *Handlers {
	return &Handlers{
		taskSvc:          taskSvc,
		workflowCtrl:     workflowCtrl,
		clarificationSvc: clarificationSvc,
		messageCreator:   messageCreator,
		sessionRepo:      sessionRepo,
		taskRepo:         taskRepo,
		eventBus:         eventBus,
		planService:      planService,
		sessionLauncher:  sessionLauncher,
		logger:           log.WithFields(zap.String("component", "mcp-handlers")),
	}
}

// SetConfigDeps sets the config-mode dependencies for agent-native configuration handlers.
// These are optional and only needed when config-mode MCP sessions are used.
func (h *Handlers) SetConfigDeps(
	workflowSvc *workflowsvc.Service,
	agentSettingsCtrl *agentsettingscontroller.Controller,
	mcpConfigSvc *mcpconfig.Service,
) {
	h.workflowSvc = workflowSvc
	h.agentSettingsCtrl = agentSettingsCtrl
	h.mcpConfigSvc = mcpConfigSvc
}

// RegisterHandlers registers all MCP handlers with the dispatcher.
func (h *Handlers) RegisterHandlers(d *ws.Dispatcher) {
	// Task-mode handlers (always registered)
	d.RegisterFunc(ws.ActionMCPListWorkspaces, h.handleListWorkspaces)
	d.RegisterFunc(ws.ActionMCPListWorkflows, h.handleListWorkflows)
	d.RegisterFunc(ws.ActionMCPListWorkflowSteps, h.handleListWorkflowSteps)
	d.RegisterFunc(ws.ActionMCPListTasks, h.handleListTasks)
	d.RegisterFunc(ws.ActionMCPCreateTask, h.handleCreateTask)
	d.RegisterFunc(ws.ActionMCPUpdateTask, h.handleUpdateTask)
	d.RegisterFunc(ws.ActionMCPAskUserQuestion, h.handleAskUserQuestion)
	d.RegisterFunc(ws.ActionMCPCreateTaskPlan, h.handleCreateTaskPlan)
	d.RegisterFunc(ws.ActionMCPGetTaskPlan, h.handleGetTaskPlan)
	d.RegisterFunc(ws.ActionMCPUpdateTaskPlan, h.handleUpdateTaskPlan)
	d.RegisterFunc(ws.ActionMCPDeleteTaskPlan, h.handleDeleteTaskPlan)
	d.RegisterFunc(ws.ActionMCPClarificationTimeout, h.handleClarificationTimeout)
	count := 12

	// Config-mode handlers (registered when config deps are set)
	if h.workflowSvc != nil {
		d.RegisterFunc(ws.ActionMCPCreateWorkflow, h.handleCreateWorkflow)
		d.RegisterFunc(ws.ActionMCPUpdateWorkflow, h.handleUpdateWorkflow)
		d.RegisterFunc(ws.ActionMCPDeleteWorkflow, h.handleDeleteWorkflow)
		d.RegisterFunc(ws.ActionMCPCreateWorkflowStep, h.handleCreateWorkflowStep)
		d.RegisterFunc(ws.ActionMCPUpdateWorkflowStep, h.handleUpdateWorkflowStep)
		d.RegisterFunc(ws.ActionMCPDeleteWorkflowStep, h.handleDeleteWorkflowStep)
		d.RegisterFunc(ws.ActionMCPReorderWorkflowStep, h.handleReorderWorkflowSteps)
		count += 7
	}
	if h.agentSettingsCtrl != nil {
		d.RegisterFunc(ws.ActionMCPListAgents, h.handleListAgents)
		d.RegisterFunc(ws.ActionMCPUpdateAgent, h.handleUpdateAgent)
		d.RegisterFunc(ws.ActionMCPListAgentProfiles, h.handleListAgentProfiles)
		d.RegisterFunc(ws.ActionMCPCreateAgentProfile, h.handleCreateAgentProfile)
		d.RegisterFunc(ws.ActionMCPUpdateAgentProfile, h.handleUpdateAgentProfile)
		d.RegisterFunc(ws.ActionMCPDeleteAgentProfile, h.handleDeleteAgentProfile)
		count += 6
	}
	// list_executor_profiles is always available (read-only, used in task mode for create_task)
	if h.taskSvc != nil {
		d.RegisterFunc(ws.ActionMCPListExecutorProfiles, h.handleListExecutorProfiles)
		count++
	}
	if h.mcpConfigSvc != nil {
		d.RegisterFunc(ws.ActionMCPGetMcpConfig, h.handleGetMcpConfig)
		d.RegisterFunc(ws.ActionMCPUpdateMcpConfig, h.handleUpdateMcpConfig)
		count += 2
	}
	if h.taskSvc != nil {
		d.RegisterFunc(ws.ActionMCPMoveTask, h.handleMoveTask)
		d.RegisterFunc(ws.ActionMCPDeleteTask, h.handleDeleteTask)
		d.RegisterFunc(ws.ActionMCPArchiveTask, h.handleArchiveTask)
		d.RegisterFunc(ws.ActionMCPUpdateTaskState, h.handleUpdateTaskState)
		count += 4

		// Executor mutation handlers (config-mode only)
		if h.workflowSvc != nil {
			d.RegisterFunc(ws.ActionMCPCreateExecutorProfile, h.handleCreateExecutorProfile)
			d.RegisterFunc(ws.ActionMCPUpdateExecutorProfile, h.handleUpdateExecutorProfile)
			d.RegisterFunc(ws.ActionMCPDeleteExecutorProfile, h.handleDeleteExecutorProfile)
			count += 3
		}
	}

	h.logger.Info("registered MCP handlers", zap.Int("count", count))
}

// handleListWorkspaces lists all workspaces.
func (h *Handlers) handleListWorkspaces(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		h.logger.Error("failed to list workspaces", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list workspaces", nil)
	}
	dtos := make([]dto.WorkspaceDTO, 0, len(workspaces))
	for _, w := range workspaces {
		dtos = append(dtos, dto.FromWorkspace(w))
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.ListWorkspacesResponse{Workspaces: dtos, Total: len(dtos)})
}

// unmarshalStringField unmarshals a JSON payload and returns the value of a single string field.
func unmarshalStringField(payload json.RawMessage, fieldName string) (string, error) {
	var m map[string]string
	if err := json.Unmarshal(payload, &m); err != nil {
		return "", err
	}
	return m[fieldName], nil
}

// handleListByField is a generic handler for listing resources identified by a single string field.
func (h *Handlers) handleListByField(
	ctx context.Context, msg *ws.Message,
	fieldName, logErrMsg, clientErrMsg string,
	fn func(context.Context, string) (any, error),
) (*ws.Message, error) {
	value, err := unmarshalStringField(msg.Payload, fieldName)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if value == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, fieldName+" is required", nil)
	}
	resp, err := fn(ctx, value)
	if err != nil {
		h.logger.Error(logErrMsg, zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, clientErrMsg, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// handleDeleteByField is a generic handler for deleting a resource identified by a single string field.
func (h *Handlers) handleDeleteByField(
	ctx context.Context, msg *ws.Message,
	fieldName, logErrMsg, clientErrMsg string,
	fn func(context.Context, string) error,
) (*ws.Message, error) {
	value, err := unmarshalStringField(msg.Payload, fieldName)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if value == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, fieldName+" is required", nil)
	}
	if err := fn(ctx, value); err != nil {
		h.logger.Error(logErrMsg, zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, clientErrMsg+": "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

// handleListWorkflows lists workflows for a workspace.
func (h *Handlers) handleListWorkflows(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "workspace_id", "failed to list workflows", "Failed to list workflows",
		func(ctx context.Context, workspaceID string) (any, error) {
			workflows, err := h.taskSvc.ListWorkflows(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			dtos := make([]dto.WorkflowDTO, 0, len(workflows))
			for _, w := range workflows {
				dtos = append(dtos, dto.FromWorkflow(w))
			}
			return dto.ListWorkflowsResponse{Workflows: dtos, Total: len(dtos)}, nil
		})
}

// handleListWorkflowSteps lists workflow steps for a workflow.
func (h *Handlers) handleListWorkflowSteps(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "workflow_id", "failed to list workflow steps", "Failed to list workflow steps",
		func(ctx context.Context, workflowID string) (any, error) {
			return h.workflowCtrl.ListStepsByWorkflow(ctx, workflowctrl.ListStepsRequest{WorkflowID: workflowID})
		})
}

// handleListTasks lists tasks for a workflow.
func (h *Handlers) handleListTasks(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "workflow_id", "failed to list tasks", "Failed to list tasks",
		func(ctx context.Context, workflowID string) (any, error) {
			tasks, err := h.taskSvc.ListTasks(ctx, workflowID)
			if err != nil {
				return nil, err
			}
			dtos := make([]dto.TaskDTO, 0, len(tasks))
			for _, t := range tasks {
				dtos = append(dtos, dto.FromTask(t))
			}
			return dto.ListTasksResponse{Tasks: dtos, Total: len(dtos)}, nil
		})
}

// mcpRepositoryInput matches the repository input structure from MCP create_task
type mcpRepositoryInput struct {
	RepositoryID string `json:"repository_id"`
	LocalPath    string `json:"local_path"`
	BaseBranch   string `json:"base_branch"`
}

// handleCreateTask creates a new task and optionally auto-starts an agent session.
func (h *Handlers) handleCreateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	// Use local struct with JSON tags since dto.CreateTaskRequest lacks them
	var req struct {
		ParentID          string               `json:"parent_id"`
		SourceTaskID      string               `json:"source_task_id"`
		WorkspaceID       string               `json:"workspace_id"`
		WorkflowID        string               `json:"workflow_id"`
		WorkflowStepID    string               `json:"workflow_step_id"`
		Title             string               `json:"title"`
		Description       string               `json:"description"`
		AgentProfileID    string               `json:"agent_profile_id"`
		ExecutorProfileID string               `json:"executor_profile_id"`
		StartAgent        *bool                `json:"start_agent"`  // nil means default to true for backward compatibility
		Repositories      []mcpRepositoryInput `json:"repositories"` // explicit repositories for top-level tasks
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Title == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "title is required", nil)
	}

	// Default start_agent to true for backward compatibility
	startAgent := req.StartAgent == nil || *req.StartAgent

	// Only require description for subtasks if we're starting an agent
	if req.ParentID != "" && req.Description == "" && startAgent {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "description is required for subtasks: it is the sub-agent's initial prompt and the only context it receives to start working", nil)
	}

	// Resolve repositories and inherit workspace/workflow from parent if needed.
	resolved, err := h.resolveTaskRepositories(ctx, req.ParentID, req.SourceTaskID, req.Repositories)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	repos := resolved.Repos
	if req.WorkspaceID == "" {
		req.WorkspaceID = resolved.WorkspaceID
	}
	if req.WorkflowID == "" {
		req.WorkflowID = resolved.WorkflowID
	}

	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}

	task, err := h.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		ParentID:       req.ParentID,
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		Repositories:   repos,
	})
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}

	// Auto-start agent session asynchronously only if requested
	if startAgent {
		h.autoStartTask(task, req.AgentProfileID, req.ExecutorProfileID, req.SourceTaskID)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}

// taskRepoResult holds the output of resolveTaskRepositories.
type taskRepoResult struct {
	Repos       []service.TaskRepositoryInput
	WorkspaceID string // inherited from parent, empty otherwise
	WorkflowID  string // inherited from parent, empty otherwise
}

// resolveTaskRepositories builds the repository list for a new task.
// Priority: explicit repositories > parent task repos > source task repos.
// When inheriting from a parent, it also returns the parent's workspace/workflow IDs.
func (h *Handlers) resolveTaskRepositories(
	ctx context.Context,
	parentID, sourceTaskID string,
	explicit []mcpRepositoryInput,
) (taskRepoResult, error) {
	if len(explicit) > 0 {
		var repos []service.TaskRepositoryInput
		for _, r := range explicit {
			repos = append(repos, service.TaskRepositoryInput{
				RepositoryID: r.RepositoryID,
				LocalPath:    r.LocalPath,
				BaseBranch:   r.BaseBranch,
			})
		}
		return taskRepoResult{Repos: repos}, nil
	}

	if parentID != "" {
		parent, err := h.taskSvc.GetTask(ctx, parentID)
		if err != nil {
			return taskRepoResult{}, fmt.Errorf("invalid parent_id: %w", err)
		}
		var repos []service.TaskRepositoryInput
		for _, r := range parent.Repositories {
			repos = append(repos, service.TaskRepositoryInput{
				RepositoryID:   r.RepositoryID,
				BaseBranch:     r.BaseBranch,
				CheckoutBranch: r.CheckoutBranch,
			})
		}
		return taskRepoResult{
			Repos:       repos,
			WorkspaceID: parent.WorkspaceID,
			WorkflowID:  parent.WorkflowID,
		}, nil
	}

	// For top-level tasks, inherit from the calling agent's current task.
	if sourceTaskID != "" {
		sourceTask, err := h.taskSvc.GetTask(ctx, sourceTaskID)
		if err != nil {
			h.logger.Warn("source task not found, skipping repo inheritance",
				zap.String("source_task_id", sourceTaskID), zap.Error(err))
			return taskRepoResult{}, nil
		}
		var repos []service.TaskRepositoryInput
		for _, r := range sourceTask.Repositories {
			repos = append(repos, service.TaskRepositoryInput{
				RepositoryID:   r.RepositoryID,
				BaseBranch:     r.BaseBranch,
				CheckoutBranch: r.CheckoutBranch,
			})
		}
		return taskRepoResult{Repos: repos}, nil
	}

	return taskRepoResult{}, nil
}

// autoStartTask launches an agent session for a newly created task in the background.
// It resolves the agent profile: explicit > parent's session > source task's session > workspace default.
// It resolves the executor: explicit executor_profile_id > parent's executor_profile_id >
// source task's executor_profile_id > parent's executor_id > "exec-worktree" (default for MCP-created tasks).
func (h *Handlers) autoStartTask(task *models.Task, agentProfileID, executorProfileID, sourceTaskID string) {
	if h.sessionLauncher == nil {
		return
	}

	executorID := h.inheritFromParentSession(task.ParentID, &agentProfileID, &executorProfileID)

	// For top-level tasks, inherit from the source task (the calling agent's task)
	if task.ParentID == "" && sourceTaskID != "" {
		sourceExecutorID := h.inheritFromParentSession(sourceTaskID, &agentProfileID, &executorProfileID)
		if executorID == "" {
			executorID = sourceExecutorID
		}
	}

	// Fall back to workspace defaults for agent profile and worktree executor
	if agentProfileID == "" {
		workspace, err := h.taskSvc.GetWorkspace(context.Background(), task.WorkspaceID)
		if err == nil && workspace.DefaultAgentProfileID != nil {
			agentProfileID = *workspace.DefaultAgentProfileID
		}
	}
	if executorID == "" && executorProfileID == "" {
		executorID = models.ExecutorIDWorktree
	}

	if agentProfileID == "" {
		h.logger.Warn("no agent profile available, skipping auto-start",
			zap.String("task_id", task.ID))
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), constants.AgentLaunchTimeout)
		defer cancel()

		resp, err := h.sessionLauncher.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
			TaskID:            task.ID,
			Intent:            orchestrator.IntentStart,
			AgentProfileID:    agentProfileID,
			ExecutorID:        executorID,
			ExecutorProfileID: executorProfileID,
			Prompt:            task.Description,
		})
		if err != nil {
			h.logger.Error("failed to auto-start task",
				zap.String("task_id", task.ID), zap.Error(err))
			return
		}
		h.logger.Info("auto-started agent for MCP-created task",
			zap.String("task_id", task.ID),
			zap.String("session_id", resp.SessionID))
	}()
}

// inheritFromParentSession fills agentProfileID and executorProfileID from the parent
// task's primary session when not explicitly provided. It returns the parent's ExecutorID
// as a fallback for when the parent session has no executor profile (common for
// UI-created sessions). If ExecutorProfileID is resolved, ExecutorID is redundant
// since the profile already encodes the executor reference.
func (h *Handlers) inheritFromParentSession(parentID string, agentProfileID, executorProfileID *string) string {
	if parentID == "" {
		return ""
	}
	parent, err := h.taskSvc.GetPrimarySession(context.Background(), parentID)
	if err != nil || parent == nil {
		return ""
	}
	if *agentProfileID == "" {
		*agentProfileID = parent.AgentProfileID
	}
	if *executorProfileID == "" {
		*executorProfileID = parent.ExecutorProfileID
	}
	// Only return ExecutorID as fallback when no profile was resolved.
	// An executor profile already encodes its executor reference.
	if *executorProfileID == "" {
		return parent.ExecutorID
	}
	return ""
}

// handleUpdateTask updates an existing task.
func (h *Handlers) handleUpdateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	// Use local struct with JSON tags since dto.UpdateTaskRequest lacks them
	var req struct {
		TaskID      string  `json:"task_id"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
		State       *string `json:"state"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	task, err := h.taskSvc.UpdateTask(ctx, req.TaskID, &service.UpdateTaskRequest{
		Title:       req.Title,
		Description: req.Description,
	})
	if err != nil {
		h.logger.Error("failed to update task", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.FromTask(task))
}

// handleAskUserQuestion creates a clarification request and blocks until the user responds.
// The agent's MCP tool call stays open (same turn) while waiting. If the agent times out,
// the event-based fallback in the orchestrator handles resuming with a new turn.
func (h *Handlers) handleAskUserQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string                 `json:"session_id"`
		TaskID    string                 `json:"task_id"`
		Question  clarification.Question `json:"question"`
		Context   string                 `json:"context"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	if req.Question.Prompt == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "question.prompt is required", nil)
	}

	// Generate question ID if missing
	if req.Question.ID == "" {
		req.Question.ID = "q1"
	}
	// Generate option IDs if missing
	for i := range req.Question.Options {
		if req.Question.Options[i].ID == "" {
			req.Question.Options[i].ID = generateOptionID(0, i)
		}
	}

	// Look up task ID from session if not provided
	taskID := req.TaskID
	if taskID == "" {
		session, err := h.sessionRepo.GetTaskSession(ctx, req.SessionID)
		if err != nil {
			h.logger.Warn("failed to look up task for session",
				zap.String("session_id", req.SessionID),
				zap.Error(err))
		} else if session != nil {
			taskID = session.TaskID
		}
	}

	// Create the clarification request
	clarificationReq := &clarification.Request{
		SessionID: req.SessionID,
		TaskID:    taskID,
		Question:  req.Question,
		Context:   req.Context,
	}
	pendingID := h.clarificationSvc.CreateRequest(clarificationReq)

	// Create the message in the database (triggers WS event to frontend)
	if h.messageCreator != nil {
		if _, err := h.messageCreator.CreateClarificationRequestMessage(
			ctx, taskID, req.SessionID, pendingID, req.Question, req.Context,
		); err != nil {
			h.logger.Error("failed to create clarification request message",
				zap.String("pending_id", pendingID),
				zap.String("session_id", req.SessionID),
				zap.Error(err))
		}
	}

	// Update session and task states to waiting for input
	h.setSessionWaitingForInput(ctx, taskID, req.SessionID)

	h.logger.Info("clarification request created, waiting for user response",
		zap.String("pending_id", pendingID),
		zap.String("session_id", req.SessionID),
		zap.String("task_id", taskID))

	// Block until user responds or context is cancelled (agent MCP timeout).
	// With MCP_TIMEOUT set to 2h for Claude Code, this will wait long enough.
	// If the agent times out, the entry is cleaned up and the event-based
	// fallback in the orchestrator handles resuming with a new turn.
	resp, err := h.clarificationSvc.WaitForResponse(ctx, pendingID)
	if err != nil {
		h.logger.Warn("clarification wait ended without response",
			zap.String("pending_id", pendingID),
			zap.String("session_id", req.SessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"Clarification request timed out or was cancelled", nil)
	}

	// User responded — set session back to running
	h.setSessionRunning(ctx, taskID, req.SessionID)

	h.logger.Info("clarification answered, returning to agent",
		zap.String("pending_id", pendingID),
		zap.String("session_id", req.SessionID),
		zap.Bool("rejected", resp.Rejected))

	// Return response in format expected by agentctl's extractQuestionAnswer
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// setSessionRunning restores the session state to running after a clarification is answered.
func (h *Handlers) setSessionRunning(ctx context.Context, taskID, sessionID string) {
	if err := h.sessionRepo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateRunning, ""); err != nil {
		h.logger.Warn("failed to update session state to RUNNING",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	if taskID != "" {
		if err := h.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
			h.logger.Warn("failed to update task state to IN_PROGRESS",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
	}

	// Publish session state changed event
	if h.eventBus != nil {
		eventData := map[string]any{
			"task_id":    taskID,
			"session_id": sessionID,
			"new_state":  string(models.TaskSessionStateRunning),
		}
		_ = h.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"mcp-handlers",
			eventData,
		))
	}
}

// setSessionWaitingForInput updates the session and task states to waiting for input
func (h *Handlers) setSessionWaitingForInput(ctx context.Context, taskID, sessionID string) {
	// Update session state to WAITING_FOR_INPUT
	if err := h.sessionRepo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		h.logger.Warn("failed to update session state to WAITING_FOR_INPUT",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}

	// Update task state to REVIEW
	if taskID != "" {
		if err := h.taskRepo.UpdateTaskState(ctx, taskID, v1.TaskStateReview); err != nil {
			h.logger.Warn("failed to update task state to REVIEW",
				zap.String("task_id", taskID),
				zap.Error(err))
		}
	}

	// Publish session state changed event
	if h.eventBus != nil {
		eventData := map[string]interface{}{
			"task_id":    taskID,
			"session_id": sessionID,
			"new_state":  string(models.TaskSessionStateWaitingForInput),
		}
		_ = h.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"mcp-handlers",
			eventData,
		))
	}
}

// generateOptionID generates an option ID for a question.
func generateOptionID(questionIndex, optionIndex int) string {
	return fmt.Sprintf("q%d_opt%d", questionIndex+1, optionIndex+1)
}

// handleCreateTaskPlan creates a new task plan.
func (h *Handlers) handleCreateTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		CreatedBy string `json:"created_by"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content is required", nil)
	}

	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "agent"
	}

	plan, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID:    req.TaskID,
		Title:     req.Title,
		Content:   req.Content,
		CreatedBy: createdBy,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// handleGetTaskPlan retrieves a task plan.
func (h *Handlers) handleGetTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	plan, err := h.planService.GetPlan(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task plan", nil)
	}
	if plan == nil {
		// Return empty object if no plan exists
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{})
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// handleUpdateTaskPlan updates an existing task plan.
func (h *Handlers) handleUpdateTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		Title     string `json:"title"`
		Content   string `json:"content"`
		CreatedBy string `json:"created_by"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Content == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "content is required", nil)
	}

	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "agent"
	}

	plan, err := h.planService.UpdatePlan(ctx, service.UpdatePlanRequest{
		TaskID:    req.TaskID,
		Title:     req.Title,
		Content:   req.Content,
		CreatedBy: createdBy,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskPlanNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task plan not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, dto.TaskPlanFromModel(plan))
}

// handleDeleteTaskPlan deletes a task plan.
func (h *Handlers) handleDeleteTaskPlan(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	err := h.planService.DeletePlan(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		if errors.Is(err, service.ErrTaskPlanNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Task plan not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete task plan: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"success": true})
}

// handleClarificationTimeout is called by agentctl when the agent's MCP client
// disconnects while waiting for a clarification response. It cancels the pending
// clarification so the user's eventual answer goes through the event fallback path
// (new turn) instead of the primary path (which would be dropped).
func (h *Handlers) handleClarificationTimeout(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	cancelled := h.clarificationSvc.CancelSession(req.SessionID)
	h.logger.Info("cancelled pending clarifications on agent MCP timeout",
		zap.String("session_id", req.SessionID),
		zap.Int("count", len(cancelled)))

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"ok": true, "cancelled": len(cancelled)})
}
