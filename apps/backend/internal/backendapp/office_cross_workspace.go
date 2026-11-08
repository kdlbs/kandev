package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/coordinator"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeruntime "github.com/kandev/kandev/internal/office/runtime"
	runmodels "github.com/kandev/kandev/internal/runs/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type officeRuntimeRunReader interface {
	GetRun(context.Context, string) (*runmodels.Run, error)
}

type officeRuntimeAgentReader interface {
	GetAgentInstance(context.Context, string) (*officemodels.AgentInstance, error)
}

type officeRuntimeTaskReader interface {
	GetTask(context.Context, string) (*models.Task, error)
	GetTaskForPrompt(context.Context, string) (*models.Task, error)
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
	GetWorkspace(context.Context, string) (*models.Workspace, error)
	GetWorkflow(context.Context, string) (*models.Workflow, error)
	ListWorkspaces(context.Context) ([]*models.Workspace, error)
}

type officeRuntimeScoper interface {
	ScopeOverridingIdentity(context.Context, string) (context.Context, error)
	ScopePrincipal(context.Context, string, string) (context.Context, error)
}

type officeRuntimeActionCaller interface {
	RequestPayload(context.Context, string, interface{}, interface{}) error
}

// This bridge keeps the real Office principal and current task-domain grants.
// It grants no general Office MCP surface and never impersonates a Kanban task.
type officeCrossWorkspaceGateway struct {
	runs         officeRuntimeRunReader
	agents       officeRuntimeAgentReader
	tasks        officeRuntimeTaskReader
	scoper       officeRuntimeScoper
	boundary     taskservice.WorkspaceCollaborationBoundary
	mcp          officeRuntimeActionCaller
	coordinators *coordinator.Service
}

func buildOfficeCrossWorkspaceGateway(p routeParams) officeruntime.CrossWorkspaceGateway {
	if p.services == nil || p.services.OfficeSvcs == nil || p.services.OfficeSvcs.Agents == nil ||
		p.officeRepo == nil || p.taskRepo == nil || p.taskSvc == nil || p.gateway == nil {
		return nil
	}
	scoper := mcpscope.NewResolver(p.taskRepo, p.authSvc,
		func() bool { return p.authSvc != nil && p.authSvc.Mode() != auth.ModeDisabled }, p.log)
	return &officeCrossWorkspaceGateway{
		runs: p.officeRepo, agents: p.services.OfficeSvcs.Agents, tasks: p.taskSvc, scoper: scoper,
		boundary: &workspaceCollaborationBoundary{enabled: p.features.WorkspaceCollaboration, store: p.taskRepo},
		mcp:      mcpserver.NewDispatcherBackendClient(p.gateway.Dispatcher, p.log), coordinators: p.services.Coordinator,
	}
}

func officeRuntimeDenied() error {
	return &officeruntime.CrossWorkspaceError{Status: http.StatusForbidden, Message: "signed Office run is not an active task-scoped collaboration caller"}
}

func officeRuntimeNotFound() error {
	return &officeruntime.CrossWorkspaceError{Status: http.StatusNotFound, Message: "workspace or task is not reachable"}
}

func officeRuntimeTargetError(err error) error {
	if err == nil || errors.Is(err, repoerrors.ErrTaskNotFound) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) ||
		errors.Is(err, repoerrors.ErrWorkflowNotFound) || taskservice.IsForbidden(err) {
		return officeRuntimeNotFound()
	}
	return err
}

func (g *officeCrossWorkspaceGateway) scope(
	ctx context.Context, runCtx officeruntime.RunContext, capability string,
) (context.Context, error) {
	if runCtx.TaskID == "" || runCtx.SessionID == "" || runCtx.RunID == "" || runCtx.WorkspaceID == "" {
		return nil, officeRuntimeDenied()
	}
	if err := g.validateRun(ctx, runCtx, capability); err != nil {
		return nil, err
	}
	scoped, err := g.scoper.ScopeOverridingIdentity(ctx, runCtx.TaskID)
	if err != nil {
		return nil, officeRuntimeDenied()
	}
	scoped, err = g.scoper.ScopePrincipal(scoped, runCtx.TaskID, runCtx.SessionID)
	if err != nil {
		return nil, officeRuntimeDenied()
	}
	principal, ok := mcpscope.PrincipalFromContext(scoped)
	if !ok || principal.Surface != mcpprofile.SurfaceOfficeTask || principal.WorkspaceID != runCtx.WorkspaceID {
		return nil, officeRuntimeDenied()
	}
	if err := g.validateSource(scoped, runCtx); err != nil {
		return nil, err
	}
	return scoped, nil
}

func (g *officeCrossWorkspaceGateway) validateSource(ctx context.Context, runCtx officeruntime.RunContext) error {
	task, err := g.tasks.GetTask(ctx, runCtx.TaskID)
	if err != nil || task == nil || task.WorkspaceID != runCtx.WorkspaceID || task.ArchivedAt != nil {
		return officeRuntimeDenied()
	}
	session, err := g.tasks.GetTaskSession(ctx, runCtx.SessionID)
	if err != nil || session == nil || session.TaskID != task.ID || officeRuntimeSessionEnded(session.State) {
		return officeRuntimeDenied()
	}
	return nil
}

// A terminated source session (for example after its agent lost the task's
// role) ends collaboration even while its run row is still claimed.
func officeRuntimeSessionEnded(state models.TaskSessionState) bool {
	return state == models.TaskSessionStateCompleted || state == models.TaskSessionStateFailed ||
		state == models.TaskSessionStateCancelled
}

func (g *officeCrossWorkspaceGateway) configured() bool {
	return g.runs != nil && g.agents != nil && g.tasks != nil && g.scoper != nil && g.boundary != nil && g.mcp != nil
}

func (g *officeCrossWorkspaceGateway) validateRun(
	ctx context.Context, runCtx officeruntime.RunContext, capability string,
) error {
	if !g.configured() {
		return officeruntime.ErrRuntimeDependencyMissing
	}
	run, err := g.runs.GetRun(ctx, runCtx.RunID)
	if err != nil || run == nil || run.Status != runmodels.RunStatusClaimed ||
		run.AgentProfileID != runCtx.AgentID || run.SessionID != runCtx.SessionID ||
		officeRuntimePayloadTaskID(run.Payload) != runCtx.TaskID {
		return officeRuntimeDenied()
	}
	agent, err := g.agents.GetAgentInstance(ctx, runCtx.AgentID)
	if err != nil || agent == nil || agent.WorkspaceID != runCtx.WorkspaceID ||
		!runCtx.Capabilities.Allows(capability) || !officeruntime.FromAgent(agent).Allows(capability) {
		return officeRuntimeDenied()
	}
	return nil
}

func officeRuntimePayloadTaskID(payload string) string {
	var fields map[string]string
	_ = json.Unmarshal([]byte(payload), &fields)
	return strings.TrimSpace(fields["task_id"])
}

func (g *officeCrossWorkspaceGateway) authorizeWorkspace(
	ctx context.Context, runCtx officeruntime.RunContext, workspaceID string, action models.WorkspaceCollaborationAction,
) error {
	if workspaceID == "" {
		return officeRuntimeNotFound()
	}
	if workspaceID != runCtx.WorkspaceID && !g.boundary.Enabled() {
		return &officeruntime.CrossWorkspaceError{Status: http.StatusForbidden, Message: "workspace collaboration is disabled"}
	}
	if err := g.boundary.Authorize(ctx, workspaceID, action); err != nil {
		if errors.Is(err, taskservice.ErrForbidden) {
			return officeRuntimeNotFound()
		}
		return err
	}
	return nil
}

func (g *officeCrossWorkspaceGateway) Directory(ctx context.Context, runCtx officeruntime.RunContext) (json.RawMessage, error) {
	scoped, err := g.scope(ctx, runCtx, officeruntime.CapabilityListTasks)
	if err != nil {
		return nil, err
	}
	visible, err := g.tasks.ListWorkspaces(scoped)
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]interface{}, 0, len(visible))
	for _, workspace := range visible {
		if workspace == nil {
			continue
		}
		if err := g.authorizeWorkspace(scoped, runCtx, workspace.ID, models.CollaborationRead); err != nil {
			var denied *officeruntime.CrossWorkspaceError
			if errors.As(err, &denied) {
				continue
			}
			return nil, err
		}
		entry, err := g.workspaceDirectoryEntry(scoped, runCtx, workspace)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return json.Marshal(map[string]interface{}{"cross_workspace_access_enabled": g.boundary.Enabled(), "workspaces": entries})
}

func (g *officeCrossWorkspaceGateway) workspaceDirectoryEntry(
	ctx context.Context, runCtx officeruntime.RunContext, workspace *models.Workspace,
) (map[string]interface{}, error) {
	entry := map[string]interface{}{
		"id": workspace.ID, "name": workspace.Name, "self": workspace.ID == runCtx.WorkspaceID,
		"is_office": workspace.OfficeWorkflowID != "",
	}
	if workspace.ID != runCtx.WorkspaceID {
		coordinator, err := g.workspaceCoordinator(ctx, workspace.ID)
		if err != nil {
			return nil, err
		}
		entry["coordinator"] = coordinator
	}
	return entry, nil
}

func (g *officeCrossWorkspaceGateway) Coordinator(
	ctx context.Context, runCtx officeruntime.RunContext, workspaceID string,
) (json.RawMessage, error) {
	scoped, err := g.scope(ctx, runCtx, officeruntime.CapabilityListTasks)
	if err != nil {
		return nil, err
	}
	if err := g.authorizeWorkspace(scoped, runCtx, workspaceID, models.CollaborationRead); err != nil {
		return nil, err
	}
	if _, err := g.tasks.GetWorkspace(scoped, workspaceID); err != nil {
		return nil, officeRuntimeTargetError(err)
	}
	entry, err := g.workspaceCoordinator(scoped, workspaceID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"coordinator": entry})
}

func (g *officeCrossWorkspaceGateway) workspaceCoordinator(ctx context.Context, workspaceID string) (map[string]interface{}, error) {
	result := map[string]interface{}{"workspace_id": workspaceID, "source": "none"}
	if g.coordinators == nil {
		return result, nil
	}
	entries, err := g.coordinators.ListCoordinators(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Coordinator == nil || entry.Coordinator.ConversationTaskID == nil || *entry.Coordinator.ConversationTaskID == "" {
			continue
		}
		task, err := g.tasks.GetTask(ctx, *entry.Coordinator.ConversationTaskID)
		if errors.Is(err, repoerrors.ErrTaskNotFound) || (err == nil && (task == nil || task.ArchivedAt != nil)) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if task.WorkspaceID == workspaceID {
			return map[string]interface{}{"workspace_id": workspaceID, "task_id": task.ID, "task_title": task.Title, "source": "workspace_coordinator"}, nil
		}
	}
	return result, nil
}

func (g *officeCrossWorkspaceGateway) ListTasks(
	ctx context.Context, runCtx officeruntime.RunContext, workspaceID, workflowID string,
) (json.RawMessage, error) {
	scoped, err := g.scope(ctx, runCtx, officeruntime.CapabilityListTasks)
	if err != nil {
		return nil, err
	}
	if err := g.authorizeWorkspace(scoped, runCtx, workspaceID, models.CollaborationRead); err != nil {
		return nil, err
	}
	workflowIDs, err := g.workspaceWorkflows(scoped, workspaceID, workflowID)
	if err != nil {
		return nil, err
	}
	tasks := make([]json.RawMessage, 0)
	for _, id := range workflowIDs {
		var page struct {
			Tasks []json.RawMessage `json:"tasks"`
		}
		if err := g.call(scoped, ws.ActionMCPListTasks, map[string]string{"workflow_id": id}, &page); err != nil {
			return nil, err
		}
		tasks = append(tasks, page.Tasks...)
	}
	return json.Marshal(map[string]interface{}{"workspace_id": workspaceID, "workflow_ids": workflowIDs, "tasks": tasks, "total": len(tasks)})
}

func (g *officeCrossWorkspaceGateway) workspaceWorkflows(ctx context.Context, workspaceID, workflowID string) ([]string, error) {
	if workflowID != "" {
		workflow, err := g.tasks.GetWorkflow(ctx, workflowID)
		if err != nil || workflow == nil || workflow.WorkspaceID != workspaceID {
			return nil, officeRuntimeTargetError(err)
		}
		return []string{workflowID}, nil
	}
	var response struct {
		Workflows []struct {
			ID string `json:"id"`
		} `json:"workflows"`
	}
	if err := g.call(ctx, ws.ActionMCPListWorkflows, map[string]string{"workspace_id": workspaceID}, &response); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(response.Workflows))
	for _, workflow := range response.Workflows {
		ids = append(ids, workflow.ID)
	}
	return ids, nil
}

func (g *officeCrossWorkspaceGateway) taskScope(
	ctx context.Context, runCtx officeruntime.RunContext, taskID string, prompt bool,
) (context.Context, error) {
	capability, action := officeruntime.CapabilityListTasks, models.CollaborationRead
	if prompt {
		capability, action = officeruntime.CapabilityPostComment, models.CollaborationPrompt
	}
	scoped, err := g.scope(ctx, runCtx, capability)
	if err != nil {
		return nil, err
	}
	if err := g.authorizeTarget(scoped, runCtx, taskID, action); err != nil {
		return nil, err
	}
	return scoped, nil
}

func (g *officeCrossWorkspaceGateway) authorizeTarget(
	scoped context.Context, runCtx officeruntime.RunContext, taskID string, action models.WorkspaceCollaborationAction,
) error {
	getTask := g.tasks.GetTask
	if action == models.CollaborationPrompt {
		getTask = g.tasks.GetTaskForPrompt
	}
	task, err := getTask(scoped, taskID)
	if err != nil || task == nil {
		return officeRuntimeTargetError(err)
	}
	return g.authorizeWorkspace(scoped, runCtx, task.WorkspaceID, action)
}

func (g *officeCrossWorkspaceGateway) TaskSessions(ctx context.Context, runCtx officeruntime.RunContext, taskID string) (json.RawMessage, error) {
	scoped, err := g.taskScope(ctx, runCtx, taskID, false)
	if err != nil {
		return nil, err
	}
	return g.callRaw(scoped, ws.ActionMCPListTaskSessions, map[string]string{"task_id": taskID})
}

func (g *officeCrossWorkspaceGateway) TaskConversation(
	ctx context.Context, runCtx officeruntime.RunContext, taskID string, query officeruntime.ConversationQuery,
) (json.RawMessage, error) {
	scoped, err := g.taskScope(ctx, runCtx, taskID, false)
	if err != nil {
		return nil, err
	}
	payload := map[string]interface{}{"task_id": taskID, "session_id": query.SessionID, "before": query.Before, "after": query.After, "sort": query.Sort}
	if query.Limit > 0 {
		payload["limit"] = query.Limit
	}
	return g.callRaw(scoped, ws.ActionMCPGetTaskConversation, payload)
}

func (g *officeCrossWorkspaceGateway) TaskPlan(ctx context.Context, runCtx officeruntime.RunContext, taskID string) (json.RawMessage, error) {
	scoped, err := g.taskScope(ctx, runCtx, taskID, false)
	if err != nil {
		return nil, err
	}
	return g.callRaw(scoped, ws.ActionMCPGetTaskPlan, map[string]string{"task_id": taskID})
}

func (g *officeCrossWorkspaceGateway) SendMessage(
	ctx context.Context, runCtx officeruntime.RunContext, req officeruntime.CrossWorkspaceMessage,
) (json.RawMessage, error) {
	scoped, err := g.scope(ctx, runCtx, officeruntime.CapabilityPostComment)
	if err != nil {
		return nil, err
	}
	authorized, err := mcphandlers.WithOfficeRuntimeMessageAuthority(scoped, runCtx.TaskID, runCtx.SessionID)
	if err != nil {
		return nil, officeRuntimeDenied()
	}
	payload := map[string]interface{}{
		"task_id": req.TaskID, "prompt": req.Prompt, "operation_id": req.OperationID,
		"sender_task_id": runCtx.TaskID, "sender_session_id": runCtx.SessionID,
	}
	// A retained receipt belongs to this live sender, not to the target, so it
	// is read before the target is re-checked: a lost response stays
	// recoverable after the target is deleted or its prompt grant is revoked.
	// Only an identity with no retained record continues to a new send.
	if req.OperationID != "" {
		raw, err := g.callRaw(mcphandlers.WithSendOperationReplayOnly(authorized), ws.ActionMCPMessageTask, payload)
		var notRetained *officeruntime.CrossWorkspaceError
		if !errors.As(err, &notRetained) || notRetained.Code != mcphandlers.SendOperationNotRetainedCode {
			return raw, err
		}
	}
	if err := g.authorizeTarget(scoped, runCtx, req.TaskID, models.CollaborationPrompt); err != nil {
		return nil, err
	}
	return g.callRaw(authorized, ws.ActionMCPMessageTask, payload)
}

func (g *officeCrossWorkspaceGateway) callRaw(ctx context.Context, action string, payload interface{}) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := g.call(ctx, action, payload, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (g *officeCrossWorkspaceGateway) call(ctx context.Context, action string, payload, result interface{}) error {
	err := g.mcp.RequestPayload(ctx, action, payload, result)
	if err == nil {
		return nil
	}
	var backend *mcpserver.BackendError
	if errors.As(err, &backend) {
		return &officeruntime.CrossWorkspaceError{Status: officeMCPErrorStatus(backend.Code), Code: backend.Code, Message: backend.Message}
	}
	return fmt.Errorf("office collaboration %s: %w", action, err)
}

func officeMCPErrorStatus(code string) int {
	switch code {
	case ws.ErrorCodeBadRequest, ws.ErrorCodeValidation:
		return http.StatusBadRequest
	case ws.ErrorCodeNotFound:
		return http.StatusNotFound
	case ws.ErrorCodeForbidden, ws.ErrorCodeUnauthorized:
		return http.StatusForbidden
	case ws.ErrorCodeConflict, mcphandlers.SendOperationConflictCode:
		return http.StatusConflict
	case ws.ErrorCodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}
