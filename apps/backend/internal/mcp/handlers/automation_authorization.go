package handlers

import (
	"context"
	"encoding/json"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// mcpActionRegistrar is the small registration surface shared by the normal
// dispatcher and the automation-aware wrapper.
type mcpActionRegistrar interface {
	RegisterFunc(string, ws.HandlerFunc)
}

type guardedMCPDispatcher struct {
	*ws.Dispatcher
	handlers *Handlers
}

func (d *guardedMCPDispatcher) RegisterFunc(action string, handler ws.HandlerFunc) {
	d.Dispatcher.RegisterFunc(action, func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		guarded, replacement, err := d.handlers.authorizeAutomationRequest(ctx, msg)
		if guarded != nil {
			return guarded, err
		}
		return handler(ctx, replacement)
	})
}

// automationSurfaceActions is the execution-time mirror of the fixed
// SurfaceAutomation catalog. Discovery alone is not an authorization boundary
// because an agent can still send a raw WebSocket action.
var automationSurfaceActions = map[string]struct{}{
	ws.ActionMCPListWorkspaces:              {},
	ws.ActionMCPListWorkflows:               {},
	ws.ActionMCPListWorkflowSteps:           {},
	ws.ActionMCPListRepositories:            {},
	ws.ActionMCPListTasks:                   {},
	ws.ActionMCPListAgents:                  {},
	ws.ActionMCPListExecutors:               {},
	ws.ActionMCPListExecutorProfiles:        {},
	ws.ActionMCPListRelatedTasks:            {},
	ws.ActionMCPGetTaskConversation:         {},
	ws.ActionMCPListTaskSessions:            {},
	ws.ActionMCPCreateTask:                  {},
	ws.ActionMCPUpdateTask:                  {},
	ws.ActionMCPMoveTask:                    {},
	ws.ActionMCPArchiveTask:                 {},
	ws.ActionMCPAddTaskDependency:           {},
	ws.ActionMCPRemoveTaskDependency:        {},
	ws.ActionMCPMessageTask:                 {},
	ws.ActionMCPStopTask:                    {},
	ws.ActionMCPSpawnSession:                {},
	ws.ActionMCPListPendingQuestions:        {},
	ws.ActionMCPAnswerQuestion:              {},
	ws.ActionMCPListPendingAgentPermissions: {},
	ws.ActionMCPResolveAgentPermission:      {},
}

// ActionMCPArchiveTask is deliberately absent: a hidden automation_run task
// archiving itself is its normal end-of-run completion signal, not the kind
// of self-mutation (messaging, stopping, spawning a session on itself) this
// list exists to block.
var automationSelfDeniedActions = map[string]struct{}{
	ws.ActionMCPCreateTask:                  {},
	ws.ActionMCPUpdateTask:                  {},
	ws.ActionMCPMoveTask:                    {},
	ws.ActionMCPAddTaskDependency:           {},
	ws.ActionMCPRemoveTaskDependency:        {},
	ws.ActionMCPMessageTask:                 {},
	ws.ActionMCPStopTask:                    {},
	ws.ActionMCPSpawnSession:                {},
	ws.ActionMCPListPendingQuestions:        {},
	ws.ActionMCPAnswerQuestion:              {},
	ws.ActionMCPListPendingAgentPermissions: {},
	ws.ActionMCPResolveAgentPermission:      {},
}

// authorizeAutomationRequest is the one execution-time boundary for the
// fixed automation surface. It rejects raw actions outside the catalog,
// verifies supplied resources belong to the principal workspace, rejects
// self-targets for coordination operations, and replaces caller attribution
// fields with the trusted stream identity.
func (h *Handlers) authorizeAutomationRequest(ctx context.Context, msg *ws.Message) (*ws.Message, *ws.Message, error) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || !principal.IsAutomation() {
		return nil, msg, nil
	}
	if _, allowed := automationSurfaceActions[msg.Action]; !allowed {
		response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnknownAction,
			"tool is not available on the automation MCP surface", nil)
		return response, nil, err
	}
	if h.taskSvc == nil || principal.WorkspaceID == "" {
		return automationNotFound(msg)
	}

	fields, err := automationPayloadFields(msg.Payload)
	if err != nil {
		response, responseErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest,
			"Invalid payload: "+err.Error(), nil)
		return response, nil, responseErr
	}
	if !h.authorizeAutomationScalarFields(ctx, principal, msg.Action, fields) ||
		!h.authorizeAutomationReferenceFields(ctx, principal, msg.Action, fields) {
		return automationNotFound(msg)
	}
	changed := rewriteAutomationPayload(principal, msg.Action, fields)
	if !changed {
		return nil, msg, nil
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return automationNotFound(msg)
	}
	replacement := *msg
	replacement.Payload = payload
	return nil, &replacement, nil
}

func automationPayloadFields(payload json.RawMessage) (map[string]json.RawMessage, error) {
	fields := make(map[string]json.RawMessage)
	if len(payload) == 0 {
		return fields, nil
	}
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	return fields, nil
}

func (h *Handlers) authorizeAutomationScalarFields(
	ctx context.Context,
	principal mcpscope.Principal,
	action string,
	fields map[string]json.RawMessage,
) bool {
	workspaceID := jsonStringField(fields, "workspace_id")
	if workspaceID != "" && workspaceID != principal.WorkspaceID {
		return false
	}
	for _, field := range []string{"source_task_id", "caller_task_id"} {
		value := jsonStringField(fields, field)
		if value != "" && value != principal.CallerTaskID {
			return false
		}
	}
	parentID := jsonStringField(fields, "parent_id")
	if parentID != "" && (parentID == principal.CallerTaskID || !h.authorizeAutomationTask(ctx, principal, parentID, action, false)) {
		return false
	}
	taskID := jsonStringField(fields, "task_id")
	return taskID == "" || h.authorizeAutomationTask(ctx, principal, taskID, action, true)
}

func (h *Handlers) authorizeAutomationReferenceFields(
	ctx context.Context,
	principal mcpscope.Principal,
	action string,
	fields map[string]json.RawMessage,
) bool {
	if workflowID := jsonStringField(fields, "workflow_id"); workflowID != "" {
		workflow, err := h.taskSvc.GetWorkflow(ctx, workflowID)
		if err != nil || workflow == nil || workflow.WorkspaceID != principal.WorkspaceID {
			return false
		}
	}
	if sessionID := jsonStringField(fields, "session_id"); sessionID != "" {
		session, err := h.taskSvc.GetTaskSession(ctx, sessionID)
		if err != nil || session == nil || !h.authorizeAutomationTask(ctx, principal, session.TaskID, action, true) {
			return false
		}
	}
	if dependencyID := jsonStringField(fields, "depends_on_task_id"); dependencyID != "" &&
		!h.authorizeAutomationTask(ctx, principal, dependencyID, action, true) {
		return false
	}
	for _, blockedByID := range jsonStringSliceField(fields, "blocked_by") {
		if !h.authorizeAutomationTask(ctx, principal, blockedByID, action, true) {
			return false
		}
	}
	return h.authorizeAutomationRepositoryFields(ctx, principal, fields)
}

func (h *Handlers) authorizeAutomationRepositoryFields(
	ctx context.Context,
	principal mcpscope.Principal,
	fields map[string]json.RawMessage,
) bool {
	if repositoryID := jsonStringField(fields, "repository_id"); repositoryID != "" &&
		!h.authorizeAutomationRepository(ctx, principal, repositoryID) {
		return false
	}
	for _, repositoryID := range jsonRepositoryIDs(fields, "repositories") {
		if !h.authorizeAutomationRepository(ctx, principal, repositoryID) {
			return false
		}
	}
	return true
}

func (h *Handlers) authorizeAutomationRepository(ctx context.Context, principal mcpscope.Principal, repositoryID string) bool {
	repository, err := h.taskSvc.GetRepository(ctx, repositoryID)
	return err == nil && repository != nil && repository.WorkspaceID == principal.WorkspaceID
}

func rewriteAutomationPayload(principal mcpscope.Principal, action string, fields map[string]json.RawMessage) bool {
	changed := false
	if action == ws.ActionMCPListPendingQuestions || action == ws.ActionMCPListRepositories {
		fields["workspace_id"], _ = json.Marshal(principal.WorkspaceID)
		changed = true
	}
	for key, value := range map[string]string{
		"sender_task_id":    principal.CallerTaskID,
		"sender_session_id": principal.CallerSessionID,
		"caller_task_id":    principal.CallerTaskID,
		"source_task_id":    principal.CallerTaskID,
		"source_session_id": principal.CallerSessionID,
	} {
		fields[key], _ = json.Marshal(value)
		changed = true
	}
	if action == ws.ActionMCPCreateTask && jsonStringField(fields, "workspace_id") == "" {
		fields["workspace_id"], _ = json.Marshal(principal.WorkspaceID)
		changed = true
	}
	return changed
}

func (h *Handlers) authorizeAutomationTask(
	ctx context.Context,
	principal mcpscope.Principal,
	taskID, action string,
	denySelf bool,
) bool {
	task, err := h.taskSvc.GetTask(ctx, taskID)
	if err != nil || task == nil || task.WorkspaceID != principal.WorkspaceID {
		return false
	}
	if denySelf && task.ID == principal.CallerTaskID {
		if _, denied := automationSelfDeniedActions[action]; denied {
			return false
		}
	}
	return true
}

func jsonStringField(fields map[string]json.RawMessage, key string) string {
	var value string
	if raw, ok := fields[key]; ok {
		_ = json.Unmarshal(raw, &value)
	}
	return value
}

func jsonStringSliceField(fields map[string]json.RawMessage, key string) []string {
	var values []string
	if raw, ok := fields[key]; ok {
		_ = json.Unmarshal(raw, &values)
	}
	return values
}

func jsonRepositoryIDs(fields map[string]json.RawMessage, key string) []string {
	var inputs []struct {
		RepositoryID string `json:"repository_id"`
	}
	if raw, ok := fields[key]; ok {
		_ = json.Unmarshal(raw, &inputs)
	}
	ids := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if input.RepositoryID != "" {
			ids = append(ids, input.RepositoryID)
		}
	}
	return ids
}

func automationNotFound(msg *ws.Message) (*ws.Message, *ws.Message, error) {
	response, err := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target not found", nil)
	return response, nil, err
}

var _ mcpActionRegistrar = (*guardedMCPDispatcher)(nil)
