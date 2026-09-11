package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/authz"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type mcpCreateTaskRequest struct {
	ParentID               string               `json:"parent_id"`
	SourceTaskID           string               `json:"source_task_id"`
	SourceSessionID        string               `json:"source_session_id"`
	WorkspaceID            string               `json:"workspace_id"`
	WorkflowID             string               `json:"workflow_id"`
	WorkflowStepID         string               `json:"workflow_step_id"`
	WorkspaceMode          string               `json:"workspace_mode"`
	Title                  string               `json:"title"`
	Description            string               `json:"description"`
	Autopilot              bool                 `json:"autopilot"`
	AgentProfileID         string               `json:"agent_profile_id"`
	ExecutorProfileID      string               `json:"executor_profile_id"`
	StartAgent             *bool                `json:"start_agent"`
	Repositories           []mcpRepositoryInput `json:"repositories"`
	BaseBranch             string               `json:"base_branch"`
	BlockedBy              []string             `json:"blocked_by"`
	StartWhenUnblocked     *bool                `json:"start_when_unblocked"`
	AssigneeAgentProfileID string               `json:"assignee_agent_profile_id"`
	ExternalID             string               `json:"external_id"`
}

type mcpCreateTaskAdmission struct {
	discardInheritedSourceRepositories bool
}

type mcpCreateTaskAdmissionError struct {
	code    string
	message string
}

func (e *mcpCreateTaskAdmissionError) Error() string { return e.message }

func denyMCPCreateTask(code, message string) error {
	return &mcpCreateTaskAdmissionError{code: code, message: message}
}

func (h *Handlers) admitMCPCreateTask(
	ctx context.Context,
	req *mcpCreateTaskRequest,
) (mcpCreateTaskAdmission, error) {
	principal, hasPrincipal := mcpscope.PrincipalFromContext(ctx)
	trustedExternal := mcporigin.IsTrustedExternalTransport(ctx)
	if hasPrincipal && trustedExternal {
		return mcpCreateTaskAdmission{}, denyMCPCreateTask(
			ws.ErrorCodeUnauthorized,
			"MCP task creation has conflicting session and external caller identity",
		)
	}
	if !hasPrincipal && !trustedExternal {
		return mcpCreateTaskAdmission{}, denyMCPCreateTask(
			ws.ErrorCodeUnauthorized,
			"MCP task creation requires a trusted session or external transport",
		)
	}
	if h.taskSvc == nil {
		return mcpCreateTaskAdmission{}, denyMCPCreateTask(
			ws.ErrorCodeInternalError,
			"MCP task creation service is unavailable",
		)
	}

	if trustedExternal {
		return h.admitExternalMCPCreateTask(ctx, req)
	}
	if principal.IsAutomation() {
		return mcpCreateTaskAdmission{}, nil
	}
	if principal.Surface != mcpprofile.SurfaceKanbanTask {
		return mcpCreateTaskAdmission{}, denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"Office sessions create tasks through their skills and runtime CLI; task-creation MCP is not available",
		)
	}
	if err := h.validateMCPKanbanPrincipal(ctx, principal, req); err != nil {
		return mcpCreateTaskAdmission{}, err
	}
	return h.admitMCPDestination(ctx, req, principal.WorkspaceID, true)
}

func (h *Handlers) admitExternalMCPCreateTask(
	ctx context.Context,
	req *mcpCreateTaskRequest,
) (mcpCreateTaskAdmission, error) {
	if req.SourceTaskID != "" || req.SourceSessionID != "" {
		return mcpCreateTaskAdmission{}, denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"external MCP cannot provide session creator identity",
		)
	}
	return h.admitMCPDestination(ctx, req, "", false)
}

func (h *Handlers) validateMCPKanbanPrincipal(
	ctx context.Context,
	principal mcpscope.Principal,
	req *mcpCreateTaskRequest,
) error {
	sourceTask, err := h.taskSvc.GetTask(ctx, principal.CallerTaskID)
	if err != nil || sourceTask == nil {
		return denyMCPCreateTask(
			ws.ErrorCodeUnauthorized,
			"the session caller identity could not be verified",
		)
	}
	if err := h.validateMCPPrincipalSession(ctx, principal, sourceTask); err != nil {
		return err
	}
	sourceWorkspace, err := h.taskSvc.GetWorkspace(ctx, principal.WorkspaceID)
	if err != nil || sourceWorkspace == nil || sourceTask.WorkspaceID != sourceWorkspace.ID {
		return denyMCPCreateTask(
			ws.ErrorCodeUnauthorized,
			"the session caller workspace could not be verified",
		)
	}
	if sourceTask.IsFromOffice != isMCPOfficeWorkspace(sourceWorkspace) {
		return denyMCPCreateTask(
			ws.ErrorCodeUnauthorized,
			"the session caller workspace and task mode do not agree",
		)
	}
	if req.SourceTaskID != "" && req.SourceTaskID != principal.CallerTaskID {
		return denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"source_task_id must match the current session task",
		)
	}
	if req.SourceSessionID != "" && req.SourceSessionID != principal.CallerSessionID {
		return denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"source_session_id must match the current session",
		)
	}
	// The verified session remains the creator even when the request omits the
	// optional source fields. Keeping these fields server-bound preserves
	// creator profile/runtime inheritance and causal ledger attribution.
	req.SourceTaskID = principal.CallerTaskID
	req.SourceSessionID = principal.CallerSessionID
	return nil
}

func (h *Handlers) admitMCPDestination(
	ctx context.Context,
	req *mcpCreateTaskRequest,
	defaultWorkspaceID string,
	kanbanOnly bool,
) (mcpCreateTaskAdmission, error) {
	explicitWorkspace := req.WorkspaceID != ""
	parent, err := h.mcpCreateTaskReference(ctx, req.ParentID)
	if err != nil {
		return mcpCreateTaskAdmission{}, err
	}
	targetWorkspaceID, err := h.resolveMCPCreateWorkspace(
		ctx, req.WorkspaceID, defaultWorkspaceID, parent,
	)
	if err != nil {
		return mcpCreateTaskAdmission{}, err
	}
	if parent != nil && kanbanOnly && parent.IsFromOffice {
		return mcpCreateTaskAdmission{}, denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"session MCP task creation is restricted to Kanban tasks",
		)
	}
	if err := h.validateMCPCreateWorkspace(ctx, targetWorkspaceID, kanbanOnly); err != nil {
		return mcpCreateTaskAdmission{}, err
	}
	if err := h.validateMCPCreateWorkflow(ctx, req.WorkflowID, targetWorkspaceID); err != nil {
		return mcpCreateTaskAdmission{}, err
	}
	req.WorkspaceID = targetWorkspaceID
	return mcpCreateTaskAdmission{
		discardInheritedSourceRepositories: kanbanOnly && parent == nil &&
			explicitWorkspace && targetWorkspaceID != defaultWorkspaceID &&
			len(req.Repositories) == 0,
	}, nil
}

func (h *Handlers) resolveMCPCreateWorkspace(
	ctx context.Context,
	workspaceID, defaultWorkspaceID string,
	parent *models.Task,
) (string, error) {
	if parent != nil {
		if workspaceID == "" {
			return parent.WorkspaceID, nil
		}
		if workspaceID != parent.WorkspaceID {
			return "", denyMCPCreateTask(
				ws.ErrorCodeValidation,
				"parent_id and workspace_id must refer to the same workspace",
			)
		}
		return workspaceID, nil
	}
	if workspaceID != "" {
		return workspaceID, nil
	}
	if defaultWorkspaceID != "" {
		return defaultWorkspaceID, nil
	}
	workspaces, err := h.taskSvc.ListWorkspaces(ctx)
	if err != nil {
		return "", denyMCPCreateTask(
			ws.ErrorCodeInternalError,
			"failed to resolve the authorized workspace",
		)
	}
	if len(workspaces) != 1 {
		return "", denyMCPCreateTask(
			ws.ErrorCodeValidation,
			"workspace_id is required unless exactly one authorized workspace exists",
		)
	}
	return workspaces[0].ID, nil
}

func (h *Handlers) validateMCPCreateWorkspace(
	ctx context.Context,
	workspaceID string,
	kanbanOnly bool,
) error {
	workspace, err := h.taskSvc.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil {
		return denyMCPCreateTask(ws.ErrorCodeNotFound, "target workspace not found")
	}
	if err := h.taskSvc.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		if service.IsForbidden(err) {
			return denyMCPCreateTask(ws.ErrorCodeForbidden, "target workspace is not writable")
		}
		return denyMCPCreateTask(ws.ErrorCodeNotFound, "target workspace not found")
	}
	if kanbanOnly && isMCPOfficeWorkspace(workspace) {
		return denyMCPCreateTask(
			ws.ErrorCodeValidation,
			"session MCP task creation is restricted to Kanban workspaces",
		)
	}
	return nil
}

// validateMCPFoundCreateTask re-checks the task returned by an external_id
// lookup before it crosses the MCP boundary. The create admission check
// protects the requested destination, but an idempotency hit can return an
// older task, including a project-linked Office task in a legacy mixed-mode
// workspace. Keep that result private to Kanban callers without narrowing the
// existing external management surface.
func (h *Handlers) validateMCPFoundCreateTask(
	ctx context.Context,
	req *mcpCreateTaskRequest,
	task *models.Task,
) error {
	if task == nil || task.WorkspaceID == "" || task.WorkspaceID != req.WorkspaceID {
		return denyMCPCreateTask(ws.ErrorCodeNotFound, "task not found")
	}
	workspace, err := h.taskSvc.GetWorkspace(ctx, task.WorkspaceID)
	if err != nil || workspace == nil {
		return denyMCPCreateTask(ws.ErrorCodeNotFound, "task not found")
	}
	if err := h.taskSvc.AuthorizeWorkspaceScope(ctx, task.WorkspaceID, authz.ScopeTaskWrite); err != nil {
		return denyMCPCreateTask(ws.ErrorCodeNotFound, "task not found")
	}
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok &&
		principal.Surface == mcpprofile.SurfaceKanbanTask && task.IsFromOffice {
		return denyMCPCreateTask(
			ws.ErrorCodeForbidden,
			"session MCP task creation is restricted to Kanban tasks",
		)
	}
	return nil
}

func (h *Handlers) validateMCPCreateWorkflow(
	ctx context.Context,
	workflowID, workspaceID string,
) error {
	if workflowID == "" {
		return nil
	}
	workflow, err := h.taskSvc.GetWorkflow(ctx, workflowID)
	if err != nil || workflow == nil {
		return denyMCPCreateTask(
			ws.ErrorCodeValidation,
			"workflow_id was not found",
		)
	}
	if workflow.WorkspaceID != workspaceID {
		return denyMCPCreateTask(
			ws.ErrorCodeValidation,
			"workflow_id and workspace_id must refer to the same workspace",
		)
	}
	return nil
}

func (h *Handlers) validateMCPPrincipalSession(
	ctx context.Context,
	principal mcpscope.Principal,
	task *models.Task,
) error {
	session, err := h.taskSvc.GetTaskSession(ctx, principal.CallerSessionID)
	if err != nil || session == nil || session.TaskID != task.ID {
		return denyMCPCreateTask(
			ws.ErrorCodeUnauthorized,
			"the session caller identity could not be verified",
		)
	}
	return nil
}

func (h *Handlers) mcpCreateTaskReference(ctx context.Context, id string) (*models.Task, error) {
	if id == "" {
		return nil, nil
	}
	task, err := h.taskSvc.GetTask(ctx, id)
	if err != nil || task == nil {
		return nil, denyMCPCreateTask(ws.ErrorCodeNotFound, "target parent task not found")
	}
	return task, nil
}

func isMCPOfficeWorkspace(workspace *models.Workspace) bool {
	return workspace != nil && workspace.OfficeWorkflowID != ""
}

func mcpCreateTaskAdmissionErrorCode(err error) string {
	var admissionErr *mcpCreateTaskAdmissionError
	if errors.As(err, &admissionErr) {
		return admissionErr.code
	}
	return ws.ErrorCodeInternalError
}

func mcpCreateTaskAdmissionErrorMessage(err error) string {
	var admissionErr *mcpCreateTaskAdmissionError
	if errors.As(err, &admissionErr) {
		return admissionErr.message
	}
	return "MCP task creation admission failed: " + err.Error()
}

func (h *Handlers) logMCPCreateAdmissionRejection(ctx context.Context, code, message string) {
	if h.logger == nil {
		return
	}
	fields := []zap.Field{
		zap.String("code", code),
		zap.String("reason", message),
	}
	if principal, ok := mcpscope.PrincipalFromContext(ctx); ok {
		fields = append(fields,
			zap.String("caller_task_id", principal.CallerTaskID),
			zap.String("caller_session_id", principal.CallerSessionID),
			zap.String("workspace_id", principal.WorkspaceID),
			zap.String("surface", string(principal.Surface)),
		)
	}
	if mcporigin.IsTrustedExternalTransport(ctx) {
		fields = append(fields, zap.String("origin", "external"))
	}
	h.logger.Warn("MCP task creation admission rejected", fields...)
}
