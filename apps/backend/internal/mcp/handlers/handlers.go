// Package handlers provides WebSocket handlers for MCP tool requests.
// These handlers are called by agentctl via the WS tunnel and execute
// operations against the backend services directly.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/scriptengine"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	workflowctrl "github.com/kandev/kandev/internal/workflow/controller"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowsvc "github.com/kandev/kandev/internal/workflow/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ClarificationService defines the interface for clarification operations.
type ClarificationService interface {
	CreateRequest(req *clarification.Request) (string, bool)
	WaitForResponse(ctx context.Context, pendingID string) (*clarification.Response, error)
	CancelRequest(pendingID string) bool
}

// SessionCanceller detaches in-memory clarification waiters while keeping DB
// messages pending. Used by the MCP-timeout handler.
type SessionCanceller interface {
	DetachSessionAndNotify(ctx context.Context, sessionID string) int
}

// MessageCreator creates messages for clarification requests.
type MessageCreator interface {
	CreateClarificationRequestMessages(ctx context.Context, taskID, sessionID, pendingID string, questions []clarification.Question, clarificationContext string) ([]string, error)
}

// SessionRepository interface for updating session state.
type SessionRepository interface {
	UpdateTaskSessionState(ctx context.Context, sessionID string, state models.TaskSessionState, errorMessage string) error
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	// SetSessionMetadataKey is used by handleStepComplete (ADR 0015) to
	// atomically write the pending-completion bag without clobbering other
	// metadata keys.
	SetSessionMetadataKey(ctx context.Context, sessionID, key string, value interface{}) error
}

// TaskRepository interface for updating task state.
type TaskRepository interface {
	UpdateTaskState(ctx context.Context, taskID string, state v1.TaskState) error
}

// EventBus interface for publishing events.
type EventBus interface {
	Publish(ctx context.Context, topic string, event *bus.Event) error
}

// SessionLauncher provides session launch and prompt-dispatch capabilities.
// Implemented by *orchestrator.Service.
type SessionLauncher interface {
	LaunchSession(ctx context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error)
	PromptTask(ctx context.Context, taskID, sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment, dispatchOnly bool) (*orchestrator.PromptResult, error)
	StartCreatedSession(ctx context.Context, taskID, sessionID, agentProfileID, prompt string, skipMessageRecord, planMode, autoStart bool, attachments []v1.MessageAttachment) (*executor.TaskExecution, error)
	ResumeTaskSession(ctx context.Context, taskID, sessionID string) (*executor.TaskExecution, error)
	ProcessOnTurnStart(ctx context.Context, taskID, sessionID string) error
	GetMessageQueue() *messagequeue.Service
}

// MessageQueuer queues a prompt message for delivery to a session on its next turn.
// TakeQueued is exposed so move_task can roll back the hand-off prompt when the
// underlying MoveTask call fails — without it, a queued "you were moved..."
// message would survive a failed move and be delivered on the next agent turn.
type MessageQueuer interface {
	QueueMessage(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, error)
	SetPendingMove(ctx context.Context, sessionID string, move *messagequeue.PendingMove)
	TakeQueued(ctx context.Context, sessionID string) (*messagequeue.QueuedMessage, bool)
}

// Handlers provides MCP WebSocket handlers.
type Handlers struct {
	taskSvc          *service.Service
	workflowCtrl     *workflowctrl.Controller
	clarificationSvc ClarificationService
	sessionCanceller SessionCanceller
	messageCreator   MessageCreator
	sessionRepo      SessionRepository
	taskRepo         TaskRepository
	eventBus         EventBus
	planService      *service.PlanService
	sessionLauncher  SessionLauncher
	messageQueue     MessageQueuer
	logger           *logger.Logger

	// Config-mode dependencies (optional, set via SetConfigDeps)
	workflowSvc       *workflowsvc.Service
	agentSettingsCtrl *agentsettingscontroller.Controller
	mcpConfigSvc      *mcpconfig.Service

	// Cross-task handoff service (optional, set via SetHandoffService).
	// Wires the list_related_tasks_kandev / *_task_document_kandev
	// MCP tools introduced in office task handoffs phase 2.
	handoffSvc *service.HandoffService

	// Optional PR lister (set via SetTaskPRLister) used to enrich
	// task-listing responses with associated pull requests.
	taskPRLister TaskPRLister
}

// NewHandlers creates new MCP handlers.
func NewHandlers(
	taskSvc *service.Service,
	workflowCtrl *workflowctrl.Controller,
	clarificationSvc ClarificationService,
	sessionCanceller SessionCanceller,
	messageCreator MessageCreator,
	sessionRepo SessionRepository,
	taskRepo TaskRepository,
	eventBus EventBus,
	planService *service.PlanService,
	sessionLauncher SessionLauncher,
	messageQueue MessageQueuer,
	log *logger.Logger,
) *Handlers {
	return &Handlers{
		taskSvc:          taskSvc,
		workflowCtrl:     workflowCtrl,
		clarificationSvc: clarificationSvc,
		sessionCanceller: sessionCanceller,
		messageCreator:   messageCreator,
		sessionRepo:      sessionRepo,
		taskRepo:         taskRepo,
		eventBus:         eventBus,
		planService:      planService,
		sessionLauncher:  sessionLauncher,
		messageQueue:     messageQueue,
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
	d.RegisterFunc(ws.ActionMCPListRepositories, h.handleListRepositories)
	d.RegisterFunc(ws.ActionMCPListTasks, h.handleListTasks)
	d.RegisterFunc(ws.ActionMCPCreateTask, h.handleCreateTask)
	d.RegisterFunc(ws.ActionMCPUpdateTask, h.handleUpdateTask)
	d.RegisterFunc(ws.ActionMCPAddBranchToTask, h.handleAddBranchToTask)
	d.RegisterFunc(ws.ActionMCPUpdateRepositoryBaseBranch, h.handleUpdateRepositoryBaseBranch)
	d.RegisterFunc(ws.ActionMCPStepComplete, h.handleStepComplete)
	d.RegisterFunc(ws.ActionMCPMessageTask, h.handleMessageTask)
	d.RegisterFunc(ws.ActionMCPGetTaskConversation, h.handleGetTaskConversation)
	d.RegisterFunc(ws.ActionMCPAskUserQuestion, h.handleAskUserQuestion)
	d.RegisterFunc(ws.ActionMCPCreateTaskPlan, h.handleCreateTaskPlan)
	d.RegisterFunc(ws.ActionMCPGetTaskPlan, h.handleGetTaskPlan)
	d.RegisterFunc(ws.ActionMCPUpdateTaskPlan, h.handleUpdateTaskPlan)
	d.RegisterFunc(ws.ActionMCPDeleteTaskPlan, h.handleDeleteTaskPlan)
	d.RegisterFunc(ws.ActionMCPClarificationTimeout, h.handleClarificationTimeout)
	count := 17

	// Config-mode handlers (registered when config deps are set)
	if h.workflowSvc != nil {
		d.RegisterFunc(ws.ActionMCPCreateWorkflow, h.handleCreateWorkflow)
		d.RegisterFunc(ws.ActionMCPUpdateWorkflow, h.handleUpdateWorkflow)
		d.RegisterFunc(ws.ActionMCPDeleteWorkflow, h.handleDeleteWorkflow)
		d.RegisterFunc(ws.ActionMCPImportWorkflow, h.handleImportWorkflow)
		d.RegisterFunc(ws.ActionMCPCreateWorkflowStep, h.handleCreateWorkflowStep)
		d.RegisterFunc(ws.ActionMCPUpdateWorkflowStep, h.handleUpdateWorkflowStep)
		d.RegisterFunc(ws.ActionMCPDeleteWorkflowStep, h.handleDeleteWorkflowStep)
		d.RegisterFunc(ws.ActionMCPReorderWorkflowStep, h.handleReorderWorkflowSteps)
		count += 8
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
	if h.handoffSvc != nil {
		d.RegisterFunc(ws.ActionMCPListRelatedTasks, h.handleListRelatedTasks)
		d.RegisterFunc(ws.ActionMCPListTaskDocuments, h.handleListTaskDocuments)
		d.RegisterFunc(ws.ActionMCPGetTaskDocument, h.handleGetTaskDocument)
		d.RegisterFunc(ws.ActionMCPWriteTaskDocument, h.handleWriteTaskDocument)
		count += 4
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
			workflows, err := h.taskSvc.ListWorkflows(ctx, workspaceID, false)
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

// handleListRepositories lists repositories for a workspace. Exposes the same
// data the kanban "Edit task → Repositories" picker reads, so an MCP-driven
// agent (e.g. the Slack triage runner) can match a request against an actual
// repo instead of guessing or making up an ID.
func (h *Handlers) handleListRepositories(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.handleListByField(ctx, msg, "workspace_id", "failed to list repositories", "Failed to list repositories",
		func(ctx context.Context, workspaceID string) (any, error) {
			repos, err := h.taskSvc.ListRepositories(ctx, workspaceID)
			if err != nil {
				return nil, err
			}
			dtos := make([]dto.RepositoryDTO, 0, len(repos))
			for _, r := range repos {
				dtos = append(dtos, dto.FromRepository(r))
			}
			return dto.ListRepositoriesResponse{Repositories: dtos, Total: len(dtos)}, nil
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
			h.enrichTasksWithPRs(ctx, dtos)
			return dto.ListTasksResponse{Tasks: dtos, Total: len(dtos)}, nil
		})
}

// mcpRepositoryInput matches the repository input structure from MCP create_task
type mcpRepositoryInput struct {
	RepositoryID string `json:"repository_id"`
	LocalPath    string `json:"local_path"`
	GitHubURL    string `json:"github_url"`
	BaseBranch   string `json:"base_branch"`
}

// handleCreateTask creates a new task and optionally auto-starts an agent session.
func (h *Handlers) handleCreateTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	// Use local struct with JSON tags since dto.CreateTaskRequest lacks them
	var req struct {
		ParentID               string               `json:"parent_id"`
		SourceTaskID           string               `json:"source_task_id"`
		WorkspaceID            string               `json:"workspace_id"`
		WorkflowID             string               `json:"workflow_id"`
		WorkflowStepID         string               `json:"workflow_step_id"`
		Title                  string               `json:"title"`
		Description            string               `json:"description"`
		AgentProfileID         string               `json:"agent_profile_id"`
		ExecutorProfileID      string               `json:"executor_profile_id"`
		StartAgent             *bool                `json:"start_agent"`               // nil means default to true for backward compatibility
		Repositories           []mcpRepositoryInput `json:"repositories"`              // explicit repositories for top-level tasks
		BaseBranch             string               `json:"base_branch"`               // top-level fallback applied to every resolved repo only when no per-repo entries are supplied; explicit per-repo BaseBranch is authoritative when Repositories is set
		BlockedBy              []string             `json:"blocked_by"`                // task IDs that must complete before this task
		AssigneeAgentProfileID string               `json:"assignee_agent_profile_id"` // agent instance to assign the task to
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Title == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "title is required", nil)
	}
	if req.AssigneeAgentProfileID != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "assignee_agent_profile_id is office-only and cannot be set via create_task_kandev", nil)
	}

	// Default start_agent to true for backward compatibility
	startAgent := req.StartAgent == nil || *req.StartAgent

	// Subtask-description guard is applied AFTER startup-prompt resolution
	// below so a repository's startup_prompt can satisfy the sub-agent's
	// initial-prompt requirement without the caller re-typing a description.

	// Resolve repositories and inherit workspace/workflow from parent if needed.
	resolved, err := h.resolveTaskRepositories(ctx, req.ParentID, req.SourceTaskID, req.Repositories)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	repos := resolved.Repos
	// Top-level base_branch override: when the caller passes base_branch
	// without any per-repo entries, apply it to every repo in the resolved
	// list. This is the only path that lets a same-repo subtask override
	// the parent's inherited base_branch without also restating the
	// repository identifier. When the caller provided explicit per-repo
	// entries we leave their BaseBranch alone — those are authoritative.
	if req.BaseBranch != "" && len(req.Repositories) == 0 {
		for i := range repos {
			repos[i].BaseBranch = req.BaseBranch
		}
	}
	if req.WorkspaceID == "" {
		req.WorkspaceID = resolved.WorkspaceID
	}
	if req.WorkflowID == "" {
		req.WorkflowID = resolved.WorkflowID
	}

	// Auto-resolve workspace/workflow when not provided and there's exactly one option.
	if req.WorkspaceID == "" && h.taskSvc != nil {
		if workspaces, wsErr := h.taskSvc.ListWorkspaces(ctx); wsErr != nil {
			h.logger.Warn("failed to auto-resolve workspace", zap.Error(wsErr))
		} else if len(workspaces) == 1 {
			req.WorkspaceID = workspaces[0].ID
		}
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}

	if req.WorkflowID == "" && h.taskSvc != nil {
		if workflows, wfErr := h.taskSvc.ListWorkflows(ctx, req.WorkspaceID, false); wfErr != nil {
			h.logger.Warn("failed to auto-resolve workflow", zap.String("workspace_id", req.WorkspaceID), zap.Error(wfErr))
		} else if len(workflows) == 1 {
			req.WorkflowID = workflows[0].ID
		}
	}
	if req.WorkflowID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workflow_id is required", nil)
	}

	pendingTask := &models.Task{
		ParentID:       req.ParentID,
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
	}
	launchConfig, metadata, err := h.resolveMCPLaunchMetadata(ctx, pendingTask, req.AgentProfileID, req.ExecutorProfileID, req.SourceTaskID)
	if err != nil {
		code := ws.ErrorCodeInternalError
		if errors.Is(err, errMCPAgentProfileRequired) {
			code = ws.ErrorCodeValidation
		}
		return ws.NewError(msg.ID, msg.Action, code, err.Error(), nil)
	}

	// When the caller supplied no description, fall back to the target
	// repository's startup_prompt (if any). Ticket placeholders resolve only
	// from parent-inherited metadata (parent_id: "self" or an explicit parent);
	// unresolved placeholder lines are dropped by ResolveStartupPrompt.
	description := req.Description
	if description == "" {
		description = h.resolveStartupPromptForMCP(ctx, req.WorkspaceID, req.ParentID, req.Title, repos)
	}

	// Subtasks with a live agent need an initial prompt. Check AFTER startup
	// prompt resolution so a repository's startup_prompt can satisfy this
	// requirement without the caller re-typing a description.
	if req.ParentID != "" && description == "" && startAgent {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "description is required for subtasks: it is the sub-agent's initial prompt and the only context it receives to start working", nil)
	}

	task, err := h.taskSvc.CreateTask(ctx, &service.CreateTaskRequest{
		ParentID:               req.ParentID,
		WorkspaceID:            req.WorkspaceID,
		WorkflowID:             req.WorkflowID,
		WorkflowStepID:         req.WorkflowStepID,
		Title:                  req.Title,
		Description:            description,
		Repositories:           repos,
		BlockedBy:              req.BlockedBy,
		AssigneeAgentProfileID: req.AssigneeAgentProfileID,
		Metadata:               metadata,
	})
	if err != nil {
		h.logger.Error("failed to create task", zap.Error(err))
		// Defense-in-depth: resolveTaskRepositories already catches this for the
		// MCP path, but non-MCP callers (UI, internal engine) reach here directly.
		if errors.Is(err, service.ErrSubtaskDepthExceeded) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create task", nil)
	}

	// Auto-start agent session asynchronously only if requested
	if startAgent && h.sessionLauncher != nil {
		h.launchAutoStartTask(ctx, task, launchConfig)
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
//
// For subtasks (parentID set) workspace and workflow always come from the
// parent. Explicit repositories override the parent's repos when supplied,
// otherwise the parent's repos are inherited verbatim — letting an agent
// spin up a subtask that targets a sibling repo while staying in the same
// workspace/workflow.
//
// For top-level tasks (parentID empty) explicit repos win over source-task
// inheritance; workspace falls back to the source task when available.
func (h *Handlers) resolveTaskRepositories(
	ctx context.Context,
	parentID, sourceTaskID string,
	explicit []mcpRepositoryInput,
) (taskRepoResult, error) {
	explicitRepos := h.explicitRepoInputsWithDefaults(ctx, explicit)

	if parentID != "" {
		parent, err := h.taskSvc.GetTask(ctx, parentID)
		if err != nil {
			return taskRepoResult{}, fmt.Errorf("invalid parent_id: %w", err)
		}
		if parent.IsEphemeral {
			return taskRepoResult{}, fmt.Errorf("cannot create subtasks of an ephemeral task (quick chat); omit parent_id to create a top-level task")
		}
		if parent.ParentID != "" && !parent.IsFromOffice {
			return taskRepoResult{}, service.ErrSubtaskDepthExceeded
		}
		repos := explicitRepos
		if repos == nil {
			repos = inheritedRepoInputs(parent.Repositories)
		}
		return taskRepoResult{
			Repos:       repos,
			WorkspaceID: parent.WorkspaceID,
			WorkflowID:  parent.WorkflowID,
		}, nil
	}

	if explicitRepos != nil {
		result := taskRepoResult{Repos: explicitRepos}
		// Inherit workspace from source task so multi-workspace installs don't
		// fail auto-resolution when the agent supplies an explicit repository.
		if sourceTaskID != "" && h.taskSvc != nil {
			src, srcErr := h.taskSvc.GetTask(ctx, sourceTaskID)
			if srcErr != nil {
				h.logger.Warn("source task lookup failed, skipping workspace inheritance",
					zap.String("source_task_id", sourceTaskID), zap.Error(srcErr))
			} else {
				result.WorkspaceID = src.WorkspaceID
			}
		}
		return result, nil
	}

	// For top-level tasks, inherit repos and workspace from the calling agent's current task.
	if sourceTaskID != "" {
		sourceTask, err := h.taskSvc.GetTask(ctx, sourceTaskID)
		if err != nil {
			h.logger.Warn("source task not found, skipping inheritance",
				zap.String("source_task_id", sourceTaskID), zap.Error(err))
			return taskRepoResult{}, nil
		}
		return taskRepoResult{
			Repos:       inheritedRepoInputs(sourceTask.Repositories),
			WorkspaceID: sourceTask.WorkspaceID,
		}, nil
	}

	return taskRepoResult{}, nil
}

// explicitRepoInputsWithDefaults maps the MCP-side explicit repo list to
// service inputs. Returns nil when no explicit repos were supplied so callers
// can distinguish "agent didn't pass repos" from "agent passed an empty list".
//
// When an explicit entry pins a repository_id without a base_branch, the
// repository's default_branch is filled in. This anchors cross-repo subtasks
// (a parent on feature/foo creating a child in another repo) to a known-good
// branch instead of an empty value that would force every downstream consumer
// to recompute the default.
func (h *Handlers) explicitRepoInputsWithDefaults(ctx context.Context, explicit []mcpRepositoryInput) []service.TaskRepositoryInput {
	if len(explicit) == 0 {
		return nil
	}
	repos := make([]service.TaskRepositoryInput, 0, len(explicit))
	for _, r := range explicit {
		baseBranch := r.BaseBranch
		if baseBranch == "" && r.RepositoryID != "" && h.taskSvc != nil {
			if repo, err := h.taskSvc.GetRepository(ctx, r.RepositoryID); err == nil && repo != nil {
				baseBranch = repo.DefaultBranch
			}
		}
		repos = append(repos, service.TaskRepositoryInput{
			RepositoryID: r.RepositoryID,
			LocalPath:    r.LocalPath,
			GitHubURL:    r.GitHubURL,
			BaseBranch:   baseBranch,
		})
	}
	return repos
}

// inheritedRepoInputs maps an existing task's repository list onto service
// inputs for a new task that inherits from it. RepositoryID and BaseBranch
// carry over so a same-repo subtask branches off the same point as the
// parent (sibling branches off the same base, ergonomically aligned for
// stacked PRs). CheckoutBranch is dropped on purpose: two worktrees cannot
// share a working branch, so the subtask's session generates a fresh one.
// Agents that need a different base for a same-repo subtask must pass
// base_branch explicitly. If the inherited base_branch is missing on the
// remote at launch time, the worktree manager's fallback recovers to the
// repository's default_branch and surfaces a warning.
func inheritedRepoInputs(src []*models.TaskRepository) []service.TaskRepositoryInput {
	if len(src) == 0 {
		return nil
	}
	repos := make([]service.TaskRepositoryInput, 0, len(src))
	for _, r := range src {
		if r == nil {
			continue
		}
		repos = append(repos, service.TaskRepositoryInput{
			RepositoryID: r.RepositoryID,
			BaseBranch:   r.BaseBranch,
		})
	}
	return repos
}

// ticketMetadataKeys are the keys copied from a parent task's metadata onto
// startup-prompt resolution so subtasks created via `parent_id: "self"` inherit
// their parent's ticket context (Jira issue key/URL or Linear identifier/URL).
var ticketMetadataKeys = []string{
	"jira_issue_key",
	"jira_issue_url",
	"linear_issue_identifier",
	"linear_issue_url",
}

// resolveStartupPromptForMCP looks up the first-listed repository's
// startup_prompt and resolves it using the parent task's ticket metadata (when
// a parent is present) and the new task's title. Callers who identify their
// repo by local_path or github_url instead of repository_id are handled via
// Service.FindRepositoryForInput so the same repo lookup works before
// CreateTask has had a chance to resolve those alternate forms. Returns empty
// string when no repo is resolvable or when the repo has no startup_prompt.
func (h *Handlers) resolveStartupPromptForMCP(ctx context.Context, workspaceID, parentID, taskTitle string, repos []service.TaskRepositoryInput) string {
	if h.taskSvc == nil || len(repos) == 0 {
		return ""
	}
	repo, err := h.taskSvc.FindRepositoryForInput(ctx, workspaceID, repos[0])
	if err != nil || repo == nil || repo.StartupPrompt == "" {
		return ""
	}
	metadata := h.inheritedTicketMetadata(ctx, parentID)
	return scriptengine.ResolveStartupPrompt(repo.StartupPrompt, taskTitle, metadata)
}

// inheritedTicketMetadata returns the parent task's ticket-related metadata
// keys so a startup prompt using {{TICKET_ID}} / {{TICKET_URL}} resolves for
// subtasks created via `parent_id: "self"`. Returns nil when there is no
// parent or the parent lookup fails.
func (h *Handlers) inheritedTicketMetadata(ctx context.Context, parentID string) map[string]interface{} {
	if parentID == "" || h.taskSvc == nil {
		return nil
	}
	parent, err := h.taskSvc.GetTask(ctx, parentID)
	if err != nil || parent == nil || parent.Metadata == nil {
		return nil
	}
	var inherited map[string]interface{}
	for _, key := range ticketMetadataKeys {
		if v, ok := parent.Metadata[key]; ok {
			if inherited == nil {
				inherited = make(map[string]interface{}, len(ticketMetadataKeys))
			}
			inherited[key] = v
		}
	}
	return inherited
}

type mcpAutoStartConfig struct {
	AgentProfileID    string
	ExecutorID        string
	ExecutorProfileID string
}

var errMCPAgentProfileRequired = errors.New("agent_profile_id is required because no agent profile can be resolved from the parent task, source task, workflow, or workspace defaults")

// autoStartTask launches an agent session for a newly created task in the background.
// It is kept as a small compatibility wrapper for direct tests; handleCreateTask
// uses resolveMCPAutoStartConfig before persisting so invalid auto-start
// requests fail synchronously.
func (h *Handlers) autoStartTask(task *models.Task, agentProfileID, executorProfileID, sourceTaskID string) {
	ctx := context.Background()
	config := h.resolveMCPAutoStartConfig(ctx, task, agentProfileID, executorProfileID, sourceTaskID)
	h.launchAutoStartTask(ctx, task, config)
}

// resolveMCPAutoStartConfig resolves the agent profile and executor for
// create_task_kandev auto-start. Agent profile resolution follows the MCP
// ergonomics first (explicit > parent/source session), then the same durable
// defaults used by task opening where this handler can see them (workflow step
// when a workflow controller is wired, workflow default, workspace default).
func (h *Handlers) resolveMCPAutoStartConfig(ctx context.Context, task *models.Task, agentProfileID, executorProfileID, sourceTaskID string) mcpAutoStartConfig {
	config, _ := h.resolveMCPAutoStartConfigWithError(ctx, task, agentProfileID, executorProfileID, sourceTaskID)
	return config
}

func (h *Handlers) resolveMCPLaunchMetadata(ctx context.Context, task *models.Task, agentProfileID, executorProfileID, sourceTaskID string) (mcpAutoStartConfig, map[string]interface{}, error) {
	launchConfig, err := h.resolveMCPAutoStartConfigWithError(ctx, task, agentProfileID, executorProfileID, sourceTaskID)
	if err != nil {
		return mcpAutoStartConfig{}, nil, fmt.Errorf("failed to resolve launch profile: %w", err)
	}
	if launchConfig.AgentProfileID == "" {
		return mcpAutoStartConfig{}, nil, errMCPAgentProfileRequired
	}
	metadata := map[string]interface{}{
		models.MetaKeyAgentProfileID: launchConfig.AgentProfileID,
	}
	if launchConfig.ExecutorID != "" {
		metadata[models.MetaKeyExecutorID] = launchConfig.ExecutorID
	}
	if launchConfig.ExecutorProfileID != "" {
		metadata[models.MetaKeyExecutorProfileID] = launchConfig.ExecutorProfileID
	}
	return launchConfig, metadata, nil
}

func (h *Handlers) resolveMCPAutoStartConfigWithError(ctx context.Context, task *models.Task, agentProfileID, executorProfileID, sourceTaskID string) (mcpAutoStartConfig, error) {
	executorID, err := h.inheritFromTask(ctx, task.ParentID, &agentProfileID, &executorProfileID)
	if err != nil {
		return mcpAutoStartConfig{}, fmt.Errorf("inherit from parent task %s: %w", task.ParentID, err)
	}

	// For top-level tasks, inherit from the source task (the calling agent's task).
	if task.ParentID == "" && sourceTaskID != "" {
		sourceExecutorID, err := h.inheritFromTask(ctx, sourceTaskID, &agentProfileID, &executorProfileID)
		if err != nil {
			return mcpAutoStartConfig{}, fmt.Errorf("inherit from source task %s: %w", sourceTaskID, err)
		}
		if executorID == "" {
			executorID = sourceExecutorID
		}
	}

	if agentProfileID == "" {
		var err error
		agentProfileID, err = h.resolveWorkflowAgentProfileWithError(ctx, task.WorkflowStepID, task.WorkflowID)
		if err != nil {
			return mcpAutoStartConfig{}, fmt.Errorf("resolve workflow agent profile: %w", err)
		}
	}
	if agentProfileID == "" && h.taskSvc != nil {
		workspace, err := h.taskSvc.GetWorkspace(ctx, task.WorkspaceID)
		if err != nil {
			return mcpAutoStartConfig{}, fmt.Errorf("get workspace %s: %w", task.WorkspaceID, err)
		}
		if workspace != nil && workspace.DefaultAgentProfileID != nil {
			agentProfileID = *workspace.DefaultAgentProfileID
		}
	}
	if executorID == "" && executorProfileID == "" {
		executorID = models.ExecutorIDWorktree
	}

	return mcpAutoStartConfig{
		AgentProfileID:    agentProfileID,
		ExecutorID:        executorID,
		ExecutorProfileID: executorProfileID,
	}, nil
}

func (h *Handlers) resolveWorkflowAgentProfile(ctx context.Context, workflowStepID, workflowID string) string {
	profileID, _ := h.resolveWorkflowAgentProfileWithError(ctx, workflowStepID, workflowID)
	return profileID
}

func (h *Handlers) resolveWorkflowAgentProfileWithError(ctx context.Context, workflowStepID, workflowID string) (string, error) {
	profileID, resolvedWorkflowID := h.resolveWorkflowControllerAgentProfile(ctx, workflowStepID, workflowID)
	if profileID != "" {
		return profileID, nil
	}
	if workflowID == "" {
		workflowID = resolvedWorkflowID
	}
	profileID, err := h.workflowDefaultAgentProfileWithError(ctx, workflowID)
	if err != nil {
		return "", err
	}
	return profileID, nil
}

func (h *Handlers) resolveWorkflowControllerAgentProfile(ctx context.Context, workflowStepID, workflowID string) (string, string) {
	if h.workflowCtrl == nil {
		return "", workflowID
	}
	if workflowStepID != "" {
		return h.resolveWorkflowStepAgentProfile(ctx, workflowStepID, workflowID)
	}
	if workflowID == "" {
		return "", ""
	}
	return h.resolveWorkflowStartStepAgentProfile(ctx, workflowID), workflowID
}

func (h *Handlers) resolveWorkflowStepAgentProfile(ctx context.Context, workflowStepID, workflowID string) (string, string) {
	resp, err := h.workflowCtrl.GetStep(ctx, workflowStepID)
	if err != nil || resp == nil || resp.Step == nil {
		return "", workflowID
	}
	if workflowID == "" {
		workflowID = resp.Step.WorkflowID
	}
	return resp.Step.AgentProfileID, workflowID
}

func (h *Handlers) resolveWorkflowStartStepAgentProfile(ctx context.Context, workflowID string) string {
	resp, err := h.workflowCtrl.ListStepsByWorkflow(ctx, workflowctrl.ListStepsRequest{WorkflowID: workflowID})
	if err != nil || resp == nil {
		return ""
	}
	return startStepAgentProfile(resp.Steps)
}

func (h *Handlers) workflowDefaultAgentProfile(ctx context.Context, workflowID string) string {
	profileID, _ := h.workflowDefaultAgentProfileWithError(ctx, workflowID)
	return profileID
}

func (h *Handlers) workflowDefaultAgentProfileWithError(ctx context.Context, workflowID string) (string, error) {
	if workflowID == "" || h.taskSvc == nil {
		return "", nil
	}
	workflow, err := h.taskSvc.GetWorkflow(ctx, workflowID)
	if err != nil || workflow == nil {
		return "", err
	}
	return workflow.AgentProfileID, nil
}

func startStepAgentProfile(steps []*workflowmodels.WorkflowStep) string {
	if len(steps) == 0 {
		return ""
	}
	var firstByPosition *workflowmodels.WorkflowStep
	for _, step := range steps {
		if step == nil {
			continue
		}
		if firstByPosition == nil || step.Position < firstByPosition.Position {
			firstByPosition = step
		}
		if step.IsStartStep {
			return step.AgentProfileID
		}
	}
	if firstByPosition == nil {
		return ""
	}
	return firstByPosition.AgentProfileID
}

func (h *Handlers) launchAutoStartTask(ctx context.Context, task *models.Task, config mcpAutoStartConfig) {
	if h.sessionLauncher == nil {
		return
	}
	if config.AgentProfileID == "" {
		h.logger.Warn("no agent profile available, skipping auto-start",
			zap.String("task_id", task.ID))
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), constants.AgentLaunchTimeout)
		defer cancel()

		resp, err := h.sessionLauncher.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
			TaskID:            task.ID,
			Intent:            orchestrator.IntentStart,
			AgentProfileID:    config.AgentProfileID,
			ExecutorID:        config.ExecutorID,
			ExecutorProfileID: config.ExecutorProfileID,
			WorkflowStepID:    task.WorkflowStepID,
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

// inheritFromTask fills agentProfileID and executorProfileID from another task's
// durable launch metadata or primary session when not explicitly provided. It
// returns a bare ExecutorID only when no executor profile was resolved, because
// an executor profile already encodes its executor reference.
func (h *Handlers) inheritFromTask(ctx context.Context, taskID string, agentProfileID, executorProfileID *string) (string, error) {
	if taskID == "" || h.taskSvc == nil {
		return "", nil
	}

	agentProfileExplicit := *agentProfileID != ""
	executorProfileExplicit := *executorProfileID != ""
	executorID := h.inheritFromTaskMetadata(ctx, taskID, agentProfileID, executorProfileID, "")
	session, err := h.taskSvc.GetPrimarySession(ctx, taskID)
	if err != nil {
		if errors.Is(err, taskrepo.ErrNoPrimarySession) {
			return executorID, nil
		}
		return "", err
	}
	if session != nil {
		executorID = inheritWorkflowRoutedSession(session, agentProfileID, executorProfileID, executorID, agentProfileExplicit, executorProfileExplicit)
		sessionExecutorID := inheritFromSession(session, agentProfileID, executorProfileID, executorID == "")
		if executorID == "" {
			executorID = sessionExecutorID
		}
	}

	if *executorProfileID != "" {
		return "", nil
	}
	return executorID, nil
}

func inheritWorkflowRoutedSession(
	session *models.TaskSession,
	agentProfileID, executorProfileID *string,
	executorID string,
	agentProfileExplicit, executorProfileExplicit bool,
) string {
	if !isWorkflowSwitchedSession(session) {
		return executorID
	}
	if !agentProfileExplicit && session.AgentProfileID != "" {
		*agentProfileID = session.AgentProfileID
	}
	if executorProfileExplicit {
		return executorID
	}
	if session.ExecutorProfileID != "" {
		*executorProfileID = session.ExecutorProfileID
		return ""
	}
	if session.ExecutorID != "" {
		*executorProfileID = ""
		return session.ExecutorID
	}
	return executorID
}

func isWorkflowSwitchedSession(session *models.TaskSession) bool {
	if session == nil {
		return false
	}
	return metadataString(session.Metadata, models.SessionMetaKeyCreatedBy) == models.SessionCreatedByWorkflowSwitch
}

func inheritFromSession(session *models.TaskSession, agentProfileID, executorProfileID *string, inheritExecutor bool) string {
	if *agentProfileID == "" {
		*agentProfileID = session.AgentProfileID
	}
	if !inheritExecutor {
		return ""
	}
	if *executorProfileID == "" {
		*executorProfileID = session.ExecutorProfileID
	}
	if *executorProfileID != "" {
		return ""
	}
	return session.ExecutorID
}

func (h *Handlers) inheritFromTaskMetadata(ctx context.Context, taskID string, agentProfileID, executorProfileID *string, executorID string) string {
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return executorID
	}
	inheritMetadataValue(task.Metadata, models.MetaKeyAgentProfileID, agentProfileID)
	inheritMetadataValue(task.Metadata, models.MetaKeyExecutorProfileID, executorProfileID)
	if executorID == "" && *executorProfileID == "" {
		executorID = metadataString(task.Metadata, models.MetaKeyExecutorID)
	}
	return executorID
}

func inheritMetadataValue(metadata map[string]interface{}, key string, target *string) {
	if *target != "" {
		return
	}
	*target = metadataString(metadata, key)
}

func metadataString(metadata map[string]interface{}, key string) string {
	if v, ok := metadata[key].(string); ok && v != "" {
		return v
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

// handleAddBranchToTask attaches a new (repository, checkout_branch) pair to
// an existing task. Mirrors create-time multi-repo attachment but additive:
// the same repository may be added on a different branch, materializing a
// second worktree under the task's directory.
func (h *Handlers) handleAddBranchToTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID         string `json:"task_id"`
		RepositoryID   string `json:"repository_id"`
		LocalPath      string `json:"local_path"`
		GitHubURL      string `json:"github_url"`
		BaseBranch     string `json:"base_branch"`
		CheckoutBranch string `json:"checkout_branch"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	// Mutual exclusion across the three repo identifiers. resolveRepoInput
	// applies a silent precedence (repository_id > github_url > local_path),
	// so an agent that accidentally passes two of them gets a behaviour
	// change with no signal. Reject early instead so the agent sees the
	// mistake.
	if locatorCount := boolCount(req.RepositoryID != "", req.LocalPath != "", req.GitHubURL != ""); locatorCount > 1 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"pass at most one of repository_id, github_url, local_path", nil)
	}
	// repository_id / local_path / github_url are all optional: the service
	// defaults to the task's only repository (or its primary row) when none
	// is supplied. Multi-repo tasks force the agent to pass one explicitly
	// via the service-level error. local_path and github_url are
	// agent-ergonomic alternatives — when supplied the service resolves
	// them through the same workspace-scoped find-or-create path used by
	// create_task.
	taskRepo, err := h.taskSvc.AddBranchToTask(ctx, service.AddBranchToTaskRequest{
		TaskID:         req.TaskID,
		RepositoryID:   req.RepositoryID,
		LocalPath:      req.LocalPath,
		GitHubURL:      req.GitHubURL,
		BaseBranch:     req.BaseBranch,
		CheckoutBranch: req.CheckoutBranch,
	})
	if err != nil {
		h.logger.Error("failed to add branch to task", zap.Error(err))
		code := classifyAddBranchError(err)
		return ws.NewError(msg.ID, msg.Action, code, "Failed to add branch: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"id":              taskRepo.ID,
		keyTaskID:         taskRepo.TaskID,
		keyRepositoryID:   taskRepo.RepositoryID,
		keyBaseBranch:     taskRepo.BaseBranch,
		keyCheckoutBranch: taskRepo.CheckoutBranch,
		keyPosition:       taskRepo.Position,
	})
}

// handleUpdateRepositoryBaseBranch updates the base_branch on a single
// task_repositories row. The agentctl side is notified live via the service's
// AgentBaseBranchPusher hook so the changes-panel diff stats reflect the new
// base immediately, not just at next session start.
func (h *Handlers) handleUpdateRepositoryBaseBranch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID           string `json:"task_id"`
		TaskRepositoryID string `json:"task_repository_id"`
		BaseBranch       string `json:"base_branch"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.TaskRepositoryID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_repository_id is required", nil)
	}
	taskRepo, err := h.taskSvc.UpdateRepositoryBaseBranch(ctx, service.UpdateRepositoryBaseBranchRequest{
		TaskID:           req.TaskID,
		TaskRepositoryID: req.TaskRepositoryID,
		BaseBranch:       req.BaseBranch,
	})
	if err != nil {
		h.logger.Error("failed to update repository base branch", zap.Error(err))
		if errors.Is(err, service.ErrTaskRepositoryNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
		}
		// Caller-facing validation messages (required-field, invalid ref
		// name) pass through verbatim so MCP agents can react; internal
		// faults stay opaque so DB-level details don't leak.
		if isValidationError(err) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update base branch", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"id":              taskRepo.ID,
		keyTaskID:         taskRepo.TaskID,
		keyRepositoryID:   taskRepo.RepositoryID,
		keyBaseBranch:     taskRepo.BaseBranch,
		keyCheckoutBranch: taskRepo.CheckoutBranch,
		keyPosition:       taskRepo.Position,
	})
}

// boolCount returns how many of the supplied boolean flags are true. Used
// to enforce mutual exclusion across optional input fields without a chain
// of nested ifs.
func boolCount(flags ...bool) int {
	n := 0
	for _, b := range flags {
		if b {
			n++
		}
	}
	return n
}

// isValidationError matches the user-facing fragments emitted by the
// service-layer validators (required fields, invalid ref names). Shared by
// every MCP write handler so service-side message tweaks need only one
// place to flow through to the MCP error classification. Kept narrow on
// purpose — DB / IO failures often carry "invalid" in their message and
// must surface as InternalError, not Validation.
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "is required") ||
		strings.Contains(msg, "not allowed in a git ref name")
}

// classifyAddBranchError maps service-layer add_branch failures to ws error
// codes so MCP agents can react to user-fixable input mistakes (missing
// task, duplicate branch, wrong executor) instead of treating them as
// backend faults.
func classifyAddBranchError(err error) string {
	if err == nil {
		return ws.ErrorCodeInternalError
	}
	if errors.Is(err, taskrepo.ErrTaskNotFound) {
		return ws.ErrorCodeNotFound
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "does not belong to task's workspace"):
		return ws.ErrorCodeNotFound
	case strings.Contains(msg, "is already attached"),
		strings.Contains(msg, "conflicts with existing branch"):
		return ws.ErrorCodeConflict
	case strings.Contains(msg, "repository_id is required"),
		strings.Contains(msg, "only supported on the worktree executor"),
		strings.Contains(msg, "task_id is required"),
		strings.Contains(msg, "cannot resolve base_branch"):
		return ws.ErrorCodeValidation
	case strings.Contains(msg, "GitHub URL"),
		strings.Contains(msg, "github.com/owner/repo"),
		strings.Contains(msg, "does not belong to workspace"):
		// User-fixable failures from ResolveRepositoryRef / parseGitHubRepoURL:
		// malformed URL, non-github host, cross-workspace repository_id.
		// Narrow patterns (not a broad "resolve repository:" prefix) so
		// downstream DB / system errors from CreateRepository / ListRepositories
		// still classify as InternalError.
		return ws.ErrorCodeValidation
	}
	return ws.ErrorCodeInternalError
}

// handleStepComplete records the agent's explicit step-completion signal
// (ADR 0015). The handler:
//
//   - Loads the session and the task to identify the current workflow step.
//   - Dedupes: if a pending signal already exists for the same step, returns
//     {accepted: false, reason: "already_signaled"} without overwriting.
//     When the session is WAITING_FOR_INPUT, the bus event is re-published
//     so a failed first-attempt publish can still drive the subscriber.
//   - Otherwise writes a PendingStepCompletionSignal blob under
//     TaskSession.Metadata[SessionMetaKeyPendingStepCompletion] via
//     SetSessionMetadataKey (json_set — preserves other metadata keys).
//   - Publishes events.WorkflowStepCompletionSignaled so the orchestrator
//     subscriber can drive the on_turn_complete transition for steps with
//     AutoAdvanceRequiresSignal=true. Steps that don't opt in ignore the
//     signal entirely; the bag entry is cleared on the next turn start
//     (no separate audit trail is persisted).
//
// Idempotency is intentionally lossy — a second call within the same step
// silently keeps the first signal's payload (summary/handoff/blockers). The
// orchestrator treats the first signal as authoritative.
func (h *Handlers) handleStepComplete(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
		Summary   string `json:"summary"`
		Handoff   string `json:"handoff"`
		Blockers  string `json:"blockers"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" || req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id and session_id are required", nil)
	}
	if strings.TrimSpace(req.Summary) == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "summary is required", nil)
	}

	session, task, errMsg, err := h.resolveStepCompleteTarget(ctx, msg, req.TaskID, req.SessionID)
	if errMsg != nil {
		return errMsg, err
	}

	// Idempotency: if a pending signal exists for the current step, return
	// without overwriting. A stale signal for a different step (left over
	// from a transition that hasn't yet cleared the bag) is treated as
	// absent and overwritten — the new step's signal supersedes.
	//
	// Re-publish on the dedup path when the session is WAITING_FOR_INPUT.
	// Without this, an agent's retry after a publish failure short-circuits
	// to `already_signaled` without firing the out-of-band subscriber,
	// leaving the session stuck until the user replies. Publish is
	// idempotent on the subscriber side (it re-checks bag + step), so a
	// double-fire when the first publish actually landed is harmless.
	if existing, ok := models.LoadPendingStepSignal(session.Metadata); ok && existing.StepID == task.WorkflowStepID {
		if session.State == models.TaskSessionStateWaitingForInput {
			if errMsg, err := h.publishStepCompletionEvent(ctx, msg, req.TaskID, req.SessionID, task.WorkflowStepID, existing); errMsg != nil {
				return errMsg, err
			}
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"accepted": false,
			"reason":   "already_signaled",
		})
	}

	signal := models.PendingStepCompletionSignal{
		StepID:     task.WorkflowStepID,
		Source:     models.StepCompletionSourceAgent,
		Summary:    strings.TrimSpace(req.Summary),
		Handoff:    strings.TrimSpace(req.Handoff),
		Blockers:   strings.TrimSpace(req.Blockers),
		SignaledAt: time.Now().UTC(),
	}
	if err := h.sessionRepo.SetSessionMetadataKey(ctx, req.SessionID, models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		h.logger.Error("failed to persist step-completion signal",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to record signal", nil)
	}

	if errMsg, err := h.publishStepCompletionEvent(ctx, msg, req.TaskID, req.SessionID, task.WorkflowStepID, signal); errMsg != nil {
		return errMsg, err
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"accepted":    true,
		"step_id":     task.WorkflowStepID,
		"signaled_at": signal.SignaledAt,
	})
}

// resolveStepCompleteTarget loads the session + task the signal applies to
// and runs the up-front validation (ownership, terminal-state guard,
// workflow-step presence). Returns a populated session+task pair on success,
// or a ready-to-send WS error envelope (and its marshal error if any) on
// any failed precondition.
func (h *Handlers) resolveStepCompleteTarget(
	ctx context.Context, msg *ws.Message, taskID, sessionID string,
) (*models.TaskSession, *models.Task, *ws.Message, error) {
	session, err := h.sessionRepo.GetTaskSession(ctx, sessionID)
	if err != nil {
		// Session repo has no exported not-found sentinel; classify by
		// substring and treat anything else as transient so the agent
		// retries instead of abandoning the session.
		if strings.Contains(err.Error(), "not found") {
			errMsg, mErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found", nil)
			return nil, nil, errMsg, mErr
		}
		h.logger.Error("step_complete: failed to load session",
			zap.String("session_id", sessionID), zap.Error(err))
		errMsg, mErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load session", nil)
		return nil, nil, errMsg, mErr
	}
	if session.TaskID != taskID {
		errMsg, mErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session does not belong to task", nil)
		return nil, nil, errMsg, mErr
	}
	// Terminal sessions cannot consume a signal: the orchestrator's
	// out-of-band subscriber short-circuits on every state other than
	// WAITING_FOR_INPUT, and no future turn-end will fire on a closed
	// session. Reject up front so the agent gets a clear error instead of
	// `accepted: true` followed by silent no-op.
	switch session.State {
	case models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled:
		errMsg, mErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"cannot signal completion for a terminal session (state: "+string(session.State)+")", nil)
		return nil, nil, errMsg, mErr
	}

	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil {
		// Task repo exports ErrTaskNotFound; anything else is a transient
		// load failure that the agent should retry rather than interpret
		// as "task gone".
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			errMsg, mErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
			return nil, nil, errMsg, mErr
		}
		h.logger.Error("step_complete: failed to load task",
			zap.String("task_id", taskID), zap.Error(err))
		errMsg, mErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to load task", nil)
		return nil, nil, errMsg, mErr
	}
	if task.WorkflowStepID == "" {
		errMsg, mErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task has no current workflow step", nil)
		return nil, nil, errMsg, mErr
	}
	return session, task, nil, nil
}

// publishStepCompletionEvent emits the bus event the orchestrator's
// out-of-band subscriber listens for. If publish fails the bag is already
// persisted but the subscriber will not fire — surface the error to the
// agent so it can retry. The handler-level idempotency guard guarantees the
// retry either succeeds end-to-end or short-circuits with `already_signaled`
// once the publish lands. Returns (nil, nil) on success or when no bus is wired.
func (h *Handlers) publishStepCompletionEvent(
	ctx context.Context, msg *ws.Message, taskID, sessionID, stepID string,
	signal models.PendingStepCompletionSignal,
) (*ws.Message, error) {
	if h.eventBus == nil {
		return nil, nil
	}
	if err := h.eventBus.Publish(ctx, events.WorkflowStepCompletionSignaled, bus.NewEvent(
		events.WorkflowStepCompletionSignaled, "mcp-handlers",
		map[string]interface{}{
			"task_id":     taskID,
			"session_id":  sessionID,
			"step_id":     stepID,
			"source":      signal.Source,
			"summary":     signal.Summary,
			"signaled_at": signal.SignaledAt,
		},
	)); err != nil {
		h.logger.Error("failed to publish step-completion signal",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to notify orchestrator (signal persisted; retry)", nil)
	}
	return nil, nil
}

// handleMessageTask sends a prompt to an existing task on behalf of an agent
// in another task. The MCP server (agentctl) injects the sender's task_id and
// session_id into the payload; this handler validates the sender, looks up its
// title, wraps the prompt in a <kandev-system> attribution block (so the
// receiving agent knows the message is from a peer agent), and dispatches via
// one of three paths depending on the target session state:
//
//   - RUNNING/STARTING : message is queued and drained at turn end
//   - WAITING/COMPLETED: message is recorded and the agent is prompted (auto-resuming if needed)
//   - CREATED          : message is recorded then the agent is started with it as initial prompt
//
// Strict validation: missing sender_task_id, self-message, and unknown sender
// task all reject with an MCP error rather than silently delivering an
// unattributed message.
func (h *Handlers) handleMessageTask(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID          string `json:"task_id"`
		Prompt          string `json:"prompt"`
		SenderTaskID    string `json:"sender_task_id"`
		SenderSessionID string `json:"sender_session_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Prompt == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "prompt is required", nil)
	}
	if req.SenderTaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "sender_task_id is required (the calling agent's MCP server must supply this)", nil)
	}
	if req.SenderTaskID == req.TaskID {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task cannot send a message to itself", nil)
	}

	// Sender lookup is global, not workspace-scoped: cross-workspace agent
	// messaging is intentionally allowed (badge URL handles cross-workspace
	// nav). Task IDs are UUIDs, so this is not exploitable in practice — and
	// scoping would require a product-level decision about cross-workspace
	// auth/visibility/discovery that we don't want to bake in here.
	senderTask, err := h.taskSvc.GetTask(ctx, req.SenderTaskID)
	if err != nil || senderTask == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "sender task not found", nil)
	}

	// Verify the target task exists before looking up its session, so a bad
	// task_id (e.g. a truncated UUID prefix) reports "task not found" instead
	// of the misleading "no primary session" error from the session lookup.
	// This is purely an existence check — GetTask returns a wrapped
	// ErrTaskNotFound on no-rows, never (nil, nil).
	if _, err := h.taskSvc.GetTask(ctx, req.TaskID); err != nil {
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound,
				"target task not found: "+req.TaskID+" (pass the full task UUID, not a truncated prefix)", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to look up target task: "+err.Error(), nil)
	}

	session, err := h.taskSvc.GetPrimarySession(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, taskrepo.ErrNoPrimarySession) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound,
				"target task exists but has no active session", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to get session for task: "+err.Error(), nil)
	}

	wrappedPrompt, senderMeta := wrapAgentMessage(req.Prompt, senderTask, req.SenderSessionID)

	result, err := h.dispatchTaskMessage(ctx, req.TaskID, session, wrappedPrompt, senderMeta)
	if err != nil {
		var qfErr *queueFullDispatchError
		if errors.As(err, &qfErr) {
			return ws.NewError(msg.ID, msg.Action, messagequeue.QueueFullErrorCode,
				fmt.Sprintf("target task has %d queued messages (max %d) — retry after the next turn completes", qfErr.queueSize, qfErr.max),
				qfErr.toPayload())
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id":    req.TaskID,
		"session_id": result.sessionID,
		"status":     result.status,
	})
}

// handleGetTaskConversation returns paginated conversation history for a task.
// If session_id is omitted, it uses the task's primary session.
func (h *Handlers) handleGetTaskConversation(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	req, errResp := parseTaskConversationRequest(msg)
	if errResp != nil {
		return errResp, nil
	}

	session, errResp := h.resolveConversationSession(ctx, msg, req.TaskID, req.SessionID)
	if errResp != nil {
		return errResp, nil
	}

	messages, hasMore, err := h.taskSvc.ListMessagesPaginated(ctx, service.ListMessagesRequest{
		TaskSessionID: session.ID,
		Limit:         conversationLimit(req.Limit),
		Before:        req.Before,
		After:         req.After,
		Sort:          req.Sort,
	})
	if err != nil {
		h.logger.Error("failed to list task conversation", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task conversation", nil)
	}

	result := filterAndConvertMessages(messages, req.Types)
	cursor := conversationCursor(messages)

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id":    req.TaskID,
		"session_id": session.ID,
		"messages":   result,
		"total":      len(result),
		"has_more":   hasMore,
		"cursor":     cursor,
	})
}

type taskConversationRequest struct {
	TaskID    string   `json:"task_id"`
	SessionID string   `json:"session_id"`
	Limit     int      `json:"limit"`
	Before    string   `json:"before"`
	After     string   `json:"after"`
	Sort      string   `json:"sort"`
	Types     []string `json:"message_types"`
}

func parseTaskConversationRequest(msg *ws.Message) (*taskConversationRequest, *ws.Message) {
	var req taskConversationRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error())
	}
	if req.TaskID == "" {
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required")
	}
	if req.Before != "" && req.After != "" {
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "only one of before or after can be set")
	}
	if req.Sort != "" && req.Sort != "asc" && req.Sort != "desc" {
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "sort must be asc or desc")
	}
	if req.Limit < 0 {
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "limit must be non-negative")
	}
	return &req, nil
}

func (h *Handlers) resolveConversationSession(ctx context.Context, msg *ws.Message, taskID, sessionID string) (*models.TaskSession, *ws.Message) {
	if sessionID != "" {
		session, err := h.taskSvc.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil {
			return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "session not found")
		}
		if session.TaskID != taskID {
			return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id does not belong to task_id")
		}
		return session, nil
	}
	session, err := h.taskSvc.GetPrimarySession(ctx, taskID)
	if err != nil || session == nil {
		return nil, wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task has no session")
	}
	return session, nil
}

func wsError(id, action, code, message string) *ws.Message {
	resp, _ := ws.NewError(id, action, code, message, nil)
	return resp
}

func filterAndConvertMessages(messages []*models.Message, types []string) []*v1.Message {
	filterTypes := make(map[string]struct{}, len(types))
	for _, mt := range types {
		if mt == "" {
			continue
		}
		filterTypes[mt] = struct{}{}
	}

	result := make([]*v1.Message, 0, len(messages))
	for _, message := range messages {
		if len(filterTypes) > 0 {
			if _, ok := filterTypes[string(message.Type)]; !ok {
				continue
			}
		}
		result = append(result, message.ToAPI())
	}
	return result
}

func conversationLimit(requested int) int {
	if requested > 0 {
		return requested
	}
	return service.DefaultMessagesPageSize
}

func conversationCursor(messages []*models.Message) string {
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].ID
}

// queueFullDispatchError is returned by dispatchTaskMessage when an inter-task
// message can't be queued because the target session's queue is full. It
// carries enough metadata for handleMessageTask to surface a structured
// "queue_full" error to the calling agent so its LLM can decide whether to
// retry, abort, or message a different task.
type queueFullDispatchError struct {
	sessionID string
	queueSize int
	max       int
	entries   []messagequeue.QueuedMessage
}

func (e *queueFullDispatchError) Error() string {
	return fmt.Sprintf("queue full: %d/%d messages pending for session %s", e.queueSize, e.max, e.sessionID)
}

// toPayload builds the structured "data" body for the MCP error response.
// Sender / queued_at fields surface enough context for the LLM to reason about
// the wedge state without leaking the queued message contents.
func (e *queueFullDispatchError) toPayload() map[string]interface{} {
	queued := make([]map[string]interface{}, 0, len(e.entries))
	for _, entry := range e.entries {
		queued = append(queued, map[string]interface{}{
			"id":        entry.ID,
			"sender":    entry.QueuedBy,
			"queued_at": entry.QueuedAt,
		})
	}
	// The WS error envelope already carries the code; we duplicate it here so
	// callers reading the structured details body still see it without parsing
	// the envelope. Tests assert on details.error directly.
	return map[string]interface{}{
		errorField:        messagequeue.QueueFullErrorCode,
		"queue_size":      e.queueSize,
		"max":             e.max,
		"retry_after":     "next_turn",
		"queued_messages": queued,
	}
}

// errorField names the well-known structured details key used to surface error
// codes in MCP tool responses (extracted to satisfy goconst's repeated-string rule).
const errorField = "error"

// MCP payload / response keys reused across multiple handlers. Extracted so
// goconst doesn't flag the literals as repeated, and so a future rename of
// a wire-protocol key updates every handler in one place.
const (
	keyTaskID           = "task_id"
	keyRepositoryID     = "repository_id"
	keyTaskRepositoryID = "task_repository_id"
	keyBaseBranch       = "base_branch"
	keyCheckoutBranch   = "checkout_branch"
	keyPosition         = "position"
)

type taskMessageDispatchResult struct {
	status    string
	sessionID string
}

type taskMessageReviewRollback struct {
	changed        bool
	restoreTask    bool
	taskState      v1.TaskState
	workflowStepID string
	sessions       []taskMessageSessionRollback
	sessionIDs     map[string]struct{}
	selectedID     string
	queues         map[string]taskMessageQueueRollback
}

type taskMessageSessionRollback struct {
	sessionID            string
	state                models.TaskSessionState
	error                string
	completedAt          *time.Time
	isPrimary            bool
	agentProfileID       string
	executorProfileID    string
	agentProfileSnapshot map[string]interface{}
	metadata             map[string]interface{}
}

type taskMessageQueueRollback struct {
	entries        []messagequeue.QueuedMessage
	hadPendingMove bool
	pendingMove    *messagequeue.PendingMove
}

type taskMessageSessionRollbackRepository interface {
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	UpdateTaskSession(ctx context.Context, session *models.TaskSession) error
	SetSessionPrimary(ctx context.Context, sessionID string) error
	DeleteTaskSession(ctx context.Context, id string) error
	UpdateSessionMetadata(ctx context.Context, sessionID string, metadata map[string]interface{}) error
}

type taskMessageExecutorReader interface {
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
}

func (r *taskMessageReviewRollback) captureSessions(ctx context.Context, repo SessionRepository, taskID string, fallback *models.TaskSession) error {
	if !r.changed || fallback == nil {
		return nil
	}
	sessions := []*models.TaskSession{fallback}
	if snapshotRepo, ok := repo.(interface {
		ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error)
	}); ok {
		listed, err := snapshotRepo.ListTaskSessions(ctx, taskID)
		if err != nil {
			return err
		}
		sessions = listed
	}
	r.sessionIDs = make(map[string]struct{}, len(sessions))
	r.sessions = make([]taskMessageSessionRollback, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		r.sessionIDs[session.ID] = struct{}{}
		r.sessions = append(r.sessions, captureTaskMessageSession(session))
	}
	return nil
}

func (r *taskMessageReviewRollback) captureQueues(ctx context.Context, queue *messagequeue.Service) {
	if !r.changed || queue == nil || len(r.sessions) == 0 {
		return
	}
	r.queues = make(map[string]taskMessageQueueRollback, len(r.sessions))
	for _, session := range r.sessions {
		status := queue.GetStatus(ctx, session.sessionID)
		snapshot := taskMessageQueueRollback{
			entries: cloneTaskMessageQueuedMessages(status.Entries),
		}
		if move, ok := queue.GetPendingMove(ctx, session.sessionID); ok {
			snapshot.hadPendingMove = true
			snapshot.pendingMove = cloneTaskMessagePendingMove(move)
		}
		r.queues[session.sessionID] = snapshot
	}
}

func (r *taskMessageReviewRollback) captureSelectedSession(session *models.TaskSession) {
	if !r.changed || session == nil {
		return
	}
	r.selectedID = session.ID
}

func captureTaskMessageSession(session *models.TaskSession) taskMessageSessionRollback {
	var completedAt *time.Time
	if session.CompletedAt != nil {
		copy := *session.CompletedAt
		completedAt = &copy
	}
	snapshot := taskMessageSessionRollback{
		sessionID:            session.ID,
		state:                session.State,
		error:                session.ErrorMessage,
		completedAt:          completedAt,
		isPrimary:            session.IsPrimary,
		agentProfileID:       session.AgentProfileID,
		executorProfileID:    session.ExecutorProfileID,
		agentProfileSnapshot: cloneTaskMessageMetadataMap(session.AgentProfileSnapshot),
	}
	if session.Metadata != nil {
		snapshot.metadata = cloneTaskMessageMetadataMap(session.Metadata)
	}
	return snapshot
}

func cloneTaskMessageMetadataMap(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		cloned[key] = cloneTaskMessageMetadataValue(value)
	}
	return cloned
}

func cloneTaskMessageMetadataValue(value interface{}) interface{} {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var cloned interface{}
	if err := json.Unmarshal(data, &cloned); err != nil {
		return value
	}
	return cloned
}

func cloneTaskMessageQueuedMessages(entries []messagequeue.QueuedMessage) []messagequeue.QueuedMessage {
	if len(entries) == 0 {
		return nil
	}
	cloned := make([]messagequeue.QueuedMessage, 0, len(entries))
	for _, entry := range entries {
		copy := entry
		copy.Metadata = cloneTaskMessageMetadataMap(entry.Metadata)
		copy.Attachments = append([]messagequeue.MessageAttachment(nil), entry.Attachments...)
		cloned = append(cloned, copy)
	}
	return cloned
}

func cloneTaskMessagePendingMove(move *messagequeue.PendingMove) *messagequeue.PendingMove {
	if move == nil {
		return nil
	}
	copy := *move
	return &copy
}

// dispatchTaskMessage routes a message to the right delivery path based on session state.
// Returns the action taken: "queued", "sent", or "started".
//
// metadata is the Message.Metadata map to attach to the resulting user message
// row (sender_task_id, sender_task_title, sender_session_id when called from
// handleMessageTask). It is propagated to all three delivery paths so the
// receiving task's chat displays the sender badge consistently.
func (h *Handlers) dispatchTaskMessage(ctx context.Context, taskID string, session *models.TaskSession, prompt string, metadata map[string]interface{}) (taskMessageDispatchResult, error) {
	if h.sessionLauncher == nil {
		return taskMessageDispatchResult{}, errors.New("orchestrator not available")
	}

	switch session.State {
	case models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return taskMessageDispatchResult{}, fmt.Errorf("session is %s — cannot send message", session.State)

	case models.TaskSessionStateRunning, models.TaskSessionStateStarting:
		return h.queueTaskMessage(ctx, taskID, session, prompt, metadata)

	default:
		reviewRollback, err := h.ensureTaskInProgressForTaskMessage(ctx, taskID)
		if err != nil {
			return taskMessageDispatchResult{}, err
		}
		if err := reviewRollback.captureSessions(ctx, h.sessionRepo, taskID, session); err != nil {
			h.restoreTaskReviewForTaskMessage(ctx, taskID, reviewRollback)
			return taskMessageDispatchResult{}, err
		}
		reviewRollback.captureQueues(ctx, h.sessionLauncher.GetMessageQueue())
		session, err := h.prepareSessionForTaskMessage(ctx, taskID, session)
		if err != nil {
			h.restoreTaskReviewForTaskMessage(ctx, taskID, reviewRollback)
			return taskMessageDispatchResult{}, err
		}
		reviewRollback.captureSelectedSession(session)
		result, err := h.dispatchPreparedTaskMessage(ctx, taskID, session, prompt, metadata)
		if err != nil {
			h.restoreTaskReviewForTaskMessage(ctx, taskID, reviewRollback)
		}
		return result, err
	}
}

func (h *Handlers) dispatchPreparedTaskMessage(ctx context.Context, taskID string, session *models.TaskSession, prompt string, metadata map[string]interface{}) (taskMessageDispatchResult, error) {
	switch session.State {
	case models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return taskMessageDispatchResult{}, fmt.Errorf("session is %s — cannot send message", session.State)
	case models.TaskSessionStateRunning, models.TaskSessionStateStarting:
		return h.queueTaskMessage(ctx, taskID, session, prompt, metadata)
	default:
		if h.shouldStartTaskMessageSession(ctx, session) {
			// Record before starting so the message is tied to the turn produced
			// by launch. If launch fails, delete the row below.
			recorded := h.recordUserMessage(ctx, taskID, session.ID, prompt, metadata)
			if _, err := h.sessionLauncher.StartCreatedSession(ctx, taskID, session.ID, session.AgentProfileID, prompt, true, false, true, nil); err != nil {
				h.deleteRecordedUserMessage(ctx, recorded)
				return taskMessageDispatchResult{}, fmt.Errorf("failed to start session: %w", err)
			}
			return taskMessageDispatchResult{status: "started", sessionID: session.ID}, nil
		}
		// Record before prompting so the message is tied to the turn that
		// PromptTask dispatches. If dispatch fails, delete the row below so a
		// REVIEW rollback does not keep a prompt the agent never saw.
		recorded := h.recordUserMessage(ctx, taskID, session.ID, prompt, metadata)
		status, err := h.promptWithAutoResume(ctx, taskID, session.ID, prompt)
		if err != nil {
			h.deleteRecordedUserMessage(ctx, recorded)
			return taskMessageDispatchResult{}, err
		}
		return taskMessageDispatchResult{status: status, sessionID: session.ID}, nil
	}
}

func (h *Handlers) shouldStartTaskMessageSession(ctx context.Context, session *models.TaskSession) bool {
	if session.State == models.TaskSessionStateCreated {
		return true
	}
	// on_turn_start may mark a never-launched CREATED session as
	// WAITING_FOR_INPUT before an executor row exists. It may also switch to a
	// fresh waiting primary session. In both cases the first message still needs
	// the launch path; already-bound waiting sessions use prompt/resume.
	if session.State != models.TaskSessionStateWaitingForInput {
		return false
	}
	if session.AgentExecutionID == "" {
		return true
	}
	reader, ok := h.sessionRepo.(taskMessageExecutorReader)
	if !ok {
		return false
	}
	running, err := reader.GetExecutorRunningBySessionID(ctx, session.ID)
	return err == nil && running != nil && running.Status == models.ExecutorRunningStatusPrepared
}

func (h *Handlers) queueTaskMessage(ctx context.Context, taskID string, session *models.TaskSession, prompt string, metadata map[string]interface{}) (taskMessageDispatchResult, error) {
	queue := h.sessionLauncher.GetMessageQueue()
	if queue == nil {
		return taskMessageDispatchResult{}, errors.New("message queue not available")
	}
	if _, err := queue.QueueMessageWithMetadata(ctx, session.ID, taskID, prompt, "", messagequeue.QueuedByAgent, false, nil, metadata); err != nil {
		if errors.Is(err, messagequeue.ErrQueueFull) {
			status := queue.GetStatus(ctx, session.ID)
			return taskMessageDispatchResult{}, &queueFullDispatchError{
				sessionID: session.ID,
				queueSize: status.Count,
				max:       status.Max,
				entries:   status.Entries,
			}
		}
		return taskMessageDispatchResult{}, fmt.Errorf("failed to queue message: %w", err)
	}
	h.publishQueueStatusEvent(ctx, session.ID, queue)
	return taskMessageDispatchResult{status: "queued", sessionID: session.ID}, nil
}

func (h *Handlers) prepareSessionForTaskMessage(ctx context.Context, taskID string, session *models.TaskSession) (*models.TaskSession, error) {
	if err := h.sessionLauncher.ProcessOnTurnStart(ctx, taskID, session.ID); err != nil {
		return nil, fmt.Errorf("failed to process on_turn_start for task message: %w", err)
	}
	resolved, err := h.resolveSessionAfterTaskMessageTurnStart(ctx, taskID, session)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

func (h *Handlers) ensureTaskInProgressForTaskMessage(ctx context.Context, taskID string) (taskMessageReviewRollback, error) {
	if h.taskSvc == nil {
		return taskMessageReviewRollback{}, errors.New("task service not available")
	}
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil {
		return taskMessageReviewRollback{}, err
	}
	rollback := taskMessageReviewRollback{
		changed:        true,
		restoreTask:    true,
		taskState:      task.State,
		workflowStepID: task.WorkflowStepID,
	}
	if task.State != v1.TaskStateReview {
		return rollback, nil
	}
	if task.AssigneeAgentProfileID != "" {
		return rollback, nil
	}
	if _, err := h.taskSvc.UpdateTaskState(ctx, taskID, v1.TaskStateInProgress); err != nil {
		return taskMessageReviewRollback{}, fmt.Errorf("failed to transition task from REVIEW to IN_PROGRESS for task message: %w", err)
	}
	h.logger.Info("task transitioned from REVIEW to IN_PROGRESS for task message",
		zap.String("task_id", taskID))
	return rollback, nil
}

func (h *Handlers) restoreTaskReviewForTaskMessage(ctx context.Context, taskID string, rollback taskMessageReviewRollback) {
	if !rollback.changed {
		return
	}
	if err := h.restoreTaskMessageSessions(ctx, rollback); err != nil {
		h.logger.Warn("failed to restore task session after task message dispatch failure",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	if !rollback.restoreTask {
		return
	}
	taskState := rollback.taskState
	if taskState == "" {
		taskState = v1.TaskStateReview
	}
	if _, err := h.taskSvc.UpdateTask(ctx, taskID, &service.UpdateTaskRequest{
		State:          &taskState,
		WorkflowStepID: &rollback.workflowStepID,
	}); err != nil {
		h.logger.Warn("failed to restore task to REVIEW after task message dispatch failure",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
}

func (h *Handlers) restoreTaskMessageSessions(ctx context.Context, rollback taskMessageReviewRollback) error {
	if len(rollback.sessions) == 0 {
		return nil
	}
	repo, ok := h.sessionRepo.(taskMessageSessionRollbackRepository)
	if !ok {
		return errors.New("session rollback repository not available")
	}
	for _, snapshot := range rollback.sessions {
		if err := restoreTaskMessageSessionSnapshot(ctx, repo, snapshot); err != nil {
			return err
		}
	}
	if err := h.restoreTaskMessageQueues(ctx, rollback); err != nil {
		return err
	}
	return h.restoreSelectedTaskMessageSession(ctx, repo, rollback)
}

func (h *Handlers) restoreSelectedTaskMessageSession(ctx context.Context, repo taskMessageSessionRollbackRepository, rollback taskMessageReviewRollback) error {
	if rollback.selectedID == "" {
		return nil
	}
	primaryID := rollback.primarySessionID()
	if _, ok := rollback.sessionIDs[rollback.selectedID]; ok {
		return nil
	}
	if primaryID != "" && rollback.selectedID != primaryID {
		h.clearTaskMessageQueue(ctx, rollback.selectedID)
	}
	return repo.DeleteTaskSession(ctx, rollback.selectedID)
}

func (r taskMessageReviewRollback) primarySessionID() string {
	for _, session := range r.sessions {
		if session.isPrimary {
			return session.sessionID
		}
	}
	return ""
}

func (h *Handlers) restoreTaskMessageQueues(ctx context.Context, rollback taskMessageReviewRollback) error {
	if len(rollback.queues) == 0 {
		return nil
	}
	queue := h.sessionLauncher.GetMessageQueue()
	if queue == nil {
		return nil
	}
	for sessionID, snapshot := range rollback.queues {
		if err := h.restoreTaskMessageQueue(ctx, queue, sessionID, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handlers) restoreTaskMessageQueue(ctx context.Context, queue *messagequeue.Service, sessionID string, snapshot taskMessageQueueRollback) error {
	var pendingMove *messagequeue.PendingMove
	if snapshot.hadPendingMove {
		pendingMove = cloneTaskMessagePendingMove(snapshot.pendingMove)
	}
	return queue.RestoreSession(ctx, sessionID, cloneTaskMessageQueuedMessages(snapshot.entries), pendingMove)
}

func (h *Handlers) clearTaskMessageQueue(ctx context.Context, sessionID string) {
	queue := h.sessionLauncher.GetMessageQueue()
	if queue == nil {
		return
	}
	if _, err := queue.CancelAll(ctx, sessionID); err != nil {
		h.logger.Warn("failed to clear task message queue during rollback",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
	_, _ = queue.TakePendingMove(ctx, sessionID)
}

func restoreTaskMessageSessionSnapshot(ctx context.Context, repo taskMessageSessionRollbackRepository, rollback taskMessageSessionRollback) error {
	session, err := repo.GetTaskSession(ctx, rollback.sessionID)
	if err != nil {
		return err
	}
	session.State = rollback.state
	session.ErrorMessage = rollback.error
	session.CompletedAt = rollback.completedAt
	session.IsPrimary = rollback.isPrimary
	session.AgentProfileID = rollback.agentProfileID
	session.ExecutorProfileID = rollback.executorProfileID
	session.AgentProfileSnapshot = cloneTaskMessageMetadataMap(rollback.agentProfileSnapshot)
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		return err
	}
	if rollback.isPrimary {
		if err := repo.SetSessionPrimary(ctx, rollback.sessionID); err != nil {
			return err
		}
	}
	if err := repo.UpdateSessionMetadata(ctx, rollback.sessionID, cloneTaskMessageMetadataMap(rollback.metadata)); err != nil {
		return err
	}
	return nil
}

func (h *Handlers) resolveSessionAfterTaskMessageTurnStart(ctx context.Context, taskID string, submitted *models.TaskSession) (*models.TaskSession, error) {
	if h.taskSvc == nil {
		return nil, errors.New("task service not available")
	}
	reloaded, err := h.taskSvc.GetTaskSession(ctx, submitted.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload session after on_turn_start: %w", err)
	}
	primary, err := h.taskSvc.GetPrimarySession(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve primary session after on_turn_start: %w", err)
	}
	if primary != nil && primary.ID != submitted.ID {
		return primary, nil
	}
	if reloaded.State != models.TaskSessionStateCompleted {
		return reloaded, nil
	}
	if primary == nil {
		return nil, errors.New("session was switched but no active session found")
	}
	if primary.ID != submitted.ID {
		return primary, nil
	}
	if submitted.State == models.TaskSessionStateCompleted {
		return reloaded, nil
	}
	return nil, errors.New("session was marked completed by on_turn_start but primary was not switched")
}

// recordUserMessage writes the prompt to the task's chat as a user message so it
// is visible in the conversation. Mirrors the message.add path used by the UI.
// metadata is attached to the resulting Message row (used for sender_task_id /
// sender_task_title / sender_session_id when called from handleMessageTask).
func (h *Handlers) recordUserMessage(ctx context.Context, taskID, sessionID, prompt string, metadata map[string]interface{}) *models.Message {
	if h.taskSvc == nil {
		return nil
	}
	message, err := h.taskSvc.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: sessionID,
		TaskID:        taskID,
		Content:       prompt,
		AuthorType:    "user",
		Metadata:      metadata,
	})
	if err != nil {
		h.logger.Warn("failed to record user message for message_task",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil
	}
	return message
}

func (h *Handlers) deleteRecordedUserMessage(ctx context.Context, message *models.Message) {
	if h.taskSvc == nil || message == nil {
		return
	}
	if err := h.taskSvc.DeleteMessage(ctx, message.ID); err != nil {
		h.logger.Warn("failed to delete rejected user message for message_task",
			zap.String("message_id", message.ID),
			zap.String("task_id", message.TaskID),
			zap.String("session_id", message.TaskSessionID),
			zap.Error(err))
	}
	if err := h.taskSvc.AbandonOpenTurns(ctx, message.TaskSessionID); err != nil {
		h.logger.Warn("failed to abandon rejected user message turn for message_task",
			zap.String("message_id", message.ID),
			zap.String("task_id", message.TaskID),
			zap.String("session_id", message.TaskSessionID),
			zap.Error(err))
	}
}

// promptWithAutoResume sends a prompt to a session and resumes the agent
// transparently if it has been torn down (mirrors message.add behaviour).
// Uses dispatch-only mode so the MCP tool returns once the prompt is accepted
// rather than blocking for the entire target turn.
func (h *Handlers) promptWithAutoResume(ctx context.Context, taskID, sessionID, prompt string) (string, error) {
	_, err := h.sessionLauncher.PromptTask(ctx, taskID, sessionID, prompt, "", false, nil, true)
	if err == nil {
		return "sent", nil
	}
	if !errors.Is(err, executor.ErrExecutionNotFound) {
		return "", fmt.Errorf("failed to send prompt: %w", err)
	}
	if _, resumeErr := h.sessionLauncher.ResumeTaskSession(ctx, taskID, sessionID); resumeErr != nil {
		return "", fmt.Errorf("failed to resume session: %w", resumeErr)
	}
	// ResumeTaskSession starts the agent asynchronously. Poll until the session
	// is ready to accept prompts so the retry doesn't race the agent boot.
	if waitErr := h.taskSvc.WaitForSessionReady(ctx, sessionID); waitErr != nil {
		return "", fmt.Errorf("session not ready after resume: %w", waitErr)
	}
	if _, retryErr := h.sessionLauncher.PromptTask(ctx, taskID, sessionID, prompt, "", false, nil, true); retryErr != nil {
		return "", fmt.Errorf("failed to send prompt after resume: %w", retryErr)
	}
	return "sent", nil
}

// publishQueueStatusEvent fires a queue.status_changed event so the frontend
// can update the queue indicator.
func (h *Handlers) publishQueueStatusEvent(ctx context.Context, sessionID string, queue *messagequeue.Service) {
	if h.eventBus == nil {
		return
	}
	status := queue.GetStatus(ctx, sessionID)
	_ = h.eventBus.Publish(ctx, events.MessageQueueStatusChanged, bus.NewEvent(
		events.MessageQueueStatusChanged,
		"mcp-handlers",
		map[string]interface{}{
			"session_id": sessionID,
			"entries":    status.Entries,
			"count":      status.Count,
			"max":        status.Max,
		},
	))
}

// handleAskUserQuestion creates a clarification request and blocks until the user responds.
// The agent's MCP tool call stays open (same turn) while waiting. If the agent times out,
// the event-based fallback in the orchestrator handles resuming with a new turn.
func (h *Handlers) handleAskUserQuestion(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string                   `json:"session_id"`
		TaskID    string                   `json:"task_id"`
		Questions []clarification.Question `json:"questions"`
		Context   string                   `json:"context"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}
	// Single source of truth — same validator the HTTP handler uses, so
	// duplicate IDs / bad option counts / empty prompts can't slip through
	// either path.
	if errMsg := clarification.NormalizeAndValidateQuestions(req.Questions); errMsg != "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, errMsg, nil)
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
		Questions: req.Questions,
		Context:   req.Context,
	}
	pendingID, isNew := h.clarificationSvc.CreateRequest(clarificationReq)

	// Create one chat message per question (triggers WS events to frontend).
	// If the create fails, the in-store pending entry must be cancelled too —
	// otherwise the agent's WaitForResponse would block for the full 2-hour
	// timeout while the user never sees clarification cards.
	// When dedup fires (isNew=false) the messages already exist, so skip creation.
	if isNew && h.messageCreator != nil {
		if _, err := h.messageCreator.CreateClarificationRequestMessages(
			ctx, taskID, req.SessionID, pendingID, req.Questions, req.Context,
		); err != nil {
			h.logger.Error("failed to create clarification request messages",
				zap.String("pending_id", pendingID),
				zap.String("session_id", req.SessionID),
				zap.Error(err))
			h.clarificationSvc.CancelRequest(pendingID)
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
				"failed to create clarification messages: "+err.Error(), nil)
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
		return
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
		if updatedAt, ok := h.sessionUpdatedAtForStateEvent(ctx, sessionID); ok {
			eventData["updated_at"] = updatedAt
		} else {
			h.logger.Warn("skipping session state_changed publish; could not load authoritative updated_at",
				zap.String("session_id", sessionID))
			return
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
		return
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
		if updatedAt, ok := h.sessionUpdatedAtForStateEvent(ctx, sessionID); ok {
			eventData["updated_at"] = updatedAt
		} else {
			h.logger.Warn("skipping session state_changed publish; could not load authoritative updated_at",
				zap.String("session_id", sessionID))
			return
		}
		_ = h.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
			events.TaskSessionStateChanged,
			"mcp-handlers",
			eventData,
		))
	}
}

func (h *Handlers) sessionUpdatedAtForStateEvent(ctx context.Context, sessionID string) (string, bool) {
	if session, err := h.sessionRepo.GetTaskSession(ctx, sessionID); err == nil && session != nil && !session.UpdatedAt.IsZero() {
		return session.UpdatedAt.UTC().Format(time.RFC3339Nano), true
	}
	return "", false
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
func (h *Handlers) handleClarificationTimeout(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.SessionID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
	}

	if h.sessionCanceller == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "sessionCanceller is required", nil)
	}
	cancelled := h.sessionCanceller.DetachSessionAndNotify(ctx, req.SessionID)
	h.logger.Info("detached pending clarifications on agent MCP timeout",
		zap.String("session_id", req.SessionID),
		zap.Int("count", cancelled))

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{"ok": true, "cancelled": cancelled})
}
